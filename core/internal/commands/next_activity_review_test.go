// parlay-feature: parlay-tool/backlog-and-activity
// parlay-artifact: test

package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

func review(t *testing.T, cfg *config.Context, exclude ...string) activityReviewOutput {
	t.Helper()
	prev := nextActivityExclude
	nextActivityExclude = exclude
	t.Cleanup(func() { nextActivityExclude = prev })

	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runNextActivityReview(cmd, nil); err != nil {
		t.Fatalf("review failed: %v", err)
	}
	var got activityReviewOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output did not parse: %v\n%s", err, out.String())
	}
	return got
}

func addFeature(t *testing.T, cfg *config.Context, slug string) string {
	t.Helper()
	if err := runAddFeature(testCommandWithContext(t, cfg), []string{slug}); err != nil {
		t.Fatal(err)
	}
	return cfg.FeaturePath(slug)
}

// EVERY option must be a command the tool actually accepts.
//
// An option the caller runs and gets refused costs them the attempt
// before they learn it was never available. `unpark` in particular must
// not be offered on an undeclared feature: there is no parking to clear
// and the command refuses when the feature is not parked.
func TestNextActivityReview_OptionsAreExecutable(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	got := review(t, cfg)
	if got.Subject == nil {
		t.Fatal("expected a subject")
	}

	for _, o := range got.Subject.Options {
		if o.ID == "unpark" {
			t.Errorf("unpark offered on an undeclared feature; the command refuses that")
		}
	}
	// And the offered park actually works.
	var parkCmdText string
	for _, o := range got.Subject.Options {
		if o.ID == "park" {
			parkCmdText = o.Command
		}
	}
	if parkCmdText == "" {
		t.Fatal("an undeclared feature must be parkable")
	}
	if !strings.Contains(parkCmdText, "--reason") || !strings.Contains(parkCmdText, "--by") {
		t.Errorf("the command must carry the required flags: %q", parkCmdText)
	}
	if err := park(t, cfg, "widget", "superseded", "", "dwht"); err != nil {
		t.Fatalf("the offered option must be accepted: %v", err)
	}
}

// A stale parking offers only unpark: park would be refused as a no-op
// transition on an already-parked feature.
func TestNextActivityReview_StaleParkingOffersOnlyUnpark(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	if err := park(t, cfg, "widget", "superseded", "", "dwht"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featurePath, "capabilities.yaml"), []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := review(t, cfg)
	if got.Subject == nil || got.Subject.Kind != reviewKindStale {
		t.Fatalf("want a stale-parking subject, got %+v", got.Subject)
	}
	if len(got.Subject.Options) != 1 || got.Subject.Options[0].ID != "unpark" {
		t.Fatalf("want unpark only, got %+v", got.Subject.Options)
	}
	// Park really would be refused here, which is why it is not offered.
	if err := park(t, cfg, "widget", "again", "", "dwht"); err == nil {
		t.Error("park should be refused on an already-parked feature")
	}
}

