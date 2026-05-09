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

func TestDispatchStudioHook_PromptWordingArtifacts(t *testing.T) {
	out := &bytes.Buffer{}
	in := strings.NewReader("n\n")
	err := dispatchStudioHook(dispatchStudioHookOptions{
		Pctx:          hookCtx(t, true),
		TrioCommand:   "create-artifacts",
		Mode:          "default",
		In:            in,
		Out:           out,
		IsInteractive: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	want := "Open Studio to review the produced artifacts? (y/N) "
	if out.String() != want {
		t.Errorf("artifacts prompt = %q, want %q", out.String(), want)
	}
}

func TestDispatchStudioHook_PromptWordingSync(t *testing.T) {
	out := &bytes.Buffer{}
	in := strings.NewReader("n\n")
	err := dispatchStudioHook(dispatchStudioHookOptions{
		Pctx:          hookCtx(t, true),
		TrioCommand:   "sync",
		Mode:          "default",
		In:            in,
		Out:           out,
		IsInteractive: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	want := "Open Studio to reconcile this drift visually? (y/N) "
	if out.String() != want {
		t.Errorf("sync prompt = %q, want %q", out.String(), want)
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
	cases := map[string]string{
		"create-domain-model": "domain-edit",
		"create-artifacts":    "artifacts-review",
		"sync":                "reconcile",
	}
	for trio, want := range cases {
		got, ok := trioToStudioSubcommand[trio]
		if !ok {
			t.Errorf("missing entry for %q", trio)
			continue
		}
		if got != want {
			t.Errorf("trioToStudioSubcommand[%q] = %q, want %q", trio, got, want)
		}
	}
}
