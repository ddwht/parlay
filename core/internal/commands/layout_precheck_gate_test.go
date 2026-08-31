// parlay-feature: parlay-tool/page-layout-field
// parlay-cross-cutting-id: layout-precheck-contract
// parlay-artifact: test
//
// Tests the view-page/lock-page layout-validation precheck gate: a page
// with no layout artifact on disk proceeds through the legacy
// region-based flow unaffected; a page with a failing layout artifact
// refuses before assembling/locking, surfacing the precheck verdict.

package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// chdirForTest changes the working directory to dir for the duration of
// the test. view-page's fragment scan (parser.ScanAllSurfaces) reads
// config.SpecDir as a plain cwd-relative path, independent of the
// resolved *config.Context — a pre-existing property of that command,
// not something this gate changes.
func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
}

func newPageTestContext(t *testing.T) (*config.Context, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, config.SpecDir, config.PagesDir), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, config.SpecDir, "intents"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, config.ParlayDir, config.AdaptersDir), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Path: root, Kind: config.RootKindStandalone},
	}, nil)
	return cfg, root
}

func TestViewPage_NoLayoutArtifactProceedsUnaffected(t *testing.T) {
	cfg, root := newPageTestContext(t)
	chdirForTest(t, root)
	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runViewPage(cmd, []string{"dashboard"}); err != nil {
		t.Fatalf("expected the legacy flow to proceed with no layout artifact; got err=%v", err)
	}
	if !strings.Contains(out.String(), "No fragments target page") {
		t.Errorf("expected the legacy no-fragments message; got %q", out.String())
	}
}

func TestViewPage_FailingStandaloneLayoutRefusesWithPrecheckRefusal(t *testing.T) {
	cfg, root := newPageTestContext(t)
	// Missing schema_version — an adapter-independent violation.
	layoutBody := "componentVocabulary: clarity@17\nnodes:\n  - id: root\n    type: clarity.region\n"
	layoutPath := filepath.Join(root, config.SpecDir, config.PagesDir, "dashboard.layout.yaml")
	if err := os.WriteFile(layoutPath, []byte(layoutBody), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := testCommandWithContext(t, cfg)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)

	err := runViewPage(cmd, []string{"dashboard"})
	if err == nil {
		t.Fatal("expected the gate to refuse view-page for a failing layout artifact")
	}
	if !strings.Contains(errBuf.String(), "precheck-refusal") {
		t.Errorf("expected precheck-refusal in stderr; got %q", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "missing-schema-version") {
		t.Errorf("expected missing-schema-version in stderr; got %q", errBuf.String())
	}
	if strings.Contains(out.String(), "Assembled view") {
		t.Errorf("expected assembly to be refused, not attempted; got stdout %q", out.String())
	}
}

func TestViewPage_PassingStandaloneLayoutProceedsUnaffected(t *testing.T) {
	cfg, root := newPageTestContext(t)
	chdirForTest(t, root)
	layoutBody := "componentVocabulary: clarity@17\nschema_version: 1\nnodes:\n  - id: root\n    type: clarity.region\n"
	layoutPath := filepath.Join(root, config.SpecDir, config.PagesDir, "dashboard.layout.yaml")
	if err := os.WriteFile(layoutPath, []byte(layoutBody), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runViewPage(cmd, []string{"dashboard"}); err != nil {
		t.Fatalf("expected a well-formed layout to pass the gate; got err=%v", err)
	}
	if !strings.Contains(out.String(), "No fragments target page") {
		t.Errorf("expected the legacy flow to proceed past the gate; got %q", out.String())
	}
}

func TestLockPage_FailingStandaloneLayoutRefusesBeforeGeneratingManifest(t *testing.T) {
	cfg, root := newPageTestContext(t)
	// Wiring field in the layout — an adapter-independent violation.
	layoutBody := "componentVocabulary: clarity@17\nschema_version: 1\nnodes:\n  - id: root\n    type: clarity.region\n    dataSource: orders\n"
	layoutPath := filepath.Join(root, config.SpecDir, config.PagesDir, "dashboard.layout.yaml")
	if err := os.WriteFile(layoutPath, []byte(layoutBody), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := testCommandWithContext(t, cfg)
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)

	err := runLockPage(cmd, []string{"dashboard"})
	if err == nil {
		t.Fatal("expected the gate to refuse lock-page for a failing layout artifact")
	}
	if !strings.Contains(errBuf.String(), "precheck-refusal") {
		t.Errorf("expected precheck-refusal in stderr; got %q", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "wiring-in-layout") {
		t.Errorf("expected wiring-in-layout in stderr; got %q", errBuf.String())
	}
	manifestPath := filepath.Join(root, config.SpecDir, config.PagesDir, "dashboard.page.md")
	if _, statErr := os.Stat(manifestPath); statErr == nil {
		t.Errorf("expected no manifest to be written when the gate refuses")
	}
}
