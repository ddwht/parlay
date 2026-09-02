// parlay-feature: parlay-tool/backlog-and-activity
// parlay-artifact: test

package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

func writeActivity(t *testing.T, dir, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parser.ActivityPath(dir), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const parkedDeclaration = `schema_version: 1
history:
  - event: parked
    reason: Superseded by the shipped implementation
    until: after adapter-set v2 lands
    at: 2026-04-18T09:12:00Z
    by: dwht
`

func TestReadActivity_Parked(t *testing.T) {
	dir := writeActivity(t, t.TempDir(), parkedDeclaration)
	r := readActivity(dir)
	if got := r.Resolve(false); got != string(parser.ActivityParked) {
		t.Errorf("want parked, got %q", got)
	}
	if !strings.Contains(r.Detail(), "Superseded") || !strings.Contains(r.Detail(), "until after adapter-set v2") {
		t.Errorf("detail should carry the reason and the until: %q", r.Detail())
	}
}

// The contextual rule: no declaration plus observed pipeline activity is
// active, not unclassified. Reporting a missing disposition for work
// whose activity is evident is a permanent non-problem.
func TestReadActivity_UndeclaredResolvesAgainstObservation(t *testing.T) {
	dir := t.TempDir()
	if got := readActivity(dir).Resolve(false); got != string(parser.ActivityUnclassified) {
		t.Errorf("no declaration, no boundary: want unclassified, got %q", got)
	}
	if got := readActivity(dir).Resolve(true); got != string(parser.ActivityActive) {
		t.Errorf("no declaration, boundary observed: want active, got %q", got)
	}
}

// THE INVARIANT. Parse-safe is not authoring-valid.
//
// This file parses cleanly — known version, no unknown fields, one
// document — and declares nothing. A caller that went straight from parse
// to Current would report a confident `unclassified` for it, which is
// exactly the ambiguity the artifact exists to remove, wearing the
// artifact's own filename. readActivity runs shape validation too, so the
// only reachable answer is `unavailable`.
func TestReadActivity_EmptyHistoryIsUnavailableNotUnclassified(t *testing.T) {
	dir := writeActivity(t, t.TempDir(), "schema_version: 1\nhistory: []\n")
	r := readActivity(dir)
	if got := r.Resolve(false); got != ActivityUnavailable {
		t.Fatalf("want %q, got %q", ActivityUnavailable, got)
	}
	if _, unusable := r.Unusable(); !unusable {
		t.Error("an empty declaration must be reported unusable")
	}
	if !strings.Contains(r.Detail(), "declares nothing") {
		t.Errorf("detail should say why: %q", r.Detail())
	}
}

// A parked event with no reason parses fine and is meaningless. Same
// invariant, second shape.
func TestReadActivity_ParkedWithoutReasonIsUnavailable(t *testing.T) {
	dir := writeActivity(t, t.TempDir(), `schema_version: 1
history:
  - event: parked
    at: 2026-04-18T09:12:00Z
    by: dwht
`)
	if got := readActivity(dir).Resolve(false); got != ActivityUnavailable {
		t.Errorf("want %q, got %q", ActivityUnavailable, got)
	}
}

// A broken declaration is never resolved against observation, and never
// reported as its last known state.
func TestReadActivity_UnparseableIsUnavailableEvenWithObservedActivity(t *testing.T) {
	dir := writeActivity(t, t.TempDir(), "schema_version: 1\nhistroy: [oops]\n")
	r := readActivity(dir)
	if got := r.Resolve(true); got != ActivityUnavailable {
		t.Errorf("observed activity must not paper over a broken file: got %q", got)
	}
	if got := r.Resolve(false); got != ActivityUnavailable {
		t.Errorf("want %q, got %q", ActivityUnavailable, got)
	}
}

// A newer schema_version is unavailable, not last-known and not
// unclassified.
func TestReadActivity_UnknownSchemaVersionIsUnavailable(t *testing.T) {
	dir := writeActivity(t, t.TempDir(), `schema_version: 3
history:
  - event: parked
    reason: whatever a v2 parking means
    at: 2026-04-18T09:12:00Z
    by: dwht
`)
	if got := readActivity(dir).Resolve(false); got != ActivityUnavailable {
		t.Errorf("want %q, got %q", ActivityUnavailable, got)
	}
}

// A declaration outranks observation: somebody said so, and the gate did
// not.
func TestReadActivity_DeclarationOutranksObservation(t *testing.T) {
	dir := writeActivity(t, t.TempDir(), parkedDeclaration)
	if got := readActivity(dir).Resolve(true); got != string(parser.ActivityParked) {
		t.Errorf("a parked feature that still passes a gate is parked, got %q", got)
	}
}

// The CLI validator and the commands path must give the same answer —
// two routes to one opinion, not a second opinion.
func TestValidateActivityContent_AgreesWithReadActivity(t *testing.T) {
	dir := writeActivity(t, t.TempDir(), "schema_version: 1\nhistory: []\n")
	content, err := os.ReadFile(filepath.Join(dir, parser.ActivityFile))
	if err != nil {
		t.Fatal(err)
	}
	outcomes := validateActivityContent(parser.ActivityPath(dir), content)
	if len(outcomes) == 0 {
		t.Fatal("the validator accepted a declaration readActivity rejects")
	}
}

// ---------------------------------------------------------------------
// End to end through the CLI.
//
// The agent tests can all pass while `activity` is missing from
// validateTypes or from runValidate's switch, because neither is reachable
// from the agent package. These two pin the registration itself.
// ---------------------------------------------------------------------

func TestValidateCLI_ActivityTypeIsRegistered(t *testing.T) {
	var found bool
	for _, ty := range validateTypes {
		if ty == "activity" {
			found = true
		}
	}
	if !found {
		t.Fatalf("activity missing from validateTypes: %v", validateTypes)
	}
}

func TestValidateCLI_ActivityCleanDeclarationExitsClean(t *testing.T) {
	dir := writeActivity(t, t.TempDir(), parkedDeclaration)
	prev := validateType
	validateType = "activity"
	t.Cleanup(func() { validateType = prev })

	cmd := testCommandWithContext(t, testContext(t))
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)

	if err := runValidate(cmd, []string{parser.ActivityPath(dir)}); err != nil {
		t.Fatalf("a clean declaration must validate: %v (stderr: %s)", err, errBuf.String())
	}
}

func TestValidateCLI_ActivityMissingReasonEmitsItsCode(t *testing.T) {
	dir := writeActivity(t, t.TempDir(), `schema_version: 1
history:
  - event: parked
    at: 2026-04-18T09:12:00Z
    by: dwht
`)
	prev := validateType
	validateType = "activity"
	t.Cleanup(func() { validateType = prev })

	cmd := testCommandWithContext(t, testContext(t))
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)

	err := runValidate(cmd, []string{parser.ActivityPath(dir)})
	combined := out.String() + errBuf.String()
	if err == nil && !strings.Contains(combined, "activity-parked-without-reason") {
		t.Fatalf("a parking with no reason must be reported; got err=%v output=%q", err, combined)
	}
	if err != nil && !strings.Contains(err.Error()+combined, "activity-parked-without-reason") {
		t.Errorf("the published code must reach the user: err=%v output=%q", err, combined)
	}
}
