package config

// RootKind classifies a root's role in the parent-child topology.
type RootKind string

const (
	RootKindParent     RootKind = "parent"
	RootKindChild      RootKind = "child"
	RootKindStandalone RootKind = "standalone"
)

// Root is a single parlay root: a directory containing .parlay/.
type Root struct {
	// Name is the short slug used to address the root within its parent's
	// roots index. For standalone or parent roots, defaults to filepath.Base(Path).
	Name string
	// Path is the absolute path to the directory containing .parlay/.
	Path string
	// ParentPath is the absolute path to the parent root, when Kind == Child.
	// Empty for Parent and Standalone roots.
	ParentPath string
	// RelativePath is the path of this root relative to its parent root,
	// when Kind == Child. Empty otherwise.
	RelativePath string
	// Kind is one of RootKindParent, RootKindChild, RootKindStandalone.
	Kind RootKind
	// Description is a one-line description copied into the agent-rules
	// multi-root section, when set.
	Description string
}

// RootsIndex is the parent root's registry of children. The on-disk
// format lives at <parent>/.parlay/roots.yaml.
type RootsIndex struct {
	ParentPath string
	Children   []Root
}

// Lookup returns the named child root and true on hit, zero-value Root
// and false on miss.
func (idx *RootsIndex) Lookup(name string) (Root, bool) {
	if idx == nil {
		return Root{}, false
	}
	for _, c := range idx.Children {
		if c.Name == name {
			return c, true
		}
	}
	return Root{}, false
}

// Names returns the names of every registered child, sorted.
func (idx *RootsIndex) Names() []string {
	if idx == nil {
		return nil
	}
	out := make([]string, 0, len(idx.Children))
	for _, c := range idx.Children {
		out = append(out, c.Name)
	}
	return out
}

// ResolutionSource records why a particular root won resolution.
type ResolutionSource string

const (
	SourceCwdWalkUp           ResolutionSource = "cwd-walk-up"
	SourceParlayRootEnv       ResolutionSource = "parlay-root-env"
	SourceRootFlag            ResolutionSource = "root-flag"
	SourcePrefix              ResolutionSource = "prefix"
	SourceDisambiguation      ResolutionSource = "disambiguation"
	SourceAutoResolvedOnlyOne ResolutionSource = "auto-resolved-only-match"
)

// ResolutionResult is the resolved active root and the source of the decision.
type ResolutionResult struct {
	ActiveRoot           Root
	Source               ResolutionSource
	AnnouncementRequired bool
}

// CandidateReason explains why a root surfaced as a candidate during
// disambiguation.
type CandidateReason string

const (
	ReasonDiscoveredBelowCwd CandidateReason = "discovered-below-cwd"
	ReasonInRootsIndex       CandidateReason = "in-parents-roots-index"
	ReasonContainsFeature    CandidateReason = "contains-feature"
)

// Candidate is one option presented in disambiguation output. It carries
// just enough information to render a user prompt; the full Root is loaded
// later when the user makes a choice.
type Candidate struct {
	Name         string
	RelativePath string
	Reason       CandidateReason
}

// ResourceType is one of the resource categories a root can resolve.
type ResourceType string

const (
	ResourceSchemas        ResourceType = "schemas"
	ResourceAdapter        ResourceType = "adapter"
	ResourceDeployedSkills ResourceType = "deployed-skills"
	ResourceDomainModel    ResourceType = "domain-model"
	ResourceIntents        ResourceType = "intents"
	ResourceDialogs        ResourceType = "dialogs"
	ResourceSurface        ResourceType = "surface"
)

// ResourceProvider names which root supplied a resource.
type ResourceProvider string

const (
	ProvidedByParent   ResourceProvider = "parent-root"
	ProvidedByChild    ResourceProvider = "child-root"
	ProvidedByRepo     ResourceProvider = "repo-level-root"
	ProvidedByRootOnly ResourceProvider = "root-only"
)

// OverrideStatus describes whether a child overrode the parent's resource.
type OverrideStatus string

const (
	OverrideOverride OverrideStatus = "override"
	OverrideFallback OverrideStatus = "fallback"
	OverrideRootOnly OverrideStatus = "root-only"
)

// ResourceLoad records one resource resolution decision (used for verbose output).
type ResourceLoad struct {
	ResourceType   ResourceType
	ProvidedBy     ResourceProvider
	OverrideStatus OverrideStatus
}

// ForbiddenRule names which root-only rule a child violated.
type ForbiddenRule string

const (
	RuleSchemasParentOnly      ForbiddenRule = "schemas-parent-only"
	RuleAgentSurfaceParentOnly ForbiddenRule = "agent-surface-parent-only"
)

// ForbiddenDirectoryViolation captures a child root that contains a
// parent-only directory.
type ForbiddenDirectoryViolation struct {
	ChildRootPath string
	Directory     string
	Rule          ForbiddenRule
}

// Error implements the error interface so callers can return a violation
// directly. The message names the rule and the offending path.
func (v *ForbiddenDirectoryViolation) Error() string {
	switch v.Rule {
	case RuleSchemasParentOnly:
		return "schemas live at the parent root only; found " + v.Directory
	case RuleAgentSurfaceParentOnly:
		return "deployed agent surface lives at the repo-level root only; found " + v.Directory
	}
	return "forbidden directory in child root: " + v.Directory
}