// An unreadable declaration offers neither park nor unpark, because both
// refuse to append to a history they cannot read.
func TestNextActivityReview_UnavailableOffersRepairOnly(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	if err := os.WriteFile(parser.ActivityPath(featurePath), []byte("schema_version: 1\nhistory: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := review(t, cfg)
	if got.Subject == nil || got.Subject.Kind != reviewKindUnavailable {
		t.Fatalf("want an unavailable subject, got %+v", got.Subject)
	}
	for _, o := range got.Subject.Options {
		if o.ID == "park" || o.ID == "unpark" {
			t.Errorf("%s is refused on an unreadable declaration and must not be offered", o.ID)
		}
	}
	// The findings and their fixes travel with the subject, so the
	// reviewer is told how to resolve the fault rather than only that one
	// exists.
	if len(got.Subject.Findings) == 0 {
		t.Fatal("an unavailable subject must carry its findings")
	}
	for _, f := range got.Subject.Findings {
		if strings.TrimSpace(f.Fix) == "" {
			t.Errorf("finding %q reached the reviewer with no fix", f.Code)
		}
	}
}

// Broken declarations first, then stale parkings, then the undeclared —
// smallest and most urgent pile first.
func TestNextActivityReview_OrdersByUrgency(t *testing.T) {
	cfg, _ := parkFixture(t, "undeclared-one")
	brokenPath := addFeature(t, cfg, "broken-decl")
	if err := os.WriteFile(parser.ActivityPath(brokenPath), []byte("schema_version: 1\nhistroy: [x]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := review(t, cfg)
	if got.Subject == nil || got.Subject.Feature != "broken-decl" {
		t.Fatalf("a broken declaration must come first, got %+v", got.Subject)
	}
}

// Nothing is written by the review itself. The decision is made by park
// and unpark, which carry the attribution.
func TestNextActivityReview_WritesNothing(t *testing.T) {
	cfg, featurePath := parkFixture(t, "widget")
	before, _ := os.ReadDir(featurePath)
	_ = review(t, cfg)
	after, _ := os.ReadDir(featurePath)
	if len(before) != len(after) {
		t.Errorf("the review wrote something: %d files before, %d after", len(before), len(after))
	}
	if _, err := os.Stat(parser.ActivityPath(featurePath)); !os.IsNotExist(err) {
		t.Error("the review created a declaration")
	}
}

// --exclude moves the sitting along, and the summary still counts what
// was excluded so the reviewer sees the real remaining total.
func TestNextActivityReview_ExcludeAdvancesWithoutHidingTheCount(t *testing.T) {
	cfg, _ := parkFixture(t, "alpha")
	addFeature(t, cfg, "beta")

	first := review(t, cfg)
	if first.Subject == nil {
		t.Fatal("expected a subject")
	}
	second := review(t, cfg, first.Subject.Feature)
	if second.Subject == nil {
		t.Fatal("expected a second subject")
	}
	if second.Subject.Feature == first.Subject.Feature {
		t.Error("--exclude did not advance the sitting")
	}
	if second.Summary.Remaining != first.Summary.Remaining {
		t.Errorf("excluding must not shrink the remaining count: %d then %d",
			first.Summary.Remaining, second.Summary.Remaining)
	}
}

// When everything is decided there is no subject, which is how a caller
// knows the sitting is over.
func TestNextActivityReview_NoSubjectWhenEverythingIsDecided(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	if err := park(t, cfg, "widget", "superseded", "", "dwht"); err != nil {
		t.Fatal(err)
	}

	got := review(t, cfg)
	if got.Subject != nil {
		t.Fatalf("nothing should be left to review, got %+v", got.Subject)
	}
	if got.Summary.Remaining != 0 {
		t.Errorf("remaining should be 0, got %d", got.Summary.Remaining)
	}
	if !strings.Contains(got.Note, "Nothing left") {
		t.Errorf("the note should say the sitting is over: %q", got.Note)
	}
}

// A bare parent — every feature in child roots — is a supported topology
// that status renders without complaint. The review must not refuse to
// run in a project whose only fault is that its work lives one level down.
func TestNextActivityReview_ToleratesABareParent(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	cfg := rootCtxAt(tmp)

	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runNextActivityReview(cmd, nil); err != nil {
		t.Fatalf("a bare parent must not error: %v", err)
	}
	var got activityReviewOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output did not parse: %v", err)
	}
	if got.Summary.Total != 0 || got.Subject != nil {
		t.Errorf("want an empty review, got %+v", got)
	}
}

// ---------------------------------------------------------------------
// Multi-root aggregation.
//
// A parent review reporting "every feature has a disposition" while a
// child holds unclassified work is false — and doctor invokes this from
// the parent, so refusing with routing instructions would only move the
// problem.
// ---------------------------------------------------------------------

// A REAL parent with a child holding pending activity. The earlier test
// covered only an empty directory, which proved that an empty tree does
// not crash and nothing more.
func TestNextActivityReview_AggregatesChildRoots(t *testing.T) {
	parent := t.TempDir()
	parent, _ = filepath.EvalSymlinks(parent)
	if err := os.MkdirAll(filepath.Join(parent, ".parlay"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A bare parent: no features of its own.
	if err := os.MkdirAll(filepath.Join(parent, config.SpecDir, config.IntentsDir), 0o755); err != nil {
		t.Fatal(err)
	}

	child := filepath.Join(parent, "core")
	childCtx := rootCtxAt(child)
	if err := os.MkdirAll(filepath.Join(child, config.SpecDir, config.IntentsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runAddFeature(testCommandWithContext(t, childCtx), []string{"child", "feature"}); err != nil {
		t.Fatal(err)
	}

	cfg := parentWithChild(t, parent, "core", child)
	got := review(t, cfg)

	if got.Summary.Total != 1 {
		t.Fatalf("the parent must count the child's features: %+v", got.Summary)
	}
	if got.Summary.Remaining != 1 {
		t.Fatalf("the child's undeclared feature must be pending: %+v", got.Summary)
	}
	if got.Subject == nil {
		t.Fatal("a parent review must hand back the child's feature, not an empty result")
	}
	if got.Subject.Root != "core" {
		t.Errorf("the subject must name its root, got %q", got.Subject.Root)
	}
	// The emitted commands must target that root, or they run against the
	// parent and report success having changed nothing.
	for _, o := range got.Subject.Options {
		if o.Command != "" && !strings.Contains(o.Command, "--root core") {
			t.Errorf("option %q omits the root: %q", o.ID, o.Command)
		}
	}
	// And the exclude token is root-qualified, emitted rather than left
	// for the caller to reconstruct.
	if got.Subject.Exclude != "core:child-feature" {
		t.Errorf("want a root-qualified exclude token, got %q", got.Subject.Exclude)
	}
	if len(got.RootsExamined) != 2 {
		t.Errorf("both roots should be reported as examined: %v", got.RootsExamined)
	}
}

// Two roots holding the same slug must not collide: excluding one must
// not silently skip the other.
func TestNextActivityReview_SameSlugInTwoRootsDoesNotCollide(t *testing.T) {
	parent := t.TempDir()
	parent, _ = filepath.EvalSymlinks(parent)
	if err := os.MkdirAll(filepath.Join(parent, ".parlay"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(parent, config.SpecDir, config.IntentsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runAddFeature(testCommandWithContext(t, rootCtxAt(parent)), []string{"widget"}); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(parent, "core")
	if err := os.MkdirAll(filepath.Join(child, config.SpecDir, config.IntentsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runAddFeature(testCommandWithContext(t, rootCtxAt(child)), []string{"widget"}); err != nil {
		t.Fatal(err)
	}

	cfg := parentWithChild(t, parent, "core", child)
	first := review(t, cfg)
	if first.Summary.Remaining != 2 {
		t.Fatalf("both widgets are pending: %+v", first.Summary)
	}

	second := review(t, cfg, first.Subject.Exclude)
	if second.Subject == nil {
		t.Fatal("excluding one widget must leave the other")
	}
	if second.Subject.Exclude == first.Subject.Exclude {
		t.Errorf("the same subject came back: %q", second.Subject.Exclude)
	}
	if second.Summary.Remaining != 2 {
		t.Errorf("excluding must not shrink the real count: %d", second.Summary.Remaining)
	}
}

// parentWithChild builds a parent context with one registered child,
// matching the shape status_test uses.
func parentWithChild(t *testing.T, parent, childName, childPath string) *config.Context {
	t.Helper()
	idx := &config.RootsIndex{
		ParentPath: parent,
		Children:   []config.Root{{Name: childName, Path: childPath, Kind: config.RootKindChild, ParentPath: parent}},
	}
	return config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Path: parent, Kind: config.RootKindParent},
	}, idx)
}

// A registered child whose path is unreadable must not vanish. An earlier
// cut returned early from the walk, so the child was absent from
// RootsExamined, absent from RootErrors, and absent from a summary that
// then claimed everything had a disposition.
func TestNextActivityReview_UnreadableChildIsReportedNotDropped(t *testing.T) {
	parent := t.TempDir()
	parent, _ = filepath.EvalSymlinks(parent)
	if err := os.MkdirAll(filepath.Join(parent, config.SpecDir, config.IntentsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	// Registered, but the path does not exist.
	cfg := parentWithChild(t, parent, "ghost", filepath.Join(parent, "no-such-child"))

	got := review(t, cfg)
	if len(got.RootErrors) == 0 {
		t.Fatalf("an unreadable child must be reported: %+v", got)
	}
	var named bool
	for _, name := range got.RootsExamined {
		if name == "ghost" {
			named = true
		}
	}
	if !named {
		t.Errorf("the failed child must still be named as examined: %v", got.RootsExamined)
	}
	if strings.Contains(got.Note, "Every feature has an activity disposition") {
		t.Errorf("the note must not claim completeness while a root could not be read: %q", got.Note)
	}
}

// PARITY across the shared option type.
//
// activityReviewOption is produced by two commands. When backlog's
// options started carrying Argv and activity's did not, a consumer
// taught to prefer the structured form got something runnable from one
// producer and an empty slice from the other — with nothing reporting
// the gap. A field on a shared type is a promise every producer keeps.
func TestReviewOptions_CommandAndArgvAgreeAcrossBothProducers(t *testing.T) {
	check := func(t *testing.T, where string, opts []activityReviewOption) {
		t.Helper()
		for _, o := range opts {
			switch {
			case o.Command != "" && len(o.Argv) == 0:
				t.Errorf("%s option %q carries a command but no argv: %q", where, o.ID, o.Command)
			case o.Command == "" && len(o.Argv) > 0:
				t.Errorf("%s option %q carries argv but no command: %v", where, o.ID, o.Argv)
			case o.Command == "" && o.Path == "":
				t.Errorf("%s option %q offers neither a command nor a path", where, o.ID)
			}
			if len(o.Argv) == 0 {
				continue
			}
			if o.Argv[0] != "parlay" {
				t.Errorf("%s option %q argv does not start with the binary: %v", where, o.ID, o.Argv)
			}
			// The two must describe the SAME invocation: every argv
			// token has to appear in the command string, or one of them
			// is targeting a different root or verb than the other.
			for _, tok := range o.Argv[1:] {
				if !strings.Contains(o.Command, tok) {
					t.Errorf("%s option %q argv token %q is absent from the command %q", where, o.ID, tok, o.Command)
				}
			}
		}
	}

	// Activity, in every state that produces options, both roots.
	for _, rootName := range []string{"", "child"} {
		for _, state := range []string{ActivityUnavailable, string(parser.ActivityParked), "active", ""} {
			for _, stale := range []bool{false, true} {
				opts := activityReviewOptions("widget", rootName, "spec/intents/widget/activity.yaml", state, stale)
				check(t, fmt.Sprintf("activity(state=%q,stale=%v,root=%q)", state, stale, rootName), opts)
			}
		}
	}

	// Backlog, ranked and untriaged, both roots — untriaged adds `rank`.
	for _, rootName := range []string{"", "child"} {
		for _, priority := range []string{"", "P1"} {
			c := &backlogReviewCandidate{
				item:     &parser.BacklogItem{ID: "20260901T000000.000000Z-abcd-x", Priority: priority},
				root:     "child",
				rootName: rootName,
			}
			check(t, fmt.Sprintf("backlog(priority=%q,root=%q)", priority, rootName), backlogReviewOptions(c))
		}
	}
}

// The pasteable form must keep the quoting HINT on a free-text
// placeholder and must not be mangled by shell-quoting it.
//
// Routing placeholders through shellQuote produced `--reason '<why>'`:
// correct shell, wrong hint, and a silent change to text that was
// already deployed.
func TestReviewOptions_PlaceholdersKeepTheirQuotingHint(t *testing.T) {
	opts := activityReviewOptions("widget", "", "spec/intents/widget/activity.yaml", string(parser.ActivityParked), false)
	c := &backlogReviewCandidate{item: &parser.BacklogItem{ID: "20260901T000000.000000Z-abcd-x"}}
	opts = append(opts, backlogReviewOptions(c)...)

	for _, o := range opts {
		if o.Command == "" {
			continue
		}
		if strings.Contains(o.Command, "'<") {
			t.Errorf("option %q shell-quoted a placeholder, losing the hint: %q", o.ID, o.Command)
		}
		if strings.Contains(o.Command, "<why>") && !strings.Contains(o.Command, `"<why>"`) {
			t.Errorf("option %q left the free-text placeholder unquoted: %q", o.ID, o.Command)
		}
		// And the argv must carry the BARE placeholder, since a caller
		// substituting into it is passing one argument, not shell text.
		for _, a := range o.Argv {
			if strings.Contains(a, `"<`) || strings.Contains(a, "'<") {
				t.Errorf("option %q argv carries a quoted placeholder %q; argv is not shell text", o.ID, a)
			}
		}
	}
}

// The parity test above is LEXICAL — strings.Contains cannot tell
// `--root child` from `--root child2`, nor catch reordered arguments.
// So the activity argv is also EXECUTED, structurally, against a real
// two-root project: the intended root must change and the other must
// not. Backlog already had this; activity did not.
func TestActivityReviewOptions_ArgvExecutesAgainstTheIntendedRootOnly(t *testing.T) {
	// A FRESH project per subtest. Sharing one meant the active-root
	// case parked the parent, and the child case then saw an
	// already-parked "other" root and reported a cross-root hit that
	// had not happened.
	project := func(t *testing.T) (string, string) {
		t.Helper()
		parent := t.TempDir()
		parent, _ = filepath.EvalSymlinks(parent)
		childPath := filepath.Join(parent, "child")
		for _, root := range []string{parent, childPath} {
			if err := os.MkdirAll(filepath.Join(root, config.SpecDir, config.IntentsDir), 0o755); err != nil {
				t.Fatal(err)
			}
			// THE SAME SLUG in both — the case a wrong root silently hits.
			if err := runAddFeature(testCommandWithContext(t, rootCtxAt(root)), []string{"widget"}); err != nil {
				t.Fatal(err)
			}
		}
		return parent, childPath
	}

	for _, tc := range []struct {
		name     string
		rootName string
		child    bool
	}{
		{"active root", "", false},
		{"child root", "child", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent, childPath := project(t)
			target, other := parent, childPath
			if tc.child {
				target, other = childPath, parent
			}

			opts := activityReviewOptions("widget", tc.rootName, "", "", false)
			var park *activityReviewOption
			for i := range opts {
				if opts[i].ID == "park" {
					park = &opts[i]
				}
			}
			if park == nil {
				t.Fatal("no park option to execute")
			}

			// Execute the ARGV structurally — no shell text is split.
			argv := park.Argv
			if argv[0] != "parlay" {
				t.Fatalf("argv does not start with the binary: %v", argv)
			}
			argv = argv[1:]
			if tc.rootName != "" {
				if len(argv) < 2 || argv[0] != "--root" || argv[1] != tc.rootName {
					t.Fatalf("argv does not target %q: %v", tc.rootName, park.Argv)
				}
				argv = argv[2:]
			} else if len(argv) > 0 && argv[0] == "--root" {
				t.Fatalf("an active-root option carries --root: %v", park.Argv)
			}
			if argv[0] != "park" {
				t.Fatalf("argv verb is %q, want park: %v", argv[0], park.Argv)
			}
			ref := argv[1]

			parkReason, parkUntil, parkBy = "waiting on the upstream decision", "", "dwht"
			t.Cleanup(func() { parkReason, parkUntil, parkBy = "", "", "" })
			if err := runPark(testCommandWithContext(t, rootCtxAt(target)), []string{ref}); err != nil {
				t.Fatalf("the emitted park argv is not runnable: %v", err)
			}

			if got := readActivity(rootCtxAt(target).FeaturePath("widget")).Resolve(false); got != string(parser.ActivityParked) {
				t.Errorf("the intended root's widget is %q, want parked", got)
			}
			if got := readActivity(rootCtxAt(other).FeaturePath("widget")).Resolve(false); got == string(parser.ActivityParked) {
				t.Error("the command reached the OTHER root's widget of the same name")
			}
		})
	}
}
