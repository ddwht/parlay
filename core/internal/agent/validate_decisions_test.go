package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// The decisions: block is where codegen records an implementation judgment call
// and names the files that enforce it. The propagation check (WP7) exists so a
// recorded reason provably reaches the code it governs: a file that carries the
// decision but not its id strands the reason where the next reader never meets
// it, and "fixes" the unexplained line. These tests hold the check to its three
// cases — reached, stranded, and not-yet-written.

// writeDecisionsBuildfile lays down a buildfile carrying a single decisions:
// entry whose enforced-by names one file, and returns the buildfile path and
// the source root.
func writeDecisionsBuildfile(t *testing.T, enforcedRel string) (bfPath, root string) {
	t.Helper()
	root = t.TempDir()
	bfDir := filepath.Join(root, ".parlay", "build", "submit-expense")
	if err := os.MkdirAll(bfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bfPath = filepath.Join(bfDir, "buildfile.yaml")
	if err := os.WriteFile(bfPath, []byte(`feature: submit-expense
schema_version: 1
adapter: angular-clarity

components:
  wizard-step:
    widget: ClrInput

decisions:
  - id: DEC-optimistic-write
    component: wizard-step
    decided: write-through the store before the server acks
    why: the framework rerenders on store change, not on promise resolution
    enforced-by:
      - `+enforcedRel+`
    obsolete-when: the store gains a pending-state channel
`), 0o644); err != nil {
		t.Fatal(err)
	}
	return bfPath, root
}

func findRationaleStranded(errs []ValidationError) *ValidationError {
	for i := range errs {
		if errs[i].Code == "rationale-stranded" {
			return &errs[i]
		}
	}
	return nil
}

// TestDecisionPropagation_StrandedWhenIdAbsent: the enforcing file exists but
// never names the decision id — the reason is on disk in the buildfile and
// absent from the file a later reader edits. That is exactly the stranding.
func TestDecisionPropagation_StrandedWhenIdAbsent(t *testing.T) {
	bfPath, root := writeDecisionsBuildfile(t, "src/wizard-step.ts")
	// The file exists but carries no reference to the decision id.
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src/wizard-step.ts"),
		[]byte("export function wizardStep() { store.write(); }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := findRationaleStranded(ValidateBuildfileDeepStructured(bfPath, ""))
	if got == nil {
		t.Fatal("expected rationale-stranded for an enforcing file that omits the decision id")
	}
	if got.Severity != "warning" {
		t.Errorf("rationale-stranded severity = %q, want warning", got.Severity)
	}
}

// TestDecisionPropagation_ReachedWhenIdPresent: the enforcing file names the id
// (a comment is enough), so the reason reached the code and nothing is stranded.
func TestDecisionPropagation_ReachedWhenIdPresent(t *testing.T) {
	bfPath, root := writeDecisionsBuildfile(t, "src/wizard-step.ts")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src/wizard-step.ts"),
		[]byte("// DEC-optimistic-write: write-through before ack\nexport function wizardStep() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := findRationaleStranded(ValidateBuildfileDeepStructured(bfPath, "")); got != nil {
		t.Errorf("rationale-stranded fired for a file that names the id: %s", got.Message)
	}
}

// TestDecisionPropagation_SilentWhenFileUnwritten: a plan.creates path recorded
// in enforced-by before codegen has written it is not stranded — it is simply
// unwritten. Firing on it would make the check noise for every buildfile
// validated between build and generate.
func TestDecisionPropagation_SilentWhenFileUnwritten(t *testing.T) {
	bfPath, _ := writeDecisionsBuildfile(t, "src/not-yet-written.ts")
	if got := findRationaleStranded(ValidateBuildfileDeepStructured(bfPath, "")); got != nil {
		t.Errorf("rationale-stranded fired for a file that does not yet exist: %s", got.Message)
	}
}
