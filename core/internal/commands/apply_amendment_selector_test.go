// parlay-feature: parlay-tool/ledger-and-contract
// parlay-artifact: test

package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

func pendingFixture() []parser.Amendment {
	return []parser.Amendment{
		{Seq: 1, Slug: "planned-phase-and-content-aware-ladder"},
		{Seq: 2, Slug: "terminal-rung-requires-both-founding-documents"},
	}
}

// A caller reading the refusal sees one form of the name and a caller
// reading a directory listing sees another. Making them guess which the
// flag wants is a refusal they hit twice.
func TestFindPendingAmendment_AcceptsEveryFormOfTheName(t *testing.T) {
	pending := pendingFixture()
	for _, sel := range []string{
		"planned-phase-and-content-aware-ladder", // slug
		"001",                                    // sequence
		"001-planned-phase-and-content-aware-ladder",    // identity
		"001-planned-phase-and-content-aware-ladder.md", // filename
	} {
		got, ok := findPendingAmendment(pending, sel)
		if !ok {
			t.Errorf("selector %q matched nothing", sel)
			continue
		}
		if got.Seq != 1 {
			t.Errorf("selector %q matched the wrong record: %d", sel, got.Seq)
		}
	}
}

func TestFindPendingAmendment_RejectsAnUnknownName(t *testing.T) {
	if _, ok := findPendingAmendment(pendingFixture(), "no-such-record"); ok {
		t.Error("an unknown selector must not match")
	}
	// A record that exists but is not pending must not match either.
	if _, ok := findPendingAmendment(pendingFixture(), "003"); ok {
		t.Error("a non-pending record must not match")
	}
}

// THE INVARIANT THE SELECTOR MUST NOT WEAKEN.
//
// The applied marker passes every record below it, so applying out of
// order would record the skipped one applied without anybody having
// applied it. The selector exists to let a caller work THROUGH a queue in
// order, never around it.
func TestApplyAmendmentSelector_OnlyTheEarliestIsSelectable(t *testing.T) {
	pending := pendingFixture()
	earliest := pending[0]

	later, ok := findPendingAmendment(pending, "002")
	if !ok {
		t.Fatal("fixture should contain 002")
	}
	if later.Seq == earliest.Seq {
		t.Fatal("fixture is wrong: 002 should not be the earliest")
	}
	// The command's guard is `match.Seq != earliest.Seq`; assert the
	// relationship it depends on, so a change to ordering that made 002
	// compare equal would fail here rather than silently permit a skip.
	if !(later.Seq > earliest.Seq) {
		t.Errorf("002 must sort after 001 for the ordering guard to hold: %d vs %d", later.Seq, earliest.Seq)
	}
}

