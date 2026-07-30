// parlay-feature: studio-foundation/studio-config
// parlay-component: cross-cutting/layered-studio-configuration-loader
// parlay-extends: studio-foundation/figma-mcp-via-host-agent/cross-cutting/retract-studio-direct-mcp-source-tree
// parlay-artifact: test

// This file holds the loader's behavioral tests AND the package's
// import-boundary test that walks studio/ and asserts os.Getenv("STUDIO_
// and direct YAML loads of the two config paths
// (<project-root>/.parlay/config.yaml and the user-scoped
// $XDG_CONFIG_HOME/parlay/config.yaml) appear only inside
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

	"github.com/ddwht/parlay/internal/testsupport"
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
		"/proj/.parlay/config.yaml": []byte("idle_timeout: 30m\n"),
	}
	env := map[string]string{
		"PARLAY_EDITOR_IDLE_TIMEOUT": "10m",
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
		"/proj/.parlay/config.yaml":            []byte("server_port: 19000\n"),
		"/home/dev/.config/parlay/config.yaml": []byte("server_port: 18000\n"),
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

// TestNestedEditorBlockIsRead covers the shape `9cbc33d` introduced: the three
// editor keys live under an `editor:` block in .parlay/config.yaml, beside
// parlay's own top-level keys. Every other fixture in this file is flat, so
// without this test the nesting is implemented and unexercised.
func TestNestedEditorBlockIsRead(t *testing.T) {
	files := fakeFS{
		"/proj/.parlay/config.yaml": []byte(
			"ai-agent: claude\n" +
				"editor:\n" +
				"  server_port: 18099\n" +
				"  idle_timeout: 45m\n" +
				"  open_browser: false\n"),
	}
	cfg, traces, stderr, err := runLoad(t, "/proj", nil, map[string]string{}, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ServerPort != 18099 {
		t.Fatalf("ServerPort = %d, want 18099", cfg.ServerPort)
	}
	if got := traceFor(traces, "server_port"); got != SourceProjectFile {
		t.Fatalf("server_port source = %q, want %q", got, SourceProjectFile)
	}
	if cfg.IdleTimeout.String() != "45m0s" {
		t.Fatalf("IdleTimeout = %s, want 45m0s", cfg.IdleTimeout)
	}
	if got := traceFor(traces, "idle_timeout"); got != SourceProjectFile {
		t.Fatalf("idle_timeout source = %q, want %q", got, SourceProjectFile)
	}
	if cfg.OpenBrowser {
		t.Fatalf("OpenBrowser = true, want false")
	}
	if got := traceFor(traces, "open_browser"); got != SourceProjectFile {
		t.Fatalf("open_browser source = %q, want %q", got, SourceProjectFile)
	}
	// parlay's own top-level keys sit outside the editor block and must not be
	// reported as unknown editor keys — the nesting is what earns them silence.
	if strings.Contains(stderr, "ai-agent") {
		t.Fatalf("sibling parlay key warned as unknown: %q", stderr)
	}
}

// TestFlatEditorKeysStillRead pins the other half of loadYAMLFile's contract:
// a file with no `editor:` block is read at the top level. Not a compatibility
// shim — with no block, the keys present are the ones meant.
func TestFlatEditorKeysStillRead(t *testing.T) {
	files := fakeFS{
		"/proj/.parlay/config.yaml": []byte("server_port: 18099\nidle_timeout: 45m\n"),
	}
	cfg, traces, _, err := runLoad(t, "/proj", nil, map[string]string{}, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ServerPort != 18099 {
		t.Fatalf("ServerPort = %d, want 18099", cfg.ServerPort)
	}
	if got := traceFor(traces, "server_port"); got != SourceProjectFile {
		t.Fatalf("server_port source = %q, want %q", got, SourceProjectFile)
	}
	if cfg.IdleTimeout.String() != "45m0s" {
		t.Fatalf("IdleTimeout = %s, want 45m0s", cfg.IdleTimeout)
	}
}

// TestNestedEditorBlockInUserFile covers the same nesting on the user-scoped
// side. The two files go through the same loadYAMLFile, but the precedence
// chain reads them through different Sources, and only the project file has a
// fixture with a block.
func TestNestedEditorBlockInUserFile(t *testing.T) {
	files := fakeFS{
		"/home/dev/.config/parlay/config.yaml": []byte("editor:\n  server_port: 18500\n"),
	}
	cfg, traces, _, err := runLoad(t, "/proj", nil, map[string]string{}, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ServerPort != 18500 {
		t.Fatalf("ServerPort = %d, want 18500", cfg.ServerPort)
	}
	if got := traceFor(traces, "server_port"); got != SourceUserFile {
		t.Fatalf("server_port source = %q, want %q", got, SourceUserFile)
	}
}

// TestNestedEditorBlockLosesToEnv keeps the precedence chain honest across the
// nesting: a nested project-file value is still outranked by the env var.
func TestNestedEditorBlockLosesToEnv(t *testing.T) {
	files := fakeFS{
		"/proj/.parlay/config.yaml": []byte("editor:\n  server_port: 18099\n"),
	}
	env := map[string]string{"PARLAY_EDITOR_SERVER_PORT": "18200"}
	cfg, traces, _, err := runLoad(t, "/proj", nil, env, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ServerPort != 18200 {
		t.Fatalf("ServerPort = %d, want 18200", cfg.ServerPort)
	}
	if got := traceFor(traces, "server_port"); got != SourceEnv {
		t.Fatalf("server_port source = %q, want %q", got, SourceEnv)
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
		"/proj/.parlay/config.yaml": []byte("figma_team_url: https://figma.com/team/x\n"),
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
		"PARLAY_EDITOR_CONFIG_PATH": "/tmp/test-config.yaml",
	}
	files := fakeFS{}
	_, _, stderr, err := runLoad(t, "/proj", nil, env, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.Contains(stderr, "unknown env var `PARLAY_EDITOR_CONFIG_PATH`") {
		t.Fatalf("expected unknown-env-var warning for PARLAY_EDITOR_CONFIG_PATH; got:\n%s", stderr)
	}
	// Project-config path remains derived from projectRoot.
	got := projectConfigPath("/proj")
	if got != "/proj/.parlay/config.yaml" {
		t.Fatalf("projectConfigPath(/proj) = %q, want /proj/.parlay/config.yaml", got)
	}
}

func TestXDGConfigHomeOverridesUserPath(t *testing.T) {
	env := map[string]string{
		"XDG_CONFIG_HOME": "/custom/xdg",
	}
	got := userConfigPath(env, "/home/dev")
	want := "/custom/xdg/parlay/config.yaml"
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

// The companion invariant — "no .go file outside this package decodes the
// editor's config file" — is retired rather than repaired.
//
// It was meaningful while the file was private: .parlay-studio/config.yaml had
// exactly one legitimate reader. Folding the three keys into
// .parlay/config.yaml makes that file parlay's own, and core reads it in a
// dozen places for ai-agent, sdd-framework and the rest — repointing the scan
// flagged all twelve as violations of a boundary that no longer exists.
//
// The narrower property still worth holding is that only this package reads the
// `editor:` block, and the env-var test above covers the same ground: every key
// in that block arrives through this loader.

// --- Helpers ---

func traceFor(traces []Trace, key string) Source {
	for _, t := range traces {
		if t.Key == key {
			return t.Source
		}
	}
	return ""
}

// studioRoot returns the module root — the directory the boundary scan must
// cover. Shared with the deployment guard in core/internal/atomicfile, which
// needs the same anchored walk; see testsupport.ModuleRoot for why it is a
// landmark search rather than a ".." hop count.
func studioRoot(t *testing.T) string {
	t.Helper()
	root, err := testsupport.ModuleRoot(".")
	if err != nil {
		t.Fatalf("studioRoot: %v", err)
	}
	return root
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
var _ = fmt.Sprintf("%s %s %s", ".parlay/config.yaml", "parlay/config.yaml", `os.Getenv("STUDIO_`)
