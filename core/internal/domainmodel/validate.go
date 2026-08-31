// parlay-feature: parlay-tool/domain-document-api
// parlay-component: cross-cutting/out-of-process-validate-endpoint
//
// The component name still says out-of-process and there is no longer either
// a process boundary or an endpoint. Validation used to shell out to `parlay
// validate --type domain-model --json` because the editor was a separate Go
// module and could not reach Core's rules any other way; then the modules
// merged and it became a function call; then the editor itself went away. What
// survived every one of those moves is the property that mattered: Core owns
// every rule, and this package invents, renames, and reclassifies nothing.
//
// The validator still arrives as a ValidatorFunc rather than an import, and
// the reason has changed. It was Go's internal rule — core/internal/agent was
// unreachable from outside core/. Now the constraint is an import cycle:
// core/internal/agent imports THIS package for Model and Diff, so this package
// cannot import agent back. The seam is what keeps the dependency one-way, and
// it is the right shape regardless: the requirement here is "something that
// turns draft YAML into findings", and Core, which owns both ends, is the
// layer that knows agent satisfies it.

package domainmodel

import (
	"bytes"
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
)

// WholeModelPath is the distinguished element path Core emits for a finding
// that names the whole model rather than a specific entity/field/enum/
// relationship (e.g. missing-schema-version, invalid-yaml). It is passed
// through verbatim; a consumer that anchors findings to elements reads it as
// "anchored to nothing in particular".
const WholeModelPath = "<domain-model>"

// Finding is one validation finding as this package surfaces it. It is a
// verbatim projection of a Core finding: the closed error Code, the element
// path (Field) anchoring it, the Core-emitted Severity (domain-operations-
// deprecated is the sole warning; every other code is an error), and the
// human Message plus the actionable Fix. Nothing here invents, renames, or
// reclassifies a finding.
type Finding struct {
	Field    string `json:"field"`
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Fix      string `json:"fix,omitempty"`
}

// IsError reports whether this finding is error-severity — the gate predicate.
// Only findings Core classifies as error block a save; warnings never do.
func (f Finding) IsError() bool { return f.Severity == "error" }

// CoreFinding is one finding as Core reports it: the closed code, the element
// path in Context (or the whole-model token), the human message, the actionable
// fix, and the authoring-mode severity.
//
// It mirrors the field set of Core's own finding type deliberately, and is a
// separate declaration only because this package cannot import the package that
// owns it. Nothing here reinterprets a field — see Validate.
type CoreFinding struct {
	Code     string
	Message  string
	Context  string
	Fix      string
	Severity string
}

// ValidatorFunc turns draft YAML into Core's findings. Core supplies the real
// one (its domain-model validator in authoring mode); tests supply a chosen
// finding set.
//
// It takes a ctx the real implementation does not use. Validation is pure CPU
// over bytes already in memory, with nothing to cancel — but a seam that cannot
// carry cancellation is one that cannot learn to, and every caller here already
// has a request context to hand.
type ValidatorFunc func(ctx context.Context, draftYAML []byte) []CoreFinding

// StdinLabel is the path label findings should be anchored to. The CLI's --json
// mode uses "<stdin>" when a model arrives that way, and drafts here are always
// in-memory, so reusing the label keeps messages byte-identical across the two
// entry points — which is what the parity suite compares. Exported so the
// caller wiring the validator anchors findings the same way.
const StdinLabel = "<stdin>"

// Validate obtains findings for a draft by calling the supplied validator —
// Core's domain-model rules, in authoring mode, the same ones `parlay validate
// --type domain-model --json` runs, so no two reach paths can disagree about
// a model. This package reimplements no rule.
//
// It is a pure function over the submitted draft: it reads nothing from disk,
// mutates nothing, and returns findings computed from the draft bytes alone,
// independent of any on-disk model.
//
// The draft is serialized to YAML (including a populated deprecated operations
// block, so the validator sees exactly what was submitted). Each Core finding is
// mapped to a Finding with the closed code, element path, severity, and message
// left UNCHANGED.
//
// The error return survives though the in-process path can only populate it from
// serialization: the handlers already branch on it, and a validator that cannot
// fail today is no reason to build one that could not report failing tomorrow.
func Validate(ctx context.Context, validate ValidatorFunc, model Model) ([]Finding, error) {
	if validate == nil {
		return nil, fmt.Errorf("domainmodel: no validator wired; cannot validate")
	}
	yamlBytes, err := marshalForValidation(model)
	if err != nil {
		return nil, fmt.Errorf("domainmodel: serialize draft for validation: %w", err)
	}

	raw := validate(ctx, yamlBytes)

	findings := make([]Finding, 0, len(raw))
	for _, c := range raw {
		findings = append(findings, Finding{
			Field:    c.Context,
			Code:     c.Code,
			Severity: c.Severity,
			Message:  c.Message,
			Fix:      c.Fix,
		})
	}
	return findings, nil
}

// HasErrorFinding reports whether any finding in the set is error-severity —
// the save-gate predicate. Warnings (domain-operations-deprecated) never make
// this true, alone or alongside errors.
func HasErrorFinding(findings []Finding) bool {
	for _, f := range findings {
		if f.IsError() {
			return true
		}
	}
	return false
}

// validationDraft is the marshaling shape used to reproduce the submitted
// draft as YAML for the validator. Unlike the persistence serializer (which
// splices the on-disk operations block back byte-for-byte via rawOperations),
// this shape renders the wire draft's parsed Operations directly, so a draft
// submitted over JSON with a deprecated operations block is validated with
// that block present. schema_version is omitted when zero/absent so a draft
// that never carried one reproduces the missing-schema-version condition.
type validationDraft struct {
	SchemaVersion int              `yaml:"schema_version,omitempty"`
	Enums         []Enum           `yaml:"enums,omitempty"`
	Entities      []Entity         `yaml:"entities,omitempty"`
	Relationships []Relationship   `yaml:"relationships,omitempty"`
	Operations    []map[string]any `yaml:"operations,omitempty"`
}

// marshalForValidation renders the in-memory draft to the deterministic YAML
// the validator consumes on stdin.
func marshalForValidation(model Model) ([]byte, error) {
	draft := validationDraft{
		SchemaVersion: model.SchemaVersion,
		Enums:         model.Enums,
		Entities:      model.Entities,
		Relationships: model.Relationships,
		Operations:    model.Operations,
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(draft); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// The subprocess plumbing that used to close this file is gone:
// realExecValidateJSON, which ran `parlay validate --type domain-model --json -`
// with the draft on stdin; locateParlayBinary, which resolved the executable
// once through a PARLAY_BIN gate and then a PATH lookup; and
// classifyParlayCandidate, which stat'd the result to confirm an executable bit.
//
// Roughly sixty lines whose entire job was to find and talk to a program that
// ships in the same binary as this code. PARLAY_BIN went with them — it existed
// to override a lookup that no longer happens, and keeping an env var that
// silently does nothing is worse than not having one.
