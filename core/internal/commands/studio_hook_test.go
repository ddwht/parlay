// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: studio-hook-shared-helper
// parlay-artifact: test

package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

// hookCtx builds a *config.Context with a known StudioDetection
// installed. Used by every dispatch test.
func hookCtx(t *testing.T, detected bool) *config.Context {
	t.Helper()
	c := config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{
			Name: "test", Path: "/workspace/example-project", Kind: config.RootKindStandalone,
		},
	}, nil)
	d := config.StudioDetection{Reason: config.StudioReasonAbsentFromPath}
	if detected {
		d = config.StudioDetection{
			Detected:   true,
			BinaryPath: "/usr/local/bin/parlay-studio",
			Version:    "1.4.0",
			Reason:     config.StudioReasonDetected,
		}
	}
	c.SetStudioDetection(d)
	return c
}

// boolPtr returns &b — used to drive the IsInteractive override.
func boolPtr(b bool) *bool { return &b }

func TestDispatchStudioHook_NoStudioFlagShortCircuits(t *testing.T) {
	out := &bytes.Buffer{}
	in := strings.NewReader("y\n")
	err := dispatchStudioHook(dispatchStudioHookOptions{
		Pctx:          hookCtx(t, true),
		TrioCommand:   "create-domain-model",
		Mode:          "brownfield",
		NoStudio:      true,
		In:            in,
		Out:           out,
		IsInteractive: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("expected no prompt output when NoStudio=true, got %q", out.String())
	}
}

func TestDispatchStudioHook_NotDetectedShortCircuits(t *testing.T) {
	out := &bytes.Buffer{}
	in := strings.NewReader("y\n")
	err := dispatchStudioHook(dispatchStudioHookOptions{
		Pctx:          hookCtx(t, false),
		TrioCommand:   "create-domain-model",
		Mode:          "brownfield",
		In:            in,
		Out:           out,
		IsInteractive: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("expected no prompt output when not detected, got %q", out.String())
	}
}

func TestDispatchStudioHook_NonInteractiveShortCircuits(t *testing.T) {
	out := &bytes.Buffer{}
	in := strings.NewReader("y\n")
	err := dispatchStudioHook(dispatchStudioHookOptions{
		Pctx:          hookCtx(t, true),
		TrioCommand:   "create-domain-model",
		Mode:          "brownfield",
		In:            in,
		Out:           out,
		IsInteractive: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("non-interactive should suppress prompt, got %q", out.String())
	}
}

func TestDispatchStudioHook_PromptWordingBrownfield(t *testing.T) {
	out := &bytes.Buffer{}
	in := strings.NewReader("n\n") // decline so we don't try to exec
	err := dispatchStudioHook(dispatchStudioHookOptions{
		Pctx:          hookCtx(t, true),
		TrioCommand:   "create-domain-model",
		Mode:          "brownfield",
		In:            in,
		Out:           out,
		IsInteractive: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	want := "Open Studio's Domain Model Editor against this model? (y/N) "
	if out.String() != want {
		t.Errorf("brownfield prompt = %q, want %q", out.String(), want)
	}
}

func TestDispatchStudioHook_PromptWordingGreenfield(t *testing.T) {
	out := &bytes.Buffer{}
	in := strings.NewReader("\n")
	err := dispatchStudioHook(dispatchStudioHookOptions{
		Pctx:          hookCtx(t, true),
		TrioCommand:   "create-domain-model",
		Mode:          "greenfield",
		In:            in,
		Out:           out,
		IsInteractive: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	want := "Empty domain model created — ready to author. Open Studio's Domain Model Editor? (y/N) "
	if out.String() != want {
		t.Errorf("greenfield prompt = %q, want %q", out.String(), want)
	}
}

func TestDispatchStudioHook_DeclineDoesNotInvokeStudio(t *testing.T) {
	// "n" answer must NOT trigger a subprocess; if it did, the test
	// would fail because /usr/local/bin/parlay-studio doesn't exist
	// in the test environment (and the failure-line would land on
	// stderr).
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	in := strings.NewReader("n\n")
	err := dispatchStudioHook(dispatchStudioHookOptions{
		Pctx:          hookCtx(t, true),
		TrioCommand:   "create-domain-model",
		Mode:          "brownfield",
		In:            in,
		Out:           out,
		ErrOut:        errOut,
		IsInteractive: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("decline should produce nil err, got %v", err)
	}
	if errOut.Len() != 0 {
		t.Errorf("decline should leave stderr empty, got %q", errOut.String())
	}
}

func TestDispatchStudioHook_UnknownTrioCommandIsError(t *testing.T) {
	out := &bytes.Buffer{}
	in := strings.NewReader("y\n")
	err := dispatchStudioHook(dispatchStudioHookOptions{
		Pctx:          hookCtx(t, true),
		TrioCommand:   "bogus-command",
		Mode:          "default",
		In:            in,
		Out:           out,
		IsInteractive: boolPtr(true),
	})
	if err == nil {
		t.Fatalf("expected error for unknown trio command")
	}
}

func TestReadYNAnswer(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"n\n", false},
		{"\n", false},
		{"", false},
	}
	for _, tc := range cases {
		got := readYNAnswer(strings.NewReader(tc.input))
		if got != tc.want {
			t.Errorf("readYNAnswer(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestTrioToStudioSubcommandTable(t *testing.T) {
	// Only create-domain-model. `artifacts-review` and `reconcile` were named
	// here and implemented nowhere, so the map pointed at subcommands that did
	// not exist — and once unknown commands started exiting 1, accepting either
	// prompt made a successful trio command return an error.
	if got, want := len(trioToStudioSubcommand), 1; got != want {
		t.Fatalf("trioToStudioSubcommand has %d entries, want %d: %v",
			got, want, trioToStudioSubcommand)
	}
	if got := trioToStudioSubcommand["create-domain-model"]; got != "domain-edit" {
		t.Errorf("trioToStudioSubcommand[create-domain-model] = %q, want domain-edit", got)
	}
	for _, unbuilt := range []string{"create-artifacts", "sync"} {
		if sub, ok := trioToStudioSubcommand[unbuilt]; ok {
			t.Errorf("%q maps to %q, but no such surface exists — re-adding it "+
				"resurrects a prompt that can only fail", unbuilt, sub)
		}
	}
}

// The wordings for the two dropped hooks must not come back on their own. A
// wording with no dispatch behind it is how the broken offer survived review.
func TestDroppedHookWordingsAreGone(t *testing.T) {
	for _, trio := range []string{"create-artifacts", "sync"} {
		if _, ok := hookPromptWording[trio]; ok {
			t.Errorf("hookPromptWording still carries %q — the surface it offers is unbuilt", trio)
		}
	}
	if _, ok := hookPromptWording["create-domain-model"]; !ok {
		t.Error("create-domain-model wording disappeared; that hook works and must stay")
	}
}
