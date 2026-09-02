// parlay-feature: parlay-tool/backlog-and-activity
// parlay-artifact: test

package agent

import (
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

const validBacklogItem = `schema_version: 1
id: 20260901T104500.000000Z-ab3f-something
kind: gap
title: Something is missing
captured:
  at: 2026-09-01T10:45:00Z
  by: claude
`

func withLine(base, extra string) string { return base + extra }

// Every published code, pinned end to end from authored bytes.
//
// Backlog had recreated activity's pre-fix coverage level: the routing
// apparatus was built but nothing asserted which code a given file
// produces, which is exactly the gap that let substring routing survive
// on the activity side.
func TestValidateBacklog_PublishedCodes(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"unknown field", strings.Replace(validBacklogItem, "kind: gap", "knid: gap", 1), BacklogCodeNotParseable},
		{"unknown kind", strings.Replace(validBacklogItem, "kind: gap", "kind: friction", 1), BacklogCodeNotParseable},
		{"unknown version", strings.Replace(validBacklogItem, "schema_version: 1", "schema_version: 9", 1), BacklogCodeNotParseable},
		{"unknown event", withLine(validBacklogItem, "history:\n  - event: mothballed\n    at: 2026-09-02T00:00:00Z\n    by: x\n"), BacklogCodeNotParseable},
		{"missing title", strings.Replace(validBacklogItem, "title: Something is missing\n", "", 1), "backlog-item-incomplete"},
		{"missing capture", strings.Replace(validBacklogItem, "  by: claude\n", "", 1), "backlog-item-capture-incomplete"},
		{"bad capture timestamp", strings.Replace(validBacklogItem, "at: 2026-09-01T10:45:00Z", "at: last tuesday", 1), "backlog-timestamp-not-rfc3339"},
		{"bad priority", withLine(validBacklogItem, "priority: urgent\n"), "backlog-item-priority-invalid"},
		// A ref nobody can resolve is durable state the scoped read
		// silently misses, so it is refused at the boundary rather than
		// stored and quietly ignored.
		{"about is free text", withLine(validBacklogItem, "about:\n  - the widget thing\n"), "backlog-about-ref-invalid"},
		{"about has an unknown kind", withLine(validBacklogItem, "about:\n  - \"@widget/newkind:x\"\n"), "backlog-about-ref-invalid"},
		{"promoted with no becomes", withLine(validBacklogItem, "history:\n  - event: promoted\n    at: 2026-09-02T00:00:00Z\n    by: x\n"), "backlog-disposition-incomplete"},
		{"declined with becomes", withLine(validBacklogItem, "history:\n  - event: declined\n    reason: no\n    becomes: \"@x\"\n    at: 2026-09-02T00:00:00Z\n    by: x\n"), "backlog-disposition-incomplete"},
		{"event after terminal", withLine(validBacklogItem, "history:\n  - event: promoted\n    becomes: \"@f\"\n    at: 2026-09-02T00:00:00Z\n    by: x\n  - event: deferred\n    reason: r\n    at: 2026-09-03T00:00:00Z\n    by: x\n"), "backlog-terminal-event-not-last"},
		{"event missing attribution", withLine(validBacklogItem, "history:\n  - event: deferred\n    reason: r\n    at: 2026-09-02T00:00:00Z\n"), "backlog-item-capture-incomplete"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := ValidateBacklog(ModeAuthoring, "item.yaml", []byte(tc.content))
			if len(out) == 0 {
				t.Fatal("expected a finding, got none")
			}
			var codes []string
			var found bool
			for _, o := range out {
				codes = append(codes, o.Code)
				if o.Code == tc.want {
					found = true
				}
				if strings.TrimSpace(o.Fix) == "" {
					t.Errorf("finding %q carries no fix", o.Code)
				}
			}
			if !found {
				t.Errorf("want %q, got %v", tc.want, codes)
			}
		})
	}
}

func TestValidateBacklog_CleanItemHasNoFindings(t *testing.T) {
	if out := ValidateBacklog(ModeAuthoring, "item.yaml", []byte(validBacklogItem)); len(out) != 0 {
		t.Fatalf("a clean item produced findings: %+v", out)
	}
}

// Distinct problems keep distinct codes rather than collapsing into the
// default. A router that answers one code for everything is no router.
func TestValidateBacklog_MultipleProblemsKeepDistinctCodes(t *testing.T) {
	content := `schema_version: 1
id: x
kind: gap
priority: urgent
title: t
about:
  - the widget thing
captured:
  at: not-a-time
  by: c
history:
  - event: promoted
    at: 2026-09-02T00:00:00Z
    by: d
`
	got := map[string]bool{}
	for _, o := range ValidateBacklog(ModeAuthoring, "item.yaml", []byte(content)) {
		got[o.Code] = true
	}
	for _, want := range []string{
		"backlog-item-priority-invalid",
		"backlog-about-ref-invalid",
		"backlog-timestamp-not-rfc3339",
		"backlog-disposition-incomplete",
	} {
		if !got[want] {
			t.Errorf("code %q collapsed away; got %v", want, keys(got))
		}
	}
}

// EXHAUSTIVENESS over the registry, with the same honest boundary as
// activity's: it catches a kind that is registered and unmapped, and
// cannot catch one declared and emitted but never registered.
func TestBacklogDiagnostic_EveryRegisteredKindIsMapped(t *testing.T) {
	for _, kind := range parser.KnownBacklogProblemKinds() {
		code, fix := BacklogDiagnostic(kind)
		if code == BacklogUnmappedCode {
			t.Errorf("kind %q has no published diagnostic", kind)
		}
		if strings.TrimSpace(code) == "" || strings.TrimSpace(fix) == "" {
			t.Errorf("kind %q maps to an empty code or fix", kind)
		}
	}
}

// An unknown kind is loud, never quietly plausible.
func TestBacklogDiagnostic_UnknownKindIsLoud(t *testing.T) {
	code, fix := BacklogDiagnostic(parser.BacklogProblemKind("invented-later"))
	if code != BacklogUnmappedCode {
		t.Errorf("want %q, got %q", BacklogUnmappedCode, code)
	}
	if !strings.Contains(fix, "bug in parlay") {
		t.Errorf("the fix should say it is a bug: %q", fix)
	}
}
