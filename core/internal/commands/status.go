// parlay-feature: parlay-tool/status-feature-phases
// parlay-component: hierarchical-listing
// parlay-extends: parlay-tool/status-feature-phases/phase-column
// parlay-extends: parlay-tool/status-feature-phases/unavailable-child-diagnostic
// parlay-extends: parlay-tool/status-feature-phases/json-envelope
// parlay-extends: parlay-tool/status-feature-phases/exit-and-stream-discipline
// parlay-extends: parlay-tool/status-feature-phases/status-command-call-site
// parlay-extends: parlay-tool/status-feature-phases/cross-root-walk

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
)

// statusJSON is the package-level toggle for `parlay status --json`.
// Default (false) preserves the human tabular output. When true, the
// command emits a single JSON document on stdout — never both, never
// interleaved.
var statusJSON bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the active parlay root, its features, and any registered child roots",
	Long: `Print a short summary of the resolved parlay state for the current
invocation: the active root path and kind, the features authored under
spec/intents/, and (for parent roots) any registered child roots.

A parent root with an empty spec/intents/ — i.e. all features live in
children — is a normal supported state. status reports zero parent
features and lists the children, with no warning.

When --json is passed, the human listing is suppressed and a single
JSON document is written to stdout in the shape:

  {
    "schema_version": 1,
    "root":     { "path": ..., "kind": ..., "source": ..., "features": [...] },
    "children": [ { "name": ..., "path": ..., "features": [...], "unavailable": "..." }, ... ]
  }

children is always present and is [] for child or standalone active
roots. Each feature entry has at minimum {id, phase}.`,
	Args: cobra.NoArgs,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false,
		"Emit machine-readable JSON output on stdout (suppresses human listing)")
}

// ---------------------------------------------------------------------
// JSON envelope shapes — snake_case json tags, contract floor.
// ---------------------------------------------------------------------

type featureEntry struct {
	ID    string       `json:"id"`
	Phase FeaturePhase `json:"phase"`
}

type rootSection struct {
	Path     string         `json:"path"`
	Kind     config.RootKind `json:"kind"`
	Source   string         `json:"source"`
	Features []featureEntry `json:"features"`
}

type childRootEntry struct {
	Name        string         `json:"name"`
	Path        string         `json:"path"`
	Features    []featureEntry `json:"features"`
	Unavailable string         `json:"unavailable,omitempty"`
}

type statusEnvelope struct {
	SchemaVersion int              `json:"schema_version"`
	Root          rootSection      `json:"root"`
	Children      []childRootEntry `json:"children"`
}

// statusSchemaVersion is the integer schema_version pinned in the
// envelope. Bumped only on breaking changes; additive fields do not
// bump.
const statusSchemaVersion = 1

// ---------------------------------------------------------------------
// Entry point — dispatch on --json.
// ---------------------------------------------------------------------

func runStatus(cmd *cobra.Command, args []string) error {
	pctx := config.FromCtx(cmd.Context())
	if pctx == nil {
		// No active root resolved. Stderr discipline: error returns
		// flow up through cobra to stderr; stdout stays empty (no
		// partial envelope, no empty object). The JSON contract is
		// broken by absence, never by an invalid envelope.
		return fmt.Errorf("no active parlay root")
	}

	if statusJSON {
		return runStatusJSON(pctx)
	}
	return runStatusHuman(pctx)
}

// ---------------------------------------------------------------------
// Human path — hierarchical listing with the Phase column.
// ---------------------------------------------------------------------

