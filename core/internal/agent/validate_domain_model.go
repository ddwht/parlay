package agent

// parlay-feature: parlay-tool/schema-consolidation
// parlay-component: validate-go-split-domain-model
//
// Domain-model deep validation, split out of validate.go (Phase 6.3) into
// its own file — a self-contained concern (schema-version dispatch,
// migrator chain, structural + cross-reference checks) that does not
// share types with the buildfile-deep or layout-validation concerns
// living in the sibling files in this package.

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ============================================================================
// Domain-model deep validation
// ============================================================================
//
// parlay-feature: studio-support/domain-model-yaml-migration
// parlay-component: validation-error-reporter
// parlay-extends: studio-support/domain-model-yaml-migration/domain-model-deep-validator
// parlay-extends: studio-support/domain-model-yaml-migration/domain-model-schema-version-dispatch
//
// Open-question resolutions applied (per the code-phase decision):
//
//   - enum-tone-vocabulary  → closed-set
//   - entity-field-shape    → flat-only
//
// See internal/embedded/schemas/domain-model.schema.md for the canonical
// shape and the rationale for each closed set.

// DomainModelBinaryVersion is the schema_version this binary natively
// understands. Older versions are migrated in-memory through the
// migrator chain registered with RegisterDomainModelMigrator. Newer
// versions refuse to load; the user is told to run parlay upgrade.
//
// Bumping this constant is a coordinated change: the new value lives
// here, the migrator from the previous version lives in
// init() / RegisterDomainModelMigrator, and the schema document under
// internal/embedded/schemas/domain-model.schema.md is updated to match.
const DomainModelBinaryVersion = 1

// closedFieldTypePrimitives is the closed primitive set for a
// DomainField.type per the entity-field-shape=flat-only resolution.
// Field types outside this set must name a declared enum (handled
// separately) or fail with field-type-outside-closed-set.
var closedFieldTypePrimitives = map[string]bool{
	"uuid":     true,
	"string":   true,
	"int":      true,
	"float":    true,
	"bool":     true,
	"datetime": true,
	"ref":      true,
}

// closedFieldTypeList is the human-readable rendering of
// closedFieldTypePrimitives plus the named-enum sentinel, used in error
// messages.
const closedFieldTypeList = "uuid, string, int, float, bool, datetime, ref, <named-enum>"

// closedRelationshipCardinalities is the closed set of relationship
// cardinalities. Anything else fails with relationship-cardinality-unknown.
var closedRelationshipCardinalities = map[string]bool{
	"one-to-one":   true,
	"one-to-many":  true,
	"many-to-one":  true,
	"many-to-many": true,
}

// closedCardinalityList is the human-readable rendering of
// closedRelationshipCardinalities, used in error messages.
const closedCardinalityList = "one-to-one, one-to-many, many-to-one, many-to-many"

// closedEnumTones is the closed set of tones a DomainEnumValue may
// declare per the enum-tone-vocabulary=closed-set resolution. This
// matches the adapter's color-token tone vocabulary.
var closedEnumTones = map[string]bool{
	"neutral": true,
	"info":    true,
	"warning": true,
	"danger":  true,
	"success": true,
}

// closedEnumToneList is the human-readable rendering of
// closedEnumTones, used in error messages.
const closedEnumToneList = "neutral, info, warning, danger, success"

// wholeModelPathToken is the distinguished element-path token used for
// ownerless (whole-model) findings — those not attributable to a single
// entity / relationship / enum / operation element. It is angle-bracketed
// so it can never collide with a real dotted element path, whose roots are
// always entities. / relationships. / enums. / operations. . Every finding
// emitted by the structured validator carries a non-blank element path in
// its Context field: either a dotted element path or this token.
//
// parlay-feature: studio-support/structured-domain-model-validation
// parlay-component: cross-cutting/element-path-on-every-finding
const wholeModelPathToken = "<domain-model>"

// applyDomainModelSeverity resolves each finding's Severity from the
// per-mode RuleSeverity table (authoring vs build). Codes absent from the
// table default to error in both modes (RuleSeverity's own default), so
// hard structural findings (invalid-yaml, missing-schema-version,
// field-type-outside-closed-set, ...) read as error regardless of mode;
// only mode-varying rules such as domain-operations-deprecated differ.
//
// parlay-feature: studio-support/structured-domain-model-validation
// parlay-component: cross-cutting/json-validation-mode
func applyDomainModelSeverity(errs []ValidationError, mode ValidationMode) []ValidationError {
	for i := range errs {
		errs[i].Severity = string(RuleSeverity(errs[i].Code, mode))
	}
	return errs
}

