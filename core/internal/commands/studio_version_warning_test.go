// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: version-mismatch-warning
// parlay-artifact: test

package commands

import (
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

func TestFormatStudioVersionWarning_Mismatch(t *testing.T) {
	d := config.StudioDetection{
		Detected:   true,
		BinaryPath: "/usr/local/bin/parlay-studio",
		Version:    "0.9.0",
		Reason:     config.StudioReasonDetected,
	}
	want := "warning: parlay-studio version 0.9.0 is older than expected (need >=0.1.0); some hooks may not work."
	if got := FormatStudioVersionWarning(d); got != want {
		t.Errorf("FormatStudioVersionWarning = %q, want %q", got, want)
	}
}

func TestFormatStudioVersionWarning_CompatibleStaysSilent(t *testing.T) {
	d := config.StudioDetection{
		Detected: true,
		Version:  "1.4.0",
		Reason:   config.StudioReasonDetected,
	}
	if got := FormatStudioVersionWarning(d); got != "" {
		t.Errorf("compatible version should produce empty warning, got %q", got)
	}
}

func TestFormatStudioVersionWarning_AbsentStaysSilent(t *testing.T) {
	d := config.StudioDetection{
		Detected: false,
		Version:  "0.5.0", // version present but Detected=false
	}
	if got := FormatStudioVersionWarning(d); got != "" {
		t.Errorf("not-detected should produce empty warning, got %q", got)
	}
}

func TestFormatStudioVersionWarning_VersionUnknownStaysSilent(t *testing.T) {
	d := config.StudioDetection{
		Detected: true,
		Version:  "",
		Reason:   config.StudioReasonVersionUnknown,
	}
	if got := FormatStudioVersionWarning(d); got != "" {
		t.Errorf("version-unknown should produce empty warning, got %q", got)
	}
}
