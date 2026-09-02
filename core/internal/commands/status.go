// parlay-feature: parlay-tool/status-feature-phases
// parlay-component: hierarchical-listing
// parlay-extends: parlay-tool/status-feature-phases/phase-column
// parlay-extends: parlay-tool/status-feature-phases/unavailable-child-diagnostic
// parlay-extends: parlay-tool/status-feature-phases/json-envelope
// parlay-extends: parlay-tool/status-feature-phases/exit-and-stream-discipline
// parlay-extends: parlay-tool/status-feature-phases/status-command-call-site
// parlay-extends: parlay-tool/status-feature-phases/cross-root-walk
// parlay-extends: parlay-tool/multi-root/status-with-bare-parent
// parlay-extends: parlay-tool/multi-root/status-topology-indicator
// parlay-cross-cutting: status-topology-line-renderer
// parlay-cross-cutting: bare-parent-empty-spec-handling

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
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
    "schema_version": 2,
    "root":     { "path": ..., "kind": ..., "source": ..., "features": [...],
                  "orphaned_build_dirs": [...], "backlog": {...} },
    "children": [ { "name": ..., "path": ..., "features": [...],
                    "orphaned_build_dirs": [...], "backlog": {...},
                    "unavailable": "..." }, ... ]
  }

children is always present and is [] for child or standalone active
roots. Each feature entry has at minimum {id, phase}.

backlog is OPTIONAL and per root: {open, untriaged, unreadable}. It is
absent when there is nothing outstanding, and absence means zero — the
same thing the human listing says by printing no backlog line. unreadable
counts records that could not be parsed and are therefore NOT in the
other two counts, so "no open items" and "no open items we could read"
stay distinguishable.`,
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
	// Kind distinguishes a hand-authored unit from an ordinary feature.
	// Additive and omitempty, so no schema_version bump: a reader that
	// does not know the field sees exactly what it saw before, and a
	// feature — the overwhelming majority — carries no kind at all.
	Kind string `json:"kind,omitempty"`

	// Activity is the SECOND axis, and it is deliberately a separate
	// field rather than more values crowded into Phase.
	//
	// Phase says how far the work has come; Activity says whether the
	// pause was chosen. They are orthogonal — a feature can be at
	// `dialogs` and parked, or at `dialogs` and simply undeclared — and
	// collapsing them into one token would destroy exactly the
	// distinction this feature exists to make. `active | parked |
	// unclassified | unavailable`.
	//
	// `active` is emitted EXPLICITLY rather than omitted as the boring
	// default, so an ordinary feature entry always carries a computed
	// answer rather than letting the common case share a representation
	// with silence.
	//
	// The field is nonetheless omitempty, and absence has exactly one
	// meaning within this schema version: the entry is a hand-authored
	// unit, for which the activity axis does not apply — a unit's code is
	// already written, so there is no work to pause. It does NOT mean
	// "active", and it does not distinguish an older parlay; the
	// schema_version does that.
	Activity string `json:"activity,omitempty"`

	// ActivityDetail is why: the parking reason, or the fault in a
	// declaration that cannot be used. A state without its reason is
	// half a record, and the complaint that started this work was that
	// seventeen lines said `dialogs` and none of them said why.
	ActivityDetail string `json:"activity_detail,omitempty"`

	// ActivityStale marks a parked feature that has since acquired
	// pipeline evidence. The state stays `parked` — a declaration
	// outranks observation — but the record has quietly stopped being
	// true, and printing it as a clean parked line would assert a
	// disposition nobody currently holds.
	ActivityStale bool `json:"activity_stale,omitempty"`
}

// KindHandAuthored is the only non-default value of featureEntry.Kind.
const KindHandAuthored = "hand-authored"

type rootSection struct {
	Path              string          `json:"path"`
	Kind              config.RootKind `json:"kind"`
	Source            string          `json:"source"`
	Features          []featureEntry  `json:"features"`
	OrphanedBuildDirs []string        `json:"orphaned_build_dirs"`
	// Backlog is what this root has observed and not done. Reported
	// alongside features because it is the other half of the same
	// question — a root with four features and eleven untriaged
	// observations is not the same project as one with four features
	// and none, and status was only ever showing the first number.
	//
	// POINTER AND OMITEMPTY, so a healthy root emits nothing. As a
	// value it was emitted on every root as {open:0,untriaged:0},
	// which is a REQUIRED new member under a schema_version already
	// deployed — the opposite of the additive change the version pin's
	// own rule allows without a bump. Absence means zero outstanding,
	// the same thing the human line says by staying silent.
	Backlog *backlogSummary `json:"backlog,omitempty"`
}

