// parlay-feature: studio-foundation/studio-config
// parlay-component: cross-cutting/layered-studio-configuration-loader
// parlay-extends: studio-foundation/figma-mcp-via-host-agent/cross-cutting/retract-studio-direct-mcp-source-tree

package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Stable error codes surfaced by Load. The string form of each sentinel is
// the code itself so callers can match either with errors.Is or by string
// equality against the stable code.

// ErrSecretInProjectFile (studio-config-secret-in-project-file) — a
// secret-tagged key appeared in the project-scoped config file. The
// project file is shared with collaborators; secrets belong in the
// user-scoped file or in the environment.
var ErrSecretInProjectFile = errors.New("studio-config-secret-in-project-file")

// envPrefix is the namespace for Studio-owned environment variables. The
// loader rejects nothing outside this prefix and warns (without failing) on
// recognised but unmapped PARLAY_EDITOR_* names.
// Renamed from STUDIO_ with the module merge. With one binary there is no
// separate product to namespace against, and a STUDIO_-prefixed variable
// configuring a `parlay` command is a puzzle rather than a convention.
const envPrefix = "PARLAY_EDITOR_"

// knownEnvVars is the closed set of STUDIO_* names this loader maps to a
// Config field. STUDIO_PROJECT_ROOT is consumed by ResolveProjectRoot, not by
// Load, but it lives on this list so unknown-env-var warnings don't fire
// against it.
//
// STUDIO_CONFIG_PATH is intentionally NOT on this list — the loader rejects
// the escape-hatch idea and surfaces a WARN instead.
var knownEnvVars = map[string]bool{
	"PARLAY_EDITOR_SERVER_PORT":  true,
	"PARLAY_EDITOR_IDLE_TIMEOUT": true,
	"PARLAY_EDITOR_OPEN_BROWSER": true,
	"PARLAY_ROOT":                true, // consumed by ResolveProjectRoot
	"XDG_CONFIG_HOME":            true, // consulted for user-file path
}

// fileSnapshot is the in-memory representation of one config file. Path is
// the absolute path the snapshot was loaded from; Raw is the decoded YAML
// (string-keyed for forward compatibility — unknown keys WARN instead of
// erroring). Present is false when the file was not on disk; loaders treat
// "not present" the same as "no keys present".
type fileSnapshot struct {
	Path    string
	Present bool
	Raw     map[string]any
	// Scoped reports whether Raw came from an `editor:` block rather than
	// from the file's top level. It decides whether unknown-key warnings are
	// meaningful: inside an editor: block every key is ours and an unrecognised
	// one is a typo worth flagging, but at the top level of a flat file the
	// keys belong to parlay itself and warning about them is noise about
	// keys the same binary wrote.
	Scoped bool
}

// LoadOptions controls non-default Load behavior. Callers in production pass
// the zero value; tests override the I/O surface.
type LoadOptions struct {
	// CWD is the current working directory used for resolving relative
	// override paths. Defaults to os.Getwd() when empty.
	CWD string
	// Home is the operator's home directory. Defaults to os.Getenv("HOME").
	Home string
	// Stderr captures the log output. Defaults to os.Stderr.
	Stderr io.Writer
	// ReadFile is the file reader. Defaults to os.ReadFile. Tests inject a
	// fake to avoid touching disk.
	ReadFile func(path string) ([]byte, error)
	// Stat reports whether a path exists. Defaults to os.Stat. Tests inject a
	// fake fs.
	Stat func(path string) (os.FileInfo, error)
}

