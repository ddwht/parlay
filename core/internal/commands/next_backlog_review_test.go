// parlay-feature: parlay-tool/backlog-and-activity

package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// The review must reach child roots, or a parent-run sitting reports
// "nothing left" while a child holds unreviewed observations. doctor
// invokes this from the parent, so that falsehood is the default case
// rather than an edge one.
func TestNextBacklogReview_ReachesChildRootsAndQualifiesTheExclude(t *testing.T) {
	cfg, _ := multiRootBacklogFixture(t)

	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	nextBacklogExclude = nil
	if err := runNextBacklogReview(cmd, nil); err != nil {
		t.Fatal(err)
	}
	var got backlogReviewOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output did not parse: %v", err)
	}

	if len(got.RootsExamined) < 2 {
		t.Fatalf("the review never looked at the child root: %v", got.RootsExamined)
	}
	if got.Summary.Total != 2 {
		t.Errorf("want both roots' items counted, got %d", got.Summary.Total)
	}
	if got.Subject == nil {
		t.Fatal("two open items and no subject")
	}
	// Root-qualified, because two roots may hold items and an
	// unqualified exclusion could skip the wrong one.
	if !strings.HasPrefix(got.Subject.Exclude, got.Subject.Root+":") {
		t.Errorf("exclude token is not root-qualified: %q in root %q", got.Subject.Exclude, got.Subject.Root)
	}

	// Excluding the first subject must hand back the OTHER root's item,
	// not repeat the same one and not stop early.
	first := got.Subject.Exclude
	firstID := got.Subject.ID
	out.Reset()
	nextBacklogExclude = []string{first}
	t.Cleanup(func() { nextBacklogExclude = nil })
	if err := runNextBacklogReview(testCommandWithContext(t, cfg), nil); err != nil {
		t.Fatal(err)
	}
	cmd2 := testCommandWithContext(t, cfg)
	cmd2.SetOut(&out)
	if err := runNextBacklogReview(cmd2, nil); err != nil {
		t.Fatal(err)
	}
	var second backlogReviewOutput
	if err := json.Unmarshal(out.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second.Subject == nil {
		t.Fatal("excluding one of two items ended the sitting early")
	}
	if second.Subject.ID == firstID {
		t.Error("the exclusion did not take: the same item came back")
	}

	// And every emitted command must carry --root for a child item, or
	// it runs against whichever project the shell resolves to.
	if second.Subject.Root != activeRootLabel(cfg) {
		for _, o := range second.Subject.Options {
			if !strings.Contains(o.Command, "--root") {
				t.Errorf("a child item's %q command carries no --root: %q", o.ID, o.Command)
			}
		}
	}
}

// The CHILD's emitted commands are the ones whose correctness matters
// most, and until now no test ran one: the argv builder did not exist,
// and splitting the pasteable string rejected `parlay --root 'child'
// backlog ...` outright.
func TestNextBacklogReview_ChildOptionsAreExecutableAgainstTheChild(t *testing.T) {
	cfg, parent := multiRootBacklogFixture(t)
	childCtx := rootCtxAt(filepath.Join(parent, "child"))

	// Drain the active root so the subject is the child's item.
	inv := collectBacklogInventory(cfg, backlogListFilters{openOnly: true})
	var excludes []string
	for _, it := range inv.Items {
		if it.Root != "child" {
			excludes = append(excludes, backlogExcludeToken(it.Root, it.ID))
		}
	}
	nextBacklogExclude = excludes
	t.Cleanup(func() { nextBacklogExclude = nil })

	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runNextBacklogReview(cmd, nil); err != nil {
		t.Fatal(err)
	}
	var got backlogReviewOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Subject == nil || got.Subject.Root != "child" {
		t.Fatalf("want the child's item as subject, got %+v", got.Subject)
	}

	for _, o := range got.Subject.Options {
		if len(o.Argv) < 3 || o.Argv[1] != "--root" || o.Argv[2] != "child" {
			t.Errorf("option %q does not target the child root: %v", o.ID, o.Argv)
		}
	}

	// rank and one disposition, run as emitted, against the child.
	byID := map[string]activityReviewOption{}
	for _, o := range got.Subject.Options {
		byID[o.ID] = o
	}
	subject := got.Subject.ID
	if _, err := backlogCLI(t, childCtx, reviewArgv(t, byID["rank"], subject, "")...); err != nil {
		t.Fatalf("the child's rank command is not runnable: %v", err)
	}
	if got := itemByID(t, childCtx, subject).Priority; got == "" {
		t.Error("the child's rank command set no priority")
	}
	if _, err := backlogCLI(t, childCtx, reviewArgv(t, byID["decline"], subject, "")...); err != nil {
		t.Fatalf("the child's decline command is not runnable: %v", err)
	}
	if st := itemByID(t, childCtx, subject).State(); st != parser.BacklogState(parser.EventDeclined) {
		t.Errorf("the child's decline left the item %q", st)
	}

	// And ONLY the child item changed — a command carrying --root that
	// silently hit the active root would be worse than one that failed.
	for _, it := range inv.Items {
		if it.Root == "child" {
			continue
		}
		if st := itemByID(t, rootCtxAt(parent), it.ID).State(); st != parser.StateOpen {
			t.Errorf("the child-targeted command reached the parent's item %s (state %q)", it.ID, st)
		}
	}
}

// A root that could not be enumerated OUTRANKS the clean note. Populating
// root_errors and then saying "Nothing left to review" is the exact
// falsehood the field exists to prevent, and doctor invokes this from
// the parent.
func TestNextBacklogReview_RootErrorCannotYieldNothingLeft(t *testing.T) {
	parent := t.TempDir()
	parent, _ = filepath.EvalSymlinks(parent)
	if err := os.MkdirAll(filepath.Join(parent, config.SpecDir, config.IntentsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	// Registered, path absent, and NO items anywhere — so the only thing
	// standing between this and a false "nothing left" is the guard.
	cfg := parentWithChild(t, parent, "ghost", filepath.Join(parent, "no-such-child"))

	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	nextBacklogExclude = nil
	if err := runNextBacklogReview(cmd, nil); err != nil {
		t.Fatal(err)
	}
	var got backlogReviewOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.RootErrors) == 0 {
		t.Fatal("the unreachable child was not reported at all")
	}
	if strings.Contains(got.Note, "Nothing left to review") {
		t.Errorf("a root that could not be read was reported as nothing left: %q", got.Note)
	}
	if !strings.Contains(got.Note, "root_errors") {
		t.Errorf("the note does not send the reader to root_errors: %q", got.Note)
	}
}
