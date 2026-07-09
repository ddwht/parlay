package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
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

// TestPreRun_AmbiguityAsSignal_ExitsNonZero drives persistentPreRun
// in-process. It used to build and exec a real parlay binary in a
// subprocess because os.Exit couldn't be observed inside the test
// process — now that the ambiguity-as-signal path returns an
// *ExitCodeError instead of calling os.Exit directly (the 6.1
// testability substrate), the whole scenario runs as a normal in-process
// test with no subprocess, no `go build`, and no risk of hanging in
// sandboxes where spawning a child process misbehaves.
func TestPreRun_AmbiguityAsSignal_ExitsNonZero(t *testing.T) {
	tmp := setupTestDir(t)
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

	ambiguityAsSignalFlag = true
	resetFlagsAfterTest(t, rootCmd.PersistentFlags())

	cmd := &cobra.Command{Use: "status"}
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	err := persistentPreRun(cmd, nil)
	if err == nil {
		t.Fatal("expected an *ExitCodeError, got nil")
	}
	var exitErr *ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitCodeError, got %T: %v", err, err)
	}
	if exitErr.Code != AmbiguityExitCode {
		t.Errorf("exit code: want %d, got %d", AmbiguityExitCode, exitErr.Code)
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