// Load is the loader entry point. It merges the five sources in precedence
// order (CLI flags > STUDIO_* env > project file > user file > defaults) and
// returns the resolved *Config plus a per-key Trace slice describing which
// source supplied each value.
//
// Project-root resolution is the caller's responsibility (run
// ResolveProjectRoot first; pass the resolved root in via projectRoot). The
// loader does NOT walk up the filesystem.
//
// Errors returned from Load are stable codes (errors.Is against the
// package-level sentinels) so callers can render operator-facing messages
// without parsing free-text.
func Load(ctx context.Context, args []string, projectRoot string, env map[string]string, opts LoadOptions) (*Config, []Trace, error) {
	if env == nil {
		env = envSnapshot()
	}
	opts = applyDefaults(opts)
	logger := log.New(opts.Stderr, "", 0)

	// Snapshot the project- and user-scoped files. Both are optional; a
	// missing file produces an empty snapshot, not an error.
	projFile, err := loadProjectFile(projectRoot, opts)
	if err != nil {
		return nil, nil, err
	}
	userFile, err := loadUserFile(env, opts)
	if err != nil {
		return nil, nil, err
	}

	// Warn on unrecognised STUDIO_* env vars. STUDIO_CONFIG_PATH is the
	// canonical example: the loader does not honor it, but operators who set
	// it should see a clear signal.
	warnUnknownEnvVars(env, logger)

	// Warn on unknown keys in either YAML file.
	warnUnknownKeys(projFile, logger)
	warnUnknownKeys(userFile, logger)

	// Secret-in-project-file invariant: walk Config's struct tags for any
	// field marked secret, look up its snake_case key in the project file,
	// and fail fast if found. This MUST run before per-fragment loaders so
	// the project file's offending key is reported with its stable code
	// rather than turning into a more specific per-key error.
	if key, ok := projectFileHasSecret(projFile); ok {
		return nil, nil, fmt.Errorf("%w: secret key %q present in project-scoped file %s",
			ErrSecretInProjectFile, key, projFile.Path)
	}

	cfg := defaults()
	traces := make(map[string]Trace)

	// Per-fragment loaders. Each loader applies its keys to cfg, contributes
	// per-key Traces, and may return an error with a stable code.
	if err := loadWebServerKeys(cfg, traces, args, env, projFile, userFile); err != nil {
		return nil, nil, err
	}

	// Emit user-file-only WARN lines for web-server keys (project-scoped
	// per the spec, but the loader only knows the source per key after the
	// per-fragment loaders have run).
	warnUserFileOnlyWebServer(traces, logger)

	return cfg, traceSlice(traces), nil
}

// LogMerged writes one INFO-level log line per resolved key, redacting any
// value whose struct tag carries `,secret`. The line shape is:
//
//	INFO config: <key>=<value> (source: <source>)
//
// Tests assert on the line shape, so changes to it require updating the
// matching suites. Keys are emitted in alphabetical order so the output is
// deterministic across runs.
func LogMerged(ctx context.Context, w io.Writer, cfg *Config, traces []Trace) {
	if w == nil {
		w = os.Stderr
	}
	secretKeys := secretKeySet()
	// Index traces by key for O(1) lookup; defaults fill in for any key the
	// per-fragment loaders didn't trace.
	traceByKey := make(map[string]Source, len(traces))
	for _, t := range traces {
		traceByKey[t.Key] = t.Source
	}

	rows := mergedRows(cfg)
	sort.Slice(rows, func(i, j int) bool { return rows[i].key < rows[j].key })
	for _, row := range rows {
		val := row.value
		if secretKeys[row.key] {
			if val == "" {
				val = "" // unset secret stays empty
			} else {
				val = "***"
			}
		}
		src := traceByKey[row.key]
		if src == "" {
			src = SourceDefault
		}
		fmt.Fprintf(w, "INFO config: %s=%s (source: %s)\n", row.key, val, src)
	}
}

type mergedRow struct {
	key   string
	value string
}

func mergedRows(cfg *Config) []mergedRow {
	rv := reflect.ValueOf(*cfg)
	rt := rv.Type()
	var rows []mergedRow
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("studio")
		if tag == "" {
			continue
		}
		key := strings.Split(tag, ",")[0]
		rows = append(rows, mergedRow{
			key:   key,
			value: stringify(rv.Field(i)),
		})
	}
	return rows
}

func stringify(v reflect.Value) string {
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		if v.Bool() {
			return "true"
		}
		return "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Duration is int64 under the hood; check for it explicitly so we
		// format with the Go duration string instead of the nanosecond count.
		if v.Type().String() == "time.Duration" {
			return v.Interface().(interface{ String() string }).String()
		}
		return fmt.Sprintf("%d", v.Int())
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}

// secretKeySet returns the snake_case keys of every Config field whose
// studio:"..." tag includes the ",secret" attribute.
func secretKeySet() map[string]bool {
	out := make(map[string]bool)
	rt := reflect.TypeOf(Config{})
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("studio")
		if tag == "" {
			continue
		}
		parts := strings.Split(tag, ",")
		key := parts[0]
		for _, p := range parts[1:] {
			if p == "secret" {
				out[key] = true
			}
		}
	}
	return out
}

