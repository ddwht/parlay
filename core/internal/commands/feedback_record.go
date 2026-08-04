// parlay-feature: parlay-tool/feedback-mode
// parlay-component: agent-event-intake
//
// The agent's way into the feedback log. Skills call this rather than
// appending to the file themselves, for three reasons: the format stays in
// one place, the enablement check stays in one place, and a skill running
// in a project with the mode off does nothing rather than creating a log
// nobody asked for.
//
// Every flag here is a closed enum or a value the CLI hashes on receipt.
// The previous shape took `--data key=value`, which let an agent put a
// sentence in the log — and the skill instruction explicitly asked for
// one ("changed=<what you did differently>"). An open payload cannot be
// made safe by asking nicely, so the payload is closed instead and the two
// prose fields became vocabularies.

package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/ddwht/parlay/core/internal/feedback"
	"github.com/spf13/cobra"
)

var (
	feedbackRecordKind     string
	feedbackRecordSkill    string
	feedbackRecordRun      string
	feedbackRecordPhase    string
	feedbackRecordArtifact string
	feedbackRecordCode     string
	feedbackRecordChanged  string
	feedbackRecordNeeded   string
	feedbackRecordDecision string
	feedbackRecordOption   string
	feedbackRecordSubject  string
)

// The closed vocabularies. Each `other` is deliberate: its frequency in
// the log is itself the signal that the set needs another member, which is
// the same reasoning KindNote follows.
var (
	agentKinds = []string{
		feedback.KindPhase, feedback.KindDecision, feedback.KindRetry,
		feedback.KindImprovised, feedback.KindNote,
	}
	changedValues = []string{
		"added-field", "removed-field", "changed-shape", "changed-version",
		"changed-artifact", "reordered", "other",
	}
	neededValues = []string{
		"schema-rule", "path-convention", "naming-convention",
		"adapter-capability", "example", "decision", "other",
	}
	phaseValues = []string{
		"intents", "dialogs", "artifacts", "build", "code",
	}
	artifactValues = []string{
		"intents", "dialogs", "surface", "capabilities", "infrastructure",
		"domain-model", "buildfile", "testcases", "authored", "layout", "page",
	}
	decisionValues = []string{
		"phase-boundary", "override", "overwrite", "failure", "ambiguity", "impasse",
	}
)

var feedbackRecordCmd = &cobra.Command{
	Use:   "feedback-record",
	Short: "Append one agent-side event to the feedback log (no-op when feedback mode is off)",
	Args:  cobra.NoArgs,
	RunE:  runFeedbackRecord,
}

func init() {
	f := feedbackRecordCmd.Flags()
	f.StringVar(&feedbackRecordKind, "kind", "", "Event kind: "+strings.Join(agentKinds, " | "))
	feedbackRecordCmd.MarkFlagRequired("kind")
	f.StringVar(&feedbackRecordSkill, "skill", "", "The skill or phase module emitting the event")
	f.StringVar(&feedbackRecordRun, "run", "",
		"Override the correlation id; normally unnecessary because "+feedback.RunEnvVar+" is inherited")
	f.StringVar(&feedbackRecordPhase, "phase", "", "Pipeline phase: "+strings.Join(phaseValues, " | "))
	f.StringVar(&feedbackRecordArtifact, "artifact", "", "Artifact kind: "+strings.Join(artifactValues, " | "))
	f.StringVar(&feedbackRecordCode, "code", "", "For retry: the parlay error code that caused it")
	f.StringVar(&feedbackRecordChanged, "changed", "", "For retry, what you did differently: "+strings.Join(changedValues, " | "))
	f.StringVar(&feedbackRecordNeeded, "needed", "", "For improvised, what was missing: "+strings.Join(neededValues, " | "))
	f.StringVar(&feedbackRecordDecision, "decision", "", "For decision: "+strings.Join(decisionValues, " | "))
	f.StringVar(&feedbackRecordOption, "option", "", "For decision: the option id chosen")
	f.StringVar(&feedbackRecordSubject, "subject", "",
		"Feature, unit or operation this concerns. Recorded as a per-project hash, never in plaintext")
}

