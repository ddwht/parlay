// parlay-section: cross-cutting
// parlay-extends: studio-support/studio-cli-hooks/studio-hook-shared-helper
//
// dispatchStudioHook is the single emission point for the "Open
// Studio?" prompt. Every trio command (create-domain-model,
// create-artifacts, sync) calls into this dispatch — they do not
// write the prompt text themselves. Three gates guard the prompt:
//
//  1. TTY interactivity: stdin AND stdout must both be a terminal.
//  2. Studio detection: pctx.StudioDetection().Detected must be true.
//  3. Per-invocation suppression: --no-studio must not have been
//     passed and parlay.no_studio must not be true in project config.
//
// Only when all three allow it does the helper print the one-line
// wording supplied by the trio command and read y/N. On y, it
// invokes the appropriate parlay-studio subcommand synchronously via
// os/exec with inherited stdin/stdout/stderr, waits for it to exit,
// and propagates a non-zero exit code to the trio command. There is
// no per-session memory: each invocation re-runs all gates fresh.

package commands

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
)

// trioToStudioSubcommand maps each trio command name to the
// parlay-studio subcommand it should hand off to on confirmation. The
// table is intentionally hard-coded here — the trio set is closed and
// the surface contract is part of this feature's design.
// Only create-domain-model remains. `artifacts-review` and `reconcile` were
// named here and implemented nowhere: Studio's dispatcher recognized neither,
// so the map pointed at subcommands that did not exist. That was survivable
// while unknown args fell through to a bare server boot; once unknown commands
// started exiting 1, accepting either prompt made a successful trio command
// return an error.
//
// The two surfaces they were meant to open — visual artifact review and visual
// drift reconciliation — remain unbuilt and are recorded as deferred in
// spec/intents/studio-support/studio-cli-hooks/. Naming them here again should
// wait until something answers.
var trioToStudioSubcommand = map[string]string{
	"create-domain-model": "domain-edit",
}

// hookPromptWording maps a (trio-command, mode) pair to the exact
// one-line prompt text. Wording differs only between
// create-domain-model's brownfield and greenfield modes; the other
// trio commands use a single "default" wording each. The strings are
// load-bearing — testcases assert byte-for-byte matches.
var hookPromptWording = map[string]map[string]string{
	"create-domain-model": {
		"brownfield": "Open Studio's Domain Model Editor against this model? (y/N) ",
		"greenfield": "Empty domain model created — ready to author. Open Studio's Domain Model Editor? (y/N) ",
	},
}

// dispatchStudioHookOptions packages the inputs to a single hook
// dispatch. Trio commands assemble an options struct and call
// dispatchStudioHook with it. Keeping the inputs in a struct (rather
// than a long positional argument list) lets callers omit fields they
// don't need, e.g. featureCtx for create-domain-model.
type dispatchStudioHookOptions struct {
	// Pctx is the resolved parlay context — must be non-nil for the
	// detection gate to mean anything.
	Pctx *config.Context

	// TrioCommand names the trio command requesting the hook. Must be
	// one of the keys in trioToStudioSubcommand; calls with an
	// unknown name produce an error rather than a silent no-op so
	// authoring mistakes surface during testing.
	TrioCommand string

	// Mode is the wording variant — "brownfield" / "greenfield" for
	// create-domain-model, "default" for the others.
	Mode string

	// NoStudio is the merged --no-studio + project-config disable
	// signal computed by the trio command. Logical OR of the flag
	// value and config.NoStudio; when true, the hook short-circuits
	// before any TTY or detection check.
	NoStudio bool

	// FeatureCtx is the feature/page reference the trio command was
	// run against. Empty for create-domain-model (the model is
	// project-scoped). Forwarded to the parlay-studio subprocess as
	// its first positional argument when non-empty.
	FeatureCtx string

	// In and Out / ErrOut override the default os.Stdin / os.Stdout /
	// os.Stderr — used by tests to drive the prompt with a fixed
	// reader and capture the printed text. Production callers leave
	// these nil to inherit the process streams.
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer

	// IsInteractive overrides the live TTY probe. Tests set this to
	// true or false explicitly so they don't depend on the test
	// runner's controlling terminal. Production callers leave it
	// nil; the dispatch then probes os.Stdin and os.Stdout via
	// go-isatty.
	IsInteractive *bool
}

