package commands

// Generated from buildfile component: adapter-registration
// Type: command-output | Widget: cobra-command | Layout: file-generator
//
// parlay-feature: parlay-tool/project-setup
// parlay-component: adapter-registration
// parlay-extends: parlay-tool/adapter-vocabulary-extension/adapter-parser-vocabulary-and-tokens

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var registerAdapterCmd = &cobra.Command{
	Use:   "register-adapter <path>",
	Short: "Register a framework adapter",
	Args:  cobra.ExactArgs(1),
	RunE:  runRegisterAdapter,
}

// adapterFile is the YAML-mapped representation of an adapter file. The
// componentVocabulary and tokens sections are optional — adapters that
// omit them continue to register cleanly.
type adapterFile struct {
	Name                string                      `yaml:"name"`
	Framework           string                      `yaml:"framework"`
	Version             string                      `yaml:"version"`
	Shows               map[string]interface{}      `yaml:"shows"`
	Actions             map[string]interface{}      `yaml:"actions"`
	Flows               map[string]interface{}      `yaml:"flows"`
	FileConventions     map[string]interface{}      `yaml:"file-conventions"`
	MountStrategies     map[string]interface{}      `yaml:"mount-strategies,omitempty"`
	ComponentVocabulary *adapterComponentVocabulary `yaml:"componentVocabulary,omitempty"`
	Tokens              *adapterTokens              `yaml:"tokens,omitempty"`
}

// adapterComponentVocabulary captures the versioned component vocabulary an
// adapter exposes to layouts. See internal/embedded/schemas/adapter.schema.md
// Section 8 for the full contract.
type adapterComponentVocabulary struct {
	Name       string                       `yaml:"name"`
	Components []adapterVocabularyComponent `yaml:"components"`
}

type adapterVocabularyComponent struct {
	Type            string                      `yaml:"type"`
	Category        string                      `yaml:"category"`
	Variants        []string                    `yaml:"variants,omitempty"`
	Properties      []adapterVocabularyProperty `yaml:"properties,omitempty"`
	AllowedChildren []string                    `yaml:"allowed-children,omitempty"`
}

type adapterVocabularyProperty struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"`
	EnumValues []string `yaml:"enum-values,omitempty"`
	ChildTypes []string `yaml:"child-types,omitempty"`
	Required   bool     `yaml:"required"`
}

// adapterTokens captures the design-token block an adapter declares. See
// internal/embedded/schemas/adapter.schema.md Section 9.
type adapterTokens struct {
	Modes      []string                 `yaml:"modes"`
	Spacing    []adapterSpacingToken    `yaml:"spacing,omitempty"`
	Color      []adapterColorToken      `yaml:"color,omitempty"`
	Typography []adapterTypographyToken `yaml:"typography,omitempty"`
}

type adapterSpacingToken struct {
	Name     string `yaml:"name"`
	Order    int    `yaml:"order"`
	EmitForm string `yaml:"emit-form"`
}

type adapterColorToken struct {
	Name      string   `yaml:"name"`
	Tone      string   `yaml:"tone,omitempty"`
	EmitForms []string `yaml:"emit-forms"`
}

type adapterTypographyToken struct {
	Name     string `yaml:"name"`
	UseSite  string `yaml:"use-site"`
	EmitForm string `yaml:"emit-form"`
}

// adapterCache is a per-process cache of parsed adapter files keyed by
// absolute path. It exists so that subsequent commands within a single CLI
// invocation reuse the parse result rather than re-reading and re-parsing
// the YAML each time. ParseCount tracks how many distinct parse operations
// occurred; lookups against an already-cached path do NOT increment it.
type adapterCacheEntry struct {
	Adapter  *adapterFile
	ParsedAt string
}

var (
	adapterCacheMu         sync.RWMutex
	adapterCache           = map[string]*adapterCacheEntry{}
	adapterCacheParseCount int
)

// parseAdapterFileCached reads, parses, and validates an adapter file from
// disk, caching the result keyed by absolute path. Subsequent calls for the
// same path within the same process return the cached entry without
// re-reading the file.
func parseAdapterFileCached(path string) (*adapterFile, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve adapter path: %w", err)
	}
	adapterCacheMu.RLock()
	if entry, ok := adapterCache[abs]; ok {
		adapterCacheMu.RUnlock()
		return entry.Adapter, nil
	}
	adapterCacheMu.RUnlock()

	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read adapter file: %w", err)
	}
	var adapter adapterFile
	if err := yaml.Unmarshal(data, &adapter); err != nil {
		return nil, fmt.Errorf("parse adapter: %w", err)
	}
	if err := validateAdapterDeclarations(abs, data); err != nil {
		return nil, err
	}

	adapterCacheMu.Lock()
	adapterCache[abs] = &adapterCacheEntry{Adapter: &adapter}
	adapterCacheParseCount++
	adapterCacheMu.Unlock()
	return &adapter, nil
}

