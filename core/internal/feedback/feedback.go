// parlay-feature: parlay-tool/feedback-mode
// parlay-component: recorder
//
// An opt-in, local-only record of what actually happened during a run —
// every CLI invocation and its diagnostics, plus the agent-side events no
// CLI call can observe.
//
// WHY THIS EXISTS. The most expensive failures in this toolkit are not
// crashes, they are agents working around it. A build subagent authors a v2
// buildfile, gets it rejected, retries v1; omits `models:`, gets
// missing-model-reference, re-adds it. Every phase pays that independently
// and none of it is written down, so the schema that taught by rejection
// looks identical to one that taught by documentation. The only record of
// any of it has been a person watching a run and writing prose afterwards.
//
// So this captures as the work happens rather than asking anyone to recall
// it — the same reasoning the emission manifest follows. "Now describe what
// went wrong", asked at the end of a long run, is exactly the recall an
// agent gets wrong, and the retry it papered over is the part it is least
// likely to mention.
package feedback

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Dir is the subdirectory under .parlay/ that holds the logs.
const Dir = "feedback"

// EnvVar forces the mode on or off for one invocation, whatever the
// project config says.
const EnvVar = "PARLAY_FEEDBACK"

// RunEnvVar carries the correlation id for one PIPELINE run across the
// many CLI invocations it makes.
//
// This has to come from the environment, and the first version of this
// package proved why by getting it wrong: it minted a fresh id per process
// and offered `feedback-status` as the way to read it. But status is itself
// a process, so it minted its own — an agent that asked for the run id
// received the id of the asking, and every event it then filed was
// correlated to nothing. The log looked complete and answered no question,
// which is the exact failure mode this whole feature exists to detect in
// the rest of the toolkit.
//
// A pipeline run is the unit that matters — one feature through one loop,
// spanning a dozen CLI calls and the agent events between them. Only the
// driver knows where that starts, so only the driver can name it.
const RunEnvVar = "PARLAY_RUN_ID"

// Event kinds. A closed set: an open one becomes a dozen spellings of the
// same thing, and the whole point is that these are aggregatable.
const (
	// KindInvocation is one CLI run: what was asked, what it exited with,
	// how long it took. Emitted by the CLI.
	KindInvocation = "invocation"
	// KindDiagnostic is one error code reaching a user or an agent.
	// Emitted by the CLI.
	KindDiagnostic = "diagnostic"
	// KindPhase is a pipeline phase starting or finishing. Agent-emitted.
	KindPhase = "phase"
	// KindDecision is a parlay-decision block raised, and how it resolved.
	// Agent-emitted — the CLI never sees these.
	KindDecision = "decision"
	// KindRetry is the signal this whole package exists for: an agent
	// re-attempted something after a rejection. Agent-emitted, because the
	// CLI sees two invocations and cannot know they were one intent.
	KindRetry = "retry"
	// KindImprovised is an agent proceeding without a rule it needed —
	// inventing a path, guessing a convention, weakening an assertion.
	// Agent-emitted, and the most valuable entry in the log when it appears.
	KindImprovised = "improvised"
	// KindNote is free-form. Deliberately last and deliberately vague: it
	// is the escape hatch, and a log full of notes means the closed set
	// above is missing a kind that should be added.
	KindNote = "note"
)

// Event is one line of the log.
type Event struct {
	At   string `json:"at"`
	Run  string `json:"run"`
	Kind string `json:"kind"`
	// Command is the CLI command path ("validate", "internal diff"), or the
	// skill name for agent-emitted events.
	Command string `json:"command,omitempty"`
	// Data carries the kind-specific payload. Free-form on purpose: the
	// shape of what is worth knowing about a diagnostic is not the shape of
	// what is worth knowing about a retry, and forcing one struct over both
	// would flatten exactly the detail being collected.
	Data map[string]any `json:"data,omitempty"`
}

// Recorder appends events. The zero value is a valid disabled recorder, so
// every call site can be unconditional.
//
// Package-level rather than carried on config.Context because Execute()
// brackets the whole invocation and runs before any Context exists — and
// because instrumentation that each call site has to be handed is
// instrumentation that gets dropped from the call sites nobody remembered.
type Recorder struct {
	mu      sync.Mutex
	file    *os.File
	runID   string
	enabled bool
	// failed records that a write already errored. Feedback must never
	// break a command, so the first failure disables the recorder for the
	// rest of the run rather than reporting once per event.
	failed bool
}