// projectFileHasSecret reports the first secret key (if any) present in the
// project-scoped file. Used to enforce the secret-in-project-file invariant
// before per-fragment loaders run.
//
// The post-retraction Config struct has no secret-tagged fields, but the
// invariant infrastructure is preserved so future secret fields can be added
// without re-authoring it.
func projectFileHasSecret(snap *fileSnapshot) (string, bool) {
	if !snap.Present {
		return "", false
	}
	secretKeys := secretKeySet()
	for k := range snap.Raw {
		if secretKeys[k] {
			return k, true
		}
	}
	return "", false
}

// warnUnknownKeys emits a WARN log line for every key in snap whose name is
// not recognised by any per-fragment loader. The known-keys list lives here
// (rather than being computed from struct tags) because we want to flag
// "looks like a config typo" — e.g. figma_team_url — even though no Config
// field accepts it.
func warnUnknownKeys(snap *fileSnapshot, logger *log.Logger) {
	if !snap.Present {
		return
	}
	known := knownConfigKeys()
	if !snap.Scoped {
		// A flat file's top level is shared with parlay itself, so parlay's
		// own keys are not unknown — they are simply not ours.
		//
		// Without this exemption a flat .parlay/config.yaml — exactly what
		// `parlay init` writes — produced a warning for every one of
		// ai-agent, sdd-framework and prototype-framework on every single
		// `parlay domain-edit` run. The same binary authored those keys and
		// then reported all three as unrecognised. That also defeats the
		// warning's actual purpose: three false positives every run train the
		// reader to skip past the line where a real typo would appear.
		//
		// A genuine typo at the top level still warns, because it is in
		// neither set. Inside an `editor:` block nothing is exempt — a parlay
		// key has no business there either.
		for k := range coreOwnedConfigKeys {
			known[k] = true
		}
	}
	for k := range snap.Raw {
		if !known[k] {
			logger.Printf("WARN config: unknown key `%s` in %s (ignored)", k, snap.Path)
		}
	}
}

// coreOwnedConfigKeys are parlay's own top-level keys in .parlay/config.yaml.
// The editor shares that file but does not own these; see warnUnknownKeys for
// why they must not be reported as unknown. Mirrors config.ProjectConfig in
// core/internal/config/config.go — kept as a literal rather than imported
// because internal/editor must not depend on Core's module (enforced by
// studio/internal/domain/no_core_import_test.go's sibling rule).
var coreOwnedConfigKeys = map[string]bool{
	"ai-agent":            true,
	"sdd-framework":       true,
	"prototype-framework": true,
	"parent":              true,
}

// knownConfigKeys is the union of every snake_case key any fragment loader
// reads. Add new keys here as new fragments land.
func knownConfigKeys() map[string]bool {
	out := map[string]bool{}
	for k := range secretKeySet() {
		out[k] = true
	}
	rt := reflect.TypeOf(Config{})
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("studio")
		if tag == "" {
			continue
		}
		out[strings.Split(tag, ",")[0]] = true
	}
	return out
}

func warnUnknownEnvVars(env map[string]string, logger *log.Logger) {
	for k := range env {
		if !strings.HasPrefix(k, envPrefix) {
			continue
		}
		if !knownEnvVars[k] {
			logger.Printf("WARN config: unknown env var `%s` (ignored)", k)
		}
	}
}

func warnUserFileOnlyWebServer(traces map[string]Trace, logger *log.Logger) {
	for _, key := range []string{"server_port", "idle_timeout", "open_browser"} {
		t, ok := traces[key]
		if !ok {
			continue
		}
		if t.Source == SourceUserFile {
			logger.Printf("WARN config: %s came from user-scoped file; we recommend setting it in the project-scoped config", key)
		}
	}
}

