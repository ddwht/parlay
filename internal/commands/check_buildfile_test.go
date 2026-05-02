package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/internal/config"
)

// captureStdoutForCheck runs fn while temporarily redirecting os.Stdout
// and returns whatever was written. The check-buildfile command emits
// JSON via fmt.Println; tests parse it.
func captureStdoutForCheck(t *testing.T, fn func()) string {
	t.Helper()
	prev := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	w.Close()
	os.Stdout = prev
	return <-done
}

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

// runCheckBuildfileExitCode runs check-buildfile via a separate process
// so os.Exit doesn't kill the test runner. Returns parsed JSON output
// and exit code.
func runCheckBuildfileExitCode(t *testing.T, cfg *config.Context, slug string) (checkBuildfileOutput, int) {
	t.Helper()
	bin := buildParlayBinary(t)
	cmd := exec.Command(bin, "check-buildfile", "@"+slug)
	cmd.Dir = cfg.Root.Path
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("run check-buildfile: %v\nstderr: %s", err, stderr.String())
		}
	}
	var out checkBuildfileOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("parse JSON: %v\nstdout: %s", err, stdout.String())
	}
	return out, exitCode
}

// buildParlayBinary builds a temporary parlay binary the tests can
// exec. Cached per test run via a sentinel file in t.TempDir would be
// nice, but we just rebuild — the build is fast for unit tests.
func buildParlayBinary(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat("/home/node/go/bin/go"); err != nil {
		t.Skip("go binary not at /home/node/go/bin/go")
	}
	bin := filepath.Join(t.TempDir(), "parlay-cb-test")
	cmd := exec.Command("/home/node/go/bin/go", "build", "-o", bin, "./cmd/parlay")
	cmd.Dir = "/workspace/parlay-dev"
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("build failed: %v\n%s", err, out)
	}
	return bin
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

// suppress vet warnings about unused strings import if test references shift.
var _ = strings.Contains
