package commands

import "github.com/spf13/cobra"

// Every internal command declares who reaches it.
//
// The verb lint already catches a skill naming a command that does not exist.
// Nothing caught the reverse — a command no skill names — and that is the
// failure this session produced six times: working, tested code with no route
// to it from any deployed guidance.
//
// A marker commands opt INTO would be circular, because the next command that
// skips its guidance skips the marker for the same reason. So classification is
// exhaustive: an unclassified command fails conformance, and adding one forces
// the question rather than letting it be missed.
const reachabilityKey = "parlay.reachability"

const (
	// ClassProbe reads state and emits JSON. Anything may call it; nothing has
	// to. A probe nobody calls is dead weight, not a broken promise.
	ClassProbe = "probe"

	// ClassSkillRequired acquires an authority-bearing human judgment: its
	// intended successful mutation records or withdraws one. Such a command
	// cannot be reached correctly by improvisation — the question has to be put
	// to a person a particular way — so a deployed walkthrough must name it.
	//
	// The distinction is ACQUISITION, not consumption. A command that merely
	// evaluates an approval somebody else obtained, or reports that one is
	// missing, is not acquiring it; otherwise every checker that can surface a
	// human blocker would become a human-interaction command.
	ClassSkillRequired = "skill-required"

	// ClassDirectHuman is typed by a person at a terminal, so it belongs on the
	// visible CLI rather than under `internal`.
	ClassDirectHuman = "direct-human"

	// ClassPipelineHelper is invoked by the pipeline — a phase, a gate, a
	// deployed expansion — rather than by a person choosing it.
	ClassPipelineHelper = "pipeline-helper"
)

// reachability tags a command with its class.
func reachability(cmd *cobra.Command, class string) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[reachabilityKey] = class
	return cmd
}

// ReachabilityClass reports a command's declared class, or "" when it has none.
func ReachabilityClass(cmd *cobra.Command) string {
	if cmd.Annotations == nil {
		return ""
	}
	return cmd.Annotations[reachabilityKey]
}

// ValidReachabilityClasses is the closed set.
var ValidReachabilityClasses = map[string]bool{
	ClassProbe:          true,
	ClassSkillRequired:  true,
	ClassDirectHuman:    true,
	ClassPipelineHelper: true,
}