// applyDefaults fills LoadOptions zero-value fields with their production
// equivalents. Tests pass concrete values for every field; production callers
// pass the zero value.
func applyDefaults(opts LoadOptions) LoadOptions {
	if opts.CWD == "" {
		if cwd, err := os.Getwd(); err == nil {
			opts.CWD = cwd
		}
	}
	if opts.Home == "" {
		opts.Home = os.Getenv("HOME")
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.ReadFile == nil {
		opts.ReadFile = os.ReadFile
	}
	if opts.Stat == nil {
		opts.Stat = os.Stat
	}
	return opts
}

// envSnapshot returns a map of the current process environment. Only used
// when the caller passes a nil env (production path); tests always pass a
// fixture env.
func envSnapshot() map[string]string {
	env := os.Environ()
	out := make(map[string]string, len(env))
	for _, kv := range env {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		out[kv[:eq]] = kv[eq+1:]
	}
	return out
}

// projectConfigPath derives the project-scoped config file path from the
// resolved project root. The derivation is documented in both this file and
// project_root.go; this is the canonical location.
func projectConfigPath(projectRoot string) string {
	if projectRoot == "" {
		return ""
	}
	// Folded into parlay's own config file. A second dot-directory beside
	// .parlay/ at the same root was easy to mistake for the same thing, and
	// with one binary there is no second product to own one.
	return filepath.Join(projectRoot, ".parlay", "config.yaml")
}

// userConfigPath derives the user-scoped config file path, honoring
// XDG_CONFIG_HOME when set. The derivation lives here so the import-boundary
// test only has to look in one place.
func userConfigPath(env map[string]string, home string) string {
	if xdg := env["XDG_CONFIG_HOME"]; xdg != "" {
		return filepath.Join(xdg, "parlay", "config.yaml")
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "parlay", "config.yaml")
}

func loadProjectFile(projectRoot string, opts LoadOptions) (*fileSnapshot, error) {
	path := projectConfigPath(projectRoot)
	return loadYAMLFile(path, opts)
}

func loadUserFile(env map[string]string, opts LoadOptions) (*fileSnapshot, error) {
	path := userConfigPath(env, opts.Home)
	return loadYAMLFile(path, opts)
}

// loadYAMLFile reads path and returns a fileSnapshot. A missing file produces
// a snapshot with Present=false; any other error is propagated.
func loadYAMLFile(path string, opts LoadOptions) (*fileSnapshot, error) {
	snap := &fileSnapshot{Path: path}
	if path == "" {
		return snap, nil
	}
	data, err := opts.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return snap, nil
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	if len(data) == 0 {
		snap.Present = true
		snap.Raw = map[string]any{}
		return snap, nil
	}
	if err := yaml.Unmarshal(data, &snap.Raw); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if snap.Raw == nil {
		snap.Raw = map[string]any{}
	}
	// The editor's keys live under an `editor:` block now that this file is
	// parlay's own config rather than a private one — server_port at the top
	// level of .parlay/config.yaml would sit beside ai-agent and read as a
	// parlay-wide setting.
	//
	// A flat file still loads. That is not a compatibility shim so much as the
	// honest reading: if there is no editor: block, the keys that are present
	// are the ones meant.
	if nested, ok := snap.Raw["editor"].(map[string]any); ok {
		snap.Raw = nested
		snap.Scoped = true
	}
	snap.Present = true
	return snap, nil
}

// traceSlice flattens the per-key trace map into a deterministic slice (sorted
// by key) so callers don't have to sort it themselves.
func traceSlice(traces map[string]Trace) []Trace {
	out := make([]Trace, 0, len(traces))
	for _, t := range traces {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// flagValue is a tiny single-flag parser: scan args for "--<name>" or
// "--<name>=<val>". The loader does NOT use the standard library's flag
// package because flag.Parse mutates global state and refuses unknown flags,
// while the loader needs to coexist with other flag consumers (the
// project-root flag is parsed independently). Returns ("", false) when the
// flag is absent.
func flagValue(args []string, name string) (string, bool) {
	prefix := "--" + name
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == prefix && i+1 < len(args) {
			return args[i+1], true
		}
		if strings.HasPrefix(a, prefix+"=") {
			return a[len(prefix)+1:], true
		}
	}
	return "", false
}

// flagPresent is the boolean variant for flags whose presence flips the
// value, e.g. --no-browser.
func flagPresent(args []string, name string) bool {
	prefix := "--" + name
	for _, a := range args {
		if a == prefix {
			return true
		}
	}
	return false
}
