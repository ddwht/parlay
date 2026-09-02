// parlay-feature: parlay-tool/ledger-and-contract
// parlay-artifact: test

package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/config"
)

// The selector is an AUTHORITY surface: it decides which record the
// applied marker may advance to. The helper tests prove matching; they do
// not prove runApplyAmendment enforces the rule after loading the real
// ledger and before any preflight or marker mutation. This fixture drives
// the command.

const amend001 = `---
amendment: first-record
date: 2026-09-01
affects:
  - "@ledger-demo/surface:some-fragment"
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions: []
amends_intents:
  - intent: a-promise
    mode: revise
    version:
      title: A Promise
      goal: Do the thing.
      persona: Designer
      priority: P0
      context: Context.
      action: Act.
      objects:
        - thing
      constraints:
        - "First version."
      verify:
        - "It does the thing."
---

## Change
The first change.

## Why
Because.

## Acceptance
- It does the thing.
`

const amend002 = `---
amendment: second-record
date: 2026-09-01
supersedes:
  - first-record
affects:
  - "@ledger-demo/surface:some-fragment"
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions: []
amends_intents:
  - intent: a-promise
    mode: revise
    version:
      title: A Promise
      goal: Do the thing, correctly.
      persona: Designer
      priority: P0
      context: Context.
      action: Act.
      objects:
        - thing
      constraints:
        - "Corrected version."
      verify:
        - "It does the thing correctly."
---

## Change
The correction.

## Why
The first record said the wrong thing.

## Acceptance
- It does the thing correctly.
`

