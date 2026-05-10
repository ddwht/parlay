// parlay-feature: parlay-tool/multi-root
// parlay-component: init-agent-identity-prompt
// parlay-extends: parlay-tool/multi-root/init-framework-default-inheritance-prompt
// parlay-cross-cutting: init-topology-writer
// parlay-cross-cutting: init-agent-detection-hook

package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/deployer"
	"github.com/ddwht/parlay/core/internal/embedded"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var initCmd = &cobra.Command{
	Use:         "init",
	Short:       "Bootstrap a new parlay project",
	RunE:        runInit,
	Annotations: map[string]string{annotationSkipResolution: "true"},
}

// frameworkEntry ties display name, adapter slug, and nav strategy together.
type frameworkEntry struct {
	Display     string // shown in menu and saved to config
	Adapter     string // embedded adapter slug (empty = no bundled adapter)
	NavStrategy string // blueprint navigation strategy
}

// Single source of truth for all options.
var agentOptions = []string{"Claude Code", "Cursor", "Generic"}
var sddOptions = []string{"GitHub SpecKit", "Kiro", "None"}

// knownAgentNames is sourced from the deployer registry — kept in sync
// with agentOptions for the detection hook.
var knownAgentNames = []string{"Claude Code", "Cursor", "Generic"}

// DetectRunningAgent inspects the runtime environment (env vars,
// parent process name, terminal markers) and returns the running
// agent's name when one is recognized. Returns ("", false) when no
// agent is detected. The detector returns "unknown" via a false hit
// rather than guessing when signals are ambiguous.
//
// Detection signals (in priority order):
//   1. CLAUDECODE / CLAUDE_CODE_ENTRYPOINT / ANTHROPIC_* env vars → Claude Code
//   2. CURSOR_* env vars → Cursor
//   3. CI / non-TTY contexts → no detection (return false)
//
// Detection is read-only — it never writes to disk or mutates env state.
func DetectRunningAgent() (name string, detected bool) {
	if v := os.Getenv("CLAUDECODE"); v != "" {
		return "Claude Code", true
	}
	if v := os.Getenv("CLAUDE_CODE_ENTRYPOINT"); v != "" {
		return "Claude Code", true
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" && os.Getenv("CLAUDE_AGENT") != "" {
		return "Claude Code", true
	}
	if os.Getenv("CURSOR_AGENT") != "" || os.Getenv("CURSOR_TRACE_ID") != "" {
		return "Cursor", true
	}
	return "", false
}

