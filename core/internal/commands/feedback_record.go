// parlay-feature: parlay-tool/feedback-mode
// parlay-component: agent-event-intake
//
// The agent's way into the feedback log. Skills call this rather than
// appending to the file themselves, for three reasons: the format stays in
// one place, the enablement check stays in one place, and a skill running
// in a project with the mode off does nothing rather than creating a log
// nobody asked for.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ddwht/parlay/core/internal/feedback"
	"github.com/spf13/cobra"
)

var (
	feedbackRecordKind    string
	feedbackRecordCommand string
	feedbackRecordRun     string
	feedbackRecordData    []string
)

var feedbackRecordCmd = &cobra.Command{
	Use:   "feedback-record",
	Short: "Append one agent-side event to the feedback log (no-op when feedback mode is off)",
	Args:  cobra.NoArgs,
	RunE:  runFeedbackRecord,
}

func init() {
	feedbackRecordCmd.Flags().StringVar(&feedbackRecordKind, "kind", "",
		"Event kind: "+strings.Join(agentEventKinds(), ", "))
	feedbackRecordCmd.MarkFlagRequired("kind")
	feedbackRecordCmd.Flags().StringVar(&feedbackRecordCommand, "skill", "",
		"The skill or phase emitting the event")
	feedbackRecordCmd.Flags().StringVar(&feedbackRecordRun, "run", "",
		"Override the correlation id; normally unnecessary because "+feedback.RunEnvVar+" is inherited from the environment")
	feedbackRecordCmd.Flags().StringArrayVar(&feedbackRecordData, "data", nil,
		"key=value payload entry; repeatable")
}

// agentEventKinds is the subset a skill may emit. The CLI-owned kinds are
// excluded deliberately: an agent claiming to have run an invocation, or
// produced a diagnostic, would put a fact in the log that nothing observed.
func agentEventKinds() []string {
	return []string{
		feedback.KindPhase,
		feedback.KindDecision,
		feedback.KindRetry,
		feedback.KindImprovised,
		feedback.KindNote,
	}
}

func runFeedbackRecord(cmd *cobra.Command, args []string) error {
	allowed := map[string]bool{}
	for _, k := range agentEventKinds() {
		allowed[k] = true
	}
	if !allowed[feedbackRecordKind] {
		return fmt.Errorf("unknown --kind %q — an agent may emit: %s. The invocation and diagnostic kinds are CLI-owned, because an agent asserting one would record a fact nothing observed",
			feedbackRecordKind, strings.Join(agentEventKinds(), ", "))
	}

	data, err := parseDataPairs(feedbackRecordData)
	if err != nil {
		return err
	}

	// An explicit --run overrides the inherited one. Rarely needed: the
	// correlation id normally arrives through the environment, which is
	// what makes every CLI call and every agent event in one pipeline run
	// share it without anyone passing it around.
	if feedbackRecordRun != "" {
		feedback.SetRunID(feedbackRecordRun)
	}

	if !feedback.IsEnabled() {
		// Silent no-op, not an error. A skill instruction that fails in
		// every project with the mode off would be removed from the skill
		// within a week, and the instrumentation with it.
		return nil
	}

	feedback.Record(feedbackRecordKind, feedbackRecordCommand, data)
	return nil
}

// parseDataPairs turns repeated key=value flags into the event payload.
//
// Splits on the FIRST "=", so a value may contain more of them — the
// values here are free text ("rejected: capabilities-unknown-term=widget"),
// and splitting on the last would truncate exactly the detail worth
// recording. Same rule as review-coverage's --exempt.
func parseDataPairs(pairs []string) (map[string]any, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(pairs))
	for _, p := range pairs {
		i := strings.Index(p, "=")
		if i <= 0 {
			return nil, fmt.Errorf("--data %q: expected key=value", p)
		}
		key := strings.TrimSpace(p[:i])
		val := p[i+1:]
		if key == "" {
			return nil, fmt.Errorf("--data %q: empty key", p)
		}
		// A JSON value is kept as JSON so counts stay numbers and lists
		// stay lists; anything else is a plain string.
		var parsed any
		if err := json.Unmarshal([]byte(val), &parsed); err == nil {
			out[key] = parsed
		} else {
			out[key] = val
		}
	}
	return out, nil
}

// feedbackStatusCmd answers "is this on, and where is it writing" without
// making anyone read the config to find out.
var feedbackStatusCmd = &cobra.Command{
	Use:   "feedback-status",
	Short: "Report whether feedback mode is on for this project and where it writes",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := mustContext(cmd)
		if err != nil {
			return err
		}
		enabled := feedback.IsEnabled()
		fmt.Fprintf(cmd.OutOrStdout(), "feedback: %s\n", onOff(enabled))
		if enabled {
			fmt.Fprintf(cmd.OutOrStdout(), "log:      %s\n",
				strings.Join([]string{cfg.Root.Path, ".parlay", feedback.Dir, "<date>.jsonl"}, "/"))
			// Reports the INHERITED id, and says so when there isn't one.
			// An earlier version printed feedback.RunID() unconditionally,
			// which meant this command minted an id and handed back the id
			// of the asking — every event correlated against it then joined
			// a run consisting of one status call.
			if inherited := os.Getenv(feedback.RunEnvVar); inherited != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "run:      %s (inherited from %s)\n", inherited, feedback.RunEnvVar)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(),
					"run:      (none — this invocation is standalone; set %s to correlate a pipeline run)\n",
					feedback.RunEnvVar)
			}
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(),
			"Turn on for this project with `feedback: true` in .parlay/config.yaml, or for one run with %s=1.\n",
			feedback.EnvVar)
		return nil
	},
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
