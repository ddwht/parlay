// parlay-extends: studio-support/domain-model-yaml-migration/domain-model-path-resolution
// parlay-extends: studio-support/domain-model-yaml-migration/domain-model-read-path-precedence
// parlay-extends: studio-support/studio-cli-hooks/runtime-studio-detection

package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Context carries the resolved active root and (when applicable) the
// parent's roots index. New, multi-root-aware code paths read paths
// through Context methods; legacy single-root code paths use the free
// functions in config.go (which compute paths relative to cwd).
//
// Context is constructed by the cobra entry layer's PersistentPreRunE
// from a successful ResolveActiveRoot call, and threaded through the
// per-command context via WithCtx / FromCtx.
type Context struct {
	Root       Root
	Index      *RootsIndex
	Resolution *ResolutionResult

	// domainModelCache is the per-Context cache for the parsed
	// domain-model.yaml, populated by the first LoadDomainModel call.
	// Nil means "not yet loaded"; pass a fresh Context to re-read from
	// disk. See parlay-extends: studio-support/domain-model-yaml-migration/domain-model-read-path-precedence.
	domainModelCache *DomainModelArtifact

	// studioDetection is the per-process record of whether
	// parlay-studio is on PATH and runnable, populated once during
	// root resolution (resolver.go) and consulted by the trio-command
	// hook surfaces and `parlay status`. Reads go through
	// (*Context).StudioDetection in studio.go so the field stays
	// unexported. See parlay-extends:
	// studio-support/studio-cli-hooks/runtime-studio-detection.
	studioDetection StudioDetection
}

// NewContext builds a Context from a ResolutionResult and an optional
// roots index. Either may be nil during partial setup.
//
// The returned Context carries an empty StudioDetection — call
// (*Context).SetStudioDetection from the cobra entry layer (or use
// NewContextWithStudioDetection) to record the per-process result of
// detectStudioFromOS. Keeping detection out of NewContext lets unit
// tests construct a Context without invoking the live filesystem.
func NewContext(result *ResolutionResult, idx *RootsIndex) *Context {
	c := &Context{Resolution: result, Index: idx}
	if result != nil {
		c.Root = result.ActiveRoot
	}
	return c
}

// NewContextWithStudioDetection is the production-side constructor used
// by the cobra entry layer. It wraps NewContext and attaches the
// freshly-detected StudioDetection so every command handler reads the
// same record without having to re-detect.
//
// Tests that need a Studio-aware Context should construct a *Context
// via NewContext and call SetStudioDetection with a hand-built record
// — they should NOT route through this helper, which calls into the
// live filesystem.
func NewContextWithStudioDetection(result *ResolutionResult, idx *RootsIndex) *Context {
	c := NewContext(result, idx)
	c.SetStudioDetection(detectStudioFromOS())
	return c
}

// RepoRoot returns the repo-level root path: the parent's path when the
// active root is a child, else the active root's own path. This is the
// single source of truth for "which root owns schemas and the agent
// surface."
func (c *Context) RepoRoot() string {
	if c == nil {
		return ""
	}
	if c.Root.Kind == RootKindChild && c.Root.ParentPath != "" {
		return c.Root.ParentPath
	}
	return c.Root.Path
}

// ConfigPath is the active root's .parlay/config.yaml.
func (c *Context) ConfigPath() string {
	return filepath.Join(c.Root.Path, ParlayDir, ConfigFile)
}

// SchemasPath is the repo-level root's .parlay/schemas/ — schemas always
// live at the parent and child roots inherit them.
func (c *Context) SchemasPath() string {
	return filepath.Join(c.RepoRoot(), ParlayDir, SchemasDir)
}

// BlueprintPath is the active root's .parlay/blueprint.yaml. The blueprint
// is per-root for now (no inheritance rule defined in v1).
func (c *Context) BlueprintPath() string {
	return filepath.Join(c.Root.Path, ParlayDir, BlueprintFile)
}

// AdaptersPath is the active root's .parlay/adapters/. Adapters can be
// overridden per-child; child-first lookup is performed by ResolveAdapter.
// This method returns the active root's directory only.
func (c *Context) AdaptersPath() string {
	return filepath.Join(c.Root.Path, ParlayDir, AdaptersDir)
}

// PagesPath is the active root's spec/pages/.
func (c *Context) PagesPath() string {
	return filepath.Join(c.Root.Path, SpecDir, PagesDir)
}

// BuildRoot is the active root's .parlay/build/ directory.
func (c *Context) BuildRoot() string {
	return filepath.Join(c.Root.Path, ParlayDir, BuildDir)
}