// dispatchStudioHook runs the three gates and, when all allow,
// prints the prompt and (on y) hands off to parlay-studio. Returns
// nil on no-prompt paths and on a successful Studio run; returns a
// non-nil error when Studio exited non-zero so the trio command can
// propagate the failure.
func dispatchStudioHook(opts dispatchStudioHookOptions) error {
	if opts.Pctx == nil {
		return nil
	}
	if opts.NoStudio {
		// Gate 3 fires first — --no-studio short-circuits before any
		// TTY or detection probe. This matches the testcase that
		// passing --no-studio "short-circuits before any prompt logic".
		return nil
	}

	detection := opts.Pctx.StudioDetection()
	if !detection.Detected {
		return nil
	}

	in := opts.In
	if in == nil {
		in = os.Stdin
	}
	out := opts.Out
	if out == nil {
		out = os.Stdout
	}
	errOut := opts.ErrOut
	if errOut == nil {
		errOut = os.Stderr
	}

	if !ttyInteractive(opts.IsInteractive) {
		return nil
	}

	wording, ok := lookupHookWording(opts.TrioCommand, opts.Mode)
	if !ok {
		return fmt.Errorf("dispatch studio hook: unknown trio-command/mode %q/%q", opts.TrioCommand, opts.Mode)
	}

	// The trio map still gates which commands may offer the editor at all —
	// it holds only create-domain-model now, the two entries whose surfaces
	// were never built having been removed. What it no longer supplies is a
	// subcommand name to exec.
	if _, ok := trioToStudioSubcommand[opts.TrioCommand]; !ok {
		return fmt.Errorf("dispatch studio hook: trio %q does not offer the editor", opts.TrioCommand)
	}

	fmt.Fprint(out, wording)
	answer := readYNAnswer(in)
	if !answer {
		return nil
	}

	// In-process. This used to shell out to a second binary via
	// invokeStudioSubprocess, which meant locating it on PATH, checking its
	// version, normalizing a start failure to exit code 127, and printing a
	// launch-failure line when it exited non-zero. With one binary none of that
	// has an analogue: the editor either returns an error or it does not.
	return OpenDomainEditor(context.Background(), opts.Pctx.Root.Path)
}

// ttyInteractive returns true exactly when both stdin and stdout are
// attached to a terminal. The override lets tests inject a fixed
// answer without standing up a fake PTY. The live probe uses
// os.File.Stat — a character-device file with no Mode().IsRegular()
// bit means stdin/stdout is a TTY rather than a pipe or redirected
// file. Avoids pulling in golang.org/x/term or mattn/go-isatty.
func ttyInteractive(override *bool) bool {
	if override != nil {
		return *override
	}
	return isCharDevice(os.Stdin) && isCharDevice(os.Stdout)
}

// isCharDevice returns true when f's stat reports the character-device
// bit set — the cross-platform proxy for "this is a TTY" without an
// extra dependency.
func isCharDevice(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// lookupHookWording returns the exact prompt string for a trio
// command + mode pair. Returns (string, false) when the pair is not
// in the table — callers translate this into an error.
func lookupHookWording(trio, mode string) (string, bool) {
	byMode, ok := hookPromptWording[trio]
	if !ok {
		return "", false
	}
	w, ok := byMode[mode]
	return w, ok
}

// readYNAnswer reads one line of input and returns true exactly when
// the trimmed lowercase value starts with "y". Default (Enter) and
// "n" / "no" produce false. EOF on the input stream is treated as
// "no" so a non-interactive caller that didn't gate on TTY but
// reached this code anyway gets the safe answer.
func readYNAnswer(in io.Reader) bool {
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	if answer == "" {
		return false
	}
	return strings.HasPrefix(answer, "y")
}
