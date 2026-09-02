// parlay-feature: parlay-tool/backlog-and-activity
// parlay-artifact: test

package commands

import (
	"bytes"
	"github.com/ddwht/parlay/core/internal/agent"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

func note(t *testing.T, cfg *config.Context, kind, title, by string, mut func()) (string, error) {
	t.Helper()
	// Reset at the START of every call, not only at test end. Cleanup
	// runs once per TEST, so a second capture in the same test inherited
	// the first's --priority — which silently made an "untriaged" fixture
	// ranked, and the ordering test that caught it was right to fail.
	noteBody, notePriority, noteFeature, notePhase = "", "", "", ""
	noteAbout, noteEvidence = nil, nil
	noteKind, noteTitle, noteBy = kind, title, by
	if mut != nil {
		mut()
	}
	t.Cleanup(func() {
		noteKind, noteTitle, noteBody, notePriority = "", "", "", ""
		noteFeature, notePhase, noteBy = "", "", ""
		noteAbout, noteEvidence = nil, nil
	})
	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := runNote(cmd, nil)
	return strings.TrimSpace(out.String()), err
}

func readOnlyItem(t *testing.T, cfg *config.Context) *parser.BacklogItem {
	t.Helper()
	dir := parser.BacklogRoot(cfg.Root.Path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("no backlog directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want exactly one item, got %d", len(entries))
	}
	item, err := parser.ParseBacklogFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func TestNote_WritesAValidItem(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id, err := note(t, cfg, "gap", "Rename does not check orphan collision", "claude", func() {
		noteBody = "The collision surfaces later as a lookup failure."
		noteFeature = "@widget"
		notePhase = "code"
		noteAbout = []string{"@widget/operation:rename"}
		noteEvidence = []string{"core/internal/commands/move_feature.go:118"}
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("note must print the id it created")
	}

	item := readOnlyItem(t, cfg)
	if item.Kind != parser.KindGap || item.Title == "" {
		t.Errorf("kind/title lost: %+v", item)
	}
	if item.Captured.By != "claude" || item.Captured.At == "" {
		t.Errorf("attribution lost: %+v", item.Captured)
	}
	if len(item.Evidence) != 1 || item.Evidence[0].Line != 118 {
		t.Errorf("evidence not parsed from path:line: %+v", item.Evidence)
	}
	// What was written must satisfy the published validator, not merely
	// round-trip through our own reader.
	content, _ := os.ReadFile(filepath.Join(parser.BacklogRoot(cfg.Root.Path), id+".yaml"))
	if out := agentValidateBacklog(t, id+".yaml", content); len(out) != 0 {
		t.Errorf("note wrote an item its own validator rejects: %+v", out)
	}
}

// Absent priority means UNTRIAGED, and note NEVER guesses one. A guessed
// priority is worse than an absent one: it looks like a judgment and is
// not.
func TestNote_DoesNotInventAPriority(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	if _, err := note(t, cfg, "defect", "Something is wrong", "claude", nil); err != nil {
		t.Fatal(err)
	}
	if p := readOnlyItem(t, cfg).Priority; p != "" {
		t.Errorf("priority must be absent unless given, got %q", p)
	}
}

// THE STRICT-WRITER CONTRACT.
//
// Capture is cheap for the CALLER, not sloppy in the WRITER. This is
// durable, user-facing, schema-versioned state, so malformed input is
// refused with a non-zero exit rather than written as a corrupt record.
// What the calling phase owes is to treat that failure as non-blocking.
func TestNote_RefusesMalformedInputAndWritesNothing(t *testing.T) {
	cases := []struct {
		name            string
		kind, title, by string
		mut             func()
	}{
		{name: "no kind", title: "t", by: "claude"},
		{name: "no title", kind: "gap", by: "claude"},
		{name: "no attribution", kind: "gap", title: "t"},
		{name: "unknown kind", kind: "friction", title: "t", by: "claude"},
		{name: "invalid priority", kind: "gap", title: "t", by: "claude", mut: func() { notePriority = "urgent" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _ := parkFixture(t, "widget")
			_, err := note(t, cfg, tc.kind, tc.title, tc.by, tc.mut)
			if err == nil {
				t.Fatal("malformed input must be refused")
			}
			if _, statErr := os.Stat(parser.BacklogRoot(cfg.Root.Path)); !os.IsNotExist(statErr) {
				t.Error("a refused note created a backlog directory")
			}
		})
	}
}

// The loop already mints and exports PARLAY_RUN_ID, so an item ties back
// to the run that produced it at no cost to the caller.
func TestNote_CapturesTheRunCorrelation(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	t.Setenv("PARLAY_RUN_ID", "20260901T104500Z-4412")
	if _, err := note(t, cfg, "debt", "A shortcut", "claude", nil); err != nil {
		t.Fatal(err)
	}
	if got := readOnlyItem(t, cfg).Captured.Run; got != "20260901T104500Z-4412" {
		t.Errorf("run correlation lost: %q", got)
	}
}

// agentValidateBacklog runs the published validator, so the test asserts
// against the same answer `parlay validate --type backlog` gives.
func agentValidateBacklog(t *testing.T, path string, content []byte) []string {
	t.Helper()
	var codes []string
	for _, o := range validateBacklogContent(path, content) {
		codes = append(codes, o.Code)
	}
	return codes
}

// ---------------------------------------------------------------------
// CLI boundary. The agent tests can all pass while `backlog` is missing
// from validateTypes or from runValidate's switch, because neither is
// reachable from the agent package.
// ---------------------------------------------------------------------

func TestValidateCLI_BacklogTypeIsRegistered(t *testing.T) {
	var found bool
	for _, ty := range validateTypes {
		if ty == "backlog" {
			found = true
		}
	}
	if !found {
		t.Fatalf("backlog missing from validateTypes: %v", validateTypes)
	}
}

func TestValidateCLI_BacklogCleanItemExitsClean(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id, err := note(t, cfg, "gap", "A real gap", "claude", nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parser.BacklogRoot(cfg.Root.Path), id+".yaml")

	prev := validateType
	validateType = "backlog"
	t.Cleanup(func() { validateType = prev })

	cmd := testCommandWithContext(t, cfg)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	if err := runValidate(cmd, []string{path}); err != nil {
		t.Fatalf("an item note just wrote must validate: %v (stderr: %s)", err, errBuf.String())
	}
}

func TestValidateCLI_BacklogPublishedCodeReachesTheUser(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	dir := parser.BacklogRoot(cfg.Root.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "broken.yaml")
	// Parses fine, fails shape: a promoted event naming nothing.
	if err := os.WriteFile(path, []byte(`schema_version: 1
id: x
kind: gap
title: t
captured:
  at: 2026-09-01T10:45:00Z
  by: c
history:
  - event: promoted
    at: 2026-09-02T00:00:00Z
    by: d
`), 0o644); err != nil {
		t.Fatal(err)
	}

	prev := validateType
	validateType = "backlog"
	t.Cleanup(func() { validateType = prev })

	cmd := testCommandWithContext(t, cfg)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	err := runValidate(cmd, []string{path})
	combined := out.String() + errBuf.String()
	if err != nil {
		combined += err.Error()
	}
	if !strings.Contains(combined, "backlog-disposition-incomplete") {
		t.Errorf("the published code must reach the user: %q", combined)
	}
}

// ---------------------------------------------------------------------
// The id contract, tested deterministically rather than by wall clock.
// ---------------------------------------------------------------------

// Lexically time-sortable, which is the guarantee the code actually
// makes. Driven with injected instants: five wall-clock calls could tie
// inside one microsecond and flake, and a flaky test for an ordering
// claim is worse than none.
func TestBacklogID_IsLexicallyTimeSortable(t *testing.T) {
	base := time.Date(2026, 9, 1, 10, 45, 0, 0, time.UTC)
	var ids []string
	for i := 0; i < 5; i++ {
		ids = append(ids, newBacklogIDAt(base.Add(time.Duration(i)*time.Millisecond), "an idea"))
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("distinct instants must sort: %q then %q", ids[i-1], ids[i])
		}
	}
}

// Equal instants are NOT ordered — the suffix decides, arbitrarily. This
// pins the boundary of the claim so a future comment cannot quietly widen
// it back to "capture order".
func TestBacklogID_EqualInstantsAreDistinctButUnordered(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 45, 0, 0, time.UTC)
	a := newBacklogIDAt(at, "an idea")
	b := newBacklogIDAt(at, "an idea")
	if a == b {
		t.Fatal("ids must be distinct even at the same instant")
	}
	if strings.SplitN(a, "-", 2)[0] != strings.SplitN(b, "-", 2)[0] {
		t.Error("the same instant must produce the same timestamp component")
	}
}

// The id's timestamp and captured.at describe the same instant. They were
// two separate time.Now() calls, so an item could disagree with itself.
func TestNote_IdTimestampMatchesCapturedAt(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	id, err := note(t, cfg, "idea", "An idea", "claude", nil)
	if err != nil {
		t.Fatal(err)
	}
	item := readOnlyItem(t, cfg)
	at, err := time.Parse(time.RFC3339Nano, item.Captured.At)
	if err != nil {
		t.Fatalf("captured.at is not RFC3339: %q", item.Captured.At)
	}
	if want := at.UTC().Format("20060102T150405.000000Z"); !strings.HasPrefix(id, want) {
		t.Errorf("id %q does not carry captured.at %q", id, want)
	}
}

// EVERY refusal carries a published code and a fix.
//
// The founding intent promises it, and nothing is written when a capture
// is refused — so there is no file for `parlay validate` to diagnose
// afterwards. The refusal is the only diagnosis the caller will ever
// get, and a bare sentence made the promise false at the one moment it
// matters: an agent mid-phase, told its capture failed and not told what
// would make it succeed.
func TestNote_EveryRefusalCarriesItsPublishedCodeAndFix(t *testing.T) {
	cfg, _ := parkFixture(t, "widget")
	for _, tc := range []struct{ name, kind, title, by, priority, about string }{
		{"no kind", "", "t", "dwht", "", ""},
		{"no title", "gap", "", "dwht", "", ""},
		{"no attribution", "gap", "t", "", "", ""},
		{"unknown kind", "friction", "t", "dwht", "", ""},
		{"unknown priority", "gap", "t", "dwht", "URGENT", ""},
		{"about is not a ref", "gap", "t", "dwht", "", "the widget thing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			noteKind, noteTitle, noteBy, notePriority = tc.kind, tc.title, tc.by, tc.priority
			noteAbout, noteBody, noteEvidence = nil, "", nil
			if tc.about != "" {
				noteAbout = []string{tc.about}
			}
			t.Cleanup(func() {
				noteKind, noteTitle, noteBy, notePriority = "", "", "", ""
				noteAbout = nil
			})
			cmd := testCommandWithContext(t, cfg)
			cmd.SetOut(&bytes.Buffer{})
			err := runNote(cmd, nil)
			if err == nil {
				t.Fatalf("%s was accepted", tc.name)
			}
			msg := err.Error()
			if !strings.HasPrefix(msg, "backlog-") {
				t.Errorf("the refusal carries no published code: %q", msg)
			}
			if !strings.Contains(msg, "fix:") {
				t.Errorf("the refusal tells the caller nothing that would make it succeed: %q", msg)
			}
			// And the code must be one the validator also publishes, so
			// a caller sees one vocabulary either way.
			code := strings.SplitN(msg, ":", 2)[0]
			var known bool
			for _, k := range parser.KnownBacklogProblemKinds() {
				if c, _ := agent.BacklogDiagnostic(k); c == code {
					known = true
				}
			}
			if !known {
				t.Errorf("code %q is not one the validator publishes", code)
			}
		})
	}
}
