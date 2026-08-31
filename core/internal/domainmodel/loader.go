// parlay-feature: parlay-tool/domain-document-api
// parlay-component: cross-cutting/domain-model-loader-reuse

package domainmodel

import (
	"context"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// CurrentSchemaVersion is the domain-model schema version this binary speaks.
// The first released value is 1. A model older than this is migrated in
// memory to this shape and served in that shape; the on-disk file is
// untouched until a save. A model newer than this refuses to load.
const CurrentSchemaVersion = 1

// Loader error sentinels. These are surfaced through the harness error
// envelope by the handlers — the schema-version failures map to
// validation-failed (actionable), NOT a generic server-error.
var (
	// ErrMissingSchemaVersion — the file has no integer schema_version.
	ErrMissingSchemaVersion = errors.New("missing-schema-version")

	// ErrSchemaVersionNewer — schema_version is newer than this binary.
	// The actionable fix is `parlay upgrade`.
	ErrSchemaVersionNewer = errors.New("schema-version-newer-than-binary")

	// ErrInvalidYAML — the file on disk is not parseable YAML.
	//
	// This is every bit as actionable as the schema-version failures and was
	// the one load failure that fell through to a bare server-error back when
	// this package sat behind an HTTP handler: a hand-broken
	// domain-model.yaml produced a 500 carrying nothing but a request id,
	// while the CLI on the same file reported invalid-yaml with the offending
	// line. The user whose file is broken is exactly the user who needs the
	// line number.
	ErrInvalidYAML = errors.New("invalid-yaml")
)

// Model is the in-memory domain model this package loads, edits, and saves.
// The typed fields (enums, entities, relationships) are serialized
// deterministically; the deprecated operations block is carried through load
// and save byte-for-byte unchanged via rawOperations (see serialize.go).
type Model struct {
	SchemaVersion int            `yaml:"schema_version" json:"schema_version"`
	Enums         []Enum         `yaml:"enums,omitempty" json:"enums"`
	Entities      []Entity       `yaml:"entities,omitempty" json:"entities"`
	Relationships []Relationship `yaml:"relationships,omitempty" json:"relationships,omitempty"`

	// Operations is the parsed, generic view of the deprecated operations
	// block, exposed to the JSON wire layer only. It is NEVER used to
	// re-serialize the block — passthrough uses rawOperations to guarantee
	// byte-for-byte fidelity.
	Operations []map[string]any `yaml:"-" json:"operations,omitempty"`

	// rawOperations is the verbatim bytes of the on-disk operations block
	// (including the `operations:` key line). Spliced back unchanged on
	// serialize so an unrelated edit never perturbs the operations block.
	rawOperations []byte
}

// Enum is a named closed set of values with optional presentation metadata.
type Enum struct {
	Name   string      `yaml:"name" json:"name"`
	Values []EnumValue `yaml:"values" json:"values"`
}

// EnumValue is one entry of an Enum. Label and tone are optional; when unset
// they are OMITTED from the serialized YAML, never written as empty strings.
type EnumValue struct {
	Value string `yaml:"value" json:"value"`
	Label string `yaml:"label,omitempty" json:"label,omitempty"`
	Tone  string `yaml:"tone,omitempty" json:"tone,omitempty"`
}

// Entity is a named record with an ordered list of fields.
type Entity struct {
	Name   string  `yaml:"name" json:"name"`
	Fields []Field `yaml:"fields" json:"fields"`
}

// Field is one entity field. Target is set only for ref-typed fields; enum is
// set only when type names a declared enum.
type Field struct {
	Name   string `yaml:"name" json:"name"`
	Type   string `yaml:"type" json:"type"`
	Target string `yaml:"target,omitempty" json:"target,omitempty"`
	Enum   string `yaml:"enum,omitempty" json:"enum,omitempty"`
	// Relationship names the declared relationship this ref field
	// realises — the author's way of settling which relationship a field
	// implements when two of them connect the same pair of entities.
	// Carried here so a load/save round-trip preserves it: a key this
	// serializer does not know is a key it silently drops.
	Relationship string `yaml:"relationship,omitempty" json:"relationship,omitempty"`
	Required     bool   `yaml:"required" json:"required"`
}

// Relationship is a named edge between two declared entities.
type Relationship struct {
	Name        string `yaml:"name" json:"name"`
	From        string `yaml:"from" json:"from"`
	To          string `yaml:"to" json:"to"`
	Cardinality string `yaml:"cardinality" json:"cardinality"`
}

// Load reads the resolved root's domain-model.yaml and returns the model plus
// its content-identity token. It routes through the same decode-and-migrate
// path the command-line read paths use, so every reader agrees on
// schema-version semantics.
//
// Behavior:
//   - A project with no domain-model.yaml returns an empty model at the
//     current schema_version with the sentinel etag "empty", never a
//     not-found; the first save creates the file.
//   - Missing/unreachable schema_version fails with ErrMissingSchemaVersion.
//   - A schema_version newer than this binary fails with ErrSchemaVersionNewer.
//   - An older model is migrated in memory to the current shape; the on-disk
//     file is untouched by the load.
func Load(ctx context.Context, root string) (Model, Etag, error) {
	path := resolveModelPath(root)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// Empty-model bootstrap: no file yet.
		return Model{
			SchemaVersion: CurrentSchemaVersion,
			Enums:         []Enum{},
			Entities:      []Entity{},
		}, SentinelEmpty, nil
	}
	if err != nil {
		return Model{}, "", fmt.Errorf("domainmodel: read model: %w", err)
	}

	model, err := decodeAndMigrate(raw, CurrentSchemaVersion)
	if err != nil {
		return Model{}, "", err
	}
	return model, computeEtag(raw), nil
}