// BuildPath is the per-feature build directory under the active root.
// Mirrors the qualified-identifier resolution of the legacy free
// function: "init/feat" → .parlay/build/init/feat.
func (c *Context) BuildPath(identifier string) string {
	return resolveQualifiedPath(identifier, filepath.Join(c.Root.Path, ParlayDir, BuildDir))
}

// HandoffRoot is the active root's spec/handoff/ directory.
func (c *Context) HandoffRoot() string {
	return filepath.Join(c.Root.Path, SpecDir, HandoffDir)
}

// HandoffPath is the per-feature engineering handoff directory.
func (c *Context) HandoffPath(identifier string) string {
	return resolveQualifiedPath(identifier, filepath.Join(c.Root.Path, SpecDir, HandoffDir))
}

// ProjectBuildPath is the active root's project-level build state directory.
func (c *Context) ProjectBuildPath() string {
	return filepath.Join(c.Root.Path, ParlayDir, BuildDir, "_project")
}

// FeaturePath is the per-feature directory under the active root's
// spec/intents/ tree. The identifier may include an initiative segment
// ("init/feat") which expands to spec/intents/init/feat.
func (c *Context) FeaturePath(identifier string) string {
	return resolveQualifiedPath(identifier, filepath.Join(c.Root.Path, SpecDir, IntentsDir))
}

// IntentsRoot is the active root's spec/intents/ directory.
func (c *Context) IntentsRoot() string {
	return filepath.Join(c.Root.Path, SpecDir, IntentsDir)
}

// AllFeatures enumerates every qualified feature identifier under the
// active root's spec/intents/ tree. Bypasses the package-global cache
// in scanFeatureTree, which is single-root-aware.
func (c *Context) AllFeatures() ([]string, error) {
	return ScanFeatureTree(c.IntentsRoot())
}

// LoadProjectConfig reads <activeRoot>/.parlay/config.yaml into a
// ProjectConfig struct. Returns the underlying os.ReadFile / yaml
// errors when the file is missing or malformed. The active-root-aware
// counterpart of the legacy package-level Load().
func (c *Context) LoadProjectConfig() (*ProjectConfig, error) {
	return loadProjectConfigAt(c.ConfigPath())
}

// SaveProjectConfig writes the given config to <activeRoot>/.parlay/config.yaml.
func (c *Context) SaveProjectConfig(cfg *ProjectConfig) error {
	return saveProjectConfigAt(c.ConfigPath(), cfg)
}

// ResolveAdapter performs child-first adapter file resolution: returns
// the path to <active>/.parlay/adapters/<name>.adapter.yaml when present,
// else the parent's same-named file. Returns the chosen path and a
// ResourceProvider naming the source. Empty string means no match.
func (c *Context) ResolveAdapter(name string) (string, ResourceProvider) {
	fname := name
	if filepath.Ext(fname) == "" {
		fname = name + ".adapter.yaml"
	}
	childPath := filepath.Join(c.Root.Path, ParlayDir, AdaptersDir, fname)
	if fileExists(childPath) {
		if c.Root.Kind == RootKindChild {
			return childPath, ProvidedByChild
		}
		return childPath, ProvidedByRootOnly
	}
	if c.Root.Kind == RootKindChild && c.Root.ParentPath != "" {
		parentPath := filepath.Join(c.Root.ParentPath, ParlayDir, AdaptersDir, fname)
		if fileExists(parentPath) {
			return parentPath, ProvidedByParent
		}
	}
	return "", ProvidedByRootOnly
}

// ctxKeyType is an unexported type for the context key, per the
// context.WithValue contract.
type ctxKeyType struct{}

// ctxKey is the key under which a *Context is stored on a context.Context.
var ctxKey ctxKeyType

// WithCtx returns a new context.Context carrying the parlay *Context.
// Used by the cobra entry layer (PersistentPreRunE) to thread the
// resolved root through to command handlers. A nil parent is treated
// as context.Background() so callers don't need to defend against
// cobra commands that haven't had SetContext called yet.
func WithCtx(parent context.Context, c *Context) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithValue(parent, ctxKey, c)
}

// FromCtx returns the *Context stored on ctx, or nil if none was set.
// Command handlers call this to opt into multi-root-aware path lookup.
func FromCtx(ctx context.Context) *Context {
	if ctx == nil {
		return nil
	}
	c, _ := ctx.Value(ctxKey).(*Context)
	return c
}

// --- Domain-model path resolution ---
//
// parlay-extends: studio-support/domain-model-yaml-migration/domain-model-path-resolution

// DomainModelFile is the canonical filename of the project's domain model.
// Exactly one canonical model per parlay project, located at the active
// root — never per-feature.
const DomainModelFile = "domain-model.yaml"

