// parlay-feature: parlay-tool
// parlay-component: DialogValidationResult
// parlay-artifact: test

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDialogs(t *testing.T, body string) (string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dialogs.md")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return path, []byte(body)
}

const allFourTurnForms = `# Reporting — Dialogs

---

### Submitting

**Trigger**: the employee opens the report

User: /submit
System: Your report is on its way.
System (background): recording the submission
System (condition: over the limit): This needs a second approver.
`

func TestAllFourTurnFormsValidate(t *testing.T) {
	path, content := writeDialogs(t, allFourTurnForms)
	if outcomes := ValidateDialogsDeep(ModeBuild, path, content); len(outcomes) != 0 {
		t.Errorf("the four documented turn forms should validate clean, got %v", codesOf(outcomes))
	}
}

// The parser recognises four forms and silently ignores every other line.
// A fifth modifier is therefore not an error anywhere — the turn is simply
// absent from the parsed dialog and from everything derived from it.
func TestUnknownTurnModifierIsReported(t *testing.T) {
	path, content := writeDialogs(t, "### D\n\nSystem (whenever): something\n")
	outcomes := ValidateDialogsDeep(ModeBuild, path, content)
	if !hasOutcomeCode(outcomes, "unknown-turn-form") {
		t.Errorf("want unknown-turn-form for an undocumented modifier, got %v", codesOf(outcomes))
	}
}

// A conditional turn whose `):` is broken by a space parses as nothing at
// all — the exact near-miss that is invisible without this check.
func TestMalformedConditionalTurnIsReported(t *testing.T) {
	path, content := writeDialogs(t, "### D\n\nSystem (condition: over the limit) : needs a second approver\n")
	outcomes := ValidateDialogsDeep(ModeBuild, path, content)
	if !hasOutcomeCode(outcomes, "unknown-turn-form") {
		t.Errorf("want unknown-turn-form for a malformed conditional, got %v", codesOf(outcomes))
	}
}

func TestMisspelledSpeakerIsReported(t *testing.T) {
	path, content := writeDialogs(t, "### D\n\nSystems: something\n")
	outcomes := ValidateDialogsDeep(ModeBuild, path, content)
	if !hasOutcomeCode(outcomes, "unknown-turn-form") {
		t.Errorf("want unknown-turn-form for a misspelled speaker, got %v", codesOf(outcomes))
	}
}

// The lint must not fire on prose. A check that misfires on a correct file
// is worse than no check — people stop reading its output.
func TestProseBeginningWithASpeakerWordIsNotFlagged(t *testing.T) {
	const body = `### D

System behaviour notes: the approver sees nothing until the report is submitted.
User research showed: nobody reads the banner.

User: /submit
System: Done.
`
	path, content := writeDialogs(t, body)
	if outcomes := ValidateDialogsDeep(ModeBuild, path, content); len(outcomes) != 0 {
		t.Errorf("prose is not a malformed turn, got %v", codesOf(outcomes))
	}
}

// Fenced blocks are examples — including this schema's own template — not
// transcript.
func TestTurnFormsInsideAFencedBlockAreNotFlagged(t *testing.T) {
	const body = "### D\n\n```\nSystem (whenever): illustrative only\n```\n\nUser: /submit\n"
	path, content := writeDialogs(t, body)
	if outcomes := ValidateDialogsDeep(ModeBuild, path, content); len(outcomes) != 0 {
		t.Errorf("a fenced example is not a turn, got %v", codesOf(outcomes))
	}
}

// `parlay add-feature` writes a dialogs.md with a header and nothing else.
// Dialogs have no required fields, so that file is valid, not empty-and-wrong.
func TestScaffoldedDialogsFileIsValid(t *testing.T) {
	path, content := writeDialogs(t, "# Reporting — Dialogs\n\n---\n\n")
	if outcomes := ValidateDialogsDeep(ModeBuild, path, content); len(outcomes) != 0 {
		t.Errorf("a scaffolded dialogs.md is a valid starting point, got %v", codesOf(outcomes))
	}
}

// A malformed turn blocks in both modes. The pattern only matches a
// finished speaker-and-colon line, so a half-typed turn never reaches the
// check — what it catches is a mistake at any stage of authoring, not an
// intermediate state to be tolerated.
func TestUnknownTurnFormBlocksInBothModes(t *testing.T) {
	path, content := writeDialogs(t, "### D\n\nSystem (whenever): something\n")

	for _, mode := range []ValidationMode{ModeAuthoring, ModeBuild} {
		outcomes := ValidateDialogsDeep(mode, path, content)
		if len(outcomes) != 1 || outcomes[0].Severity != SeverityError {
			t.Errorf("unknown-turn-form should be an error in %s mode, got %+v", mode, outcomes)
		}
	}
}

// A half-typed turn is not a finding — the speaker line has to be
// complete before the check has anything to say about it.
func TestAHalfTypedTurnIsNotFlagged(t *testing.T) {
	path, content := writeDialogs(t, "### D\n\nSystem (condition\n")
	if outcomes := ValidateDialogsDeep(ModeAuthoring, path, content); len(outcomes) != 0 {
		t.Errorf("a half-typed turn is a normal authoring state, got %v", codesOf(outcomes))
	}
}

func TestUnreadableDialogsFileIsReported(t *testing.T) {
	outcomes := ValidateDialogsDeep(ModeBuild, filepath.Join(t.TempDir(), "nope.md"), nil)
	if !hasOutcomeCode(outcomes, "dialogs-not-readable") {
		t.Errorf("want dialogs-not-readable, got %v", codesOf(outcomes))
	}
}

// The finding has to name the line. "Somewhere in this file there is a bad
// turn" is not actionable in a long transcript.
func TestUnknownTurnFormNamesTheLine(t *testing.T) {
	path, content := writeDialogs(t, "### D\n\nUser: /submit\nSystem (whenever): something\n")
	outcomes := ValidateDialogsDeep(ModeBuild, path, content)
	if len(outcomes) != 1 {
		t.Fatalf("want exactly one finding, got %v", codesOf(outcomes))
	}
	if want := path + ":4"; outcomes[0].Context != want {
		t.Errorf("context = %q, want %q", outcomes[0].Context, want)
	}
}
