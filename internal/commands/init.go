package commands

// Generated from buildfile component: project-setup-wizard
// Type: interactive-wizard | Widget: sequential-prompts | Layout: sequential-prompts

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/parlay/internal/config"
	"github.com/anthropics/parlay/internal/deployer"
	"github.com/anthropics/parlay/internal/embedded"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Bootstrap a new parlay project",
	RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(config.ParlayDir); err == nil {
		return fmt.Errorf("project already initialized (.parlay/ exists)")
	}

	reader := bufio.NewReader(os.Stdin)

	// Element: agent-prompt (text-output → fmt.Println)
	fmt.Println("What AI agent would you like to use?")
	// Action: read-agent (text-input → text-prompt)
	fmt.Print("> ")
	agent, _ := reader.ReadString('\n')
	agent = strings.TrimSpace(agent)

	// Element: sdd-prompt (text-output → fmt.Println)
	fmt.Println("What SDD framework do you want to use?")
	// Action: read-sdd (text-input → text-prompt)
	fmt.Print("> ")
	sdd, _ := reader.ReadString('\n')
	sdd = strings.TrimSpace(sdd)

	// Element: framework-prompt (text-output → fmt.Println)
	fmt.Println("What prototype framework do you want to use?")
	// Action: read-framework (text-input → text-prompt)
	fmt.Print("> ")
	framework, _ := reader.ReadString('\n')
	framework = strings.TrimSpace(framework)

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

	// Operation: copy-embedded schemas → ".parlay/schemas/"
	schemasPath := config.SchemasPath()
	if err := embedded.WriteSchemas(schemasPath); err != nil {
		return fmt.Errorf("failed to write schemas: %w", err)
	}
	schemaNames, _ := embedded.SchemaNames()

	// Operation: copy-bundled-adapter "{prototype-framework}" → ".parlay/adapters/"
	adapterName := copyBundledAdapter(framework)

	// Operation: create-directory "spec/intents/"
	if err := os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0755); err != nil {
		return fmt.Errorf("failed to create spec/intents/: %w", err)
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
	fmt.Printf("  .parlay/schemas/            — %d schemas\n", len(schemaNames))
	if adapterName != "" {
		fmt.Printf("  .parlay/adapters/           — %s adapter\n", adapterName)
	}
	fmt.Printf("  spec/intents/               — feature folder\n")
	if len(skills) > 0 {
		fmt.Printf("  skills                      — %d skills deployed for %s\n", len(skills), dep.Name())
	}

	// Element: next-step (text-output → fmt.Println)
	fmt.Println()
	fmt.Println("Ready. Run: parlay add-feature <name>")

	return nil
}

func copyBundledAdapter(framework string) string {
	// Map framework name to bundled adapter file
	adapterMap := map[string]string{
		"go cli":             "go-cli",
		"angular + clarity":  "angular-clarity",
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
