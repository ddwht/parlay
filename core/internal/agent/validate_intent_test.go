// parlay-feature: parlay-tool
// parlay-component: IntentValidationResult
// parlay-artifact: test

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func writeIntents(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "intents.md")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func codesOf(outcomes []ValidationOutcome) []string {
	var codes []string
	for _, o := range outcomes {
		codes = append(codes, o.Code)
	}
	return codes
}

func hasOutcomeCode(outcomes []ValidationOutcome, code string) bool {
	for _, o := range outcomes {
		if o.Code == code {
			return true
		}
	}
	return false
}

const validIntents = `# Reporting

> Everything about reports.

---

## Submit A Report

**Goal**: get a report in front of an approver
**Persona**: Employee
**Priority**: P0

## Withdraw A Report

**Goal**: take back a report before anyone acts on it
**Persona**: Employee
`

func TestValidIntentsFilePasses(t *testing.T) {
	outcomes := ValidateIntentsDeep(ModeBuild, writeIntents(t, validIntents), nil)
	if len(outcomes) != 0 {
		t.Errorf("a valid intents.md should produce no findings, got %v", codesOf(outcomes))
	}
}

func TestIntentWithoutGoalIsReported(t *testing.T) {
	const body = `## Submit A Report

**Persona**: Employee
`
	outcomes := ValidateIntentsDeep(ModeBuild, writeIntents(t, body), nil)
	if !hasOutcomeCode(outcomes, "missing-goal") {
		t.Errorf("want missing-goal, got %v", codesOf(outcomes))
	}
}

func TestIntentWithoutPersonaIsReported(t *testing.T) {
	const body = `## Submit A Report

**Goal**: get a report in front of an approver
`
	outcomes := ValidateIntentsDeep(ModeBuild, writeIntents(t, body), nil)
	if !hasOutcomeCode(outcomes, "missing-persona") {
		t.Errorf("want missing-persona, got %v", codesOf(outcomes))
	}
}

// The schema has said titles must be unique within a feature since it was
// written, and nothing enforced it. Two intents sharing a slug make
// `@feature/intent-slug` ambiguous, and the reference resolves to whichever
// one a given consumer happens to reach first.
func TestTwoIntentsSharingATitleAreReported(t *testing.T) {
	const body = `## Submit A Report

**Goal**: first
**Persona**: Employee

## Submit a report!

**Goal**: second
**Persona**: Employee
`
	outcomes := ValidateIntentsDeep(ModeBuild, writeIntents(t, body), nil)
	if !hasOutcomeCode(outcomes, "duplicate-intent-title") {
		t.Errorf("want duplicate-intent-title for two titles sharing a slug, got %v", codesOf(outcomes))
	}
}

func TestPriorityOutsideTheClosedSetIsReported(t *testing.T) {
	const body = `## Submit A Report

**Goal**: get a report in front of an approver
**Persona**: Employee
**Priority**: urgent
`
	outcomes := ValidateIntentsDeep(ModeBuild, writeIntents(t, body), nil)
	if !hasOutcomeCode(outcomes, "invalid-priority") {
		t.Errorf("want invalid-priority, got %v", codesOf(outcomes))
	}
}

// The schema says Priority defaults to P1 when omitted, so its absence must
// not be a finding — a rule that fires on a correct file trains people to
// ignore the validator.
func TestOmittedPriorityIsValid(t *testing.T) {
	const body = `## Submit A Report

**Goal**: get a report in front of an approver
**Persona**: Employee
`
	outcomes := ValidateIntentsDeep(ModeBuild, writeIntents(t, body), nil)
	if hasOutcomeCode(outcomes, "invalid-priority") {
		t.Errorf("an omitted Priority defaults to P1 and is valid, got %v", codesOf(outcomes))
	}
}

func TestUnreadableIntentsFileIsReported(t *testing.T) {
	outcomes := ValidateIntentsDeep(ModeBuild, filepath.Join(t.TempDir(), "nope.md"), nil)
	if !hasOutcomeCode(outcomes, "intents-not-readable") {
		t.Errorf("want intents-not-readable, got %v", codesOf(outcomes))
	}
}

// A scaffolded intents.md — feature header, no intent blocks — is a normal
// authoring state and a hard build failure. The mode split is the whole
// reason `parlay validate --type intent` is usable on a file someone is
// still writing.
func TestEmptyIntentsFileWarnsWhileAuthoringAndFailsAtBuild(t *testing.T) {
	path := writeIntents(t, "# Reporting\n\n> \n\n---\n\n")

	authoring := ValidateIntentsDeep(ModeAuthoring, path, nil)
	if len(authoring) != 1 || authoring[0].Code != "no-intents" {
		t.Fatalf("want a single no-intents finding, got %v", codesOf(authoring))
	}
	if authoring[0].Severity != SeverityWarning {
		t.Errorf("no-intents should warn while authoring, got %q", authoring[0].Severity)
	}

	build := ValidateIntentsDeep(ModeBuild, path, nil)
	if len(build) != 1 || build[0].Severity != SeverityError {
		t.Errorf("no-intents should block at build, got %+v", build)
	}
}

// Every finding has to say where it is and what to do about it. A code and a
// message alone leave the reader to find the block themselves.
func TestEveryIntentFindingCarriesContextAndFix(t *testing.T) {
	const body = `## Submit A Report

**Priority**: urgent
`
	for _, o := range ValidateIntentsDeep(ModeBuild, writeIntents(t, body), nil) {
		if o.Context == "" {
			t.Errorf("%s has no context", o.Code)
		}
		if o.Fix == "" {
			t.Errorf("%s has no fix", o.Code)
		}
	}
}