// ledgerFixture builds a feature with two pending amendments and returns
// its context plus the baseline path, so a test can assert the applied
// marker was not touched.
func ledgerFixture(t *testing.T) (*config.Context, string) {
	t.Helper()
	setupTestDir(t)
	if err := os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := testContext(t)
	if err := runAddFeature(testCommandWithContext(t, cfg), []string{"ledger", "demo"}); err != nil {
		t.Fatal(err)
	}
	featDir := cfg.FeaturePath("ledger-demo")

	// A real founding promise and a real contract entry, so the ledger
	// resolves and the run reaches the PROOF gate rather than stopping at
	// validation. A fixture that fails earlier than the thing under test
	// proves nothing about it.
	if err := os.WriteFile(filepath.Join(featDir, "intents.md"), []byte(`# Ledger Demo

---

## A Promise

**Goal**: Do the thing.
**Persona**: Designer
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featDir, "surface.yaml"), []byte(`feature: ledger-demo
fragments:
    - actions: (none)
      name: Some fragment
      order: 10
      page: cli
      region: command
      shows: status
      source: '@ledger-demo/a-promise'
`), 0o644); err != nil {
		t.Fatal(err)
	}

	writeAmendment(t, featDir, "001-first-record.md", amend001)
	writeAmendment(t, featDir, "002-second-record.md", amend002)
	return cfg, baselinePath(cfg, "ledger-demo")
}

func runApply(t *testing.T, cfg *config.Context, selector string) (string, error) {
	t.Helper()
	prev := applyAmendmentSelect
	applyAmendmentSelect = selector
	t.Cleanup(func() { applyAmendmentSelect = prev })

	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := runApplyAmendment(cmd, []string{"@ledger-demo"})
	return out.String(), err
}

// authoritySnapshot reads the applied-marker file, or reports its absence.
// Byte-identity is the assertion that matters: a refusal must not have
// advanced anything.
func authoritySnapshot(t *testing.T, path string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b), true
}

// The default with two pending refuses, and names the way out. The old
// message said "resolve the earlier records first" with no mechanism
// behind it, which is the defect this selector exists to close.
func TestApplyAmendment_DefaultRefusesAndNamesTheEarliest(t *testing.T) {
	cfg, baseline := ledgerFixture(t)
	before, existedBefore := authoritySnapshot(t, baseline)

	out, err := runApply(t, cfg, "")
	if err == nil {
		t.Fatal("two pending amendments must refuse by default")
	}
	msg := err.Error() + out
	if !strings.Contains(msg, "--amendment 001-first-record") {
		t.Errorf("the refusal must name the way out: %s", msg)
	}

	after, existsAfter := authoritySnapshot(t, baseline)
	if existedBefore != existsAfter || before != after {
		t.Error("a refusal advanced the applied marker")
	}
}

// Selecting a LATER record refuses on ordering, and touches nothing. The
// marker passes every record below it, so applying out of order would
// record the skipped one applied without anybody having applied it.
func TestApplyAmendment_SelectingALaterRecordRefusesAndChangesNothing(t *testing.T) {
	cfg, baseline := ledgerFixture(t)
	before, existedBefore := authoritySnapshot(t, baseline)

	out, err := runApply(t, cfg, "002")
	if err == nil {
		t.Fatal("selecting a later record must refuse")
	}
	msg := err.Error() + out
	if !strings.Contains(msg, "not the earliest") {
		t.Errorf("the refusal must say why: %s", msg)
	}
	if !strings.Contains(msg, "001-first-record") {
		t.Errorf("the refusal must name the record to apply instead: %s", msg)
	}

	after, existsAfter := authoritySnapshot(t, baseline)
	if existedBefore != existsAfter || before != after {
		t.Error("an out-of-order refusal advanced the applied marker")
	}
}

// Selecting the EARLIEST passes the sequencing gate and reaches the real
// proof obligation. That is the whole point: the artificial blocker is
// gone, the legitimate one remains, and nothing has been applied.
func TestApplyAmendment_SelectingTheEarliestReachesTheProofGate(t *testing.T) {
	cfg, baseline := ledgerFixture(t)
	before, existedBefore := authoritySnapshot(t, baseline)

	out, err := runApply(t, cfg, "001")
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	msg += out

	// It must NOT be the sequencing refusal any more.
	if strings.Contains(msg, "not the earliest") || strings.Contains(msg, "Apply the earliest first") {
		t.Fatalf("the earliest record was still refused on sequencing: %s", msg)
	}
	// It must still be refused, for the honest reason.
	if !strings.Contains(msg, "not proven") && !strings.Contains(msg, "journal") {
		t.Fatalf("expected the journal-proof refusal, got: %s", msg)
	}

	after, existsAfter := authoritySnapshot(t, baseline)
	if existedBefore != existsAfter || before != after {
		t.Error("reaching the proof gate advanced the applied marker")
	}
}

// An unknown selector is refused rather than silently falling back to the
// earliest — a fallback would apply a record the caller did not name.
func TestApplyAmendment_UnknownSelectorDoesNotFallBack(t *testing.T) {
	cfg, baseline := ledgerFixture(t)
	before, existedBefore := authoritySnapshot(t, baseline)

	_, err := runApply(t, cfg, "999-invented")
	if err == nil {
		t.Fatal("an unknown selector must refuse")
	}
	if !strings.Contains(err.Error(), "no unapplied amendment matching") {
		t.Errorf("the refusal should name the problem: %v", err)
	}

	after, existsAfter := authoritySnapshot(t, baseline)
	if existedBefore != existsAfter || before != after {
		t.Error("an unknown selector advanced the applied marker")
	}
}

// Selection narrows WHICH record must be evidenced; it never relaxes
// whether the work behind it happened.
//
// An earlier cut returned early on a matching selection and skipped the
// reached-tested check, which would have let an unproven splice through —
// the authority failure this whole proof bundle exists to prevent.
func TestApplyAmendment_SelectionDoesNotRelaxTheProofOfWork(t *testing.T) {
	cfg, baseline := ledgerFixture(t)

	// A journal that names 001 but never reached the test step.
	j := refineJournal{
		Feature:   "ledger-demo",
		Amendment: 1,
		Completed: []string{"amendment-written", "splice-applied"},
	}
	data, err := yaml.Marshal(&j)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refineJournalPath(cfg, "ledger-demo"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	before, existedBefore := authoritySnapshot(t, baseline)
	out, err := runApply(t, cfg, "001")
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	msg += out

	if err == nil {
		t.Fatal("a selected record whose work was never tested must still refuse")
	}
	if !strings.Contains(msg, "never tested") && !strings.Contains(msg, "cannot be shown complete") {
		t.Errorf("the refusal must name the missing proof of work: %s", msg)
	}
	after, existsAfter := authoritySnapshot(t, baseline)
	if existedBefore != existsAfter || before != after {
		t.Error("an unproven selected record advanced the applied marker")
	}
}

// The complement: a journal naming a DIFFERENT record than the one being
// applied is refused, so a selection cannot borrow another record's proof.
func TestApplyAmendment_SelectionCannotBorrowAnotherRecordsJournal(t *testing.T) {
	cfg, _ := ledgerFixture(t)
	j := refineJournal{
		Feature:   "ledger-demo",
		Amendment: 2, // evidences 002...
		Completed: []string{"amendment-written", "splice-applied", "rebuilt", "emitted", "tested"},
	}
	data, err := yaml.Marshal(&j)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refineJournalPath(cfg, "ledger-demo"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = runApply(t, cfg, "001") // ...while applying 001
	if err == nil {
		t.Fatal("a journal for another record must not prove this one")
	}
	if !strings.Contains(err.Error(), "journal accounts for amendment 2") {
		t.Errorf("the refusal should name the mismatch: %v", err)
	}
}
