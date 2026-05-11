// parlay-feature: studio-foundation/studio-config
// parlay-component: cross-cutting/figma-mcp-connection-configuration-keys

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Stable error codes for Figma-key resolution. Each sentinel's string form is
// the stable code; the wrapped fmt.Errorf message adds operator-facing detail.

// ErrFigmaMCPURLMissing (studio-config-figma-mcp-url-missing) — no source
// supplied a Figma MCP URL. The wrapped message names the env var, the
// project-file key, and points at studio/docs/figma-mcp-setup.md.
var ErrFigmaMCPURLMissing = errors.New("studio-config-figma-mcp-url-missing")

// ErrFigmaTokenMissing (studio-config-figma-token-missing) — no source
// supplied a Figma personal-access token. The wrapped message names the env
// var, the inline user-file key, and the pointer user-file key.
var ErrFigmaTokenMissing = errors.New("studio-config-figma-token-missing")

// ErrFigmaTokenDoubleSource (studio-config-figma-token-double-source) — the
// user-scoped config file declares BOTH figma_token (inline) and
// figma_token_file (pointer). The two are mutually exclusive.
var ErrFigmaTokenDoubleSource = errors.New("studio-config-figma-token-double-source")

// loadFigmaKeys populates Config.FigmaMCPURL and Config.FigmaToken from the
// available sources, in precedence order. It returns one error (stable code)
// or contributes one Trace per resolved key. Source mapping is documented
// inline.
func loadFigmaKeys(cfg *Config, traces map[string]Trace, args []string, env map[string]string, projFile, userFile *fileSnapshot, opts LoadOptions) error {
	// --- FigmaMCPURL ---
	// flag > env > project-file. No user-file source (project-scoped). No
	// default; fails fast.
	if v, ok := flagValue(args, "figma-mcp-url"); ok && v != "" {
		cfg.FigmaMCPURL = v
		traces["figma_mcp_url"] = Trace{Key: "figma_mcp_url", Source: SourceFlag}
	} else if v, ok := env["STUDIO_FIGMA_MCP_URL"]; ok && v != "" {
		cfg.FigmaMCPURL = v
		traces["figma_mcp_url"] = Trace{Key: "figma_mcp_url", Source: SourceEnv}
	} else if v, ok := stringFromFile(projFile, "figma_mcp_url"); ok {
		cfg.FigmaMCPURL = v
		traces["figma_mcp_url"] = Trace{Key: "figma_mcp_url", Source: SourceProjectFile}
	} else {
		return fmt.Errorf("%w: set STUDIO_FIGMA_MCP_URL or figma_mcp_url in the project-scoped config. See studio/docs/figma-mcp-setup.md", ErrFigmaMCPURLMissing)
	}

	// --- FigmaToken ---
	// flag NOT exposed (secrets don't flow through flags). env > user-file
	// inline > user-file pointer. No default; fails fast.
	if v, ok := env["STUDIO_FIGMA_TOKEN"]; ok && v != "" {
		cfg.FigmaToken = v
		traces["figma_token"] = Trace{Key: "figma_token", Source: SourceEnv}
		return nil
	}

	tok, src, err := resolveTokenSource(userFile, opts)
	if err != nil {
		return err
	}
	if tok == "" {
		return fmt.Errorf("%w: set STUDIO_FIGMA_TOKEN, figma_token, or figma_token_file in the user-scoped config. See studio/docs/figma-mcp-setup.md", ErrFigmaTokenMissing)
	}
	cfg.FigmaToken = tok
	traces["figma_token"] = Trace{Key: "figma_token", Source: src}
	return nil
}

// resolveTokenSource picks the Figma token out of the user-scoped config
// file. It implements the inline + pointer + mutex rules:
//   - figma_token (inline) and figma_token_file (pointer) MUST NOT both
//     appear in the same file.
//   - figma_token_file: relative paths are resolved against the user-scoped
//     file's directory; absolute paths are honored verbatim.
//   - The pointed-to file is read once at startup; its contents are trimmed
//     of trailing whitespace (newline is the common case).
func resolveTokenSource(userFile *fileSnapshot, opts LoadOptions) (string, Source, error) {
	if userFile == nil || !userFile.Present {
		return "", "", nil
	}

	inline, hasInline := stringFromFile(userFile, "figma_token")
	pointer, hasPointer := stringFromFile(userFile, "figma_token_file")

	if hasInline && hasPointer {
		return "", "", fmt.Errorf("%w: user-scoped file %s declares both figma_token and figma_token_file", ErrFigmaTokenDoubleSource, userFile.Path)
	}

	if hasInline {
		return inline, SourceUserFile, nil
	}
	if hasPointer {
		// Resolve relative paths against the user-scoped file's directory.
		path := pointer
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(userFile.Path), path)
		}
		data, err := opts.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return "", "", fmt.Errorf("%w: figma_token_file %s does not exist", ErrFigmaTokenMissing, path)
			}
			return "", "", fmt.Errorf("config: read figma_token_file %s: %w", path, err)
		}
		return strings.TrimRight(string(data), " \t\n\r"), SourceUserFile, nil
	}

	return "", "", nil
}

// stringFromFile extracts a string-typed key from a fileSnapshot. YAML
// decodes scalars into one of several Go types (string, int, bool); this
// helper accepts string only. Other types are reported as absent (the
// loader's WARN-on-unknown path catches typos; per-key type validation lives
// in the per-fragment loader for ints/bools/durations).
func stringFromFile(snap *fileSnapshot, key string) (string, bool) {
	if snap == nil || !snap.Present {
		return "", false
	}
	v, ok := snap.Raw[key]
	if !ok {
		return "", false
	}
	if s, ok := v.(string); ok && s != "" {
		return s, true
	}
	return "", false
}
