package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "parlay",
	Short: "Intent-first toolkit for design-to-specification workflows",
	Long:  "Parlay takes user intents and dialogues and parlays them into prototypes, surfaces, and engineering specifications.",
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("parlay %s (%s)\n", appVersion, appCommit)
	},
}

var (
	appVersion = "dev"
	appCommit  = "none"
)

// SetVersion is called from main to inject build-time values.
func SetVersion(version, commit string) {
	appVersion = version
	appCommit = commit
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(addFeatureCmd)
	rootCmd.AddCommand(createDialogsCmd)
	rootCmd.AddCommand(createSurfaceCmdImpl)
	rootCmd.AddCommand(viewPageCmdImpl)
	rootCmd.AddCommand(lockPageCmdImpl)
	rootCmd.AddCommand(syncCmdImpl)
	rootCmd.AddCommand(registerAdapterCmd)
	rootCmd.AddCommand(buildFeatureCmdImpl)
	rootCmd.AddCommand(generateCodeCmdImpl)
	rootCmd.AddCommand(generateEnggspecCmdImpl)
	rootCmd.AddCommand(extractDomainModelCmdImpl)
	rootCmd.AddCommand(loadDomainModelCmdImpl)
	rootCmd.AddCommand(createArtifactsCmdImpl)
	rootCmd.AddCommand(newInitiativeCmd)
	rootCmd.AddCommand(simplifyCmd)
	rootCmd.AddCommand(moveFeatureCmd)

	// Utility commands for agent consumption
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(parseCmd)
	rootCmd.AddCommand(checkCoverageCmd)
	rootCmd.AddCommand(collectQuestionsCmd)
	rootCmd.AddCommand(checkDriftCmd)
	rootCmd.AddCommand(checkReadinessCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(scanGeneratedCmd)
	rootCmd.AddCommand(verifyGeneratedCmd)
	rootCmd.AddCommand(saveBuildStateCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(upgradeCmd)
}