func runStatusHuman(pctx *config.Context) error {
	// Active root header — kept identical to the legacy output modulo
	// the Phase column added per feature line below.
	fmt.Printf("root:     %s\n", pctx.Root.Path)
	fmt.Printf("kind:     %s\n", pctx.Root.Kind)
	if pctx.Root.Kind == config.RootKindChild && pctx.Root.ParentPath != "" {
		fmt.Printf("parent:   %s\n", pctx.Root.ParentPath)
	}
	if pctx.Resolution != nil {
		fmt.Printf("source:   %s\n", pctx.Resolution.Source)
	}

	// Active root's own features. Treat a missing intents/ tree as
	// zero features — bare-parent topology is supported, not an
	// error. The Phase column is appended to every feature line via
	// the shared ComputeFeaturePhase helper; status.go performs no
	// file-existence logic of its own.
	features, err := scanFeaturesAtTolerant(pctx.IntentsRoot())
	if err != nil {
		return err
	}
	renderFeaturesHuman(pctx, features)

	// Cross-root walk: when the active root is a parent, iterate
	// pctx.Index.Children IN SLICE ORDER (the on-disk roots.yaml
	// order — never alphabetized, never re-sorted). Aggregation is
	// downward-only and exactly one level deep — grandchildren are
	// not walked.
	if pctx.Root.Kind == config.RootKindParent && pctx.Index != nil && len(pctx.Index.Children) > 0 {
		walkChildRoots(pctx, func(name, path string, childCtx *config.Context, childFeatures []string, unavailable error) {
			renderChildHuman(name, path, childCtx, childFeatures, unavailable)
		})
	}
	return nil
}

// renderFeaturesHuman prints the active root's features as a tabwriter
// table with the Phase column appended. Empty features render as the
// `(none)` line.
func renderFeaturesHuman(pctx *config.Context, features []string) {
	if len(features) == 0 {
		fmt.Printf("features: (none)\n")
		return
	}
	fmt.Printf("features: %d\n", len(features))
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, f := range features {
		fmt.Fprintf(w, "  - %s\t%s\n", f, ComputeFeaturePhase(pctx, f))
	}
	w.Flush()
}

// renderChildHuman prints one child section: header line, then either
// the child's features (with Phase column) or the inline diagnostic.
// Per-child error boundary: catching the error at the per-child level
// lets the walk continue with remaining children. Exit stays zero.
func renderChildHuman(name, path string, childCtx *config.Context, childFeatures []string, unavailable error) {
	fmt.Println()
	fmt.Printf("%s\n", name)
	fmt.Printf("path:     %s\n", path)
	if unavailable != nil {
		// Single human-readable diagnostic. No stack trace. No
		// partial feature listing.
		fmt.Printf("(unavailable: %s)\n", unavailable.Error())
		return
	}
	if len(childFeatures) == 0 {
		fmt.Printf("features: (none)\n")
		return
	}
	fmt.Printf("features: %d\n", len(childFeatures))
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, f := range childFeatures {
		fmt.Fprintf(w, "  - %s\t%s\n", f, ComputeFeaturePhase(childCtx, f))
	}
	w.Flush()
}

// ---------------------------------------------------------------------
// JSON path — single envelope on stdout, no log lines, no preamble.
// ---------------------------------------------------------------------

