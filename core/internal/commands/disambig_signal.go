package commands

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/ddwht/parlay/core/internal/config"
)

// AmbiguityExitCode is the distinct exit code parlay uses when it
// would otherwise prompt for disambiguation but the caller has opted
// into structured signaling. Skills wrapping the CLI catch this code,
// re-prompt the user via the agent's question mechanism, and re-invoke
// parlay with `--root <chosen>` (or a prefixed feature reference).
const AmbiguityExitCode = 11

// AmbiguityTrigger names which case produced an ambiguity signal.
type AmbiguityTrigger string

const (
	// TriggerAmbiguousActiveRoot fires when walk-up resolution fails but
	// candidate roots are discoverable (below cwd, in a parent's roots
	// index, etc.).
	TriggerAmbiguousActiveRoot AmbiguityTrigger = "ambiguous-active-root"

	// TriggerAmbiguousFeatureReference fires when a bare feature ref
	// matches in multiple registered roots. Reserved — emitted by
	// command-level resolvers, not by the persistent PreRunE.
	TriggerAmbiguousFeatureReference AmbiguityTrigger = "ambiguous-feature-reference"
)

// AmbiguitySignal is the JSON shape printed to stderr when
// --ambiguity-as-signal is set and an ambiguity is detected. Skills
// parse this and present the candidate list to the user via
// AskUserQuestion.
type AmbiguitySignal struct {
	Kind       string             `json:"kind"`
	Trigger    AmbiguityTrigger   `json:"trigger"`
	Candidates []config.Candidate `json:"candidates"`
	// Hint is a short human-readable string suggesting how to re-invoke
	// the CLI with the user's choice.
	Hint string `json:"hint"`
}

// emitAmbiguitySignal writes the envelope as a single JSON line to w.
func emitAmbiguitySignal(w io.Writer, sig AmbiguitySignal) error {
	sig.Kind = "ambiguity"
	data, err := json.Marshal(sig)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}
