// parlay-feature: parlay-tool/ledger-and-contract
// parlay-component: affected-set-probe
// parlay-artifact: test

package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAffectedSet_FindsDependentsAndDirtySet(t *testing.T) {
	dir := setupTestDir(t)
	featDir := writeVerifyFixture(t, dir) // verify-fixture

	// A ledger entry declaring one ref.
	writeAmendment(t, featDir, "001-a.md", "---\namendment: a\ndate: 2026-08-13\naffects: [\"@verify-fixture/operation:thing.create\"]\n---\n## Change\nx\n## Acceptance\n- y\n")

	// A consumer feature whose buildfile references verify-fixture, and an
	// unrelated feature that does not.
	for name, ref := range map[string]string{
		"consumer":  "@verify-fixture/operation:thing.create",
		"unrelated": "@something-else/operation:z",
	} {
		if err := os.MkdirAll(filepath.Join(dir, "spec", "intents", name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "spec", "intents", name, "intents.md"), []byte("# F\n\n## G\n\n**Goal**: g.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		buildDir := testContext(t).BuildPath(name)
		if err := os.MkdirAll(buildDir, 0o755); err != nil {
			t.Fatal(err)
		}
		bf := "feature: " + name + "\nbindings:\n  - operation: '" + ref + "'\n"
		if err := os.WriteFile(filepath.Join(buildDir, "buildfile.yaml"), []byte(bf), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	if err := runAffectedSet(cmd, []string{"@verify-fixture"}); err != nil {
		t.Fatal(err)
	}
	var out affectedSetOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, buf.String())
	}
	if len(out.Dependents) != 1 || out.Dependents[0] != "consumer" {
		t.Errorf("expected [consumer], got %v", out.Dependents)
	}
	if len(out.Affected) != 2 || out.Affected[0] != "verify-fixture" {
		t.Errorf("affected should be the feature plus dependents, got %v", out.Affected)
	}
	if len(out.DirtySet) != 1 {
		t.Errorf("expected the declared ref in dirty_set, got %v", out.DirtySet)
	}
}