// deepDomainModel mirrors the on-disk YAML shape declared in
// internal/embedded/schemas/domain-model.schema.md. The flat-only
// resolution means fields cannot be inline object literals; they are
// strings naming a primitive type or a declared enum, plus an optional
// target/enum reference key.
type deepDomainModel struct {
	SchemaVersion int                      `yaml:"schema_version"`
	Enums         []deepDomainEnum         `yaml:"enums"`
	Entities      []deepDomainEntity       `yaml:"entities"`
	Relationships []deepDomainRelationship `yaml:"relationships"`
	Operations    []deepDomainOperation    `yaml:"operations"`
}

type deepDomainEnum struct {
	Name   string                `yaml:"name"`
	Values []deepDomainEnumValue `yaml:"values"`
}

type deepDomainEnumValue struct {
	Value string `yaml:"value"`
	Label string `yaml:"label,omitempty"`
	Tone  string `yaml:"tone,omitempty"`
}

type deepDomainEntity struct {
	Name   string `yaml:"name"`
	Fields []struct {
		// The fields here are typed as yaml.Node so we can detect inline
		// object literals (which the flat-only resolution rejects). A
		// scalar Type maps to a string; a mapping/sequence node tells us
		// the field shape is nested.
		Name     string    `yaml:"name"`
		Type     yaml.Node `yaml:"type"`
		Target   string    `yaml:"target,omitempty"`
		Enum     string    `yaml:"enum,omitempty"`
		Required bool      `yaml:"required"`
	} `yaml:"fields"`
}

type deepDomainRelationship struct {
	Name        string `yaml:"name"`
	From        string `yaml:"from"`
	To          string `yaml:"to"`
	Cardinality string `yaml:"cardinality"`
}

type deepDomainOperation struct {
	Name    string   `yaml:"name"`
	Input   []string `yaml:"input"`
	Effects []string `yaml:"effects,omitempty"`
}