// DomainModelPath returns the absolute path to the active root's
// domain-model.yaml. Each child root in a multi-root project has an
// independent domain-model.yaml; one child's model never bleeds into
// another's. Path resolution is via the same active-root resolver every
// other command uses (cwd-walk, --root flag, PARLAY_ROOT) — read paths
// consult this method instead of hard-coding the path.
//
// The repo-level root holds the schema (under
// .parlay/schemas/domain-model.schema.md), shared across every active
// root via SchemasPath() — no new behavior there.
func (c *Context) DomainModelPath() string {
	if c == nil {
		return ""
	}
	return filepath.Join(c.Root.Path, DomainModelFile)
}

// LegacyDomainModelMarkdownPath returns the path to the pre-migration
// domain-model.md (if any) at the active root. The post-migration
// world treats this file as historical-only — it is never parsed,
// never merged, never consulted as a fallback by tooling. Exposed so
// migrate-domain-model and diagnostics can surface it without
// hard-coding the filename.
func (c *Context) LegacyDomainModelMarkdownPath() string {
	if c == nil {
		return ""
	}
	return filepath.Join(c.Root.Path, "domain-model.md")
}

// --- Domain-model read-path precedence ---
//
// parlay-extends: studio-support/domain-model-yaml-migration/domain-model-read-path-precedence

// ErrNoDomainModel is the sentinel returned by LoadDomainModel when
// neither domain-model.yaml nor (post-migration) any other source is
// available. Callers translate this into their own actionable errors —
// extract writes a fresh YAML; load and other consumers fail with a
// message pointing at parlay migrate-domain-model or
// parlay extract-domain-model.
var ErrNoDomainModel = errors.New("no domain-model.yaml at active root")

// DomainModelArtifact is the in-memory representation of a parsed
// domain-model.yaml. It mirrors the schema declared in
// .parlay/schemas/domain-model.schema.md and is the merged-model layer
// the deep validator and consumers operate on.
//
// The shape is deliberately permissive at the YAML level (every field
// optional except SchemaVersion); semantic validation lives in
// agent.ValidateDomainModel.
type DomainModelArtifact struct {
	SchemaVersion int                  `yaml:"schema_version"`
	Enums         []DomainEnum         `yaml:"enums,omitempty"`
	Entities      []DomainEntity       `yaml:"entities,omitempty"`
	Relationships []DomainRelationship `yaml:"relationships,omitempty"`
	Operations    []DomainOperation    `yaml:"operations,omitempty"`
}

type DomainEnum struct {
	Name   string            `yaml:"name"`
	Values []DomainEnumValue `yaml:"values"`
}

type DomainEnumValue struct {
	Value string `yaml:"value"`
	Label string `yaml:"label,omitempty"`
	Tone  string `yaml:"tone,omitempty"`
}

type DomainEntity struct {
	Name   string        `yaml:"name"`
	Fields []DomainField `yaml:"fields"`
}

type DomainField struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Target   string `yaml:"target,omitempty"`
	Enum     string `yaml:"enum,omitempty"`
	Required bool   `yaml:"required"`
}

type DomainRelationship struct {
	Name        string `yaml:"name"`
	From        string `yaml:"from"`
	To          string `yaml:"to"`
	Cardinality string `yaml:"cardinality"`
}

type DomainOperation struct {
	Name    string   `yaml:"name"`
	Input   []string `yaml:"input"`
	Effects []string `yaml:"effects,omitempty"`
}

// LoadDomainModel reads and parses the active root's domain-model.yaml.
// It is the single enforcement point for the read-path precedence rule:
//
//   - The .yaml is the only live source.
//   - The legacy .md (if present, as a pre-migration artifact) is
//     never parsed, never merged, never consulted as a fallback.
//   - When the .yaml is absent, the method returns ErrNoDomainModel —
//     callers translate this into their own actionable errors
//     (extract writes a fresh YAML; load and consumers exit non-zero
//     pointing at parlay migrate-domain-model or
//     parlay extract-domain-model).
//
// The parse is cached per-Context: a second call within the same CLI
// invocation reuses the in-memory artifact. Pass a fresh Context to
// re-read from disk.
func (c *Context) LoadDomainModel() (*DomainModelArtifact, error) {
	if c == nil {
		return nil, ErrNoDomainModel
	}
	if c.domainModelCache != nil {
		return c.domainModelCache, nil
	}
	path := c.DomainModelPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoDomainModel
		}
		return nil, fmt.Errorf("read domain-model.yaml: %w", err)
	}
	var artifact DomainModelArtifact
	if err := yaml.Unmarshal(data, &artifact); err != nil {
		return nil, fmt.Errorf("parse domain-model.yaml: %w", err)
	}
	c.domainModelCache = &artifact
	return &artifact, nil
}
