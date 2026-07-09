package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// setupCheckBuildfileProject writes a minimal parlay project structure
// at tmp and returns the resolved Context. Caller passes a buildfile
// body that's written to .parlay/build/<feature>/buildfile.yaml.
func setupCheckBuildfileProject(t *testing.T, slug, body string) (*config.Context, string) {
	t.Helper()
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	if err := os.MkdirAll(filepath.Join(tmp, config.ParlayDir, "build", slug), 0755); err != nil {
		t.Fatal(err)
	}
	bfPath := filepath.Join(tmp, config.ParlayDir, "build", slug, "buildfile.yaml")
	if err := os.WriteFile(bfPath, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Name: filepath.Base(tmp), Path: tmp, Kind: config.RootKindStandalone},
	}, nil)
	return cfg, bfPath
}

// runCheckBuildfileExitCode runs check-buildfile in-process and returns
// the parsed JSON output plus the effective exit code. This used to
// shell out to a real built binary in a subprocess because os.Exit
// couldn't be observed inside the test process; now that
// emitCheckBuildfileOutput returns an *ExitCodeError instead of calling
// os.Exit directly (the 6.1 testability substrate), the whole scenario
// runs in-process — no `go build`, no subprocess, no risk of hanging in
// sandboxes where spawning a child process misbehaves.
func runCheckBuildfileExitCode(t *testing.T, cfg *config.Context, slug string) (checkBuildfileOutput, int) {
	t.Helper()
	cmd := testCommandWithContext(t, cfg)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	runErr := runCheckBuildfile(cmd, []string{"@" + slug})

	exitCode := 0
	if runErr != nil {
		var exitErr *ExitCodeError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.Code
		} else {
			t.Fatalf("run check-buildfile: %v", runErr)
		}
	}
	var out checkBuildfileOutput
	if jsonErr := json.Unmarshal(stdout.Bytes(), &out); jsonErr != nil {
		t.Fatalf("parse JSON: %v\nstdout: %s", jsonErr, stdout.String())
	}
	return out, exitCode
}

func TestCheckBuildfile_BuildfileMissing(t *testing.T) {
	cfg, _ := setupCheckBuildfileProject(t, "x", "")
	// Remove the buildfile we just wrote to simulate "missing".
	bfPath := filepath.Join(cfg.Root.Path, config.ParlayDir, "build", "x", "buildfile.yaml")
	os.Remove(bfPath)

	out, code := runCheckBuildfileExitCode(t, cfg, "x")
	if code == 0 {
		t.Errorf("expected non-zero exit, got 0; output: %+v", out)
	}
	if len(out.Issues) == 0 || out.Issues[0].Code != "buildfile-not-found" {
		t.Errorf("expected buildfile-not-found error, got: %+v", out.Issues)
	}
}

func TestCheckBuildfile_HappyPath(t *testing.T) {
	body := `feature: x
adapter: go-cli
models: {}
components:
  comp-a:
    source: "@x/frag"
plan:
  creates:
    - path: cmd/comp_a.go
      sources: [component/comp-a]
`
	cfg, _ := setupCheckBuildfileProject(t, "x", body)
	out, code := runCheckBuildfileExitCode(t, cfg, "x")
	if code != 0 {
		t.Errorf("expected exit 0, got %d; issues: %+v", code, out.Issues)
	}
	if !out.Ready {
		t.Errorf("expected ready=true, got %+v", out)
	}
}

func TestCheckBuildfile_MissingPlanFails(t *testing.T) {
	body := `feature: x
adapter: go-cli
models: {}
components: {}
`
	cfg, _ := setupCheckBuildfileProject(t, "x", body)
	out, code := runCheckBuildfileExitCode(t, cfg, "x")
	if code == 0 {
		t.Errorf("expected non-zero exit, got 0")
	}
	found := false
	for _, issue := range out.Issues {
		if issue.Code == "missing-plan" && issue.Severity == "error" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected missing-plan error, got: %+v", out.Issues)
	}
}

func TestCheckBuildfile_PlanCollisionIsWarning(t *testing.T) {
	// Create a project with an existing source file, and a buildfile
	// whose plan.creates collides with it.
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	srcDir := filepath.Join(tmp, "cmd")
	os.MkdirAll(srcDir, 0755)
	collidePath := filepath.Join(srcDir, "comp_a.go")
	os.WriteFile(collidePath, []byte("package main\n"), 0644)

	buildDir := filepath.Join(tmp, config.ParlayDir, "build", "x")
	os.MkdirAll(buildDir, 0755)
	body := `feature: x
adapter: go-cli
models: {}
components:
  comp-a:
    source: "@x/frag"
plan:
  creates:
    - path: cmd/comp_a.go
      sources: [component/comp-a]
`
	bfPath := filepath.Join(buildDir, "buildfile.yaml")
	os.WriteFile(bfPath, []byte(body), 0644)

	cfg := config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Name: "x", Path: tmp, Kind: config.RootKindStandalone},
	}, nil)
	out, code := runCheckBuildfileExitCode(t, cfg, "x")
	// plan-create-collision is a warning, not an error → exit 0, ready=true.
	if code != 0 {
		t.Errorf("expected exit 0 for warning, got %d; issues: %+v", code, out.Issues)
	}
	found := false
	for _, issue := range out.Issues {
		if issue.Code == "plan-create-collision" && issue.Severity == "warning" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected plan-create-collision warning, got: %+v", out.Issues)
	}
}

func TestCheckBuildfile_PlanModifyMissingIsError(t *testing.T) {
	body := `feature: x
adapter: go-cli
models: {}
components: {}
cross-cutting:
  - id: my-cc
    source: "@x/intent"
    target-files:
      - internal/missing/file.go
    transform: "edit"
plan:
  modifies:
    - path: internal/missing/file.go
      sources: [cross-cutting/my-cc]
`
	cfg, _ := setupCheckBuildfileProject(t, "x", body)
	out, code := runCheckBuildfileExitCode(t, cfg, "x")
	if code == 0 {
		t.Errorf("expected non-zero exit, got 0")
	}
	found := false
	for _, issue := range out.Issues {
		if issue.Code == "plan-modify-target-missing" && issue.Severity == "error" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected plan-modify-target-missing error, got: %+v", out.Issues)
	}
}
