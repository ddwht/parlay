// parlay-feature: studio-foundation/studio-config
// parlay-component: cross-cutting/web-server-runtime-configuration-keys

package config

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Stable error codes for web-server-key resolution.

// ErrServerPortInvalid (studio-config-server-port-invalid) — ServerPort
// resolved to a value outside [0, 65535]. The bind-time conflict error
// (studio-config-server-port-conflict) is the harness's responsibility;
// this package validates the value's shape only.
var ErrServerPortInvalid = errors.New("studio-config-server-port-invalid")

// ErrIdleTimeoutInvalid (studio-config-idle-timeout-invalid) — IdleTimeout
// resolved to a negative duration. Zero is the explicit "disabled" sentinel
// and is allowed.
var ErrIdleTimeoutInvalid = errors.New("studio-config-idle-timeout-invalid")

// loadWebServerKeys populates Config.ServerPort, Config.IdleTimeout, and
// Config.OpenBrowser. Each key has a flag / env / project-file / default
// chain. Validation runs after merge so a negative env-supplied duration
// fails fast with its stable code instead of silently shifting to a default.
func loadWebServerKeys(cfg *Config, traces map[string]Trace, args []string, env map[string]string, projFile, userFile *fileSnapshot) error {
	// --- ServerPort ---
	port, portSrc, ok, err := resolveServerPort(args, env, projFile, userFile)
	if err != nil {
		return err
	}
	if ok {
		cfg.ServerPort = port
		traces["server_port"] = Trace{Key: "server_port", Source: portSrc}
	} else {
		// Default already set in defaults(); record the trace.
		traces["server_port"] = Trace{Key: "server_port", Source: SourceDefault}
	}
	if cfg.ServerPort < 0 || cfg.ServerPort > 65535 {
		return fmt.Errorf("%w: server_port=%d out of range [0, 65535]", ErrServerPortInvalid, cfg.ServerPort)
	}

	// --- IdleTimeout ---
	timeout, timeoutSrc, ok, err := resolveIdleTimeout(args, env, projFile, userFile)
	if err != nil {
		return err
	}
	if ok {
		cfg.IdleTimeout = timeout
		traces["idle_timeout"] = Trace{Key: "idle_timeout", Source: timeoutSrc}
	} else {
		traces["idle_timeout"] = Trace{Key: "idle_timeout", Source: SourceDefault}
	}
	if cfg.IdleTimeout < 0 {
		return fmt.Errorf("%w: idle_timeout=%s is negative (use 0 to disable)", ErrIdleTimeoutInvalid, cfg.IdleTimeout)
	}

	// --- OpenBrowser ---
	browse, browseSrc, ok := resolveOpenBrowser(args, env, projFile, userFile)
	if ok {
		cfg.OpenBrowser = browse
		traces["open_browser"] = Trace{Key: "open_browser", Source: browseSrc}
	} else {
		traces["open_browser"] = Trace{Key: "open_browser", Source: SourceDefault}
	}
	return nil
}

func resolveServerPort(args []string, env map[string]string, projFile, userFile *fileSnapshot) (int, Source, bool, error) {
	if v, ok := flagValue(args, "server-port"); ok && v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return 0, "", false, fmt.Errorf("%w: --server-port %q is not an integer", ErrServerPortInvalid, v)
		}
		return port, SourceFlag, true, nil
	}
	if v, ok := env["PARLAY_EDITOR_SERVER_PORT"]; ok && v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return 0, "", false, fmt.Errorf("%w: PARLAY_EDITOR_SERVER_PORT=%q is not an integer", ErrServerPortInvalid, v)
		}
		return port, SourceEnv, true, nil
	}
	if v, src, ok := intFromFile(projFile, userFile, "server_port"); ok {
		return v, src, true, nil
	}
	return 0, "", false, nil
}