type childRootEntry struct {
	Name              string          `json:"name"`
	Path              string          `json:"path"`
	Features          []featureEntry  `json:"features"`
	OrphanedBuildDirs []string        `json:"orphaned_build_dirs"`
	Backlog           *backlogSummary `json:"backlog,omitempty"`
	Unavailable       string          `json:"unavailable,omitempty"`
}

type statusEnvelope struct {
	SchemaVersion int              `json:"schema_version"`
	Root          rootSection      `json:"root"`
	Children      []childRootEntry `json:"children"`
}

// backlogSummary is one root's backlog, counted rather than listed —
// status is an inventory, and the listing is `parlay backlog list`.
//
// Unreadable is a COUNT and not a boolean: a root where one item will
// not parse is in a different state from one where nine will not, and
// collapsing that to "some" is the kind of rounding that makes a status
// line stop being read.
type backlogSummary struct {
	Open       int `json:"open"`
	Untriaged  int `json:"untriaged"`
	Unreadable int `json:"unreadable,omitempty"`
}

// summariseBacklog counts one root's items.
//
// A read failure is reported as Unreadable rather than as zero. Zero and
// "could not look" are different facts, and a parent aggregating zeros
// it never actually read would assert the children are clear.
func summariseBacklog(rootPath string) backlogSummary {
	var sum backlogSummary
	items, broken, err := loadBacklogAt(rootPath)
	sum.Unreadable = len(broken)
	if err != nil {
		// The directory itself could not be read. One unreadable
		// record is the honest floor: it is at least one thing we
		// cannot account for, and it is not zero.
		sum.Unreadable++
		return sum
	}
	for _, it := range items {
		if it.State() != parser.StateOpen {
			continue
		}
		sum.Open++
		if it.Priority == "" {
			sum.Untriaged++
		}
	}
	return sum
}

// statusSchemaVersion is the integer schema_version pinned in the
// envelope. Bumped only on breaking changes; additive fields do not
// bump.
//
// v2 adds `planned` to the `phase` enum. A new VALUE in a documented
// enum is not the same as a new optional FIELD: a consumer switching
// exhaustively on phase breaks on a token it has never seen, while one
// ignoring an unknown key does not. `schema-versioning.schema.md` treats
// this exact class as a bump — it records .code-hashes.yaml as "currently
// at 2; v2 added the `hand-authored` provenance, changing the domain of
// an existing field".
//
// `PhaseHandAuthored` added a phase value without bumping this constant,
// which looks like precedent for leaving it alone. It is not: that change
// and the .code-hashes bump landed in the SAME commit (e159f47), one
// bumping and one not, for the same kind of change. An inconsistency
// inside a single commit is not a policy to repeat.
const statusSchemaVersion = 2

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
		return runStatusJSON(cmd, pctx)
	}
	return runStatusHuman(cmd, pctx)
}

// ---------------------------------------------------------------------
// Human path — hierarchical listing with the Phase column.
// ---------------------------------------------------------------------