func runFeedbackRecord(cmd *cobra.Command, args []string) error {
	if err := requireOneOf("--kind", feedbackRecordKind, agentKinds); err != nil {
		return fmt.Errorf("%w. The finding, tally and session kinds are CLI-owned: an agent asserting one would record a fact nothing observed", err)
	}
	for _, check := range []struct {
		flag, value string
		allowed     []string
	}{
		{"--phase", feedbackRecordPhase, phaseValues},
		{"--artifact", feedbackRecordArtifact, artifactValues},
		{"--changed", feedbackRecordChanged, changedValues},
		{"--needed", feedbackRecordNeeded, neededValues},
		{"--decision", feedbackRecordDecision, decisionValues},
	} {
		if check.value == "" {
			continue
		}
		if err := requireOneOf(check.flag, check.value, check.allowed); err != nil {
			return err
		}
	}

	// An explicit --run overrides the inherited one. Rarely needed: the
	// correlation id normally arrives through the environment, which is
	// what makes every CLI call and every agent event in one pipeline run
	// share it without anyone passing it around.
	if feedbackRecordRun != "" {
		feedback.SetRunID(feedbackRecordRun)
	}

	if !feedback.IsEnabled() {
		// Silent no-op, not an error. A skill instruction that failed in
		// every project with the mode off would be removed from the skill
		// within a week, and the instrumentation with it.
		return nil
	}

	feedback.Record(feedback.AgentData{
		Kind:     feedbackRecordKind,
		Skill:    feedbackRecordSkill,
		Phase:    feedbackRecordPhase,
		Artifact: feedbackRecordArtifact,
		Code:     feedbackRecordCode,
		Changed:  feedbackRecordChanged,
		Needed:   feedbackRecordNeeded,
		Decision: feedbackRecordDecision,
		Option:   feedbackRecordOption,
		// Hashed here, on receipt. The agent passes plaintext because
		// asking an LLM to hash means it gets it wrong sometimes and,
		// worse, means the plaintext sat in a command line that a shell
		// history keeps.
		Subject: feedback.Hash(feedbackRecordSubject),
	})
	return nil
}

// requireOneOf is the closed-vocabulary check every enum flag runs.
func requireOneOf(flag, value string, allowed []string) error {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return fmt.Errorf("unknown %s %q — allowed: %s", flag, value, strings.Join(allowed, " | "))
}

// feedbackStatusCmd answers "is this on, what does it collect, and where
// does it write" without making anyone read the config or the source.
var feedbackStatusCmd = &cobra.Command{
	Use:   "feedback-status",
	Short: "Report whether feedback mode is on, what it collects, and where it writes",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := mustContext(cmd)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if !feedback.IsEnabled() {
			fmt.Fprintf(out, "feedback: off\n")
			fmt.Fprintf(out,
				"Turn on for this project with `feedback: true` in .parlay/config.yaml, or for one run with %s=1.\n",
				feedback.EnvVar)
			return nil
		}

		fmt.Fprintf(out, "feedback: on\n")
		// The real file, not a placeholder. An earlier version printed a
		// literal "<date>", which is unusable by someone being asked to
		// find and send the log.
		fmt.Fprintf(out, "log:      %s\n", feedback.LogPath(cfg.Root.Path))
		if inherited := os.Getenv(feedback.RunEnvVar); inherited != "" {
			fmt.Fprintf(out, "run:      %s (inherited from %s)\n", inherited, feedback.RunEnvVar)
		} else {
			fmt.Fprintf(out, "run:      (none — standalone; set %s to correlate a pipeline run)\n", feedback.RunEnvVar)
		}
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Collected: parlay's own error codes, phase and command names, coarse timings,")
		fmt.Fprintln(out, "           and salted hashes of feature names. No file paths, no message text,")
		fmt.Fprintln(out, "           no content from your spec files. Safe to send as-is.")
		return nil
	},
}
