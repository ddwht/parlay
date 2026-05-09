// parlay-feature: studio-support/studio-cli-hooks
// parlay-component: studio-launch-failure-line
// parlay-artifact: test

package commands

import "testing"

func TestFormatStudioLaunchFailure(t *testing.T) {
	want := "[ERR] parlay-studio domain-edit exited with code 1 — see Studio's output above. Trio-command artifact completed before Studio launched and is on disk."
	if got := FormatStudioLaunchFailure("domain-edit", 1); got != want {
		t.Errorf("FormatStudioLaunchFailure(domain-edit, 1) = %q, want %q", got, want)
	}
}

func TestFormatStudioLaunchFailure_OtherSubcommands(t *testing.T) {
	cases := []struct {
		subcommand string
		exitCode   int
		wantPrefix string
	}{
		{"artifacts-review", 2, "[ERR] parlay-studio artifacts-review exited with code 2"},
		{"reconcile", 127, "[ERR] parlay-studio reconcile exited with code 127"},
	}
	for _, tc := range cases {
		got := FormatStudioLaunchFailure(tc.subcommand, tc.exitCode)
		if len(got) < len(tc.wantPrefix) || got[:len(tc.wantPrefix)] != tc.wantPrefix {
			t.Errorf("got %q, want prefix %q", got, tc.wantPrefix)
		}
	}
}
