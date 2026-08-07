// parlay-feature: studio-foundation/studio-config
// parlay-component: cross-cutting/layered-studio-configuration-loader
// parlay-extends: studio-foundation/figma-mcp-via-host-agent/cross-cutting/retract-studio-direct-mcp-source-tree
//
// Package config is the single supported home for reading Parlay Studio
// configuration. Every other Studio package reads the merged *Config returned
// by Load — no other package may call os.Getenv("PARLAY_EDITOR_*") or YAML-load the
// project- or user-scoped config files directly. An import-boundary test in
// loader_test.go walks studio/ and asserts the invariant.
//
// Field-ownership split:
//   - config.go declares the merged Config struct and the source-trace shape.
//   - web_server.go populates Config.ServerPort, IdleTimeout, OpenBrowser.
//   - project_root.go resolves the project root BEFORE the loader runs.
//   - loader.go    performs the five-source precedence merge and emits the
//     redacted INFO log line via LogMerged.
//
// See studio/spec/intents/studio-foundation/studio-config.
package config

import "time"

// Config is the merged Studio configuration: the single typed shape every
// Studio caller reads. Fields are populated by the per-fragment loaders
// (web_server.go) at Load time. Secret fields carry the
// `studio:"secret"` struct tag so LogMerged's redaction walker can find them
// without having to maintain a parallel allow-list.
type Config struct {
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
	// ($XDG_CONFIG_HOME/parlay/config.yaml or ~/.config/parlay/config.yaml).
	SourceUserFile Source = "user-file"
	// SourceProjectFile — the project-scoped config file
	// (<resolved-project-root>/.parlay/config.yaml).
	SourceProjectFile Source = "project-file"
	// SourceEnv — a PARLAY_EDITOR_* environment variable.
	SourceEnv Source = "env"
	// SourceFlag — a CLI flag.
	SourceFlag Source = "flag"
)

// Trace records the source of one resolved key. The loader returns one Trace
// per key in Config so callers (chiefly the startup log line) can attribute
// values back to their layer. Key is the snake_case config key (e.g.
// "server_port"), not the Go field name.
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
		ServerPort:  0,                // 0 = ask OS for free port
		IdleTimeout: 30 * time.Minute, // documented default
		OpenBrowser: true,             // documented default
	}
}
