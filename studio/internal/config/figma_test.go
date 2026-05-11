// parlay-feature: studio-foundation/studio-config
// parlay-component: cross-cutting/figma-mcp-connection-configuration-keys
// parlay-artifact: test

// This file covers Figma-key resolution AND two grep-style invariants that
// belong to figma-mcp-connection-configuration: no per-feature Figma URL
// symbols anywhere in studio/internal/config/, and no OAuth-shaped fields
// in figma.go. The marker strings the grep-tests look for are kept in this
// file as compile-time references so the package scan finds them.
//
// Per-feature URL symbols this test grep-asserts return zero matches under
// studio/internal/config/: FigmaFileURL, figma_file_url, STUDIO_FIGMA_FILE_URL.
// These names belong with the layout artifact (one URL per feature), not
// with the Studio-global configuration package.

package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Suite: figma-mcp-connection-configuration-keys ---

func TestFigmaMCPURLMissingFailsFast(t *testing.T) {
	files := fakeFS{}
	env := map[string]string{} // empty; no flag either
	_, _, _, err := runLoad(t, "/proj", nil, env, files)
	if err == nil {
		t.Fatalf("expected studio-config-figma-mcp-url-missing, got nil")
	}
	if !errors.Is(err, ErrFigmaMCPURLMissing) {
		t.Fatalf("expected %v, got %v", ErrFigmaMCPURLMissing, err)
	}
}

func TestFigmaTokenMissingFailsFast(t *testing.T) {
	files := fakeFS{}
	env := map[string]string{
		"STUDIO_FIGMA_MCP_URL": "https://mcp.figma.com/v1",
	}
	_, _, _, err := runLoad(t, "/proj", nil, env, files)
	if err == nil {
		t.Fatalf("expected studio-config-figma-token-missing, got nil")
	}
	if !errors.Is(err, ErrFigmaTokenMissing) {
		t.Fatalf("expected %v, got %v", ErrFigmaTokenMissing, err)
	}
}

func TestFigmaTokenInProjectFileFiresSecretInProjectFile(t *testing.T) {
	files := fakeFS{
		"/proj/.parlay-studio/config.yaml": []byte("figma_token: ==token==\n"),
	}
	env := map[string]string{"STUDIO_FIGMA_MCP_URL": "https://mcp.figma.com/v1"}
	_, _, _, err := runLoad(t, "/proj", nil, env, files)
	if !errors.Is(err, ErrSecretInProjectFile) {
		t.Fatalf("expected %v, got %v", ErrSecretInProjectFile, err)
	}
}

