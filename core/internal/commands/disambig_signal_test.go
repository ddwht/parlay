package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

func TestEmitAmbiguitySignal_ShapeIsParseable(t *testing.T) {
	var buf bytes.Buffer
	err := emitAmbiguitySignal(&buf, AmbiguitySignal{
		Trigger: TriggerAmbiguousActiveRoot,
		Candidates: []config.Candidate{
			{Name: "web", RelativePath: "apps/web", Reason: config.ReasonDiscoveredBelowCwd},
			{Name: "api", RelativePath: "apps/api", Reason: config.ReasonDiscoveredBelowCwd},
		},
		Hint: "re-invoke with --root <name>",
	})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}

	var got AmbiguitySignal
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Kind != "ambiguity" {
		t.Errorf("kind: want ambiguity, got %q", got.Kind)
	}
	if got.Trigger != TriggerAmbiguousActiveRoot {
		t.Errorf("trigger: want %q, got %q", TriggerAmbiguousActiveRoot, got.Trigger)
	}
	if len(got.Candidates) != 2 {
		t.Errorf("candidate count: want 2, got %d", len(got.Candidates))
	}
	if got.Candidates[0].Name != "web" {
		t.Errorf("first candidate name: %s", got.Candidates[0].Name)
	}
}

func TestPreRun_AmbiguityAsSignal_ExitsNonZero(t *testing.T) {
	// Build a tiny binary that runs PreRunE, since os.Exit can't be
	// observed inside the same test process. Skip if `go` isn't on PATH
	// (common in CI sandboxes).
	if _, err := exec.LookPath("go"); err != nil {
		// Allow GOROOT-relative go binary too.
		alt := filepath.Join(os.Getenv("HOME"), "go", "bin", "go")
		if _, err := os.Stat(alt); err != nil {
			t.Skip("go binary not available")
		}
	}

	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	// Place .git at tmp so walk-up stops here, AND a child .parlay/ below.
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(tmp, "apps", "web")
	if err := os.MkdirAll(filepath.Join(child, config.ParlayDir), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, config.ParlayDir, config.ConfigFile), nil, 0644); err != nil {
		t.Fatal(err)
	}

	binPath := filepath.Join(tmp, "parlay-test")
	build := exec.Command("/home/node/go/bin/go", "build", "-o", binPath, "./core/cmd/parlay")
	build.Dir = "/workspace/parlay-dev"
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("build failed: %v\n%s", err, out)
	}

	cmd := exec.Command(binPath, "--ambiguity-as-signal", "status")
	cmd.Dir = tmp
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit, got nil")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("not an ExitError: %v", err)
	}
	if exitErr.ExitCode() != AmbiguityExitCode {
		t.Errorf("exit code: want %d, got %d", AmbiguityExitCode, exitErr.ExitCode())
	}

	// Verify the JSON envelope is on stderr.
	out := stderr.String()
	jsonLine := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") {
			jsonLine = line
			break
		}
	}
	if jsonLine == "" {
		t.Fatalf("no JSON line in stderr:\n%s", out)
	}
	var sig AmbiguitySignal
	if err := json.Unmarshal([]byte(jsonLine), &sig); err != nil {
		t.Fatalf("unmarshal: %v\nstderr: %s", err, out)
	}
	if sig.Trigger != TriggerAmbiguousActiveRoot {
		t.Errorf("trigger: want %q, got %q", TriggerAmbiguousActiveRoot, sig.Trigger)
	}
	if len(sig.Candidates) == 0 {
		t.Errorf("expected at least one candidate, got 0")
	}
}