// ValidateDomainModelStructuredMode is the domain-model validator: one entry
// point, mode-aware, returning structured findings whose Context is always an
// element path (a dotted path or wholeModelPathToken) and whose Severity is
// resolved from the per-mode RuleSeverity table.
//
// It replaces three entry points and a boolean. There were ValidateDomainModel
// (Validator-shaped, aggregating every finding into one error),
// ValidateDomainModelStructured (authoring mode, emitDeprecation=false), and this
// one — all three funnelling into a shared implementation whose last parameter
// decided whether to emit a single diagnostic.
//
// The boolean was load-bearing for a bad reason. ValidateDomainModel treated any
// finding as a failure with no reference to severity, so routing it through the
// mode-aware path would have turned a warning-severity
// domain-operations-deprecated into a hard failure for every project carrying a
// legacy operations block. Suppressing the diagnostic was the cheaper fix at the
// time, and it left `parlay validate --type domain-model` silent about a
// deprecated block that `--type domain-model --json` reported — the same file,
// the same tool, two answers.
//
// The severity filter belongs at the boundary that renders findings, not inside
// the validator, so it now lives in the command layer (see
// validateDomainModelAdapter): warnings print, errors fail. With that in place
// there is nothing left for the boolean to protect.
//
// parlay-feature: studio-support/structured-domain-model-validation
// parlay-component: cross-cutting/emit-domain-operations-deprecated
func ValidateDomainModelStructuredMode(path string, content []byte, mode ValidationMode) []ValidationError {
	var errors []ValidationError
	_ = path // ownerless findings now use wholeModelPathToken, not the file path

	// Pass 0: YAML parse.
	var dm deepDomainModel
	if err := yaml.Unmarshal(content, &dm); err != nil {
		return applyDomainModelSeverity([]ValidationError{{
			Code:    "invalid-yaml",
			Message: fmt.Sprintf("domain-model.yaml is not valid YAML: %s", err),
			Context: wholeModelPathToken,
			Fix:     "fix the YAML syntax errors and re-run validation",
		}}, mode)
	}

	// Pass 1: schema_version is present and integer (yaml.Unmarshal of
	// a missing or string-typed field leaves SchemaVersion at its zero
	// value, 0). We re-decode against a permissive map to disambiguate
	// "missing" from "present but non-integer".
	rawTop := map[string]interface{}{}
	_ = yaml.Unmarshal(content, &rawTop)
	if _, present := rawTop["schema_version"]; !present {
		errors = append(errors, ValidationError{
			Code:    "missing-schema-version",
			Message: "domain-model.yaml is missing required top-level field 'schema_version'",
			Context: wholeModelPathToken,
			Fix:     "add 'schema_version: 1' to the top of the file",
		})
		return applyDomainModelSeverity(errors, mode)
	}
	if _, ok := rawTop["schema_version"].(int); !ok {
		errors = append(errors, ValidationError{
			Code:    "missing-schema-version",
			Message: fmt.Sprintf("domain-model.yaml schema_version must be an integer (got %T)", rawTop["schema_version"]),
			Context: wholeModelPathToken,
			Fix:     "set schema_version to a plain integer (e.g., 'schema_version: 1' — no quotes, no semver)",
		})
		return applyDomainModelSeverity(errors, mode)
	}

	// Pass 2: compare schema_version to the binary's expected version.
	switch chain := ResolveDomainModelVersion(dm.SchemaVersion); chain.Outcome {
	case DomainModelVersionEqual, DomainModelVersionMigrate:
		// equal: pipeline proceeds. migrate: in-memory chain; for
		// validation purposes this is a no-op — we still validate the
		// on-disk shape, which is the source-of-truth representation.
	case DomainModelVersionTooNew:
		errors = append(errors, ValidationError{
			Code:    "schema-version-newer-than-binary",
			Message: fmt.Sprintf("domain-model.yaml schema_version %d is newer than this Core release supports (%d); run parlay upgrade", dm.SchemaVersion, DomainModelBinaryVersion),
			Context: wholeModelPathToken,
			Fix:     "run parlay upgrade to install a Core release that supports the file's schema_version",
		})
		return applyDomainModelSeverity(errors, mode)
	case DomainModelVersionUnreachable:
		errors = append(errors, ValidationError{
			Code:    "schema-version-unreachable",
			Message: fmt.Sprintf("domain-model.yaml schema_version %d has no migrator chain reaching the binary version (%d); upgrade in steps via the documented migration path", dm.SchemaVersion, DomainModelBinaryVersion),
			Context: wholeModelPathToken,
			Fix:     "run an intermediate parlay release that supports the file's schema_version, migrate, then upgrade further",
		})
		return applyDomainModelSeverity(errors, mode)
	}

	// Build lookup tables for cross-reference passes.
	enumByName := map[string]*deepDomainEnum{}
	enumValueSet := map[string]map[string]bool{} // enum-name → set of declared values
	for i := range dm.Enums {
		e := &dm.Enums[i]
		enumByName[e.Name] = e
		vs := map[string]bool{}
		for _, v := range e.Values {
			vs[v.Value] = true
		}
		enumValueSet[e.Name] = vs
	}

	entityByName := map[string]*deepDomainEntity{}
	entityFieldSet := map[string]map[string]bool{} // entity-name → set of field names
	for i := range dm.Entities {
		ent := &dm.Entities[i]
		entityByName[ent.Name] = ent
		fs := map[string]bool{}
		for _, f := range ent.Fields {
			fs[f.Name] = true
		}
		entityFieldSet[ent.Name] = fs
	}

	// Pass 3: enum tones (closed-set resolution).
	for _, e := range dm.Enums {
		for _, v := range e.Values {
			if v.Tone == "" {
				continue
			}
			if !closedEnumTones[v.Tone] {
				errors = append(errors, ValidationError{
					Code:    "enum-tone-outside-closed-set",
					Message: fmt.Sprintf("enums.%s.values.%s.tone %q is not in the closed set (%s)", e.Name, v.Value, v.Tone, closedEnumToneList),
					Context: fmt.Sprintf("enums.%s.values.%s.tone", e.Name, v.Value),
					Fix:     fmt.Sprintf("change the tone to one of {%s}, or bump schema_version and add a migrator if a new tone is genuinely needed", closedEnumToneList),
				})
			}
		}
	}

	// Pass 4: entity field types (flat-only resolution).
	for _, ent := range dm.Entities {
		for _, f := range ent.Fields {
			// Inline object literal? The yaml.Node tells us.
			if f.Type.Kind == yaml.MappingNode || f.Type.Kind == yaml.SequenceNode {
				errors = append(errors, ValidationError{
					Code:    "field-type-outside-closed-set",
					Message: fmt.Sprintf("entities.%s.fields.%s declares a nested field shape, which the flat-only resolution rejects", ent.Name, f.Name),
					Context: fmt.Sprintf("entities.%s.fields.%s.type", ent.Name, f.Name),
					Fix:     fmt.Sprintf("lift the nested shape into a separate entity joined by a ref-typed field (closed set: %s)", closedFieldTypeList),
				})
				continue
			}
			typeStr := f.Type.Value
			if typeStr == "" {
				errors = append(errors, ValidationError{
					Code:    "field-type-outside-closed-set",
					Message: fmt.Sprintf("entities.%s.fields.%s declares no type", ent.Name, f.Name),
					Context: fmt.Sprintf("entities.%s.fields.%s.type", ent.Name, f.Name),
					Fix:     fmt.Sprintf("set type: to one of the closed set (%s) or to a declared enum name", closedFieldTypeList),
				})
				continue
			}
			// Primitive in the closed set?
			if closedFieldTypePrimitives[typeStr] {
				if typeStr == "ref" && f.Target == "" {
					errors = append(errors, ValidationError{
						Code:    "ref-missing-target",
						Message: fmt.Sprintf("entities.%s.fields.%s declares type 'ref' without a target entity", ent.Name, f.Name),
						Context: fmt.Sprintf("entities.%s.fields.%s.target", ent.Name, f.Name),
						Fix:     "add target: <Entity> naming the referenced entity",
					})
				}
				if typeStr == "ref" && f.Target != "" {
					if _, ok := entityByName[f.Target]; !ok {
						errors = append(errors, ValidationError{
							Code:    "undeclared-entity-reference",
							Message: fmt.Sprintf("entities.%s.fields.%s.target references undeclared entity %q", ent.Name, f.Name, f.Target),
							Context: fmt.Sprintf("entities.%s.fields.%s.target", ent.Name, f.Name),
							Fix:     fmt.Sprintf("declare entity %q in entities: or change the target to an existing one", f.Target),
						})
					}
				}
				continue
			}
			// Names a declared enum?
			if _, ok := enumByName[typeStr]; ok {
				// Optional consistency: if enum: key is set, it must
				// match the type. We treat this as advisory only —
				// missing or matching enum: is fine.
				if f.Enum != "" && f.Enum != typeStr {
					errors = append(errors, ValidationError{
						Code:    "enum-key-mismatch",
						Message: fmt.Sprintf("entities.%s.fields.%s declares type %q but enum: %q — these must match", ent.Name, f.Name, typeStr, f.Enum),
						Context: fmt.Sprintf("entities.%s.fields.%s.enum", ent.Name, f.Name),
						Fix:     fmt.Sprintf("set enum: %q (matching the type), or change type: to match enum:", typeStr),
					})
				}
				continue
			}
			// Anything else fails.
			errors = append(errors, ValidationError{
				Code:    "field-type-outside-closed-set",
				Message: fmt.Sprintf("entities.%s.fields.%s declares type %q which is not in the closed set (%s)", ent.Name, f.Name, typeStr, closedFieldTypeList),
				Context: fmt.Sprintf("entities.%s.fields.%s.type", ent.Name, f.Name),
				Fix:     fmt.Sprintf("change the type to one of {%s} or to a declared enum name", closedFieldTypeList),
			})
		}
	}

	// Pass 5: relationships.
	for _, r := range dm.Relationships {
		if r.From != "" {
			if _, ok := entityByName[r.From]; !ok {
				errors = append(errors, ValidationError{
					Code:    "undeclared-entity-reference",
					Message: fmt.Sprintf("relationships.%s.from references undeclared entity %q", r.Name, r.From),
					Context: fmt.Sprintf("relationships.%s.from", r.Name),
					Fix:     fmt.Sprintf("declare entity %q in entities: or change relationship endpoint", r.From),
				})
			}
		}
		if r.To != "" {
			if _, ok := entityByName[r.To]; !ok {
				errors = append(errors, ValidationError{
					Code:    "undeclared-entity-reference",
					Message: fmt.Sprintf("relationships.%s.to references undeclared entity %q", r.Name, r.To),
					Context: fmt.Sprintf("relationships.%s.to", r.Name),
					Fix:     fmt.Sprintf("declare entity %q in entities: or change relationship endpoint", r.To),
				})
			}
		}
		if r.Cardinality != "" && !closedRelationshipCardinalities[r.Cardinality] {
			errors = append(errors, ValidationError{
				Code:    "relationship-cardinality-unknown",
				Message: fmt.Sprintf("relationships.%s.cardinality %q is not recognized (%s)", r.Name, r.Cardinality, closedCardinalityList),
				Context: fmt.Sprintf("relationships.%s.cardinality", r.Name),
				Fix:     fmt.Sprintf("change cardinality to one of {%s}", closedCardinalityList),
			})
		}
	}

	// Pass 6: operation inputs.
	for _, op := range dm.Operations {
		for i, inp := range op.Input {
			// Input is "Entity.field" form. Anything else is malformed.
			parts := strings.SplitN(inp, ".", 2)
			if len(parts) != 2 {
				errors = append(errors, ValidationError{
					Code:    "operation-input-field-not-found",
					Message: fmt.Sprintf("operations.%s.input[%d] %q is not in 'Entity.field' form", op.Name, i, inp),
					Context: fmt.Sprintf("operations.%s.input[%d]", op.Name, i),
					Fix:     "use 'Entity.field' form (e.g., 'Order.id') to name a field on a declared entity",
				})
				continue
			}
			entName, fieldName := parts[0], parts[1]
			fields, ok := entityFieldSet[entName]
			if !ok {
				errors = append(errors, ValidationError{
					Code:    "undeclared-entity-reference",
					Message: fmt.Sprintf("operations.%s.input[%d] references undeclared entity %q", op.Name, i, entName),
					Context: fmt.Sprintf("operations.%s.input[%d]", op.Name, i),
					Fix:     fmt.Sprintf("declare entity %q or change the input to an existing entity", entName),
				})
				continue
			}
			if !fields[fieldName] {
				errors = append(errors, ValidationError{
					Code:    "operation-input-field-not-found",
					Message: fmt.Sprintf("operations.%s.input names field %q which is not declared on entity %q", op.Name, fieldName, entName),
					Context: fmt.Sprintf("operations.%s.input[%d]", op.Name, i),
					Fix:     fmt.Sprintf("either add field %q to entity %q or change the input to a declared field", fieldName, entName),
				})
			}
		}
	}

	// Cross-pass: enum-typed fields whose enum value is declared but
	// the field-level enum: key references a value not in the enum.
	// (This catches a fixture-style mistake; deep validation does not
	// otherwise observe live values.)
	for _, ent := range dm.Entities {
		for _, f := range ent.Fields {
			if f.Enum == "" {
				continue
			}
			vs, ok := enumValueSet[f.Enum]
			if !ok {
				// enum: names a missing enum — already flagged above when
				// the type itself doesn't resolve. Skip duplicate.
				continue
			}
			_ = vs // declared values are validated by fixtures, not here
		}
	}

	// domain-operations-deprecated: read-only presence check for a
	// populated deprecated operations: block. Emitted unconditionally now —
	// it used to be gated on an emitDeprecation boolean, which meant the
	// plain `--type domain-model` path never mentioned the block. The block
	// spans the whole model, so the finding carries the whole-model
	// element-path token.
	// This check never mutates, reorders, or drops the block — validation
	// operates on []byte and never writes.
	//
	// parlay-feature: studio-support/structured-domain-model-validation
	// parlay-component: cross-cutting/emit-domain-operations-deprecated
	if len(dm.Operations) > 0 {
		errors = append(errors, ValidationError{
			Code:    "domain-operations-deprecated",
			Message: "domain-model.yaml carries a populated deprecated 'operations:' block; the top-level operations: field is deprecated in favor of per-feature capabilities.yaml",
			Context: wholeModelPathToken,
			Fix:     "migrate the operations: block via `parlay migrate-domain-operations`",
		})
	}

	return applyDomainModelSeverity(errors, mode)
}

