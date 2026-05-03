package commands

// Generated from buildfile component: adapter-registration
// Type: command-output | Widget: cobra-command | Layout: file-generator
//
// parlay-feature: parlay-tool/project-setup
// parlay-component: adapter-registration
// parlay-extends: studio-support/adapter-vocabulary-extension/adapter-parser-vocabulary-and-tokens

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
	Name                string                 `yaml:"name"`
	Framework           string                 `yaml:"framework"`
	Version             string                 `yaml:"version"`
	Shows               map[string]interface{} `yaml:"shows"`
	Actions             map[string]interface{} `yaml:"actions"`
	Flows               map[string]interface{} `yaml:"flows"`
	FileConventions     map[string]interface{} `yaml:"file-conventions"`
	MountStrategies     map[string]interface{} `yaml:"mount-strategies,omitempty"`
	ComponentVocabulary *adapterComponentVocabulary `yaml:"componentVocabulary,omitempty"`
	Tokens              *adapterTokens         `yaml:"tokens,omitempty"`
}

// adapterComponentVocabulary captures the versioned component vocabulary an
// adapter exposes to layouts. See internal/embedded/schemas/adapter.schema.md
// Section 8 for the full contract.
type adapterComponentVocabulary struct {
	Name       string                       `yaml:"name"`
	Components []adapterVocabularyComponent `yaml:"components"`
}

