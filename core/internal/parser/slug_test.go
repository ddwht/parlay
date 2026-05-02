package parser

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"upgrade plan creation", "upgrade-plan-creation"},
		{"Fleet Health Overview", "fleet-health-overview"},
		{"Check Upgrade Readiness", "check-upgrade-readiness"},
		{"  spaces  everywhere  ", "spaces-everywhere"},
		{"Special! Characters? Here.", "special-characters-here"},
		{"already-slugified", "already-slugified"},
		{"UPPERCASE", "uppercase"},
	}

	for _, tt := range tests {
		got := Slugify(tt.input)
		if got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