// adapterCacheParseCountForTest exposes the per-process parse counter for
// tests that verify caching behavior. The exported helper avoids leaking the
// raw mutex to test files.
func adapterCacheParseCountForTest() int {
	adapterCacheMu.RLock()
	defer adapterCacheMu.RUnlock()
	return adapterCacheParseCount
}

// adapterCacheResetForTest clears the per-process cache and resets the parse
// counter. Intended for use only in tests.
func adapterCacheResetForTest() {
	adapterCacheMu.Lock()
	defer adapterCacheMu.Unlock()
	adapterCache = map[string]*adapterCacheEntry{}
	adapterCacheParseCount = 0
}

// validateAdapterDeclarations runs the complete adapter validation
// (agent.ValidateAdapter — every section of adapter.schema.md, kind-conditional)
// at registration time.
//
// It used to check only `name` plus the optional componentVocabulary and tokens
// blocks, and returned on the FIRST error. Since no bundled adapter declares
// either optional block, registration effectively validated nothing but a
// non-empty name — a backend adapter with a malformed `supports:` or a
// presentation adapter missing half its widget vocabulary registered cleanly.
// Reporting every finding at once matters here: an author (human or agent)
// otherwise needs one round-trip per defect.
func validateAdapterDeclarations(path string, content []byte) error {
	var msgs []string
	for _, o := range agent.ValidateAdapter(agent.ModeBuild, path, content) {
		if o.Severity == agent.SeverityError {
			msgs = append(msgs, fmt.Sprintf("  [%s] %s", o.Code, o.Message))
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return fmt.Errorf("adapter failed validation:\n%s", strings.Join(msgs, "\n"))
}

func runRegisterAdapter(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	// Data input: adapter-path from command-argument
	adapterPath := args[0]

	// Operation: read-file, parse using adapter-schema, validate (cached)
	adapter, err := parseAdapterFileCached(adapterPath)
	if err != nil {
		return err
	}
	// Re-read the raw bytes for the copy step. The cache holds the parsed
	// representation; the file copy is mode-invariant so a fresh read is
	// the safest path.
	data, err := os.ReadFile(adapterPath)
	if err != nil {
		return fmt.Errorf("failed to read adapter file: %w", err)
	}

	// Element: adapter-name (text-output → fmt.Println)
	fmt.Fprintf(cmd.OutOrStdout(), "Registered framework adapter %q:\n", adapter.Name)

	// Element: vocab-count (data-value → fmt.Printf)
	fmt.Fprintf(cmd.OutOrStdout(), "  Shows: %d  Actions: %d  Flows: %d\n",
		len(adapter.Shows), len(adapter.Actions), len(adapter.Flows))

	// Element: mount-strategies count (data-value → fmt.Printf)
	if len(adapter.MountStrategies) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  Mount strategies: %d\n", len(adapter.MountStrategies))
	}

	// Element: componentVocabulary summary (data-value → fmt.Printf)
	if adapter.ComponentVocabulary != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  componentVocabulary %s: %d components\n",
			adapter.ComponentVocabulary.Name,
			len(adapter.ComponentVocabulary.Components))
	}

	// Element: tokens summary (data-value → fmt.Printf)
	if adapter.Tokens != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  tokens: modes=[%s]  %d spacing  %d color  %d typography\n",
			strings.Join(adapter.Tokens.Modes, ", "),
			len(adapter.Tokens.Spacing),
			len(adapter.Tokens.Color),
			len(adapter.Tokens.Typography))
	}

	// Element: conventions (text-output → fmt.Println)
	if sr, ok := adapter.FileConventions["source-root"]; ok {
		fmt.Fprintf(cmd.OutOrStdout(), "  File conventions: %s\n", sr)
	}

	// Operation: create-directory ".parlay/adapters/"
	adaptersDir := cfg.AdaptersPath()
	if err := os.MkdirAll(adaptersDir, 0755); err != nil {
		return fmt.Errorf("failed to create adapters directory: %w", err)
	}

	// Operation: copy-file to .parlay/adapters/{name}.adapter.yaml
	dstPath := filepath.Join(adaptersDir, adapter.Name+".adapter.yaml")
	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		return fmt.Errorf("failed to copy adapter: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintf(cmd.OutOrStdout(), "Adapter saved to %s\n", dstPath)
	// Point at adapter-set.yaml, not the deprecated prototype-framework field
	// this line used to recommend — the tool warns about that field elsewhere
	// and removes it in v0.3.
	kind := adapterKindOrDefault(data)
	fmt.Fprintf(cmd.OutOrStdout(), "Pin it in .parlay/adapter-set.yaml to use it:\n\n  targets:\n    %s:\n      adapter: %s\n      root: <source-root>\n", kind, adapter.Name)

	return nil
}

// adapterKindOrDefault reports an adapter file's declared kind, defaulting an
// absent one to presentation per adapter.schema.md Section 0.
func adapterKindOrDefault(content []byte) string {
	var shape struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(content, &shape); err == nil && shape.Kind != "" {
		return shape.Kind
	}
	return "presentation"
}