func runStatusHuman(cmd *cobra.Command, pctx *config.Context) error {
	// Active root header — kept identical to the legacy output modulo
	// the Phase column added per feature line below.
	fmt.Fprintf(cmd.OutOrStdout(), "root:     %s\n", pctx.Root.Path)
	fmt.Fprintf(cmd.OutOrStdout(), "kind:     %s\n", pctx.Root.Kind)
	if pctx.Root.Kind == config.RootKindChild && pctx.Root.ParentPath != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "parent:   %s\n", pctx.Root.ParentPath)
	}
	if pctx.Resolution != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "source:   %s\n", pctx.Resolution.Source)
	}

	// Topology line — sits between the root header and the feature
	// listing. Per the read-only contract, this never enumerates
	// per-mismatch detail; per-file diagnostics are reserved for
	// `parlay repair`.
	if mismatches, err := config.ScanTopology(&pctx.Root); err == nil {
		renderTopologyLine(cmd.OutOrStdout(), len(mismatches))
	}

	// Tree-parity line — deliberately separate from `topology:`.
	// ScanTopology answers "is this root wired up correctly" by reading
	// config.yaml and roots.yaml only; it never looks at the three
	// parallel trees, so a project can print `topology: ok` while
	// `parlay repair` reports missing handoff or build directories.
	// Folding the two into one word is what made that contradiction
	// invisible, so they get one line each.
	if mismatches, err := detectMismatches(pctx.IntentsRoot(), threeTreeRoots(pctx)); err == nil {
		renderTreeParityLine(cmd.OutOrStdout(), len(mismatches))
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
	renderFeaturesHuman(cmd, pctx, features)
	renderBacklogHuman(cmd, summariseBacklog(pctx.Root.Path))
	renderOrphanedBuildDirsHuman(cmd, scanOrphanedBuildDirs(pctx.IntentsRoot(), pctx.BuildRoot()))

	// Cross-root walk: when the active root is a parent, iterate
	// pctx.Index.Children IN SLICE ORDER (the on-disk roots.yaml
	// order — never alphabetized, never re-sorted). Aggregation is
	// downward-only and exactly one level deep — grandchildren are
	// not walked.
	if pctx.Root.Kind == config.RootKindParent && pctx.Index != nil && len(pctx.Index.Children) > 0 {
		walkChildRoots(pctx, func(name, path string, childCtx *config.Context, childFeatures []string, unavailable error) {
			renderChildHuman(cmd, name, path, childCtx, childFeatures, unavailable)
		})
	}
	return nil
}

// renderFeaturesHuman prints the active root's features as a tabwriter
// table with the Phase column appended. Empty features render as the
// `(none)` line.
func renderFeaturesHuman(cmd *cobra.Command, pctx *config.Context, features []string) {
	entries := featureEntriesFor(pctx, features)
	if len(entries) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "features: (none)\n")
		return
	}
	// The count stays the FEATURE count. Units are listed underneath but
	// not added in: "features: 12" meaning nine features and three units
	// would be wrong in the one number people read fastest.
	fmt.Fprintf(cmd.OutOrStdout(), "features: %d\n", len(features))
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	for _, e := range entries {
		fmt.Fprintf(w, "  - %s\t%s\t%s\n", e.ID, e.Phase, activityCell(e))
	}
	w.Flush()
}

// reportableBacklog is summariseBacklog for the JSON envelope: nil when
// there is nothing outstanding, so a healthy root emits no member at all
// and the field stays genuinely optional under schema_version 2.
func reportableBacklog(rootPath string) *backlogSummary {
	sum := summariseBacklog(rootPath)
	if sum.Open == 0 && sum.Untriaged == 0 && sum.Unreadable == 0 {
		return nil
	}
	return &sum
}

// renderBacklogHuman prints one root's backlog line.
//
// Silent when there is nothing outstanding, like the orphan line and for
// the same reason: a healthy project's status output stays unchanged,
// and a line that is always present stops being read.
//
// The UNREADABLE count is printed even when nothing is open, because
// "no open items" and "no open items that we could read" are different
// claims and only the first is good news.
func renderBacklogHuman(cmd *cobra.Command, b backlogSummary) {
	if b.Open == 0 && b.Unreadable == 0 {
		return
	}
	out := cmd.OutOrStdout()
	if b.Open > 0 {
		untriaged := ""
		if b.Untriaged > 0 {
			untriaged = fmt.Sprintf(", %d untriaged", b.Untriaged)
		}
		fmt.Fprintf(out, "backlog: %d open%s (`parlay backlog list`)\n", b.Open, untriaged)
	}
	if b.Unreadable > 0 {
		fmt.Fprintf(out, "backlog: %d record(s) could not be read — they are NOT in the counts above\n", b.Unreadable)
	}
}

