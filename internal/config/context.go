package config

import (
	"context"
	"path/filepath"
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
}

// NewContext builds a Context from a ResolutionResult and an optional
// roots index. Either may be nil during partial setup.
func NewContext(result *ResolutionResult, idx *RootsIndex) *Context {
	c := &Context{Resolution: result, Index: idx}
	if result != nil {
		c.Root = result.ActiveRoot
	}
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
