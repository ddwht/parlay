// parlay-feature: studio-foundation/studio-config
// parlay-component: cross-cutting/web-server-runtime-configuration-keys
// parlay-artifact: test

// This file covers web-server-key resolution AND the grep-style invariant
// that web_server.go does not emit the bound-port log line — that
// responsibility belongs to the web-server-harness, which knows the
// resolved port after Listen() returns. The config package only validates
// the value's shape.

package config

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// --- Suite: web-server-runtime-configuration-keys ---

func TestDefaultsPortTimeoutBrowser(t *testing.T) {
	files := fakeFS{}
	env := map[string]string{}
	cfg, _, _, err := runLoad(t, "/proj", nil, env, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ServerPort != 0 {
		t.Fatalf("ServerPort = %d, want 0", cfg.ServerPort)
	}
	if cfg.IdleTimeout != 30*time.Minute {
		t.Fatalf("IdleTimeout = %s, want 30m", cfg.IdleTimeout)
	}
	if !cfg.OpenBrowser {
		t.Fatalf("OpenBrowser = false, want true")
	}
}

func TestStudioServerPortParsesAsInt(t *testing.T) {
	files := fakeFS{}
	env := map[string]string{"STUDIO_SERVER_PORT": "18080"}
	cfg, _, _, err := runLoad(t, "/proj", nil, env, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ServerPort != 18080 {
		t.Fatalf("ServerPort = %d, want 18080", cfg.ServerPort)
	}
}

func TestServerPortOutOfRangeRejected(t *testing.T) {
	files := fakeFS{}
	env := map[string]string{"STUDIO_SERVER_PORT": "70000"}
	_, _, _, err := runLoad(t, "/proj", nil, env, files)
	if !errors.Is(err, ErrServerPortInvalid) {
		t.Fatalf("expected %v, got %v", ErrServerPortInvalid, err)
	}
}

func TestNegativeIdleTimeoutRejected(t *testing.T) {
	files := fakeFS{}
	env := map[string]string{"STUDIO_IDLE_TIMEOUT": "-1s"}
	_, _, _, err := runLoad(t, "/proj", nil, env, files)
	if !errors.Is(err, ErrIdleTimeoutInvalid) {
		t.Fatalf("expected %v, got %v", ErrIdleTimeoutInvalid, err)
	}
}

func TestZeroIdleTimeoutDisables(t *testing.T) {
	files := fakeFS{}
	env := map[string]string{"STUDIO_IDLE_TIMEOUT": "0"}
	cfg, _, _, err := runLoad(t, "/proj", nil, env, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.IdleTimeout != 0 {
		t.Fatalf("IdleTimeout = %s, want 0", cfg.IdleTimeout)
	}
}

func TestStudioOpenBrowserFalseDisables(t *testing.T) {
	files := fakeFS{}
	env := map[string]string{"STUDIO_OPEN_BROWSER": "false"}
	cfg, _, _, err := runLoad(t, "/proj", nil, env, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OpenBrowser {
		t.Fatalf("OpenBrowser = true, want false")
	}
}

func TestUserFileOnlyWebServerKeyEmitsWarn(t *testing.T) {
	files := fakeFS{
		"/home/dev/.config/parlay-studio/config.yaml": []byte("server_port: 18080\n"),
	}
	env := map[string]string{}
	cfg, _, stderr, err := runLoad(t, "/proj", nil, env, files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ServerPort != 18080 {
		t.Fatalf("ServerPort = %d, want 18080", cfg.ServerPort)
	}
	// WARN.*server_port.*user-scoped.*recommend
	re := regexp.MustCompile(`WARN.*server_port.*user-scoped.*recommend`)
	if !re.MatchString(stderr) {
		t.Fatalf("expected WARN line matching %v; got:\n%s", re, stderr)
	}
}

// TestNoBoundPortLogInConfig grep-asserts web_server.go does not contain
// the bound-port log shape ("bound to 127.0.0.1:..."). The web-server
// harness owns that log line because it knows the actual bound port after
// Listen() returns; this package would only know the configured value.
func TestNoBoundPortLogInConfig(t *testing.T) {
	pkgRoot := packageRoot(t)
	data, err := os.ReadFile(filepath.Join(pkgRoot, "web_server.go"))
	if err != nil {
		t.Fatalf("read web_server.go: %v", err)
	}
	src := string(data)
	re := regexp.MustCompile(`bound to.*127\.0\.0\.1`)
	if re.MatchString(src) {
		t.Fatalf("web_server.go must not log the bound port (harness owns that log line); matched %v", re)
	}
	// Sanity: the forbidden pattern is also not lurking as a partial match
	// across line boundaries.
	if strings.Contains(src, "bound to 127.0.0.1") {
		t.Fatalf("web_server.go literal 'bound to 127.0.0.1' found")
	}
}