// ErrNoContribution reports that a feature has no domain-model.yaml of its
// own. Contributions are optional — a project that authors only the root
// model behaves exactly as it did before they existed — so this is a normal
// state a caller distinguishes rather than an error to surface.
var ErrNoContribution = errors.New("no-contribution")

// LoadFile decodes a domain model from an explicit path, through the same
// decode-and-migrate path Load uses.
//
// A feature's contribution is the same artifact shape as the root model, and
// nothing about reading one differs — so it reads through the same decoder.
// A separate parse for the per-feature file is precisely the second decoder
// this package exists to avoid.
func LoadFile(path string) (Model, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Model{}, ErrNoContribution
	}
	if err != nil {
		return Model{}, fmt.Errorf("domainmodel: read model: %w", err)
	}
	return decodeAndMigrate(raw, CurrentSchemaVersion)
}

// decodeAndMigrate parses raw model bytes, enforces the schema-version gate
// against currentVersion, and migrates an older model in memory to
// currentVersion. It also captures the verbatim operations block for
// passthrough and the parsed operations for the JSON wire.
//
// currentVersion is a parameter (not the const) so the schema-version gate
// branches are directly table-testable.
func decodeAndMigrate(raw []byte, currentVersion int) (Model, error) {
	// First, read schema_version explicitly so a missing/newer version is a
	// typed failure rather than a generic parse error.
	var probe struct {
		SchemaVersion *int `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal(raw, &probe); err != nil {
		return Model{}, fmt.Errorf("%w: %s", ErrInvalidYAML, err)
	}
	if probe.SchemaVersion == nil {
		return Model{}, ErrMissingSchemaVersion
	}
	onDisk := *probe.SchemaVersion
	if onDisk > currentVersion {
		return Model{}, fmt.Errorf("%w: on-disk schema_version %d is newer than this binary (%d); run `parlay upgrade`",
			ErrSchemaVersionNewer, onDisk, currentVersion)
	}

	var model Model
	if err := yaml.Unmarshal(raw, &model); err != nil {
		return Model{}, fmt.Errorf("%w: %s", ErrInvalidYAML, err)
	}

	// Parse the operations block generically for the JSON wire view.
	var opsProbe struct {
		Operations []map[string]any `yaml:"operations"`
	}
	if err := yaml.Unmarshal(raw, &opsProbe); err != nil {
		return Model{}, fmt.Errorf("%w: %s", ErrInvalidYAML, err)
	}
	model.Operations = opsProbe.Operations
	model.rawOperations = captureOperationsBlock(raw)

	// Migrate an older model in memory to the current shape. v1 is the first
	// released version; migration here is the version bump itself (no field
	// transforms are defined yet). The on-disk file is not written.
	if onDisk < currentVersion {
		model.SchemaVersion = currentVersion
	}

	// Normalise the two required collections to empty slices.
	//
	// enums: and entities: are both OPTIONAL in domain-model.schema.md, so a
	// file that omits either is valid and validates clean on the CLI. Left
	// nil, they marshal to JSON `null` — Enums and Entities are the only two
	// fields on Model without json:",omitempty", so unlike relationships and
	// operations they are emitted as a present-but-null key, and `null` is
	// the shape a consumer expecting a list is least likely to survive. The
	// browser editor that provoked this typed them as non-optional arrays and
	// dereferenced .length on both, which got past TypeScript and threw at
	// render; the normalisation stays because the JSON projection outlives
	// the consumer that found the bug.
	//
	// The missing-file bootstrap in Load already builds real empty slices, so
	// a brand-new project was fine and only a real file that happened to omit
	// a key was broken — which is why this survived first-run testing.
	//
	// Normalising here rather than adding omitempty keeps the wire shape
	// stable in the direction the UI's types already promise, and mirrors the
	// idiom the validate handler uses for its own findings list.
	if model.Enums == nil {
		model.Enums = []Enum{}
	}
	if model.Entities == nil {
		model.Entities = []Entity{}
	}
	return model, nil
}