// renderOrphanedBuildDirsHuman prints an anomaly line naming every
// qualified identifier under .parlay/build/ that carries a build
// artifact but has no matching spec/intents/ feature — its intents.md
// was deleted, renamed, or moved without cleaning up the stale build
// state. Unlike the features/topology lines, this renders nothing when
// there's nothing to report: it's an anomaly flag, not a status line
// that's always present, so a healthy project's output stays unchanged.
func renderOrphanedBuildDirsHuman(cmd *cobra.Command, orphans []string) {
	if len(orphans) == 0 {
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "orphaned build dirs: %d (build artifacts with no matching spec/intents/ feature — run `parlay repair`)\n", len(orphans))
	for _, o := range orphans {
		fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", o)
	}
}

// renderChildHuman prints one child section: header line, then either
// the child's features (with Phase column) or the inline diagnostic.
// Per-child error boundary: catching the error at the per-child level
// lets the walk continue with remaining children. Exit stays zero.
func renderChildHuman(cmd *cobra.Command, name, path string, childCtx *config.Context, childFeatures []string, unavailable error) {
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", name)
	fmt.Fprintf(cmd.OutOrStdout(), "path:     %s\n", path)
	if unavailable != nil {
		// Single human-readable diagnostic. No stack trace. No
		// partial feature listing.
		fmt.Fprintf(cmd.OutOrStdout(), "(unavailable: %s)\n", unavailable.Error())
		return
	}
	if len(childFeatures) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "features: (none)\n")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "features: %d\n", len(childFeatures))
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		for _, e := range featureEntriesFor(childCtx, childFeatures) {
			fmt.Fprintf(w, "  - %s\t%s\t%s\n", e.ID, e.Phase, activityCell(e))
		}
		w.Flush()
	}
	// ONE tail for both branches. These used to be duplicated inside the
	// no-features branch and absent from the other, so a child's backlog
	// printed twice in the topology nobody has and not at all in the
	// ordinary one.
	renderBacklogHuman(cmd, summariseBacklog(childCtx.Root.Path))
	renderOrphanedBuildDirsHuman(cmd, scanOrphanedBuildDirs(childCtx.IntentsRoot(), childCtx.BuildRoot()))
}

// ---------------------------------------------------------------------
// JSON path — single envelope on stdout, no log lines, no preamble.
// ---------------------------------------------------------------------

