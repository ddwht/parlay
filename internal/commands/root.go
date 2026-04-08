package commands

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "parlay",
	Short: "Intent-first toolkit for design-to-specification workflows",
	Long:  "Parlay takes user intents and dialogues and parlays them into prototypes, surfaces, and engineering specifications.",
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

	// Utility commands for agent consumption
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(parseCmd)
	rootCmd.AddCommand(checkCoverageCmd)
	rootCmd.AddCommand(collectQuestionsCmd)
	rootCmd.AddCommand(saveBaselineCmd)
	rootCmd.AddCommand(checkDriftCmd)
	rootCmd.AddCommand(checkReadinessCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(scanGeneratedCmd)
	rootCmd.AddCommand(verifyGeneratedCmd)
	rootCmd.AddCommand(saveCodeHashesCmd)
}
