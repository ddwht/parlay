package commands

import (
	"github.com/spf13/cobra"
)

// internalCmd groups the commands an AI agent calls and a person almost
// never does: the JSON probes that report project state, and the two
// state-committing helpers the code phase drives.
//
// The split is about what `parlay --help` is for. Before this, help listed
// 45 commands with no indication that two thirds of them emit JSON for a
// skill to parse — `check-review-gate` and `new-initiative` sat side by
// side as equals. A designer scanning that list has to know parlay's
// internals to find the handful of commands meant for them.
//
// Nothing is removed. Every command here still runs, still emits the same
// JSON, and still exits with the same codes; it answers to
// `parlay internal <cmd>` and stays out of the top-level listing. The
// grouping is also a contract signal: output shapes under `internal` are
// tool-facing and may change with the skills that read them, where the
// visible commands are the stable human surface.
var internalCmd = &cobra.Command{
	Use:   "internal",
	Short: "Agent-facing probes and state helpers (JSON output)",
	Long: `Commands parlay's own skills call to inspect and commit project state.

These emit JSON for a skill to parse rather than prose for a person to read.
They are grouped here so that ` + "`parlay --help`" + ` shows the commands meant for
people. Everything here is still directly runnable — useful when debugging
what a phase saw.

Output shapes under ` + "`internal`" + ` follow the skills that consume them and are
not part of the stable CLI surface.`,
}

// registerInternalCommands moves the agent-facing commands under the
// `internal` parent. Called from root.go's init, after each command's own
// init has configured its flags.
func registerInternalCommands() {
	internalCmd.AddCommand(
		// Probes — read project state, emit JSON, change nothing.
		parseCmd,
		diffCmd,
		checkCoverageCmd,
		checkDriftCmd,
		checkWriteSetCmd,
		checkReadinessCmd,
		checkBuildfileCmd,
		checkSupportsCmd,
		checkReviewGateCmd,
		scanGeneratedCmd,
		verifyGeneratedCmd,
		collectQuestionsCmd,

		// State helper — writes the baseline and code-hashes pair. Agent-
		// driven because only the code phase knows the tests passed.
		saveBuildStateCmd,
		scaffoldSignaturesCmd,
		scaffoldPlanCmd,
		scaffoldOperationsCmd,
		toolchainPlanCmd,
		scaffoldSeedCmd,
		checkCompositionCmd,
		domainImpactCmd,
		schemaDigestCmd,
		emissionGroupsCmd,
		serveCmd,
	)
	rootCmd.AddCommand(internalCmd)
}

// hideAgentOnlyStubs marks the commands that exist only to tell a person
// they need an agent. They are kept — the message is a genuinely useful
// answer to `parlay build-feature`, which is a reasonable thing to try —
// but listing them in help advertises commands that cannot do the thing
// their name promises.
func hideAgentOnlyStubs() {
	for _, c := range []*cobra.Command{
		buildFeatureCmdImpl,
		generateCodeCmdImpl,
		generateEnggspecCmdImpl,
		createDomainModelCmdImpl,
		loadDomainModelCmdImpl,
		createArtifactsCmdImpl,
		loopCmd,
	} {
		if c != nil {
			c.Hidden = true
		}
	}
}
