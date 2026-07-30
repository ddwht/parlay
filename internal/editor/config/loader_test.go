// parlay-feature: studio-foundation/studio-config
// parlay-component: cross-cutting/layered-studio-configuration-loader
// parlay-extends: studio-foundation/figma-mcp-via-host-agent/cross-cutting/retract-studio-direct-mcp-source-tree
// parlay-artifact: test

// This file holds the loader's behavioral tests AND the package's
// import-boundary test that walks studio/ and asserts os.Getenv("STUDIO_
// and direct YAML loads of the two config paths
// (<project-root>/.parlay-studio/config.yaml and the user-scoped
// $XDG_CONFIG_HOME/parlay-studio/config.yaml) appear only inside
// internal/editor/config. The boundary is the load-bearing invariant of
// studio-config: keep all reads of STUDIO_* env vars and of either YAML
// config file confined to this package so downstream tooling cannot
// accidentally split the source-of-truth.

package config

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeFS captures every (path -> content) the test wants the loader to read.
// A missing key resolves to os.ErrNotExist.
type fakeFS map[string][]byte

func (f fakeFS) readFile(path string) ([]byte, error) {
	if data, ok := f[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

type fakeStat struct {
	exists bool
	isDir  bool
	name   string
}

func (s fakeStat) Name() string          { return s.name }
func (s fakeStat) Size() int64           { return 0 }
func (s fakeStat) Mode() os.FileMode     { return 0 }
func (s fakeStat) ModTime() (t fileTime) { return }
func (s fakeStat) IsDir() bool           { return s.isDir }
func (s fakeStat) Sys() any              { return nil }

type fileTime struct{}

func (fileTime) String() string { return "0" }

// runLoad is a small helper that wires fakeFS into LoadOptions and returns
// the merged Config, traces, captured stderr, and the error.
func runLoad(t *testing.T, projectRoot string, args []string, env map[string]string, files fakeFS) (*Config, []Trace, string, error) {
	t.Helper()
	var stderr bytes.Buffer
	cfg, traces, err := Load(context.Background(), args, projectRoot, env, LoadOptions{
		CWD:      "/tmp/cwd",
		Home:     "/home/dev",
		Stderr:   &stderr,
		ReadFile: files.readFile,
	})
	return cfg, traces, stderr.String(), err
}

// --- Suite: layered-studio-configuration-loader ---

func TestEnvBeatsProjectFile(t *testing.T) {
	files := fakeFS{
		"/proj/.parlay-studio/config.yaml": []byte("idle_timeout: 30m\n"),
	}
	env := map[string]string{
		"STUDIO_IDLE_TIMEOUT": "10m",
	}
	cfg, traces, _, err := runLoad(t, "/proj", nil, env, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.IdleTimeout.String() != "10m0s" {
		t.Fatalf("IdleTimeout = %s, want 10m0s", cfg.IdleTimeout)
	}
	if got := traceFor(traces, "idle_timeout"); got != SourceEnv {
		t.Fatalf("idle_timeout source = %q, want %q", got, SourceEnv)
	}
}

func TestProjectFileBeatsUserFile(t *testing.T) {
	files := fakeFS{
		"/proj/.parlay-studio/config.yaml":            []byte("server_port: 19000\n"),
		"/home/dev/.config/parlay-studio/config.yaml": []byte("server_port: 18000\n"),
	}
	env := map[string]string{} // no env overrides
	cfg, traces, _, err := runLoad(t, "/proj", nil, env, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ServerPort != 19000 {
		t.Fatalf("ServerPort = %d, want 19000", cfg.ServerPort)
	}
	if got := traceFor(traces, "server_port"); got != SourceProjectFile {
		t.Fatalf("server_port source = %q, want %q", got, SourceProjectFile)
	}
}

// TestSecretInProjectFileInvariantPreserved verifies the ErrSecretInProjectFile
// sentinel still exists and the invariant infrastructure is intact, even though
// the post-retraction Config struct currently has no secret-tagged fields.
// Future secret fields can be added without re-authoring the infrastructure.
func TestSecretInProjectFileInvariantPreserved(t *testing.T) {
	if ErrSecretInProjectFile == nil {
		t.Fatal("ErrSecretInProjectFile sentinel missing")
	}
	if !strings.Contains(ErrSecretInProjectFile.Error(), "studio-config-secret-in-project-file") {
		t.Fatalf("ErrSecretInProjectFile lost its stable code: %q", ErrSecretInProjectFile.Error())
	}
}

func TestLogMergedLabelsEverySource(t *testing.T) {
	cfg := &Config{
		ServerPort:  9000,
		OpenBrowser: true,
	}
	traces := []Trace{
		{Key: "server_port", Source: SourceProjectFile},
		{Key: "idle_timeout", Source: SourceEnv},
		{Key: "open_browser", Source: SourceUserFile},
		// any unmapped key falls through to "default"
	}
	var buf bytes.Buffer
	LogMerged(context.Background(), &buf, cfg, traces)
	out := buf.String()
	for _, want := range []string{
		"(source: env)",
		"(source: project-file)",
		"(source: user-file)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("LogMerged missing %q; got:\n%s", want, out)
		}
	}
}

// TestLogMergedNoSecretsToRedact confirms LogMerged emits readable lines for
// every remaining non-secret key on the post-retraction Config struct. There
// is no `***` in the output because no field is secret-tagged.
func TestLogMergedNoSecretsToRedact(t *testing.T) {
	cfg := &Config{
		ServerPort:  18080,
		OpenBrowser: true,
	}
	traces := []Trace{
		{Key: "server_port", Source: SourceProjectFile},
	}
	var buf bytes.Buffer
	LogMerged(context.Background(), &buf, cfg, traces)
	out := buf.String()
	if strings.Contains(out, "***") {
		t.Fatalf("post-retraction LogMerged should not redact any field; got:\n%s", out)
	}
	if !strings.Contains(out, "server_port=18080") {
		t.Fatalf("expected server_port verbatim in log; got:\n%s", out)
	}
}

func TestUnknownKeyInConfigFileEmitsWarn(t *testing.T) {
	files := fakeFS{
		"/proj/.parlay-studio/config.yaml": []byte("figma_team_url: https://figma.com/team/x\n"),
	}
	env := map[string]string{}
	_, _, stderr, err := runLoad(t, "/proj", nil, env, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.Contains(stderr, "unknown key `figma_team_url`") {
		t.Fatalf("WARN line missing; got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "WARN") {
		t.Fatalf("expected WARN level marker; got:\n%s", stderr)
	}
}

func TestStudioConfigPathEscapeHatchRejected(t *testing.T) {
	env := map[string]string{
		"STUDIO_CONFIG_PATH": "/tmp/test-config.yaml",
	}
	files := fakeFS{}
	_, _, stderr, err := runLoad(t, "/proj", nil, env, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.Contains(stderr, "unknown env var `STUDIO_CONFIG_PATH`") {
		t.Fatalf("expected unknown-env-var warning for STUDIO_CONFIG_PATH; got:\n%s", stderr)
	}
	// Project-config path remains derived from projectRoot.
	got := projectConfigPath("/proj")
	if got != "/proj/.parlay-studio/config.yaml" {
		t.Fatalf("projectConfigPath(/proj) = %q, want /proj/.parlay-studio/config.yaml", got)
	}
}

func TestXDGConfigHomeOverridesUserPath(t *testing.T) {
	env := map[string]string{
		"XDG_CONFIG_HOME": "/custom/xdg",
	}
	got := userConfigPath(env, "/home/dev")
	want := "/custom/xdg/parlay-studio/config.yaml"
	if got != want {
		t.Fatalf("userConfigPath = %q, want %q", got, want)
	}
}

// --- Import boundary tests ---
// TestImportBoundaryStudioEnvOnlyInConfig walks the module and fails if any
// .go file outside internal/editor/config reads STUDIO_* env vars directly
// via os.Getenv("STUDIO_...") or os.LookupEnv("STUDIO_...").
// The "STUDIO_ string literal must appear in this file because the test
// scans for it as a substring of source text.
func TestImportBoundaryStudioEnvOnlyInConfig(t *testing.T) {
	root := studioRoot(t)
	violators := scanFiles(t, root, func(path string, src string) bool {
		// Skip our own package. The path moved with the module merge:
		// studio/internal/config -> internal/editor/config, and the old
		// substring no longer matches, so the package started reporting
		// itself.
		if strings.Contains(path, "internal/editor/config") {
			return false
		}
		// Skip test files for the boundary test itself.
		if strings.Contains(path, "_test.go") {
			// Test files in other packages MAY legitimately set
			// STUDIO_* env vars to drive integration suites. The
			// non-test source is the constraint.
			return false
		}
		// Look for os.Getenv("STUDIO_ or os.LookupEnv("STUDIO_.
		return strings.Contains(src, `os.Getenv("STUDIO_`) ||
			strings.Contains(src, `os.LookupEnv("STUDIO_`)
	})
	if len(violators) > 0 {
		t.Fatalf("studio-config import-boundary violation: STUDIO_ env reads outside internal/editor/config: %v", violators)
	}
}

// TestImportBoundaryConfigYAMLLoadsOnlyInConfig walks studio/ and fails if
// any .go file outside internal/editor/config decodes either of the two
// Studio config files directly. The substrings .parlay-studio/config.yaml
// (the project-scoped path tail) and parlay-studio/config.yaml (the
// user-scoped path tail) are the literal markers the scan looks for.
func TestImportBoundaryConfigYAMLLoadsOnlyInConfig(t *testing.T) {
	root := studioRoot(t)
	violators := scanFiles(t, root, func(path string, src string) bool {
		if strings.Contains(path, "internal/editor/config") {
			return false
		}
		// Skip test files (see above).
		if strings.HasSuffix(path, "_test.go") {
			return false
		}
		return strings.Contains(src, ".parlay-studio/config.yaml") ||
			strings.Contains(src, "parlay-studio/config.yaml")
	})
	if len(violators) > 0 {
		t.Fatalf("studio-config import-boundary violation: direct config-file reads outside internal/editor/config: %v", violators)
	}
}

// --- Helpers ---

func traceFor(traces []Trace, key string) Source {
	for _, t := range traces {
		if t.Key == key {
			return t.Source
		}
	}
	return ""
}

// studioRoot walks up from this test's location to the studio module root.
// The test file lives at internal/editor/config/loader_test.go so three .. land
// at studio/.
func studioRoot(t *testing.T) string {
	t.Helper()
	cwd, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("studioRoot: %v", err)
	}
	// Three levels up: internal/editor/config -> repo root. Was two, which
	// pointed at studio/ before the module merge; post-merge two levels reaches
	// only internal/, so the invariant would have stopped covering core/ —
	// silently narrowing rather than failing.
	return filepath.Clean(filepath.Join(cwd, "..", "..", ".."))
}

// packageRoot returns the absolute path of internal/editor/config/.
// (Tests in the same package may consult this to read sibling source files
// for grep-style invariants.)
func packageRoot(t *testing.T) string {
	t.Helper()
	cwd, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("packageRoot: %v", err)
	}
	return cwd
}

// scanFiles walks root, reading every .go file, and reports relative paths
// for which pred returns true. Build artefacts and embedded directories are
// skipped.
func scanFiles(t *testing.T, root string, pred func(path, src string) bool) []string {
	t.Helper()
	var violators []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			switch name {
			case "vendor", "node_modules", ".parlay", "dist", "build", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if pred(rel, string(data)) {
			violators = append(violators, rel)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	return violators
}

// Compile-time guard: keep the helper string literals the boundary tests
// look for visible in this file so the scan-and-grep verification in
// testcases.yaml finds them.
var _ = fmt.Sprintf("%s %s %s", ".parlay-studio/config.yaml", "parlay-studio/config.yaml", `os.Getenv("STUDIO_`)
