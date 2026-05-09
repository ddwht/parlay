// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: studio-status-line
// parlay-artifact: test

package commands

import (
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

func TestFormatStudioStatusLine_Detected(t *testing.T) {
	d := config.StudioDetection{
		Detected:   true,
		BinaryPath: "/usr/local/bin/parlay-studio",
		Version:    "1.4.0",
		Reason:     config.StudioReasonDetected,
	}
	got := FormatStudioStatusLine(d, false)
	want := "Studio detected: /usr/local/bin/parlay-studio (version 1.4.0)"
	if got != want {
		t.Errorf("FormatStudioStatusLine = %q, want %q", got, want)
	}
}

func TestFormatStudioStatusLine_AbsentSilent(t *testing.T) {
	d := config.StudioDetection{
		Detected: false,
		Reason:   config.StudioReasonAbsentFromPath,
	}
	if got := FormatStudioStatusLine(d, false); got != "" {
		t.Errorf("FormatStudioStatusLine should be empty for absent, got %q", got)
	}
	// Even verbose stays silent for absent — only not-executable
	// surfaces a verbose diagnostic.
	if got := FormatStudioStatusLine(d, true); got != "" {
		t.Errorf("FormatStudioStatusLine verbose should be empty for absent, got %q", got)
	}
}

func TestFormatStudioStatusLine_NotExecutableVerbose(t *testing.T) {
	d := config.StudioDetection{
		Detected:   false,
		BinaryPath: "/usr/local/bin/parlay-studio",
		Reason:     config.StudioReasonNotExecutable,
	}
	if got := FormatStudioStatusLine(d, false); got != "" {
		t.Errorf("normal mode should stay silent for not-executable, got %q", got)
	}
	want := "studio: not detected (found at /usr/local/bin/parlay-studio but not executable)"
	if got := FormatStudioStatusLine(d, true); got != want {
		t.Errorf("verbose not-executable: got %q, want %q", got, want)
	}
}