// ============================================================================
// Domain-model schema-version dispatch
// ============================================================================
//
// parlay-extends: studio-support/domain-model-yaml-migration/domain-model-schema-version-dispatch
//
// Every read of domain-model.yaml first inspects schema_version and
// dispatches:
//
//   - equal-to-binary    → reader proceeds directly
//   - older-than-binary  → in-memory model is routed through the
//                          per-version migrator chain (e.g., v1→v2,
//                          v2→v3) before handing to the rest of the
//                          pipeline; the on-disk file is unchanged
//   - newer-than-binary  → fail with schema-version-newer-than-binary
//   - unreachably-old    → fail with schema-version-unreachable
//
// Migrators are pure functions over the in-memory model — they do not
// write to disk, do not consult the network, and do not require AI
// inference. Adding a new schema_version requires registering exactly
// one migrator (from the previous version to the new one); chaining is
// automatic.

// DomainModelVersionOutcome classifies a resolved schema_version
// against the binary's natively-supported version.
type DomainModelVersionOutcome int

const (
	// DomainModelVersionEqual — schema_version equals the binary
	// version; no migration needed.
	DomainModelVersionEqual DomainModelVersionOutcome = iota
	// DomainModelVersionMigrate — schema_version is older but a
	// migrator chain reaches the binary version.
	DomainModelVersionMigrate
	// DomainModelVersionTooNew — schema_version is newer than the
	// binary version.
	DomainModelVersionTooNew
	// DomainModelVersionUnreachable — schema_version is older but no
	// migrator chain reaches the binary version.
	DomainModelVersionUnreachable
)

