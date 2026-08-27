// parlay-section: cross-cutting
// parlay-extends: qualified-identifier-resolver/qualified-path-resolver
// parlay-extends: qualified-identifier-resolver/feature-enumeration-helper
// parlay-extends: initiatives/directory-classification-validation
// parlay-extends: initiatives/duplicate-slug-detection
// parlay-extends: initiatives/cross-tree-traversal-consistency
// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-trio-commands

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ProjectConfig struct {
	AIAgent      string `yaml:"ai-agent"`
	SDDFramework string `yaml:"sdd-framework"`
	// prototype-framework: was removed in v0.3. migrate-config still reads
	// the raw key via its own inline struct to convert old projects into an
	// adapter-set; nothing else may consult it.

	// NoEditor mirrors the parlay.no_editor key in .parlay/config.yaml:
	// when true, the offer to open the domain-model editor is suppressed
	// for every invocation in this project, regardless of the --no-editor
	// flag. The merge with the per-invocation flag is logical OR — either
	// source suppresses the offer.
	//
	// parlay-extends: studio-support/studio-cli-hooks/no-studio-flag-trio-commands
	NoEditor bool `yaml:"no_editor,omitempty"`

	// no_studio: (the pre-rename spelling of no_editor) was removed in
	// v0.3. A config still carrying it gets the default behavior; set
	// parlay.no_editor.

	// Feedback mirrors the parlay.feedback key in .parlay/config.yaml:
	// when true, findings and per-run tallies are appended to
	// .parlay/feedback/<date>.jsonl. Off by default, and overridable
	// per-run in both directions by PARLAY_FEEDBACK.
	//
	// Recorded locally and never transmitted — parlay has no network path
	// — but written to be SENT: a user reproduces a problem and forwards
	// the bundle from `parlay feedback-export`. That is why capture is
	// sanitised rather than the export being filtered. An earlier revision
	// of this comment said "local-only", which was true of the mechanism
	// and misleading about the intent.
	//
	// parlay-feature: parlay-tool/feedback-mode
	Feedback bool `yaml:"feedback,omitempty"`

	// AllowMachineCriteriaAuthority mirrors the
	// parlay.criterion-authority.allow-machine key: when true, this project
	// permits a run to generate code without a person having approved the
	// criteria it will be graded against, PROVIDED that invocation also passes
	// --authorize-criteria=machine.
	//
	// Two switches rather than one, and default deny, on purpose. The config
	// answers "may this project ever waive the separation between authoring a
	// standard and grading against it?" — a governance choice a team makes
	// once, in a committed file, where it is reviewable. The flag answers "is
	// this run exercising that permission?" A flag alone would make a
	// high-consequence waiver look like an ordinary convenience switch,
	// available in any project to anyone who found it; a setting alone would
	// make every unattended run silently self-authorizing forever after one
	// person enabled it.
	AllowMachineCriteriaAuthority bool `yaml:"criterion-authority.allow-machine,omitempty"`

	// ledger: was removed in v0.4 — the ledger-and-contract model is the
	// only regime, so there is nothing left for the flag to gate. A config
	// still carrying the key is silently inert (decoding is non-strict).
	// Old projects whose founding docs drifted before the switch run
	// `parlay migrate-ledger` once to accept current text as the founding
	// state.
}

// NoEditorEnabled reports whether this project has opted out of the
// open-editor offer via parlay.no_editor. Callers use this rather than
// reading the field so the answer stays single-sourced. (The pre-rename
// no_studio spelling was honoured here until v0.3.)
func (c *ProjectConfig) NoEditorEnabled() bool {
	if c == nil {
		return false
	}
	return c.NoEditor
}

// MachineCriteriaAuthorityAllowed reports whether this project has opted in to
// waiving the separation between authoring a grading standard and grading
// against it. Read through here rather than off the field so the answer stays
// single-sourced, and so the default — deny — lives in one place.
func (c *ProjectConfig) MachineCriteriaAuthorityAllowed() bool {
	if c == nil {
		return false
	}
	return c.AllowMachineCriteriaAuthority
}

const (
	ParlayDir     = ".parlay"
	ConfigFile    = "config.yaml"
	BlueprintFile = "blueprint.yaml"
	SchemasDir    = "schemas"
	ModulesDir    = "modules"
	AdaptersDir   = "adapters"
	BuildDir      = "build"
	SpecDir       = "spec"
	IntentsDir    = "intents"
	HandoffDir    = "handoff"
	PagesDir      = "pages"

	// AuthoredFile is the declaration that marks a directory under
	// spec/intents/ as a hand-authored unit rather than a feature: code
	// parlay must never write, declared so that parlay can still track,
	// hash and depend on it. Its presence is the sole classification
	// signal — a unit also carries intents.md, so keying on that would
	// be indistinguishable from a feature.
	AuthoredFile = "authored.yaml"
)

