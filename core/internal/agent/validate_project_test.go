package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestClassifyCrossCuttingEntry_StableAcrossCodegen guards the idempotence
// defect: an entry authored as purely-introducing (target file absent, so
// routed to plan.creates) was reclassified modifies-only the instant codegen
// created that very file, and then demanded a plan.modifies row that was
// never appropriate. The buildfile did not change — the world did — so a
// correct buildfile failed re-validation right after its own code was
// generated.
func TestClassifyCrossCuttingEntry_StableAcrossCodegen(t *testing.T) {
	root := t.TempDir()
	target := "src/app/core/domain/expense-category.ts"
	entry := deepCrossCuttingEntry{
		ID:          "single-source-category-allowlist",
		TargetFiles: []string{target},
	}
	planned := map[string]bool{target: true}

	// Authoring time: the file does not exist yet.
	if got := classifyCrossCuttingEntryWithPlan(entry, root, planned); got != "purely-introducing" {
		t.Fatalf("before codegen: kind = %q, want purely-introducing", got)
	}

	// Codegen runs and creates exactly the file the plan promised.
	abs := filepath.Join(root, target)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("export const EXPENSE_CATEGORIES = [];\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := classifyCrossCuttingEntryWithPlan(entry, root, planned); got != "purely-introducing" {
		t.Errorf("after codegen: kind = %q, want purely-introducing — classification must follow the "+
			"authored plan, not current disk state, or a correct buildfile stops re-validating", got)
	}
}

// TestClassifyCrossCuttingEntry_GenuineModifyStillDetected is the companion:
// a target the plan does NOT claim to create, which exists on disk, is a
// real modifies-only entry and must still classify as one.
func TestClassifyCrossCuttingEntry_GenuineModifyStillDetected(t *testing.T) {
	root := t.TempDir()
	target := "src/app/features/other/existing.ts"
	abs := filepath.Join(root, target)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("// pre-existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry := deepCrossCuttingEntry{ID: "audit-chokepoint", TargetFiles: []string{target}}

	// Plan claims no creates for this entry — it genuinely modifies.
	if got := classifyCrossCuttingEntryWithPlan(entry, root, map[string]bool{}); got != "modifies-only" {
		t.Errorf("kind = %q, want modifies-only", got)
	}
}

// TestApplyBuildfileSeverity_SharedTable guards the divergence where
// check-buildfile and validate --deep reported different verdicts for the
// same buildfile because each carried its own severity table.
func TestApplyBuildfileSeverity_SharedTable(t *testing.T) {
	got := ApplyBuildfileSeverity([]ValidationError{
		{Code: "plan-create-collision"},
		{Code: "cross-cutting-target-not-in-modifies"},
		{Code: "unknown-component-widget"},
	})
	want := map[string]string{
		"plan-create-collision":                string(SeverityWarning),
		"cross-cutting-target-not-in-modifies": string(SeverityError),
		"unknown-component-widget":             string(SeverityWarning),
	}
	for _, f := range got {
		if f.Severity != want[f.Code] {
			t.Errorf("%s severity = %q, want %q", f.Code, f.Severity, want[f.Code])
		}
	}
}