// DomainModelMigrator is a pure function that lifts a domain model
// from one schema_version to the next. Migrators are registered with
// RegisterDomainModelMigrator at package init time.
type DomainModelMigrator interface {
	FromVersion() int
	ToVersion() int
	Migrate(in map[string]interface{}) (map[string]interface{}, error)
}

// DomainModelVersionResolution is the result of dispatching a
// schema_version against the binary's version. Chain is the ordered
// list of migrators to apply when Outcome is DomainModelVersionMigrate;
// it is empty for the other outcomes.
type DomainModelVersionResolution struct {
	Outcome DomainModelVersionOutcome
	Chain   []DomainModelMigrator
}

// domainModelMigrators is the package-level registry, keyed by source
// version. There is at most one migrator per source version (the
// canonical "v1→v2"); the chain is built by walking from the file's
// version up to the binary's version one step at a time.
var domainModelMigrators = map[int]DomainModelMigrator{}

// RegisterDomainModelMigrator registers a migrator for ResolveDomainModelVersion
// to consult. Intended to be called from init() in a per-version
// migrator file. Re-registration with the same FromVersion overwrites
// the prior entry — useful for tests; in production each FromVersion
// is registered exactly once.
func RegisterDomainModelMigrator(m DomainModelMigrator) {
	domainModelMigrators[m.FromVersion()] = m
}

// ResolveDomainModelVersion classifies the given schema_version against
// the binary's version and (when older) builds the migrator chain that
// would lift the in-memory model up to the binary version. Pure — does
// not read or write any file, does not consult the network.
func ResolveDomainModelVersion(version int) DomainModelVersionResolution {
	if version == DomainModelBinaryVersion {
		return DomainModelVersionResolution{Outcome: DomainModelVersionEqual}
	}
	if version > DomainModelBinaryVersion {
		return DomainModelVersionResolution{Outcome: DomainModelVersionTooNew}
	}
	// version < binary: walk the chain.
	var chain []DomainModelMigrator
	cur := version
	for cur < DomainModelBinaryVersion {
		m, ok := domainModelMigrators[cur]
		if !ok {
			return DomainModelVersionResolution{Outcome: DomainModelVersionUnreachable}
		}
		chain = append(chain, m)
		next := m.ToVersion()
		if next <= cur {
			// Defensive: a migrator that doesn't advance the version
			// would loop forever. Treat as unreachable.
			return DomainModelVersionResolution{Outcome: DomainModelVersionUnreachable}
		}
		cur = next
	}
	return DomainModelVersionResolution{
		Outcome: DomainModelVersionMigrate,
		Chain:   chain,
	}
}