func resolveIdleTimeout(args []string, env map[string]string, projFile, userFile *fileSnapshot) (time.Duration, Source, bool, error) {
	if v, ok := flagValue(args, "idle-timeout"); ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return 0, "", false, fmt.Errorf("%w: --idle-timeout %q is not a valid Go duration", ErrIdleTimeoutInvalid, v)
		}
		return d, SourceFlag, true, nil
	}
	if v, ok := env["PARLAY_EDITOR_IDLE_TIMEOUT"]; ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return 0, "", false, fmt.Errorf("%w: PARLAY_EDITOR_IDLE_TIMEOUT=%q is not a valid Go duration", ErrIdleTimeoutInvalid, v)
		}
		return d, SourceEnv, true, nil
	}
	if v, src, ok := durationFromFile(projFile, userFile, "idle_timeout"); ok {
		return v, src, true, nil
	}
	return 0, "", false, nil
}

func resolveOpenBrowser(args []string, env map[string]string, projFile, userFile *fileSnapshot) (bool, Source, bool) {
	// --no-browser and --browser are the two flag forms.
	if flagPresent(args, "no-browser") {
		return false, SourceFlag, true
	}
	if flagPresent(args, "browser") {
		return true, SourceFlag, true
	}
	if v, ok := env["PARLAY_EDITOR_OPEN_BROWSER"]; ok && v != "" {
		b, parsed := parseBool(v)
		if parsed {
			return b, SourceEnv, true
		}
		// An unparseable boolean falls through to lower-precedence sources
		// rather than erroring — the env var is a convenience knob and the
		// spec doesn't define a stable code for this case. Production usage
		// passes strict values.
	}
	if v, src, ok := boolFromFile(projFile, userFile, "open_browser"); ok {
		return v, src, true
	}
	return false, "", false
}

// intFromFile reads a key from the project-scoped file first (it outranks
// user-scope for web-server keys), falling back to the user-scoped file. The
// caller distinguishes the two via the returned Source.
func intFromFile(projFile, userFile *fileSnapshot, key string) (int, Source, bool) {
	if v, ok := readInt(projFile, key); ok {
		return v, SourceProjectFile, true
	}
	if v, ok := readInt(userFile, key); ok {
		return v, SourceUserFile, true
	}
	return 0, "", false
}

func durationFromFile(projFile, userFile *fileSnapshot, key string) (time.Duration, Source, bool) {
	if v, ok := readDuration(projFile, key); ok {
		return v, SourceProjectFile, true
	}
	if v, ok := readDuration(userFile, key); ok {
		return v, SourceUserFile, true
	}
	return 0, "", false
}

func boolFromFile(projFile, userFile *fileSnapshot, key string) (bool, Source, bool) {
	if v, ok := readBool(projFile, key); ok {
		return v, SourceProjectFile, true
	}
	if v, ok := readBool(userFile, key); ok {
		return v, SourceUserFile, true
	}
	return false, "", false
}

func readInt(snap *fileSnapshot, key string) (int, bool) {
	if snap == nil || !snap.Present {
		return 0, false
	}
	v, ok := snap.Raw[key]
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	case string:
		n, err := strconv.Atoi(t)
		if err == nil {
			return n, true
		}
	}
	return 0, false
}

func readDuration(snap *fileSnapshot, key string) (time.Duration, bool) {
	if snap == nil || !snap.Present {
		return 0, false
	}
	v, ok := snap.Raw[key]
	if !ok {
		return 0, false
	}
	if s, ok := v.(string); ok {
		if d, err := time.ParseDuration(s); err == nil {
			return d, true
		}
	}
	return 0, false
}

func readBool(snap *fileSnapshot, key string) (bool, bool) {
	if snap == nil || !snap.Present {
		return false, false
	}
	v, ok := snap.Raw[key]
	if !ok {
		return false, false
	}
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		if b, ok := parseBool(t); ok {
			return b, true
		}
	}
	return false, false
}

// parseBool accepts the documented set: true/false/1/0 (case-insensitive).
// strconv.ParseBool is too permissive ("t", "f", "T", "F", "TRUE"...) and
// the spec pins the surface to the four forms above.
func parseBool(s string) (bool, bool) {
	switch s {
	case "true", "True", "TRUE", "1":
		return true, true
	case "false", "False", "FALSE", "0":
		return false, true
	}
	return false, false
}