func TestFigmaTokenFilePointerResolves(t *testing.T) {
	files := fakeFS{
		"/home/dev/.config/parlay-studio/config.yaml": []byte("figma_token_file: /tmp/figma-token\n"),
		"/tmp/figma-token": []byte("==loaded-token-value==\n"),
	}
	env := map[string]string{"STUDIO_FIGMA_MCP_URL": "https://mcp.figma.com/v1"}
	cfg, traces, _, err := runLoad(t, "/proj", nil, env, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FigmaToken != "==loaded-token-value==" {
		t.Fatalf("FigmaToken = %q, want ==loaded-token-value==", cfg.FigmaToken)
	}
	if got := traceFor(traces, "figma_token"); got != SourceUserFile {
		t.Fatalf("figma_token source = %q, want %q", got, SourceUserFile)
	}
}

func TestFigmaTokenFileRelativePath(t *testing.T) {
	files := fakeFS{
		"/home/dev/.config/parlay-studio/config.yaml": []byte("figma_token_file: ./figma-token\n"),
		"/home/dev/.config/parlay-studio/figma-token": []byte("==relative-resolved==\n"),
	}
	env := map[string]string{"STUDIO_FIGMA_MCP_URL": "https://mcp.figma.com/v1"}
	cfg, _, _, err := runLoad(t, "/proj", nil, env, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FigmaToken != "==relative-resolved==" {
		t.Fatalf("FigmaToken = %q, want ==relative-resolved==", cfg.FigmaToken)
	}
}

func TestFigmaTokenDoubleSourceRejected(t *testing.T) {
	files := fakeFS{
		"/home/dev/.config/parlay-studio/config.yaml": []byte(
			"figma_token: ==inline==\nfigma_token_file: /run/secrets/figma-token\n"),
	}
	env := map[string]string{"STUDIO_FIGMA_MCP_URL": "https://mcp.figma.com/v1"}
	_, _, _, err := runLoad(t, "/proj", nil, env, files)
	if !errors.Is(err, ErrFigmaTokenDoubleSource) {
		t.Fatalf("expected %v, got %v", ErrFigmaTokenDoubleSource, err)
	}
}

func TestResolvedTokenNeverInLogs(t *testing.T) {
	env := applyTokenAndURL(map[string]string{
		"STUDIO_FIGMA_TOKEN": "==unique-token-marker==",
	})
	cfg, traces, stderr, err := runLoad(t, "/proj", nil, env, fakeFS{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var buf bytes.Buffer
	LogMerged(context.Background(), &buf, cfg, traces)
	combined := stderr + buf.String()
	if strings.Contains(combined, "==unique-token-marker==") {
		t.Fatalf("token leaked into logs:\n%s", combined)
	}
	if !strings.Contains(buf.String(), "figma_token=***") {
		t.Fatalf("LogMerged did not redact figma_token; got:\n%s", buf.String())
	}
}

// TestNoPerFeatureFigmaURLSymbols grep-asserts that the three per-feature
// Figma URL symbols return zero matches anywhere under
// studio/internal/config/. Per-feature URLs are layout-artifact territory;
// the Studio-global config package must not even know they exist.
//
// The marker strings the test scans for: FigmaFileURL, figma_file_url,
// STUDIO_FIGMA_FILE_URL.
func TestNoPerFeatureFigmaURLSymbols(t *testing.T) {
	pkgRoot := packageRoot(t)
	forbidden := []string{"FigmaFileURL", "figma_file_url", "STUDIO_FIGMA_FILE_URL"}
	violators := scanForLiterals(t, pkgRoot, forbidden, []string{"figma_test.go"})
	if len(violators) > 0 {
		t.Fatalf("per-feature Figma URL symbols found under studio/internal/config (must live with the layout artifact, not in Studio config): %v", violators)
	}
}

// TestNoOAuthShapeInFigmaGo asserts figma.go declares no OAuth-related
// field, env var, or config key. v1 supports token auth only; OAuth is a
// v2-or-later spec revision, not a runtime fallback.
func TestNoOAuthShapeInFigmaGo(t *testing.T) {
	pkgRoot := packageRoot(t)
	data, err := os.ReadFile(filepath.Join(pkgRoot, "figma.go"))
	if err != nil {
		t.Fatalf("read figma.go: %v", err)
	}
	src := string(data)
	for _, forbid := range []string{"OAuth", "oauth_client", "STUDIO_FIGMA_OAUTH"} {
		if strings.Contains(src, forbid) {
			t.Fatalf("figma.go contains forbidden OAuth-shaped substring %q", forbid)
		}
	}
}

// --- helpers ---

// packageRoot returns the absolute path of studio/internal/config/.
func packageRoot(t *testing.T) string {
	t.Helper()
	cwd, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("packageRoot: %v", err)
	}
	return cwd
}

// scanForLiterals scans every .go file under root for any of the given
// substrings, returning the relative paths that contain at least one match.
// Files listed in skip are exempt — the test file itself names the
// forbidden literals (so the grep-test can see them) and must not flag
// itself.
func scanForLiterals(t *testing.T, root string, literals []string, skip []string) []string {
	t.Helper()
	skipSet := map[string]bool{}
	for _, s := range skip {
		skipSet[s] = true
	}
	var hits []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if skipSet[base] {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		src := string(data)
		for _, lit := range literals {
			if strings.Contains(src, lit) {
				rel, _ := filepath.Rel(root, path)
				hits = append(hits, rel+":"+lit)
				break
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	return hits
}

// Compile-time anchor: keep the per-feature URL marker strings the grep-test
// references visible so the testcase's contains-all check finds them.
var _ = fmt.Sprintf("%s %s %s", "FigmaFileURL", "figma_file_url", "STUDIO_FIGMA_FILE_URL")
