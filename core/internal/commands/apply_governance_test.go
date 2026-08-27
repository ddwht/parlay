package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// The confirmation must name the promise that dies, not just the amendment
// file. A reviewer shown "001-retire-i2" is being asked about a filename; what
// they are actually deciding is which intent stops being in force.
//
// This is also the production witness for ProspectiveAuthority. Before it, that
// mode had no non-test caller: the resolver could have been deleted whole
// without a single failure. The assertion is on the real command's output, not
// on a re-derivation of what the command ought to say — a test that recomputes
// the answer proves the algebra and nothing about whether anyone runs it.
func TestApplyGovernance_ConfirmationNamesTheDyingIntent(t *testing.T) {
	dir := setupTestDir(t)
	feat := filepath.Join(dir, "spec", "intents", "demo")
	if err := os.MkdirAll(filepath.Join(feat, "amendments"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(feat, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("intents.md", strings.Join([]string{
		"# Demo",
		"",
		"## Keep working",
		"",
		"**Goal**: stays in force",
		"**Persona**: maintainer",
		"",
		"## Old promise",
		"",
		"**Goal**: stops being in force",
		"**Persona**: maintainer",
		"",
	}, "\n"))
	write(filepath.Join("amendments", "001-premise-shift.md"), strings.Join([]string{
		"---",
		"amendment: premise-shift",
		"date: 2026-08-27",
		"trigger: \"the premise stopped holding\"",
		"supersedes_intents:",
		"  - old-promise",
		"---",
		"",
		"## Change",
		"",
		"The old promise is withdrawn.",
		"",
		"## Why",
		"",
		"It rested on a premise that no longer holds.",
		"",
		"## Acceptance",
		"",
		"- Keep working continues to hold on its own.",
		"",
	}, "\n"))

	resetFlagsAfterTest(t, applyGovernanceCmd.Flags())
	applyGovernanceConfirmed = false

	cmd := testCommandWithContext(t, testContext(t))
	cmd.Args = cobra.ExactArgs(1)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := runApplyGovernance(cmd, []string{"demo"})
	if err == nil {
		t.Fatal("applying a supersession without --confirm must refuse")
	}
	if strings.Contains(err.Error(), "unresolved error") {
		ca := computeCheckAmendments(testContext(t), "demo")
		for _, iss := range ca.Issues {
			t.Logf("[%s] %s: %s", iss.Severity, iss.Code, iss.Message)
		}
		t.Fatalf("unreached")
	}
	// The amendment is deliberately named "premise-shift", sharing no substring
	// with the intent slug. An amendment named after the intent it retires
	// would make this assertion pass on the amendment list alone, witnessing
	// nothing.
	if strings.Contains("001-premise-shift", "old-promise") {
		t.Fatal("fixture is self-defeating: the amendment name contains the intent slug")
	}
	if !strings.Contains(err.Error(), "old-promise") {
		t.Fatalf("the refusal must name the promise that would stop being in force; got: %v", err)
	}
	if strings.Contains(err.Error(), "keep-working") {
		t.Fatalf("keep-working survives this apply and must not appear as dying; got: %v", err)
	}
}