var active = &Recorder{}

// Enabled reports whether the mode should run, given the project config's
// value and the environment.
//
// The env var overrides the config in BOTH directions, which is a
// deliberate divergence from the no_studio precedent (where flag and config
// OR together and either suppresses). This is a diagnostic mode: the two
// things a person actually wants are "turn it on for this one run without
// editing config" and "turn it off for this one run without editing
// config", and an OR merge can only ever express the first.
func Enabled(configValue bool, env func(string) string) bool {
	raw, set := "", false
	if env != nil {
		raw = strings.TrimSpace(env(EnvVar))
		set = raw != ""
	}
	if !set {
		return configValue
	}
	switch strings.ToLower(raw) {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

// Start opens the log for one invocation under the given root. Errors are
// swallowed by design — a project that cannot write its feedback log still
// has to run.
//
// runID is the pipeline-run correlation id, normally read from
// RunEnvVar by the caller. Empty means this invocation is not part of a
// tracked pipeline run and gets its own id, which is correct for someone
// running a command by hand.
func Start(rootPath string, enabled bool, runID string) {
	if !enabled || rootPath == "" {
		return
	}
	dir := filepath.Join(rootPath, ".parlay", Dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	// One file per day rather than per run: a run is the wrong grain to
	// read at, because the interesting patterns — the same code four times
	// in a row, a phase that always retries — span runs.
	path := filepath.Join(dir, time.Now().UTC().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	active.mu.Lock()
	defer active.mu.Unlock()
	active.file = f
	active.enabled = true
	if id := strings.TrimSpace(runID); id != "" {
		active.runID = id
	} else {
		active.runID = newRunID()
	}
}

// Stop closes the log. Safe to call when the mode never started.
func Stop() {
	active.mu.Lock()
	defer active.mu.Unlock()
	if active.file != nil {
		active.file.Close()
		active.file = nil
	}
	active.enabled = false
}

// IsEnabled reports whether events are being recorded this run.
func IsEnabled() bool {
	active.mu.Lock()
	defer active.mu.Unlock()
	return active.enabled && active.file != nil && !active.failed
}

// RunID is the correlation id for this invocation, so an agent-emitted
// event can be tied to the CLI calls around it.
func RunID() string {
	active.mu.Lock()
	defer active.mu.Unlock()
	return active.runID
}

// SetRunID adopts a caller-supplied correlation id. Used by the
// agent-facing record command so a skill's events join the run they belong
// to rather than opening a new one per CLI call.
func SetRunID(id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	active.mu.Lock()
	defer active.mu.Unlock()
	active.runID = id
}

// Record appends one event. A no-op when the mode is off, and it never
// returns an error: a command must not fail because its telemetry did.
func Record(kind, command string, data map[string]any) {
	active.mu.Lock()
	defer active.mu.Unlock()
	if !active.enabled || active.file == nil || active.failed {
		return
	}
	ev := Event{
		At:      time.Now().UTC().Format(time.RFC3339Nano),
		Run:     active.runID,
		Kind:    kind,
		Command: command,
		Data:    data,
	}
	line, err := json.Marshal(ev)
	if err != nil {
		active.failed = true
		return
	}
	if _, err := active.file.Write(append(line, '\n')); err != nil {
		// One failure disables the rest of the run. Retrying per event
		// would turn a full disk into a stall in every command.
		active.failed = true
	}
}

// Diagnostic records one error code reaching a user or an agent.
//
// Separate from Record because this is the entry the whole log is read for:
// which codes actually fire, in which command, how often. Giving it a
// signature means the payload keys cannot drift between call sites, which
// is the failure that makes a log unaggregatable.
func Diagnostic(command, code, message string) {
	if code == "" {
		return
	}
	Record(KindDiagnostic, command, map[string]any{
		"code":    code,
		"message": message,
	})
}

func newRunID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Time-based fallback. Uniqueness matters only within a day's log,
		// and a collision costs a mis-grouped pair of events rather than a
		// wrong answer.
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
