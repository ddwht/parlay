// parlay-feature: parlay-tool/multi-root
// parlay-component: bare-parent-upgrade-error
// parlay-artifact: test

package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// TestDeployToRoot_BareParentHardErrors verifies that deployToRoot
// REFUSES to proceed at a multi-root parent registry that has
// roots.yaml but no config.yaml of its own. The previously-tolerant
// fallback (silently deploy schemas, skip skills) has been removed in
// favour of an atomic hard-error — see cross-cutting
// bare-parent-fallback-removal-in-deployToRoot. Migration path:
// `parlay repair` creates the missing config.yaml; the next `parlay
// upgrade` then runs cleanly.
func TestDeployToRoot_BareParentHardErrors(t *testing.T) {
	tmp := t.TempDir()
	parlayDir := filepath.Join(tmp, config.ParlayDir)
	if err := os.MkdirAll(parlayDir, 0o755); err != nil {
		t.Fatalf("mkdir .parlay: %v", err)
	}
	rootsPath := filepath.Join(parlayDir, config.RootsIndexFile)
	if err := os.WriteFile(rootsPath, []byte("children: []\n"), 0o644); err != nil {
		t.Fatalf("write roots.yaml: %v", err)
	}

	_, err := deployToRoot(tmp)
	if err == nil {
		t.Fatalf("expected bare-parent topology to hard-error; got nil")
	}
	if !errors.Is(err, ErrBareParentTopology) {
		t.Fatalf("expected ErrBareParentTopology, got %v", err)
	}
	if !strings.Contains(err.Error(), "parlay repair") {
		t.Fatalf("expected error message to point at parlay repair; got %v", err)
	}

	// No partial work: schemas directory must NOT exist after the refusal.
	schemasPath := filepath.Join(parlayDir, config.SchemasDir)
	if _, err := os.Stat(schemasPath); err == nil {
		t.Fatalf("expected no schemas deployed after bare-parent error; found %s", schemasPath)
	}
}

// TestDeployToRoot_MissingConfigAndRoots preserves the original "run
// parlay init first" error for the case where neither config.yaml nor
// roots.yaml exists — the user really hasn't initialized anything.
// This case is distinct from bare-parent and uses different prose.
func TestDeployToRoot_MissingConfigAndRoots(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, config.ParlayDir), 0o755); err != nil {
		t.Fatalf("mkdir .parlay: %v", err)
	}

	_, err := deployToRoot(tmp)
	if err == nil {
		t.Fatalf("expected error when both config.yaml and roots.yaml are missing")
	}
	if !strings.Contains(err.Error(), "parlay init") {
		t.Fatalf("expected uninitialized-project error to mention parlay init; got %v", err)
	}
	// Distinct from bare-parent: the uninitialized error must NOT wrap
	// ErrBareParentTopology.
	if errors.Is(err, ErrBareParentTopology) {
		t.Fatalf("uninitialized error must be distinct from bare-parent; got %v", err)
	}
}

// TestDeployToRoot_MissingAIAgentHardErrors covers the new
// missing-agent-on-upgrade-error component: a parent config.yaml that
// exists but lacks ai-agent must hard-error before any deploy.
func TestDeployToRoot_MissingAIAgentHardErrors(t *testing.T) {
	tmp := t.TempDir()
	parlayDir := filepath.Join(tmp, config.ParlayDir)
	if err := os.MkdirAll(parlayDir, 0o755); err != nil {
		t.Fatalf("mkdir .parlay: %v", err)
	}
	cfgPath := filepath.Join(parlayDir, config.ConfigFile)
	if err := os.WriteFile(cfgPath, []byte("sdd-framework: parlay-spec\n"), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	_, err := deployToRoot(tmp)
	if err == nil {
		t.Fatalf("expected missing-ai-agent to hard-error; got nil")
	}
	if !strings.Contains(err.Error(), "no agent identity declared at parent root") {
		t.Fatalf("expected missing-ai-agent prose; got %v", err)
	}
}
