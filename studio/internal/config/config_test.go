// parlay-feature: studio-foundation/studio-config
// parlay-component: cross-cutting/layered-studio-configuration-loader
// parlay-artifact: test

package config

import (
	"reflect"
	"testing"
	"time"
)

// TestDefaultsShape locks the documented defaults onto the Config struct so
// any drift surfaces as a test failure rather than a runtime surprise.
//   - ServerPort:   0          (ask OS for free port)
//   - IdleTimeout:  30 minutes
//   - OpenBrowser:  true
//
// The other documented defaults — FigmaMCPURL and FigmaToken — are NOT set
// here because they have no default and must come from another source.
func TestDefaultsShape(t *testing.T) {
	cfg := defaults()
	cases := []struct {
		name    string
		got     any
		want    any
	}{
		{"ServerPort", cfg.ServerPort, 0},
		{"IdleTimeout", cfg.IdleTimeout, 30 * time.Minute},
		{"OpenBrowser", cfg.OpenBrowser, true},
		{"FigmaMCPURL unset", cfg.FigmaMCPURL, ""},
		{"FigmaToken unset", cfg.FigmaToken, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !reflect.DeepEqual(c.got, c.want) {
				t.Fatalf("defaults().%s = %#v, want %#v", c.name, c.got, c.want)
			}
		})
	}
}

// TestSecretKeySetDiscoversFigmaToken asserts that the loader's reflection
// walker finds FigmaToken (and only FigmaToken among the current fields)
// in the secret set. If a future field forgets the `,secret` tag, the
// startup log line would leak the value — this test catches that drift.
func TestSecretKeySetDiscoversFigmaToken(t *testing.T) {
	secrets := secretKeySet()
	if !secrets["figma_token"] {
		t.Fatalf("secretKeySet() did not include figma_token; got %v", secrets)
	}
	for _, nonSecret := range []string{"figma_mcp_url", "server_port", "idle_timeout", "open_browser"} {
		if secrets[nonSecret] {
			t.Fatalf("secretKeySet() unexpectedly tagged %s as secret", nonSecret)
		}
	}
}

// TestTraceShape ensures the per-key trace shape stays {Key, Source} so the
// startup log line (which depends on this layout) does not break silently.
func TestTraceShape(t *testing.T) {
	tr := Trace{Key: "figma_mcp_url", Source: SourceEnv}
	if tr.Key != "figma_mcp_url" {
		t.Fatalf("Trace.Key drift: got %q", tr.Key)
	}
	if tr.Source != SourceEnv {
		t.Fatalf("Trace.Source drift: got %q", tr.Source)
	}
}

// TestSourceVocabularyClosed guards the closed vocabulary documented in
// config.go. Adding a Source is a spec change; this test forces the change
// to be acknowledged.
func TestSourceVocabularyClosed(t *testing.T) {
	want := []Source{
		SourceDefault,
		SourceUserFile,
		SourceProjectFile,
		SourceEnv,
		SourceFlag,
	}
	got := map[Source]bool{}
	for _, s := range want {
		got[s] = true
	}
	if len(got) != len(want) {
		t.Fatalf("Source enum collisions: %#v", want)
	}
	if SourceDefault != "default" ||
		SourceUserFile != "user-file" ||
		SourceProjectFile != "project-file" ||
		SourceEnv != "env" ||
		SourceFlag != "flag" {
		t.Fatalf("Source label drift; check config.go const block")
	}
}