// loadProjectConfigAt reads a ProjectConfig from a specific filesystem
// path. Used by the Context-aware (*Context).LoadProjectConfig and by
// the legacy single-root bootstrap path in `parlay init` (which writes
// the very first .parlay/config.yaml before any active root exists).
func loadProjectConfigAt(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveProjectConfigAt(path string, cfg *ProjectConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// --- Qualified identifier resolver (cross-cutting: qualified-path-resolver) ---

func resolveQualifiedPath(identifier, treeRoot string) string {
	if strings.Contains(identifier, "/") {
		parts := strings.SplitN(identifier, "/", 2)
		return filepath.Join(treeRoot, parts[0], parts[1])
	}
	return filepath.Join(treeRoot, identifier)
}

// --- Feature enumeration helper (cross-cutting: feature-enumeration-helper) ---
// --- Extended by: initiatives/directory-classification-validation ---
// --- Extended by: initiatives/duplicate-slug-detection ---
// --- Extended by: initiatives/cross-tree-traversal-consistency ---

// DirClass represents the classification of a directory under spec/intents/.
type DirClass int

const (
	DirClassFeature    DirClass = 1
	DirClassInitiative DirClass = 2
	DirClassDeferred   DirClass = 3
	DirClassAuthored   DirClass = 4
)

// ClassifyDir examines a directory and returns its classification.
// Authored: contains authored.yaml — a hand-authored unit, checked before
// the feature rule because a unit carries intents.md too and would
// otherwise classify as a feature. Feature: contains intents.md directly.
// Initiative: contains direct-child subdirectories with intents.md (checks
// only direct children, never recurses). Deferred: matches none of the
// above. Returns an error for hybrid directories (both intents.md and
// child dirs with intents.md) — a unit is not exempt from that rule, since
// a unit with feature children is as malformed as a feature with them.
func ClassifyDir(path string) (DirClass, error) {
	isFeature := hasIntentsMd(path)
	isAuthored := hasAuthoredYaml(path)

	children, err := os.ReadDir(path)
	if err != nil {
		if isAuthored {
			return DirClassAuthored, nil
		}
		if isFeature {
			return DirClassFeature, nil
		}
		return DirClassDeferred, nil
	}

	hasChildFeatures := false
	for _, child := range children {
		if !child.IsDir() {
			continue
		}
		if hasIntentsMd(filepath.Join(path, child.Name())) {
			hasChildFeatures = true
			break
		}
	}

	if isFeature && hasChildFeatures {
		return 0, fmt.Errorf("hybrid directory at %s: contains intents.md (feature) and subdirectories with intents.md (initiative) — a directory cannot be both", path)
	}
	if isAuthored && hasChildFeatures {
		return 0, fmt.Errorf("hybrid directory at %s: contains authored.yaml (hand-authored unit) and subdirectories with intents.md (initiative) — a unit owns no features; move the declaration down to the unit's own directory", path)
	}
	if isAuthored {
		return DirClassAuthored, nil
	}
	if isFeature {
		return DirClassFeature, nil
	}
	if hasChildFeatures {
		return DirClassInitiative, nil
	}
	return DirClassDeferred, nil
}

// CheckSlugUniqueness verifies that no two sibling directories under parentDir
// slugify to the same identifier. Returns an error listing conflicting paths
// when duplicates are found. This guards against external filesystem corruption.
func CheckSlugUniqueness(parentDir string) error {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return nil
	}

	slugMap := make(map[string][]string)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := slugifyDirName(entry.Name())
		slugMap[slug] = append(slugMap[slug], filepath.Join(parentDir, entry.Name()))
	}

	for slug, paths := range slugMap {
		if len(paths) > 1 {
			return fmt.Errorf("duplicate slug %q under %s: directories %s resolve to the same identifier — remove or rename one, then run parlay repair", slug, parentDir, strings.Join(paths, " and "))
		}
	}
	return nil
}

func slugifyDirName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, " ", "-")
	return name
}

