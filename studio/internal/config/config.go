// parlay-feature: studio-foundation/studio-config
// parlay-component: cross-cutting/layered-studio-configuration-loader
//
// Package config is the single supported home for reading Parlay Studio
// configuration. Every other Studio package reads the merged *Config returned
// by Load — no other package may call os.Getenv("STUDIO_*") or YAML-load the
// project- or user-scoped config files directly. An import-boundary test in
// loader_test.go walks studio/ and asserts the invariant.
//
// Field-ownership split:
//   - config.go declares the merged Config struct and the source-trace shape.
//   - figma.go    populates Config.FigmaMCPURL and Config.FigmaToken.
//   - web_server.go populates Config.ServerPort, IdleTimeout, OpenBrowser.
//   - project_root.go resolves the project root BEFORE the loader runs.
//   - loader.go    performs the five-source precedence merge and emits the
//                  redacted INFO log line via LogMerged.
//
// See studio/spec/intents/studio-foundation/studio-config.
package config

import "time"

// Config is the merged Studio configuration: the single typed shape every
// Studio caller reads. Fields are populated by the per-fragment loaders
// (figma.go, web_server.go) at Load time. Secret fields carry the
// `studio:"secret"` struct tag so LogMerged's redaction walker can find them
// without having to maintain a parallel allow-list.
type Config struct {
	// FigmaMCPURL is the remote Figma MCP endpoint. Project-scoped, non-secret.
	// See figma.go.
	FigmaMCPURL string `studio:"figma_mcp_url"`

	// FigmaToken is the Figma personal-access token. User-scoped, secret-tagged
	// so LogMerged redacts it as `***`. See figma.go.
	FigmaToken string `studio:"figma_token,secret"`

	// ServerPort is the bind port for Studio's HTTP server. 0 means "ask the OS
	// for a free port" — the bound port is logged by the web-server harness
	// after Listen(), not by this package. See web_server.go.
	ServerPort int `studio:"server_port"`

	// IdleTimeout is the duration after which an idle Studio process shuts
	// down. 0 disables the timeout entirely; negative values are rejected at
	// Load time. See web_server.go.
	IdleTimeout time.Duration `studio:"idle_timeout"`

	// OpenBrowser controls whether Studio opens the operator's default browser
	// to the bound URL after startup. Default true. See web_server.go.
	OpenBrowser bool `studio:"open_browser"`
}

// Source is a closed enum naming the layer that supplied a resolved value.
// Source labels are stable across releases — they appear in trace output, in
// the startup INFO log line, and (for project-root) in operator-visible
// errors.
type Source string

const (
	// SourceDefault — the built-in default supplied by this package.
	SourceDefault Source = "default"
	// SourceUserFile — the user-scoped config file
	// ($XDG_CONFIG_HOME/parlay-studio/config.yaml or ~/.config/...).
	SourceUserFile Source = "user-file"
	// SourceProjectFile — the project-scoped config file
	// (<resolved-project-root>/.parlay-studio/config.yaml).
	SourceProjectFile Source = "project-file"
	// SourceEnv — a STUDIO_* environment variable.
	SourceEnv Source = "env"
	// SourceFlag — a CLI flag.
	SourceFlag Source = "flag"
)

// Trace records the source of one resolved key. The loader returns one Trace
// per key in Config so callers (chiefly the startup log line) can attribute
// values back to their layer. Key is the snake_case config key (e.g.
// "figma_mcp_url"), not the Go field name.
type Trace struct {
	Key    string
	Source Source
}

// defaults returns a *Config populated with the package's built-in defaults.
// Every field that has a default value MUST be set here; the loader assumes
// that an unset value in defaults() means "no default, fail fast if no other
// source supplies the value".
func defaults() *Config {
	return &Config{
		// FigmaMCPURL — no default; fails fast with
		// studio-config-figma-mcp-url-missing if no source supplies it.
		// FigmaToken — no default; fails fast with
		// studio-config-figma-token-missing if no source supplies it.
		ServerPort:  0,                // 0 = ask OS for free port
		IdleTimeout: 30 * time.Minute, // documented default
		OpenBrowser: true,             // documented default
	}
}