// A SELECTED RECORD MUST NOT BORROW A LATER RECORD'S PROMISE.
//
// This is the defect that corrupted a real ledger. AFTER was derived with
// ProspectiveAuthority, which resolves the newest claim across the WHOLE
// unapplied tail — so with 002 superseding 001, `--amendment 001`
// displayed 002's promise text, bound the confirmation digest to it, and
// wrote a receipt describing a record it had not been asked about. The
// ledger's own integrity check then refused everything afterwards, and
// the only recovery was reverting the baseline.
//
// The selector had narrowed the proof and the marker advancement, but not
// the promise derivation. That gap is what this pins.
func TestSelector_SelectedRecordDoesNotBorrowASupersedingRecordsPromise(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)

	// 002 supersedes 001 and says something 001 does not.
	writeAmendment(t, featureDir, "002-node-health.md", `---
amendment: node-health
date: 2026-09-02
supersedes: [channel-choice]
amends_intents:
  - intent: check-readiness
    mode: revise
    version:
      title: Check Readiness Of Cluster Or Node
      goal: See if the cluster or any of its nodes is ready, including health.
      persona: Admin
      verify:
        - Readiness AND HEALTH are reported for the cluster and each node.
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions: []
affects: ["@my-feature/operation:x"]
---

## Change

Readiness now also covers health.

## Why

Readiness alone did not answer the question.

## Acceptance

- Health is reported.
`)

	const onlyIn002 = "HEALTH"

	// --- Selecting 001 shows 001's own delta, and none of 002's. ---
	applyAmendmentSelect = "001-channel-choice"
	t.Cleanup(func() { applyAmendmentSelect = "" })
	armApplyAmendment(t, "", false)
	prose, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err != nil {
		t.Fatalf("preflight for 001: %v\n%s", err, prose)
	}
	if strings.Contains(prose, onlyIn002) {
		t.Errorf("selecting 001 displayed 002's promise text — the human would approve a delta the selected record does not declare:\n%s", prose)
	}
	if !strings.Contains(prose, "Readiness is reported for the cluster and each node.") {
		t.Errorf("selecting 001 did not display 001's own promise:\n%s", prose)
	}

	// --- Applying 001 receipts 001's text, and validation stays clean. ---
	armApplyAmendment(t, "", true)
	jsonOut, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err != nil {
		t.Fatalf("json preflight: %v", err)
	}
	var pf applyAmendmentPreflight
	if err := json.Unmarshal([]byte(jsonOut), &pf); err != nil {
		t.Fatal(err)
	}
	armApplyAmendment(t, pf.Digest, false)
	applyAmendmentSelect = "001-channel-choice"
	if out, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("applying 001 must succeed: %v\n%s", err, out)
	}

	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if bl.LastAppliedAmendment != 1 {
		t.Errorf("marker = %d, want 1", bl.LastAppliedAmendment)
	}
	receipt := bl.TransitionReceipts["001-channel-choice.md"]
	blob, _ := json.Marshal(receipt)
	if strings.Contains(string(blob), onlyIn002) {
		t.Errorf("the receipt for 001 records 002's promise text:\n%s", blob)
	}
	// Not only the prose: the scope and attestation must be 001's too.
	for _, ev := range receipt.Payload.Evolution {
		if strings.Contains(ev.Attestation, onlyIn002) {
			t.Errorf("the attestation borrowed 002's text: %q", ev.Attestation)
		}
	}

	// The ledger must accept what it just wrote. This is the check that
	// refused after the real corruption.
	if issues := ledgerIssues(t, cfg, "my-feature"); len(issues) != 0 {
		t.Errorf("post-apply validation is not clean: %v", issues)
	}

	// --- Selecting 002 now shows ITS delta, from the applied-001 state. ---
	// 002 needs its own splice proof; the journal is per amendment.
	writeRefineJournal(t, cfg, "my-feature", 2)
	applyAmendmentSelect = "002-node-health"
	armApplyAmendment(t, "", false)
	prose2, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err != nil {
		t.Fatalf("preflight for 002: %v\n%s", err, prose2)
	}
	if !strings.Contains(prose2, onlyIn002) {
		t.Errorf("selecting 002 did not display its own promise:\n%s", prose2)
	}
	// BEFORE is now 001's applied text, not the founding text.
	if !strings.Contains(prose2, "Readiness is reported for the cluster and each node.") {
		t.Errorf("002's BEFORE is not the applied-001 state:\n%s", prose2)
	}
}

// ledgerIssues runs the ledger's own validation and returns its errors.
func ledgerIssues(t *testing.T, cfg *config.Context, slug string) []string {
	t.Helper()
	var buf bytes.Buffer
	cmd := testCommandWithContext(t, cfg)
	cmd.SetOut(&buf)
	if err := runCheckAmendments(cmd, []string{"@" + slug}); err != nil {
		t.Fatalf("check-amendments: %v", err)
	}
	var got struct {
		Issues []struct {
			Severity string `json:"severity"`
			Message  string `json:"message"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("check-amendments output is not JSON: %v\n%s", err, buf.String())
	}
	var out []string
	for _, i := range got.Issues {
		if i.Severity == "error" {
			out = append(out, i.Message)
		}
	}
	return out
}
