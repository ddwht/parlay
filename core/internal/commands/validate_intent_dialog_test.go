// parlay-feature: parlay-tool
// parlay-component: validate
// parlay-artifact: test
//
// The CLI wiring for the two designer-authored artifacts. Their schemas
// shipped and deployed while `--type intent` and `--type dialog` were both
// rejected as unknown types, so the only hand-written files in the pipeline
// were the only ones nothing could check.

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestValidateType_Intent_AcceptsAValidFile(t *testing.T) {
	path := writeTemp(t, "intents.md", `# Reporting

## Submit A Report

**Goal**: get a report in front of an approver
**Persona**: Employee
**Priority**: P0
`)
	if _, _, err := runValidateCmdForTest(t, "intent", path, ""); err != nil {
		t.Errorf("a valid intents.md should validate clean, got: %v", err)
	}
}

func TestValidateType_Intent_FailsOnAMissingGoal(t *testing.T) {
	path := writeTemp(t, "intents.md", "## Submit A Report\n\n**Persona**: Employee\n")
	_, stderr, err := runValidateCmdForTest(t, "intent", path, "")
	if err == nil {
		t.Fatal("an intent with no Goal should fail validation")
	}
	if !strings.Contains(stderr, "missing-goal") {
		t.Errorf("want missing-goal reported on stderr, got: %q", stderr)
	}
}

func TestValidateType_Intent_FailsOnTwoIntentsSharingATitle(t *testing.T) {
	path := writeTemp(t, "intents.md", `## Submit A Report

**Goal**: first
**Persona**: Employee

## Submit A Report

**Goal**: second
**Persona**: Employee
`)
	_, stderr, err := runValidateCmdForTest(t, "intent", path, "")
	if err == nil {
		t.Fatal("two intents sharing a title should fail validation")
	}
	if !strings.Contains(stderr, "duplicate-intent-title") {
		t.Errorf("want duplicate-intent-title reported on stderr, got: %q", stderr)
	}
}

func TestValidateType_Dialog_RejectsAnUnknownTurnForm(t *testing.T) {
	path := writeTemp(t, "dialogs.md", "### D\n\nSystem (whenever): something\n")
	_, stderr, err := runValidateCmdForTest(t, "dialog", path, "")
	if err == nil {
		t.Fatal("a turn form outside the documented set should fail validation")
	}
	if !strings.Contains(stderr, "unknown-turn-form") {
		t.Errorf("want unknown-turn-form reported on stderr, got: %q", stderr)
	}
}

// An intents.md that has been scaffolded but not yet written is a normal
// authoring state, so the CLI reports it without failing — the finding
// still has to reach the person, which is why warnings are surfaced here
// rather than dropped.
func TestValidateType_Intent_ScaffoldedFileWarnsWithoutFailing(t *testing.T) {
	path := writeTemp(t, "intents.md", "# Reporting\n\n> \n\n---\n\n")
	_, stderr, err := runValidateCmdForTest(t, "intent", path, "")
	if err != nil {
		t.Errorf("a scaffolded intents.md should not fail while authoring, got: %v", err)
	}
	if !strings.Contains(stderr, "no-intents") {
		t.Errorf("want no-intents reported on stderr, got: %q", stderr)
	}
}

func TestValidateType_Dialog_AcceptsTheFourDocumentedForms(t *testing.T) {
	path := writeTemp(t, "dialogs.md", `### Submitting

User: /submit
System: Your report is on its way.
System (background): recording the submission
System (condition: over the limit): This needs a second approver.
`)
	_, stderr, err := runValidateCmdForTest(t, "dialog", path, "")
	if err != nil {
		t.Errorf("the four documented turn forms should validate clean, got: %v", err)
	}
	if strings.Contains(stderr, "unknown-turn-form") {
		t.Errorf("no finding expected, got: %q", stderr)
	}
}

// The flag help, the "--type is required" message and the "unknown type"
// message were three hardcoded lists that disagreed with each other and
// with the switch. A reader of either message would have concluded a
// working type did not exist.
func TestValidateTypeListsAgreeWithWhatIsAccepted(t *testing.T) {
	list := validateTypeList()
	for _, typ := range validateTypes {
		if !strings.Contains(list, typ) {
			t.Errorf("%q is an accepted type but is missing from the rendered list", typ)
		}
	}

	// Every listed type must be reachable: --type <t> on a nonexistent
	// path reports the file, never "unknown type".
	missing := filepath.Join(t.TempDir(), "nope")
	for _, typ := range validateTypes {
		_, _, err := runValidateCmdForTest(t, typ, missing, "")
		if err != nil && strings.Contains(err.Error(), "unknown type") {
			t.Errorf("--type %s is advertised but rejected as an unknown type", typ)
		}
	}
}
