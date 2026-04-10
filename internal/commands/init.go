package commands

// Generated from buildfile component: project-setup-wizard
// Type: interactive-wizard | Widget: sequential-prompts | Layout: sequential-prompts

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ddwht/parlay/internal/config"
	"github.com/ddwht/parlay/internal/deployer"
	"github.com/ddwht/parlay/internal/embedded"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Bootstrap a new parlay project",
	RunE:  runInit,
}

// options for each prompt — single source of truth
var agentOptions = []string{"Claude Code", "Cursor", "Generic"}
var sddOptions = []string{"GitHub SpecKit", "Kiro", "None"}
var frameworkOptions = []string{"Go CLI", "React + Ant Design", "None (register adapter later)"}

func runInit(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(config.ParlayDir); err == nil {
		return fmt.Errorf("project already initialized (.parlay/ exists)")
	}

	reader := bufio.NewReader(os.Stdin)

	agent, err := promptChoice(reader, "What AI agent would you like to use?", agentOptions)
	if err != nil {
		return err
	}

	sdd, err := promptChoice(reader, "What SDD framework do you want to use?", sddOptions)
	if err != nil {
		return err
	}

	framework, err := promptChoice(reader, "What prototype framework do you want to use?", frameworkOptions)
	if err != nil {
		return err
	}

	cfg := &config.ProjectConfig{
		AIAgent:            agent,
		SDDFramework:       sdd,
		PrototypeFramework: framework,
	}

	// Operation: create-directory ".parlay/"
	if err := os.MkdirAll(config.ParlayDir, 0755); err != nil {
		return fmt.Errorf("failed to create .parlay/: %w", err)
	}

	// Operation: create-file ".parlay/config.yaml" from ProjectConfig
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Operation: scaffold ".parlay/blueprint.yaml" with navigation strategy
	blueprintStrategy := inferNavigationStrategy(framework)
	blueprintContent := fmt.Sprintf("app: \"\"\n\nnavigation:\n  strategy: %s\n", blueprintStrategy)
	if err := os.WriteFile(config.BlueprintPath(), []byte(blueprintContent), 0644); err != nil {
		return fmt.Errorf("failed to write blueprint: %w", err)
	}

	// Operation: copy-embedded schemas → ".parlay/schemas/"
	schemasPath := config.SchemasPath()
	if err := embedded.WriteSchemas(schemasPath); err != nil {
		return fmt.Errorf("failed to write schemas: %w", err)
	}
	schemaNames, _ := embedded.SchemaNames()

	// Operation: copy-bundled-adapter "{prototype-framework}" → ".parlay/adapters/"
	adapterName := copyBundledAdapter(framework)

	// Operation: create-directory "spec/intents/" (designer-authored input)
	if err := os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755); err != nil {
		return fmt.Errorf("failed to create spec/intents/: %w", err)
	}

	// Operation: create-directory "spec/handoff/" (engineering-consumed output)
	if err := os.MkdirAll(config.HandoffRoot(), 0755); err != nil {
		return fmt.Errorf("failed to create spec/handoff/: %w", err)
	}

	// Operation: create-directory ".parlay/build/" (tool-internal build artifacts)
	if err := os.MkdirAll(config.BuildRoot(), 0755); err != nil {
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

	// Element: summary (text-output → fmt.Println)
	fmt.Println()
	fmt.Println("Project bootstrapped:")
	fmt.Printf("  .parlay/config.yaml        — %s + %s + %s\n", agent, sdd, framework)
	fmt.Printf("  .parlay/blueprint.yaml     — navigation: %s\n", blueprintStrategy)
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

	// Element: next-step (text-output → fmt.Println)
	fmt.Println()
	fmt.Println("Ready. Run: parlay add-feature <name>")

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

func inferNavigationStrategy(framework string) string {
	lower := strings.ToLower(framework)
	switch {
	case strings.Contains(lower, "cli"):
		return "cli-subcommands"
	case strings.Contains(lower, "ios") || strings.Contains(lower, "android"):
		return "native-tab"
	default:
		return "browser"
	}
}

func copyBundledAdapter(framework string) string {
	// Map framework display name to bundled adapter file name
	adapterMap := map[string]string{
		"go cli":             "go-cli",
		"react + ant design": "react-antd",
	}

	adapterName := ""
	for key, name := range adapterMap {
		if strings.EqualFold(framework, key) {
			adapterName = name
			break
		}
	}

	if adapterName == "" {
		return ""
	}

	adaptersDir := config.AdaptersPath()
	os.MkdirAll(adaptersDir, 0755)

	// Try to copy from bundled adapters directory
	srcPath := filepath.Join("adapters", adapterName+".adapter.yaml")
	data, err := os.ReadFile(srcPath)
	if err != nil {
		// Try embedded adapters
		data, err = embedded.ReadAdapter(adapterName)
		if err != nil {
			return ""
		}
	}

	dstPath := filepath.Join(adaptersDir, adapterName+".adapter.yaml")
	os.WriteFile(dstPath, data, 0644)
	return adapterName
}

