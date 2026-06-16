// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-artifact: test
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/manifest-based-ownership-atomic-writes-idempotency

package deployer

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestFileStatusString(t *testing.T) {
	cases := []struct {
		status FileStatus
		want   string
	}{
		{StatusWritten, "written"},
		{StatusUnchanged, "unchanged"},
		{StatusOrphan, "orphan"},
		{StatusFailed, "failed"},
	}
	for _, c := range cases {
		if got := c.status.String(); got != c.want {
			t.Fatalf("FileStatus(%d).String() = %q, want %q", c.status, got, c.want)
		}
	}
}

func TestPrintSummaryEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintSummary(&buf, nil); err != nil {
		t.Fatalf("PrintSummary on empty slice returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "no files to report") {
		t.Fatalf("expected 'no files to report'; got %q", buf.String())
	}
}

func TestPrintSummaryLineShape(t *testing.T) {
	entries := []FileStatusEntry{
		{Path: ".claude/skills/parlay-design-loop/SKILL.md", Status: StatusWritten, Source: "parlay-design-loop"},
		{Path: ".claude/skills/parlay-old-thing/SKILL.md", Status: StatusOrphan, Source: ""},
	}
	var buf bytes.Buffer
	if err := PrintSummary(&buf, entries); err != nil {
		t.Fatalf("PrintSummary: %v", err)
	}
	line := regexp.MustCompile(`^(written|unchanged|orphan|failed): \S+ \(source: [^)]*\)( — .+)?$`)
	for _, l := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if !line.MatchString(l) {
			t.Fatalf("line %q does not match documented shape", l)
		}
	}
}

func TestPrintSummaryFailedIncludesError(t *testing.T) {
	entries := []FileStatusEntry{
		{Path: ".claude/skills/parlay-design-loop/SKILL.md", Status: StatusFailed, Source: "parlay-design-loop", Err: errors.New("disk full")},
	}
	var buf bytes.Buffer
	if err := PrintSummary(&buf, entries); err != nil {
		t.Fatalf("PrintSummary: %v", err)
	}
	if !strings.Contains(buf.String(), "failed:") {
		t.Fatalf("expected 'failed:' prefix; got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "disk full") {
		t.Fatalf("expected error text on failed line; got %q", buf.String())
	}
}

// TestStatusValuesAreExhaustive — the four-status enum is feature-stable.
// External tooling that parses deployer stdout MAY rely on these literals;
// a fifth value would be a breaking change.
func TestStatusValuesAreExhaustive(t *testing.T) {
	want := map[string]bool{
		"written":   true,
		"unchanged": true,
		"orphan":    true,
		"failed":    true,
	}
	got := map[string]bool{}
	for _, s := range []FileStatus{StatusWritten, StatusUnchanged, StatusOrphan, StatusFailed} {
		got[s.String()] = true
	}
	if len(got) != len(want) {
		t.Fatalf("status set size = %d, want %d", len(got), len(want))
	}
	for k := range want {
		if !got[k] {
			t.Fatalf("status set missing %q", k)
		}
	}
}