type adapterVocabularyComponent struct {
	Type            string                       `yaml:"type"`
	Category        string                       `yaml:"category"`
	Variants        []string                     `yaml:"variants,omitempty"`
	Properties      []adapterVocabularyProperty  `yaml:"properties,omitempty"`
	AllowedChildren []string                     `yaml:"allowed-children,omitempty"`
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
	Modes      []string                  `yaml:"modes"`
	Spacing    []adapterSpacingToken     `yaml:"spacing,omitempty"`
	Color      []adapterColorToken       `yaml:"color,omitempty"`
	Typography []adapterTypographyToken  `yaml:"typography,omitempty"`
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

// closedPropertyTypes is the closed type set for componentVocabulary property
// declarations. Anything outside this set fails parse.
var closedPropertyTypes = map[string]bool{
	"string":          true,
	"token-reference": true,
	"enum":            true,
	"boolean":         true,
	"int":             true,
	"child-list":      true,
}

// closedComponentCategories is the closed category set for componentVocabulary
// components.
var closedComponentCategories = map[string]bool{
	"container":  true,
	"leaf":       true,
	"data-shape": true,
}

// closedColorTones is the closed tone set for color tokens. Shared with the
// domain model's enum-tone metadata.
var closedColorTones = map[string]bool{
	"":        true, // tone is optional
	"neutral": true,
	"info":    true,
	"warning": true,
	"danger":  true,
	"success": true,
}

// closedTypographyUseSites is the closed use-site set for typography tokens.
var closedTypographyUseSites = map[string]bool{
	"heading-page":    true,
	"heading-section": true,
	"body":            true,
	"caption":         true,
}

// universalContainerFields is the fixed set of fields owned by the layout
// schema. Adapters MUST NOT re-declare these inside componentVocabulary
// component entries.
var universalContainerFields = map[string]bool{
	"direction": true,
	"gap":       true,
	"padding":   true,
	"alignment": true,
}

// adapterCache is a per-process cache of parsed adapter files keyed by
// absolute path. It exists so that subsequent commands within a single CLI
// invocation reuse the parse result rather than re-reading and re-parsing
// the YAML each time. ParseCount tracks how many distinct parse operations
// occurred; lookups against an already-cached path do NOT increment it.
type adapterCacheEntry struct {
	Adapter   *adapterFile
	ParsedAt  string
}

var (
	adapterCacheMu sync.RWMutex
	adapterCache   = map[string]*adapterCacheEntry{}
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
	if err := validateAdapterDeclarations(&adapter); err != nil {
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

// validateAdapterDeclarations runs the registration-time validation passes
// for the optional componentVocabulary and tokens sections. Returns the
// first error encountered; callers surface this to the user.
func validateAdapterDeclarations(adapter *adapterFile) error {
	if adapter.Name == "" {
		return fmt.Errorf("adapter file missing 'name' field")
	}

	if adapter.ComponentVocabulary != nil {
		if err := validateComponentVocabulary(adapter.ComponentVocabulary); err != nil {
			return err
		}
	}

	if adapter.Tokens != nil {
		if err := validateAdapterTokens(adapter.Tokens); err != nil {
			return err
		}
	}

	return nil
}

func validateComponentVocabulary(v *adapterComponentVocabulary) error {
	// Vocabulary name must include @<version>. Bare names are rejected.
	if v.Name == "" {
		return fmt.Errorf("componentVocabulary: vocabulary name must include @<version> (e.g., clarity@17); got empty name")
	}
	if !strings.Contains(v.Name, "@") {
		return fmt.Errorf("componentVocabulary: vocabulary name must include @<version> (e.g., clarity@17); got %q", v.Name)
	}

	for _, comp := range v.Components {
		if comp.Type == "" {
			return fmt.Errorf("componentVocabulary: component is missing required 'type' field")
		}
		if comp.Category != "" && !closedComponentCategories[comp.Category] {
			return fmt.Errorf("componentVocabulary: component %q has category %q outside the closed set {container, leaf, data-shape}", comp.Type, comp.Category)
		}
		// Property type closed-set + universal-field redeclaration check.
		for _, prop := range comp.Properties {
			if universalContainerFields[prop.Name] {
				return fmt.Errorf("componentVocabulary: component %q re-declares universal container field %q — universal container fields {direction, gap, padding, alignment} live in the layout schema and MUST NOT appear inside componentVocabulary entries", comp.Type, prop.Name)
			}
			if prop.Type == "" {
				return fmt.Errorf("componentVocabulary: component %q property %q is missing required 'type' field", comp.Type, prop.Name)
			}
			if !closedPropertyTypes[prop.Type] {
				return fmt.Errorf("componentVocabulary: component %q property %q declares type `%s` is not allowed — must be one of {string, token-reference, enum, boolean, int, child-list}", comp.Type, prop.Name, prop.Type)
			}
		}
	}
	return nil
}

func validateAdapterTokens(t *adapterTokens) error {
	// Every adapter declaring tokens: must declare at least one mode.
	if len(t.Modes) == 0 {
		return fmt.Errorf("tokens: adapter must declare at least one mode (typically `modes: [light]`); got empty mode list")
	}
	declaredModes := map[string]bool{}
	for _, m := range t.Modes {
		if strings.TrimSpace(m) == "" {
			return fmt.Errorf("tokens: mode entry must be a non-empty string")
		}
		declaredModes[m] = true
	}

	// Color tokens: every declared mode must be covered by an emit-form.
	for _, c := range t.Color {
		if !closedColorTones[c.Tone] {
			return fmt.Errorf("tokens.color: token %q has tone %q outside the closed set {neutral, info, warning, danger, success}", c.Name, c.Tone)
		}
		seenModes := map[string]bool{}
		for _, ef := range c.EmitForms {
			// emit-forms entries are of the form "mode:<value>".
			parts := strings.SplitN(ef, ":", 2)
			if len(parts) > 0 && parts[0] != "" {
				seenModes[parts[0]] = true
			}
		}
		for m := range declaredModes {
			if !seenModes[m] {
				return fmt.Errorf("tokens.color: token %q is missing an emit-form for declared mode %q", c.Name, m)
			}
		}
	}

	// Typography tokens: use-site must be in the closed set.
	for _, ty := range t.Typography {
		if !closedTypographyUseSites[ty.UseSite] {
			return fmt.Errorf("tokens.typography: token %q has use-site %q outside the closed set {heading-page, heading-section, body, caption}", ty.Name, ty.UseSite)
		}
	}

	return nil
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
	fmt.Printf("Registered framework adapter %q:\n", adapter.Name)

	// Element: vocab-count (data-value → fmt.Printf)
	fmt.Printf("  Shows: %d  Actions: %d  Flows: %d\n",
		len(adapter.Shows), len(adapter.Actions), len(adapter.Flows))

	// Element: mount-strategies count (data-value → fmt.Printf)
	if len(adapter.MountStrategies) > 0 {
		fmt.Printf("  Mount strategies: %d\n", len(adapter.MountStrategies))
	}

	// Element: componentVocabulary summary (data-value → fmt.Printf)
	if adapter.ComponentVocabulary != nil {
		fmt.Printf("  componentVocabulary %s: %d components\n",
			adapter.ComponentVocabulary.Name,
			len(adapter.ComponentVocabulary.Components))
	}

	// Element: tokens summary (data-value → fmt.Printf)
	if adapter.Tokens != nil {
		fmt.Printf("  tokens: modes=[%s]  %d spacing  %d color  %d typography\n",
			strings.Join(adapter.Tokens.Modes, ", "),
			len(adapter.Tokens.Spacing),
			len(adapter.Tokens.Color),
			len(adapter.Tokens.Typography))
	}

	// Element: conventions (text-output → fmt.Println)
	if sr, ok := adapter.FileConventions["source-root"]; ok {
		fmt.Printf("  File conventions: %s\n", sr)
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

	fmt.Println()
	fmt.Printf("Adapter saved to %s\n", dstPath)
	fmt.Println("Set it as the prototype framework in .parlay/config.yaml to use it with build-feature.")

	return nil
}
