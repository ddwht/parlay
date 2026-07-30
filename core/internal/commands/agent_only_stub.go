package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// exitAgentOnly is the status an agent-only command exits with when invoked
// from a shell.
//
// Distinct from 1 so a caller can tell "this command needs an agent and did
// nothing" apart from "this command ran and failed". They used to be
// indistinguishable from SUCCESS: every stub printed its notice to stdout and
// returned nil, so the process exited 0 and any wrapper checking $? concluded
// the artifact had been produced. For generate-enggspec that means a pipeline
// could "succeed" at writing an engineering specification that was never
// written.
const exitAgentOnly = 2

// agentOnlyStub builds the RunE for a command that only an AI agent can
// perform. The notice goes to stderr — it is a refusal, not output — and the
// non-zero status makes the refusal visible to a script.
//
// entryPoint names how to actually run the thing. Getting this right matters:
// the previous strings named `/parlay-<name>` skills that do not exist, since
// the phase skills were consolidated into `.parlay/modules/` and are reached
// through the loop. Sending someone to a skill their install does not have is
// the same failure as exiting 0 — a confident answer that is wrong.
func agentOnlyStub(name, entryPoint string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s requires an AI agent — nothing was written.\n", name)
		fmt.Fprintf(cmd.ErrOrStderr(), "Run %s in your AI agent (e.g. Claude Code).\n", entryPoint)
		return NewExitCodeError(exitAgentOnly)
	}
}
