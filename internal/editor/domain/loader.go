// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/domain-model-loader-reuse

package domain

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
)

// Model is the in-memory domain model the editor loads, edits, and saves.
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
	Name     string `yaml:"name" json:"name"`
	Type     string `yaml:"type" json:"type"`
	Target   string `yaml:"target,omitempty" json:"target,omitempty"`
	Enum     string `yaml:"enum,omitempty" json:"enum,omitempty"`
	Required bool   `yaml:"required" json:"required"`
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
// path the command-line read paths use, so the editor and CLI agree on
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
		return Model{}, "", fmt.Errorf("domain: read model: %w", err)
	}

	model, err := decodeAndMigrate(raw, CurrentSchemaVersion)
	if err != nil {
		return Model{}, "", err
	}
	return model, computeEtag(raw), nil
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
		return Model{}, fmt.Errorf("domain: parse model: %w", err)
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
		return Model{}, fmt.Errorf("domain: parse model: %w", err)
	}

	// Parse the operations block generically for the JSON wire view.
	var opsProbe struct {
		Operations []map[string]any `yaml:"operations"`
	}
	if err := yaml.Unmarshal(raw, &opsProbe); err != nil {
		return Model{}, fmt.Errorf("domain: parse operations: %w", err)
	}
	model.Operations = opsProbe.Operations
	model.rawOperations = captureOperationsBlock(raw)

	// Migrate an older model in memory to the current shape. v1 is the first
	// released version; migration here is the version bump itself (no field
	// transforms are defined yet). The on-disk file is not written.
	if onDisk < currentVersion {
		model.SchemaVersion = currentVersion
	}
	return model, nil
}
