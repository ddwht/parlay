// parlay-feature: parlay-tool/ledger-and-contract
// parlay-component: amendment-artifact
// parlay-artifact: test

package parser

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleAmendment = `---
amendment: list-status-field
date: 2026-08-13
trigger: "@export needs report status to decide exportability"
affects:
  - "@reports/operation:list-reports"
  - "@reports/surface:report-row"
supersedes: []
---

## Change
` + "`list-reports`" + ` output gains status (enum: draft | final | archived).

## Why
Export must skip drafts.

## Acceptance
- Listing shows one status badge per row.
- list-reports output includes status for every item.
`

func TestParseAmendmentBytes(t *testing.T) {
	a, err := ParseAmendmentBytes("001-list-status-field.md", []byte(sampleAmendment))
	if err != nil {
		t.Fatal(err)
	}
	if a.Slug != "list-status-field" || a.Date != "2026-08-13" {
		t.Errorf("frontmatter not parsed: %+v", a)
	}
	if len(a.Affects) != 2 || a.Affects[0] != "@reports/operation:list-reports" {
		t.Errorf("affects not parsed: %v", a.Affects)
	}
	if a.Trigger == "" {
		t.Errorf("trigger not parsed")
	}
	if a.Change == "" || a.Why != "Export must skip drafts." {
		t.Errorf("body sections not parsed: change=%q why=%q", a.Change, a.Why)
	}
	if len(a.Acceptance) != 2 || a.Acceptance[0] != "Listing shows one status badge per row." {
		t.Errorf("acceptance bullets not parsed: %v", a.Acceptance)
	}
}

func TestParseAmendmentBytes_RejectsMissingFrontmatter(t *testing.T) {
	if _, err := ParseAmendmentBytes("x.md", []byte("## Change\nno frontmatter\n")); err == nil {
		t.Error("expected error for missing frontmatter")
	}
	if _, err := ParseAmendmentBytes("x.md", []byte("---\namendment: a\nno closing fence")); err == nil {
		t.Error("expected error for unterminated frontmatter")
	}
}

func TestParseAmendmentRef(t *testing.T) {
	ref, err := ParseAmendmentRef("@parlay-tool/multi-root/operation:root.add")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Feature != "parlay-tool/multi-root" || ref.Kind != "operation" || ref.Name != "root.add" {
		t.Errorf("nested feature ref mis-parsed: %+v", ref)
	}

	for _, bad := range []string{
		"reports/operation:x",   // no @
		"@reports",              // no kind segment
		"@reports/list-reports", // intent-style ref, no kind
		"@reports/widget:x",     // unknown kind
		"@reports/operation:",   // empty name
	} {
		if _, err := ParseAmendmentRef(bad); err == nil {
			t.Errorf("expected %q to be rejected", bad)
		}
	}
}

func TestLoadFeatureAmendments_OrdersAndFilters(t *testing.T) {
	dir := t.TempDir()
	amDir := filepath.Join(dir, "amendments")
	if err := os.MkdirAll(amDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Written out of order, plus a stray file the loader must ignore.
	files := map[string]string{
		"002-second.md": "---\namendment: second\ndate: 2026-08-14\naffects: [\"@f/operation:x\"]\n---\n## Change\nb\n",
		"001-first.md":  "---\namendment: first\ndate: 2026-08-13\naffects: [\"@f/operation:x\"]\n---\n## Change\na\n",
		"notes.txt":     "not an amendment",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(amDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	amendments, err := LoadFeatureAmendments(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(amendments) != 2 {
		t.Fatalf("expected 2 amendments, got %d", len(amendments))
	}
	if amendments[0].Seq != 1 || amendments[0].FileSlug != "first" || amendments[1].Seq != 2 {
		t.Errorf("not sorted by sequence: %+v", amendments)
	}
}

func TestLoadFeatureAmendments_MissingDirIsEmptyLedger(t *testing.T) {
	amendments, err := LoadFeatureAmendments(t.TempDir())
	if err != nil || amendments != nil {
		t.Errorf("missing amendments/ should be an empty ledger, got %v, %v", amendments, err)
	}
}
