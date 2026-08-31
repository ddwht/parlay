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
	// Every command declares who reaches it. The classes are exclusive and
	// role-based: see reachability_class.go for what each means and why the
	// declaration is mandatory rather than opt-in.
	internalCmd.AddCommand(
		// Feedback mode — opt-in instrumentation, recorded locally and sent
		// only when the user chooses to. This is how a skill contributes
		// the events no CLI call can observe; it is a no-op when the mode
		// is off. Its siblings — status, export, prune — are people-facing
		// and registered at the top level in root.go.
		reachability(feedbackRecordCmd, ClassPipelineHelper),

		// Probes — read project state, emit JSON, change nothing.
		reachability(parseCmd, ClassProbe),
		reachability(diffCmd, ClassProbe),
		reachability(checkCoverageCmd, ClassProbe),
		reachability(checkDriftCmd, ClassProbe),
		reachability(checkWriteSetCmd, ClassProbe),
		reachability(checkReadinessCmd, ClassProbe),
		reachability(checkBuildfileCmd, ClassProbe),
		reachability(checkSupportsCmd, ClassProbe),
		reachability(scanGeneratedCmd, ClassProbe),
		reachability(verifyGeneratedCmd, ClassProbe),
		reachability(collectQuestionsCmd, ClassProbe),
		reachability(checkCompositionCmd, ClassProbe),
		reachability(checkAmendmentsCmd, ClassProbe),
		reachability(activeSpecCmd, ClassProbe),
		reachability(criteriaAuthorityCmd, ClassProbe),
		reachability(checkAppliedCmd, ClassProbe),
		reachability(mergedRoutesCmd, ClassProbe),
		reachability(crossCuttingIndexCmd, ClassProbe),
		reachability(affectedSetCmd, ClassProbe),
		reachability(domainImpactCmd, ClassProbe),
		reachability(schemaDigestCmd, ClassProbe),
		reachability(emissionGroupsCmd, ClassProbe),
		// Derives and prints the assembly suites a composed page requires.
		// A probe: it computes and reports, and writes nothing.
		reachability(emitAssemblyCmd, ClassProbe),
		// The code boundary's own readiness computation, exposed to the
		// phase that can still repair what it finds.
		reachability(checkTestcasesReadyCmd, ClassProbe),
		// Inventory, not a decision: it reports what is stranded and writes
		// nothing. The walkthrough that acts on it is skill-required; this
		// is the listing anything may read.
		reachability(migrateExceptionsCmd, ClassProbe),

		// The gate CONSUMES authority; it does not acquire it. It evaluates an
		// approval somebody else obtained, or consumes an explicit machine
		// waiver established by config plus flag, and reports blockers. A
		// blocker that needs a person downstream does not make the checker a
		// human-interaction command — otherwise every check here would become
		// one.
		reachability(gateCmd, ClassPipelineHelper),

		// State helper — writes the baseline and code-hashes pair. Agent-
		// driven because only the code phase knows the tests passed.
		reachability(saveBuildStateCmd, ClassPipelineHelper),
		reachability(scaffoldSignaturesCmd, ClassPipelineHelper),
		reachability(scaffoldPlanCmd, ClassPipelineHelper),
		reachability(scaffoldOperationsCmd, ClassPipelineHelper),
		reachability(toolchainPlanCmd, ClassPipelineHelper),
		reachability(scaffoldSeedCmd, ClassPipelineHelper),

		// Authority ACQUISITION. Each of these records or withdraws a human
		// judgment, so each needs a deployed walkthrough that puts the question
		// to a person properly. Reached by improvisation, they produce a ledger
		// saying somebody decided when nobody did.
		reachability(approveCriteriaCmd, ClassSkillRequired),
		reachability(applyGovernanceCmd, ClassSkillRequired),
		reachability(applyAmendmentCmd, ClassSkillRequired),
		reachability(compactCmd, ClassSkillRequired),
		reachability(recordExceptionCmd, ClassSkillRequired),
		reachability(retireDecisionCmd, ClassSkillRequired),
		reachability(deferLegacyCmd, ClassSkillRequired),
		reachability(dropLegacyCmd, ClassSkillRequired),
		// Read-only, but a workflow step rather than a probe: it exists to be
		// the one place a walkthrough gets the next question from.
		reachability(nextLegacyReviewCmd, ClassSkillRequired),

		// Refine's step journal — the one write here that is not build
		// state: it records how far an in-flight refinement got so an
		// interrupted run resumes instead of restarting.
		reachability(refineJournalCmd, ClassPipelineHelper),
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