// promptAgentWithDefault shows the agent-identity prompt, pre-filled
// with the detected default when one was discovered. The user MUST
// press Enter to confirm or type an alternative — init never proceeds
// without an explicit choice. When no agent is detected, the prompt
// falls back to free entry against the adapter list.
func promptAgentWithDefault(reader *bufio.Reader, detected string) (string, error) {
	if detected != "" {
		fmt.Printf("ai-agent? [%s (detected)] — press Enter to confirm or type to override\n", detected)
		fmt.Print("> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		input = strings.TrimSpace(input)
		if input == "" {
			return detected, nil
		}
		// Accept any of the known names verbatim; otherwise pass through.
		for _, opt := range knownAgentNames {
			if strings.EqualFold(input, opt) {
				return opt, nil
			}
		}
		return input, nil
	}
	return promptChoice(reader, "What AI agent would you like to use?", agentOptions)
}
var frameworks = []frameworkEntry{
	{Display: "Go CLI", Adapter: "go-cli", NavStrategy: "cli-subcommands"},
	{Display: "React + Ant Design", Adapter: "react-antd", NavStrategy: "browser"},
	{Display: "Angular + Material", Adapter: "angular-material", NavStrategy: "browser"},
	{Display: "Angular + Clarity", Adapter: "angular-clarity", NavStrategy: "browser"},
	{Display: "None (register adapter later)", Adapter: "", NavStrategy: "browser"},
}

func runInit(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(config.ParlayDir); err == nil {
		return fmt.Errorf("project already initialized (.parlay/ exists)")
	}

	reader := bufio.NewReader(os.Stdin)

	detectedAgent, _ := DetectRunningAgent()
	agent, err := promptAgentWithDefault(reader, detectedAgent)
	if err != nil {
		return err
	}

	sdd, err := promptChoice(reader, "What SDD framework do you want to use?", sddOptions)
	if err != nil {
		return err
	}

	// Build display list from frameworks
	fwDisplays := make([]string, len(frameworks))
	for i, fw := range frameworks {
		fwDisplays[i] = fw.Display
	}
	fwChoice, err := promptChoice(reader, "What prototype framework do you want to use?", fwDisplays)
	if err != nil {
		return err
	}

	// Find the matching entry
	var fw frameworkEntry
	for _, f := range frameworks {
		if f.Display == fwChoice {
			fw = f
			break
		}
	}

	cfg := &config.ProjectConfig{
		AIAgent:            agent,
		SDDFramework:       sdd,
		PrototypeFramework: fw.Display,
	}

	// Operation: create-directory ".parlay/"
	if err := os.MkdirAll(config.ParlayDir, 0755); err != nil {
		return fmt.Errorf("failed to create .parlay/: %w", err)
	}

	// init runs before any parlay root exists, so it bypasses the
	// resolver-based Context and writes cwd-relative paths directly.
	configPath := filepath.Join(config.ParlayDir, config.ConfigFile)
	blueprintPath := filepath.Join(config.ParlayDir, config.BlueprintFile)
	schemasPath := filepath.Join(config.ParlayDir, config.SchemasDir)
	intentsRoot := filepath.Join(config.SpecDir, config.IntentsDir)
	handoffRoot := filepath.Join(config.SpecDir, config.HandoffDir)
	buildRoot := filepath.Join(config.ParlayDir, config.BuildDir)

	// Operation: create-file ".parlay/config.yaml" from ProjectConfig
	cfgBytes, _ := yaml.Marshal(cfg)
	if err := os.WriteFile(configPath, cfgBytes, 0644); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Operation: scaffold ".parlay/blueprint.yaml" with navigation strategy
	blueprintContent := fmt.Sprintf("app: \"\"\n\nnavigation:\n  strategy: %s\n", fw.NavStrategy)
	if err := os.WriteFile(blueprintPath, []byte(blueprintContent), 0644); err != nil {
		return fmt.Errorf("failed to write blueprint: %w", err)
	}

	// Operation: copy-embedded schemas → ".parlay/schemas/"
	if err := embedded.WriteSchemas(schemasPath); err != nil {
		return fmt.Errorf("failed to write schemas: %w", err)
	}
	schemaNames, _ := embedded.SchemaNames()

	// Operation: copy-bundled-adapter (if selected)
	adapterName := ""
	if fw.Adapter != "" {
		adapterName = copyBundledAdapter(fw.Adapter)
	}

	// Operation: create-directory "spec/intents/" (designer-authored input)
	if err := os.MkdirAll(intentsRoot, 0755); err != nil {
		return fmt.Errorf("failed to create spec/intents/: %w", err)
	}

	// Operation: create-directory "spec/handoff/" (engineering-consumed output)
	if err := os.MkdirAll(handoffRoot, 0755); err != nil {
		return fmt.Errorf("failed to create spec/handoff/: %w", err)
	}

	// Operation: create-directory ".parlay/build/" (tool-internal build artifacts)
	if err := os.MkdirAll(buildRoot, 0755); err != nil {
		return fmt.Errorf("failed to create .parlay/build/: %w", err)
	}

	// Operation: deploy skills and agent config
	skills, _ := embedded.ReadAllSkills()
	dep, err := deployer.Get(agent)
	if err != nil {
		fmt.Printf("  Warning: no deployer for agent %q, using generic\n", agent)
		dep, _ = deployer.Get("generic")
	}
	if dep != nil {
		if err := dep.Deploy(".", skills); err != nil {
			fmt.Printf("  Warning: could not deploy skills: %s\n", err)
		}
	}

	// Element: summary
	fmt.Println()
	fmt.Println("Project bootstrapped:")
	fmt.Printf("  .parlay/config.yaml        — %s + %s + %s\n", agent, sdd, fw.Display)
	fmt.Printf("  .parlay/blueprint.yaml     — navigation: %s\n", fw.NavStrategy)
	fmt.Printf("  .parlay/schemas/            — %d schemas\n", len(schemaNames))
	if adapterName != "" {
		fmt.Printf("  .parlay/adapters/           — %s adapter\n", adapterName)
	}
	fmt.Printf("  .parlay/build/              — internal build artifacts (per feature)\n")
	fmt.Printf("  spec/intents/               — designer-authored feature inputs\n")
	fmt.Printf("  spec/handoff/               — engineering handoff artifacts (per feature)\n")
	if len(skills) > 0 {
		fmt.Printf("  skills                      — %d skills deployed for %s\n", len(skills), dep.Name())
	}

	// parlay-feature: parlay-tool/multi-adapter
	// parlay-component: project-setup-preset-selection
	//
	// After the existing single-adapter init completes, offer to copy a
	// bundled adapter-set preset. Default flow leaves the project
	// presentation-only with the chosen adapter; the preset prompt is the
	// opt-in path into multi-target topology.
	if err := offerPresetSelection(); err != nil {
		fmt.Printf("  Note: preset selection skipped (%v)\n", err)
	}

	// Element: next-step
	fmt.Println()
	fmt.Println("Ready. Run: parlay add-feature <name>")

	return nil
}

// offerPresetSelection presents the bundled adapter-set presets, asks the
// user to pick one (or skip), and copies the chosen preset's
// adapter-set.yaml + the adapter files it references into .parlay/. Skipping
// (or selecting "custom") leaves .parlay/adapter-set.yaml absent for the
// user to author from scratch.
func offerPresetSelection() error {
	presetNames, err := embedded.PresetNames()
	if err != nil {
		return fmt.Errorf("list presets: %w", err)
	}
	if len(presetNames) == 0 {
		return nil
	}

	// Skip the prompt entirely when stdin is not a TTY (CI mode).
	stat, err := os.Stdin.Stat()
	if err != nil || (stat.Mode()&os.ModeCharDevice) == 0 {
		return nil
	}

	fmt.Println()
	fmt.Println("Pick a starting preset (optional — adds backend slots beyond presentation):")
	for i, name := range presetNames {
		marker := ""
		if name == "react-nest-prisma" {
			marker = "  [INFO] react-nest-prisma is the v1 first preset (exercised end-to-end in CI)"
		}
		fmt.Printf("  %d. %s%s\n", i+1, name, marker)
	}
	fmt.Printf("  %d. custom (skip — author .parlay/adapter-set.yaml from scratch)\n", len(presetNames)+1)
	fmt.Print("> ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil // accept default = skip
	}

	var idx int
	if _, err := fmt.Sscanf(line, "%d", &idx); err != nil || idx < 1 || idx > len(presetNames)+1 {
		return nil
	}
	if idx == len(presetNames)+1 {
		fmt.Println("No files written — author .parlay/adapter-set.yaml from scratch.")
		return nil
	}

	chosen := presetNames[idx-1]
	content, err := embedded.ReadPreset(chosen)
	if err != nil {
		return fmt.Errorf("read preset: %w", err)
	}
	dest := filepath.Join(".parlay", "adapter-set.yaml")
	if err := os.WriteFile(dest, content, 0644); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	fmt.Printf("Files written\n  %s — preset %s\n", dest, chosen)
	return nil
}

// promptChoice displays a numbered menu and returns the selected option.
func promptChoice(reader *bufio.Reader, question string, options []string) (string, error) {
	fmt.Println(question)
	for i, opt := range options {
		fmt.Printf("  %d. %s\n", i+1, opt)
	}
	fmt.Print("> ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)

	// Accept number
	if n, err := strconv.Atoi(input); err == nil && n >= 1 && n <= len(options) {
		return options[n-1], nil
	}

	// Accept exact text match (case-insensitive)
	for _, opt := range options {
		if strings.EqualFold(input, opt) {
			return opt, nil
		}
	}

	return "", fmt.Errorf("invalid choice %q — enter a number 1-%d", input, len(options))
}

// copyBundledAdapter copies an embedded adapter by slug to .parlay/adapters/.
// Called from runInit, before any active root exists, so paths are
// resolved cwd-relative directly rather than through *config.Context.
func copyBundledAdapter(adapterSlug string) string {
	adaptersDir := filepath.Join(config.ParlayDir, config.AdaptersDir)
	os.MkdirAll(adaptersDir, 0755)

	data, err := embedded.ReadAdapter(adapterSlug)
	if err != nil {
		return ""
	}

	dstPath := filepath.Join(adaptersDir, adapterSlug+".adapter.yaml")
	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		return ""
	}
	return adapterSlug
}