// ScanFeatureTree walks the given tree root and returns qualified
// feature identifiers. This is the sole public entry point for feature
// enumeration — every caller, single-root or multi-root, should resolve
// its own tree root (typically via (*Context).IntentsRoot() and
// (*Context).AllFeatures(), the Context-bound wrapper below) and call
// this directly.
//
// A previous package-level AllFeatures()/AllFeaturePaths() pair existed
// here with a sync.Once-cached result keyed to a single cwd-relative
// spec/intents/ path. That cache was unsound for anything but a single
// cwd-anchored invocation per process — every multi-root caller, and
// every test that constructs more than one project tree in the same
// test binary, would have silently read another root's stale cached
// result. Nothing in this codebase depended on the free functions by
// the time this was noticed (every real caller already went through
// (*Context).AllFeatures()), so they were removed rather than fixed.
func ScanFeatureTree(treeRoot string) ([]string, error) {
	features, _, err := scanTree(treeRoot)
	return features, err
}

// ScanUnitTree walks the given tree root and returns qualified
// hand-authored unit identifiers — the DirClassAuthored counterpart of
// ScanFeatureTree, sharing its single traversal so the two enumerations
// can never disagree about what the tree contains.
func ScanUnitTree(treeRoot string) ([]string, error) {
	_, units, err := scanTree(treeRoot)
	return units, err
}

// scanTree is the one walk behind both enumerations. Splitting it into a
// feature walk and a unit walk would reintroduce the divergence the
// classification rules exist to prevent: the two would drift on hybrid
// handling, slug uniqueness and initiative nesting independently.
func scanTree(treeRoot string) (features []string, units []string, err error) {
	entries, err := os.ReadDir(treeRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read %s: %w", treeRoot, err)
	}

	if err := CheckSlugUniqueness(treeRoot); err != nil {
		return nil, nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		topSlug := entry.Name()
		topPath := filepath.Join(treeRoot, topSlug)

		cls, err := ClassifyDir(topPath)
		if err != nil {
			return nil, nil, err
		}

		switch cls {
		case DirClassFeature:
			features = append(features, topSlug)
		case DirClassAuthored:
			units = append(units, topSlug)
		case DirClassInitiative:
			if err := CheckSlugUniqueness(topPath); err != nil {
				return nil, nil, err
			}
			children, childErr := os.ReadDir(topPath)
			if childErr != nil {
				continue
			}
			for _, child := range children {
				if !child.IsDir() {
					continue
				}
				childPath := filepath.Join(topPath, child.Name())
				childCls, childClsErr := ClassifyDir(childPath)
				if childClsErr != nil {
					return nil, nil, childClsErr
				}
				switch childCls {
				case DirClassFeature:
					features = append(features, topSlug+"/"+child.Name())
				case DirClassAuthored:
					units = append(units, topSlug+"/"+child.Name())
				case DirClassInitiative:
					return nil, nil, fmt.Errorf("sub-initiative at %s: contains subdirectories with intents.md at depth 2, violating the flat-hierarchy rule — initiatives can only be direct children of %s", childPath, treeRoot)
				case DirClassDeferred:
					// valid, silently skipped in enumeration
				default:
					return nil, nil, unhandledDirClass(childCls, childPath)
				}
			}
		case DirClassDeferred:
			// valid, silently skipped in enumeration
		default:
			return nil, nil, unhandledDirClass(cls, topPath)
		}
	}

	return features, units, nil
}

// unhandledDirClass turns the silent-drop failure mode into a loud one.
// Both switches above used to have no default arm, so a DirClass added
// without extending them would have quietly vanished from every
// enumeration — the tree would simply stop containing those directories,
// with no error anywhere to explain it.
func unhandledDirClass(cls DirClass, path string) error {
	return fmt.Errorf("unhandled directory classification %d at %s: a DirClass was added without extending scanTree — every enumeration is now silently missing these directories", cls, path)
}

func hasIntentsMd(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "intents.md"))
	if err != nil {
		_, err = os.Stat(filepath.Join(dir, "Intents.md"))
	}
	return err == nil
}

// HasIntentsMd is the exported form for use by commands that need
// to check directory classification without the full traversal.
func HasIntentsMd(dir string) bool {
	return hasIntentsMd(dir)
}

func hasAuthoredYaml(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, AuthoredFile))
	return err == nil
}

// IsAuthoredUnit reports whether a directory carries the hand-authored
// unit declaration. The exported single-directory check, for commands
// that must branch on the class of one identifier rather than enumerate
// the tree — several of them (repair, status, check-coverage) treat a
// missing dialogs.md or handoff twin as a defect, which for a unit is
// the normal and correct state.
func IsAuthoredUnit(dir string) bool {
	return hasAuthoredYaml(dir)
}