func runStatusJSON(pctx *config.Context) error {
	env := buildStatusEnvelope(pctx)
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal status envelope: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// buildStatusEnvelope assembles the StatusEnvelope from the active root
// plus (when applicable) the registered children. Children is always
// initialized to an empty slice — never nil — so that JSON consumers
// can iterate it without a null check.
func buildStatusEnvelope(pctx *config.Context) statusEnvelope {
	env := statusEnvelope{
		SchemaVersion: statusSchemaVersion,
		Root:          buildRootSection(pctx),
		Children:      []childRootEntry{},
	}
	if pctx.Root.Kind == config.RootKindParent && pctx.Index != nil && len(pctx.Index.Children) > 0 {
		walkChildRoots(pctx, func(name, path string, childCtx *config.Context, childFeatures []string, unavailable error) {
			entry := childRootEntry{
				Name:     name,
				Path:     path,
				Features: []featureEntry{},
			}
			if unavailable != nil {
				entry.Unavailable = unavailable.Error()
			} else {
				for _, f := range childFeatures {
					entry.Features = append(entry.Features, featureEntry{
						ID:    f,
						Phase: ComputeFeaturePhase(childCtx, f),
					})
				}
			}
			env.Children = append(env.Children, entry)
		})
	}
	return env
}

// buildRootSection assembles the active root's RootSection. Missing
// spec/intents/ is treated as zero features (bare-parent topology).
// On any other scan error, features falls back to [] — the JSON
// contract requires the array to always be present.
func buildRootSection(pctx *config.Context) rootSection {
	r := rootSection{
		Path:     pctx.Root.Path,
		Kind:     pctx.Root.Kind,
		Features: []featureEntry{},
	}
	if pctx.Resolution != nil {
		r.Source = string(pctx.Resolution.Source)
	}
	features, _ := scanFeaturesAtTolerant(pctx.IntentsRoot())
	for _, f := range features {
		r.Features = append(r.Features, featureEntry{
			ID:    f,
			Phase: ComputeFeaturePhase(pctx, f),
		})
	}
	return r
}

// ---------------------------------------------------------------------
// Cross-root walker — the file-private aggregation primitive.
// ---------------------------------------------------------------------

// childRenderFn is invoked once per registered child during the
// cross-root walk. It receives the child's name, absolute path, a
// freshly-resolved *config.Context (so phase calls consult the child's
// .parlay/build, never the parent's), the enumerated features, and a
// non-nil error iff the child is unavailable.
type childRenderFn func(name, path string, childCtx *config.Context, childFeatures []string, unavailable error)

// walkChildRoots iterates pctx.Index.Children IN SLICE ORDER and calls
// render once per child. Aggregation is exactly one level deep; the
// walker never recurses into grandchildren even when a child is itself
// a parent in its own roots.yaml.
//
// Per-child error boundary: any per-child failure (missing directory,
// unreadable spec/intents/) is caught here and threaded through
// `unavailable`, so the walk continues with the remaining children.
//
// The walker is unexported by design: no external package depends on
// it. status.go owns the topology.
func walkChildRoots(pctx *config.Context, render childRenderFn) {
	parentPath := pctx.Root.Path
	if pctx.Index.ParentPath != "" {
		parentPath = pctx.Index.ParentPath
	}
	for _, child := range pctx.Index.Children {
		// Resolve the child's absolute path. roots.yaml stores a
		// parent-relative path; LoadRootsIndex normally fills in
		// child.Path, but defensively re-derive it from
		// parent + RelativePath when missing (e.g. tests that
		// construct a *RootsIndex directly).
		childPath := child.Path
		if childPath == "" {
			childPath = filepath.Join(parentPath, child.RelativePath)
		}

		childCtx := resolveChildRootContextWithPath(pctx, child, childPath)
		features, err := scanFeaturesAt(childCtx.IntentsRoot())
		render(child.Name, childPath, childCtx, features, err)
	}
}

// resolveChildRootContext instantiates a *config.Context for the child
// without re-reading roots.yaml. The returned context's Root is
// populated so per-feature path helpers (FeaturePath, BuildPath)
// resolve under the CHILD's tree, not the parent's.
func resolveChildRootContext(parentPctx *config.Context, child config.Root) *config.Context {
	return resolveChildRootContextWithPath(parentPctx, child, child.Path)
}

// resolveChildRootContextWithPath is the implementation backing
// resolveChildRootContext; it accepts the resolved absolute path so
// the cross-root walker can fill in a derived path when child.Path is
// not pre-populated.
func resolveChildRootContextWithPath(parentPctx *config.Context, child config.Root, absPath string) *config.Context {
	c := child
	c.Path = absPath
	c.Kind = config.RootKindChild
	if c.ParentPath == "" && parentPctx != nil {
		c.ParentPath = parentPctx.Root.Path
	}
	return config.NewContext(&config.ResolutionResult{ActiveRoot: c}, nil)
}

// ---------------------------------------------------------------------
// scanFeaturesAt — bare-parent-tolerant feature enumeration.
// ---------------------------------------------------------------------

// scanFeaturesAt enumerates feature identifiers under the given intents
// tree root, surfacing any os.Stat error to the caller. Used by the
// cross-root walker so per-child errors can be threaded into the
// `(unavailable: ...)` diagnostic line. The active-root path uses
// scanFeaturesAtTolerant instead, which collapses missing-on-disk to
// zero features so the bare-parent topology renders cleanly.
func scanFeaturesAt(intentsRoot string) ([]string, error) {
	if _, err := os.Stat(intentsRoot); err != nil {
		return nil, err
	}
	return config.ScanFeatureTree(intentsRoot)
}

// scanFeaturesAtTolerant enumerates features and treats a missing
// spec/intents/ tree as zero features (no error). This preserves the
// pre-existing bare-parent behaviour of `parlay status` for the active
// root.
func scanFeaturesAtTolerant(intentsRoot string) ([]string, error) {
	if _, err := os.Stat(intentsRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return config.ScanFeatureTree(intentsRoot)
}
