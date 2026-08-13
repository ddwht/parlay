// parlay-feature: parlay-tool/ledger-and-contract
// parlay-component: amendment-ledger-check
// parlay-artifact: test

package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeAmendment writes one ledger file into a feature dir.
func writeAmendment(t *testing.T, featDir, name, content string) {
	t.Helper()
	amDir := filepath.Join(featDir, "amendments")
	if err := os.MkdirAll(amDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(amDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCheckAmendments_(t *testing.T, featureRef string) (checkAmendmentsOutput, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	runErr := runCheckAmendments(cmd, []string{featureRef})
	var out checkAmendmentsOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	return out, runErr
}

func TestCheckAmendments_HealthyLedgerEmitsDirtySet(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir) // verify-fixture: op thing.create, fragments Create Form / Thing List

	writeAmendment(t, featDir, "001-tighten-create.md", `---
amendment: tighten-create
date: 2026-08-13
trigger: "duplicate names slipped through"
affects:
  - "@verify-fixture/operation:thing.create"
  - "@verify-fixture/surface:thing-list"
---

## Change
Creation rejects duplicate names case-insensitively.

## Why
Two Things differing only by case are the same thing.

## Acceptance
- Creating "Alpha" when "alpha" exists is rejected with conflict.
`)

	out, err := runCheckAmendments_(t, "@verify-fixture")
	if err != nil {
		t.Fatalf("healthy ledger should exit zero: %v (issues: %+v)", err, out.Issues)
	}
	if !out.Ready {
		t.Errorf("expected ready=true; issues: %+v", out.Issues)
	}
	if len(out.DirtySet) != 2 {
		t.Errorf("expected 2 refs in dirty set, got %v", out.DirtySet)
	}
	if len(out.Amendments) != 1 || out.Amendments[0].Slug != "tighten-create" {
		t.Errorf("amendment listing wrong: %+v", out.Amendments)
	}
}

func TestCheckAmendments_UnresolvedAffectsIsError(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)

	writeAmendment(t, featDir, "001-ghost.md", `---
amendment: ghost
date: 2026-08-13
affects:
  - "@verify-fixture/operation:does.not.exist"
---

## Change
X.

## Acceptance
- Y.
`)

	out, err := runCheckAmendments_(t, "@verify-fixture")
	if err == nil {
		t.Fatal("unresolved affects ref should exit non-zero")
	}
	if !hasIssueCode(out, "amendment-affects-unresolved") {
		t.Errorf("expected amendment-affects-unresolved; got %+v", out.Issues)
	}
	if len(out.DirtySet) != 0 {
		t.Errorf("unresolved ref must not enter the dirty set; got %v", out.DirtySet)
	}
}

func TestCheckAmendments_LedgerIntegrityFindings(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)

	// 001: frontmatter slug disagrees with filename.
	writeAmendment(t, featDir, "001-real-name.md", `---
amendment: other-name
date: 2026-08-13
affects: ["@verify-fixture/operation:thing.create"]
---

## Change
X.

## Acceptance
- Y.
`)
	// 003: gap after 001 (002 missing), supersedes something unknown.
	writeAmendment(t, featDir, "003-later.md", `---
amendment: later
date: 2026-08-14
affects: ["@verify-fixture/operation:thing.create"]
supersedes: [never-existed]
---

## Change
X.

## Acceptance
- Y.
`)
	// A stray file invisible to the ledger.
	writeAmendment(t, featDir, "07-badly-numbered.md", "whatever")

	out, err := runCheckAmendments_(t, "@verify-fixture")
	if err == nil {
		t.Fatal("integrity problems should exit non-zero")
	}
	for _, code := range []string{"amendment-slug-mismatch", "amendment-supersedes-unknown", "amendment-out-of-sequence"} {
		if !hasIssueCode(out, code) {
			t.Errorf("expected %s; got %+v", code, out.Issues)
		}
	}
	if !hasIssueWith(out, "amendment-sequence-gap", "001 -> 003") {
		t.Errorf("expected a sequence-gap warning naming 001 -> 003; got %+v", out.Issues)
	}
}

func TestCheckAmendments_EmptyLedgerIsHealthy(t *testing.T) {
	dir := setupTestDir(t)
	writeVerifyFixture(t, dir)

	out, err := runCheckAmendments_(t, "@verify-fixture")
	if err != nil {
		t.Fatalf("empty ledger should be healthy: %v", err)
	}
	if !out.Ready || len(out.Amendments) != 0 || len(out.DirtySet) != 0 {
		t.Errorf("empty ledger should be ready with nothing to report: %+v", out)
	}
}

func hasIssueCode(out checkAmendmentsOutput, code string) bool {
	for _, i := range out.Issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

func hasIssueWith(out checkAmendmentsOutput, code, substr string) bool {
	for _, i := range out.Issues {
		if i.Code == code && strings.Contains(i.Message, substr) {
			return true
		}
	}
	return false
}
