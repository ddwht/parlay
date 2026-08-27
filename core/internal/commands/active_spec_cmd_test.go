// parlay-feature: parlay-tool/intent-supersession
// parlay-component: active-specification-resolver
// parlay-artifact: test

package commands

import (
	"bytes"
	"encoding/json"
	"testing"
)

func runActiveSpec_(t *testing.T, ref string) activeSpecOutput {
	t.Helper()
	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	if err := runActiveSpec(cmd, []string{ref}); err != nil {
		t.Fatalf("active-spec failed: %v\n%s", err, buf.String())
	}
	var out activeSpecOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, buf.String())
	}
	return out
}

func TestActiveSpec_ReportsEveryPromiseWhenNothingIsRetired(t *testing.T) {
	dir := setupTestDir(t)
	writeVerifyFixture(t, dir)

	out := runActiveSpec_(t, "@verify-fixture")
	if len(out.Active) != 2 || len(out.Retired) != 0 || out.Blocked {
		t.Errorf("expected 2 active, 0 retired, unblocked; got %+v", out)
	}
}

func TestActiveSpec_CarriesWhatARetiredPromiseSaid(t *testing.T) {
	// Refinement shows the user what they are giving up, so the retired entry
	// has to carry the promise itself — a slug alone could not be presented.
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-browsing-moves-to-search.md", supersedeBrowse)
	writeBaselineApplied(t, "verify-fixture", 1)

	out := runActiveSpec_(t, "@verify-fixture")
	if len(out.Retired) != 1 {
		t.Fatalf("expected one retired promise, got %+v", out.Retired)
	}
	r := out.Retired[0]
	if r.Slug != "browse-the-things" || r.ByAmendment != "browsing-moves-to-search" {
		t.Errorf("retired entry should name the promise and the decision that replaced it: %+v", r)
	}
	if r.Goal == "" || len(r.Verify) == 0 {
		t.Errorf("a retired promise keeps its Goal and Verify so refinement can show what is being given up: %+v", r)
	}
	if out.Blocked {
		t.Error("an applied retirement blocks nothing")
	}
}

func TestActiveSpec_PendingRetirementIsStillInForceAndBlocks(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir)
	writeAmendment(t, featDir, "001-browsing-moves-to-search.md", supersedeBrowse)
	// No baseline: recorded, not applied.

	out := runActiveSpec_(t, "@verify-fixture")
	if len(out.Active) != 2 {
		t.Errorf("the promise stands until the decision is applied; active=%+v", out.Active)
	}
	if len(out.Pending) != 1 || out.Pending[0].Intent != "browse-the-things" {
		t.Errorf("expected the pending retirement named, got %+v", out.Pending)
	}
	if !out.Blocked {
		t.Error("a recorded-but-unapplied retirement must block every advancing boundary")
	}
}