func runStatusJSON(cmd *cobra.Command, pctx *config.Context) error {
	env := buildStatusEnvelope(pctx)
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal status envelope: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
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
				Name:              name,
				Path:              path,
				Features:          []featureEntry{},
				OrphanedBuildDirs: []string{},
			}
			if unavailable != nil {
				entry.Unavailable = unavailable.Error()
			} else {
				entry.Backlog = reportableBacklog(childCtx.Root.Path)
				entry.Features = featureEntriesFor(childCtx, childFeatures)
				if orphans := scanOrphanedBuildDirs(childCtx.IntentsRoot(), childCtx.BuildRoot()); orphans != nil {
					entry.OrphanedBuildDirs = orphans
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
		Path:              pctx.Root.Path,
		Kind:              pctx.Root.Kind,
		Features:          []featureEntry{},
		OrphanedBuildDirs: []string{},
	}
	if pctx.Resolution != nil {
		r.Source = string(pctx.Resolution.Source)
	}
	r.Backlog = reportableBacklog(pctx.Root.Path)
	features, _ := scanFeaturesAtTolerant(pctx.IntentsRoot())
	r.Features = featureEntriesFor(pctx, features)
	if orphans := scanOrphanedBuildDirs(pctx.IntentsRoot(), pctx.BuildRoot()); orphans != nil {
		r.OrphanedBuildDirs = orphans
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
// featureEntriesFor builds one root's entry list: its features, then its
// hand-authored units.
//
// Units are listed rather than filtered out because status is the command
// people run to ask what is in a project, and a unit is a substantial part
// of one — block-printing's geometry engine is ~2,700 lines. Omitting them
// would make status agree with the pre-unit world by simply not mentioning
// the code that motivated units in the first place.
func featureEntriesFor(ctx *config.Context, features []string) []featureEntry {
	entries := []featureEntry{}
	for _, f := range features {
		phase := ComputeFeaturePhase(ctx, f)
		reading := readActivity(ctx.FeaturePath(f))
		observed := HasObservedPipelineActivity(phase)
		entries = append(entries, featureEntry{
			ID:             f,
			Phase:          phase,
			Activity:       reading.Resolve(observed),
			ActivityDetail: reading.Detail(),
			ActivityStale:  reading.ParkingIsStale(observed),
		})
	}
	units, _ := scanUnitsAtTolerant(ctx.IntentsRoot())
	for _, u := range units {
		entries = append(entries, featureEntry{
			ID:    u,
			Phase: PhaseHandAuthored,
			Kind:  KindHandAuthored,
		})
	}
	return entries
}

// scanUnitsAtTolerant is scanFeaturesAtTolerant's counterpart for units.
func scanUnitsAtTolerant(intentsRoot string) ([]string, error) {
	if _, err := os.Stat(intentsRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return config.ScanUnitTree(intentsRoot)
}

func scanFeaturesAtTolerant(intentsRoot string) ([]string, error) {
	if _, err := os.Stat(intentsRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return config.ScanFeatureTree(intentsRoot)
}

// scanOrphanedBuildDirs finds qualified identifiers under buildRoot
// (.parlay/build/) that carry a buildfile.yaml or testcases.yaml but
// have no corresponding spec/intents/ feature at all — the feature's
// intents.md is gone (deleted, renamed, moved) but its build artifacts
// were never cleaned up. This is a different, narrower anomaly than
// `parlay repair`'s stale-initiative-buildfile check (which flags a
// build artifact sitting under a directory that reclassified as an
// initiative, not one whose feature vanished outright). status only
// surfaces the anomaly; fixing it is repair's job, not status's — this
// function never touches disk.
//
// Walks exactly two levels (top-level and one nested level under an
// initiative) to match the qualified-identifier shape features use
// elsewhere (bare slug, or initiative/feature).
func scanOrphanedBuildDirs(intentsRoot, buildRoot string) []string {
	entries, err := os.ReadDir(buildRoot)
	if err != nil {
		return nil
	}

	hasBuildArtifact := func(dir string) bool {
		return fileExistsAt(filepath.Join(dir, "buildfile.yaml")) || fileExistsAt(filepath.Join(dir, "testcases.yaml"))
	}
	isOrphaned := func(qualifiedID string) bool {
		_, err := os.Stat(filepath.Join(intentsRoot, qualifiedID, "intents.md"))
		return os.IsNotExist(err)
	}

	var orphans []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		topDir := filepath.Join(buildRoot, e.Name())
		if hasBuildArtifact(topDir) && isOrphaned(e.Name()) {
			orphans = append(orphans, e.Name())
		}

		subEntries, subErr := os.ReadDir(topDir)
		if subErr != nil {
			continue
		}
		for _, sub := range subEntries {
			if !sub.IsDir() || strings.HasPrefix(sub.Name(), "_") {
				continue
			}
			qualifiedID := filepath.Join(e.Name(), sub.Name())
			if hasBuildArtifact(filepath.Join(topDir, sub.Name())) && isOrphaned(qualifiedID) {
				orphans = append(orphans, qualifiedID)
			}
		}
	}

	sort.Strings(orphans)
	return orphans
}

// renderTopologyLine writes one line summarizing the topology check
// result. The line is uniform across single-root and multi-root
// projects; a correctly-configured single-root project also reports
// `topology: ok`. The renderer adds no new failure modes — `parlay
// status` continues to exit zero whether the topology is clean or
// dirty.
func renderTopologyLine(out interface{ Write([]byte) (int, error) }, mismatchCount int) {
	if mismatchCount == 0 {
		fmt.Fprintln(out, "topology: ok")
		return
	}
	fmt.Fprintf(out, "topology: needs repair (%d mismatches — run `parlay repair`)\n", mismatchCount)
}

// renderTreeParityLine writes one line summarizing whether the three
// parallel trees — spec/intents/, spec/handoff/ and .parlay/build/ —
// carry the same feature directories. It reports the same count
// `parlay repair --dry-run` would, from the same detector, so the two
// commands can no longer disagree. Like the topology line it never
// enumerates per-mismatch detail and never changes the exit code.
func renderTreeParityLine(out interface{ Write([]byte) (int, error) }, mismatchCount int) {
	if mismatchCount == 0 {
		fmt.Fprintln(out, "trees:    ok")
		return
	}
	fmt.Fprintf(out, "trees:    needs repair (%d mismatches — run `parlay repair`)\n", mismatchCount)
}
