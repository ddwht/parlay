// parlay-feature: parlay-tool/backlog-and-activity
// parlay-artifact: test

package agent

import (
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

// The published diagnostic codes, pinned end to end.
//
// WHY THIS TABLE EXISTS. It asserts the contract a reader of the schema
// relies on: this input produces this code. Routing is now by typed kind
// rather than by message substring, so a reword can no longer change
// which code a finding carries — but the mapping from an authored file to
// a published code still crosses parse, shape validation and this
// switch, and nothing else checks that whole path.
func TestValidateActivity_PublishedCodes(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "unknown schema version is a parse refusal",
			content: "schema_version: 3\nhistory:\n  - event: parked\n    reason: x\n    at: 2026-09-01T11:00:00Z\n    by: dwht\n",
			want:    "activity-declaration-not-parseable",
		},
		// SchemaVersion and UnknownEvent are refused by the PARSER, so
		// these two arrive as parse refusals rather than through the shape
		// pass. Their shape branches exist for programmatically built
		// values and are covered parser-side.
		{
			name:    "unknown event kind is a parse refusal",
			content: "schema_version: 1\nhistory:\n  - event: mothballed\n    at: 2026-09-01T11:00:00Z\n    by: dwht\n",
			want:    "activity-declaration-not-parseable",
		},
		{
			name:    "unknown field is a parse refusal",
			content: "schema_version: 1\nhistroy: []\n",
			want:    "activity-declaration-not-parseable",
		},
		{
			name:    "empty history",
			content: "schema_version: 1\nhistory: []\n",
			want:    "activity-declaration-incomplete",
		},
		{
			name:    "missing by",
			content: "schema_version: 1\nhistory:\n  - event: unparked\n    at: 2026-09-01T11:00:00Z\n",
			want:    "activity-declaration-incomplete",
		},
		{
			name:    "missing at",
			content: "schema_version: 1\nhistory:\n  - event: unparked\n    by: dwht\n",
			want:    "activity-declaration-incomplete",
		},
		{
			name:    "parked without a reason",
			content: "schema_version: 1\nhistory:\n  - event: parked\n    at: 2026-09-01T11:00:00Z\n    by: dwht\n",
			want:    "activity-parked-without-reason",
		},
		{
			name:    "until on an activated event",
			content: "schema_version: 2\nhistory:\n  - event: activated\n    until: someday\n    at: 2026-09-01T11:00:00Z\n    by: dwht\n",
			want:    "activity-until-on-unparked",
		},
		{
			name:    "until on an unparked event",
			content: "schema_version: 1\nhistory:\n  - event: unparked\n    until: someday\n    at: 2026-09-01T11:00:00Z\n    by: dwht\n",
			want:    "activity-until-on-unparked",
		},
		{
			name:    "timestamp is not RFC3339",
			content: "schema_version: 1\nhistory:\n  - event: unparked\n    at: last tuesday\n    by: dwht\n",
			want:    "activity-timestamp-not-rfc3339",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := ValidateActivity(ModeAuthoring, "activity.yaml", []byte(tc.content))
			if len(out) == 0 {
				t.Fatalf("expected a finding, got none")
			}
			var codes []string
			var found bool
			for _, o := range out {
				codes = append(codes, o.Code)
				if o.Code == tc.want {
					found = true
				}
			}
			if !found {
				t.Errorf("want code %q, got %v", tc.want, codes)
			}
		})
	}
}

func TestValidateActivity_CleanDeclarationHasNoFindings(t *testing.T) {
	content := `schema_version: 1
history:
  - event: parked
    reason: Superseded by the shipped implementation
    until: after adapter-set v2 lands
    at: 2026-04-18T09:12:00Z
    by: dwht
  - event: unparked
    at: 2026-09-01T11:00:00Z
    by: dwht
`
	if out := ValidateActivity(ModeAuthoring, "activity.yaml", []byte(content)); len(out) != 0 {
		t.Fatalf("a clean declaration produced findings: %+v", out)
	}
}

// Distinct shape problems must keep distinct codes rather than collapsing
// into the default. A router that answers activity-declaration-incomplete
// for everything is indistinguishable from no router at all.
func TestValidateActivity_MultipleProblemsKeepDistinctCodes(t *testing.T) {
	content := `schema_version: 1
history:
  - event: parked
    at: last tuesday
  - event: unparked
    until: someday
    at: 2026-09-01T11:00:00Z
    by: dwht
`
	out := ValidateActivity(ModeAuthoring, "activity.yaml", []byte(content))
	got := map[string]bool{}
	for _, o := range out {
		got[o.Code] = true
	}
	for _, want := range []string{
		"activity-parked-without-reason",  // first event has no reason
		"activity-timestamp-not-rfc3339",  // ...and an unparseable at
		"activity-declaration-incomplete", // ...and no by
		"activity-until-on-unparked",      // second event misuses until
	} {
		if !got[want] {
			t.Errorf("code %q collapsed away; got %v", want, keys(got))
		}
	}
}

// Every finding must carry a Fix. A diagnostic that names a problem and
// not its remedy sends the reader back to the schema to work out what the
// tool already knew.
func TestValidateActivity_EveryFindingCarriesAFix(t *testing.T) {
	content := "schema_version: 1\nhistory:\n  - event: parked\n    at: last tuesday\n"
	for _, o := range ValidateActivity(ModeAuthoring, "activity.yaml", []byte(content)) {
		if strings.TrimSpace(o.Fix) == "" {
			t.Errorf("finding %q has no fix", o.Code)
		}
		if strings.TrimSpace(o.Context) == "" {
			t.Errorf("finding %q has no context", o.Code)
		}
	}
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Every REGISTERED kind is mapped.
//
// The honest boundary: this walks parser.KnownActivityProblemKinds, so it
// catches a kind that is registered and unmapped. It does NOT catch a kind
// that is declared and emitted but never registered — Go cannot enumerate
// the constants of a string-backed enum, so no test here can. Registering
// a new kind is a convention the registry asks for, not one it enforces;
// the parser-side TestShapeProblems_EmittedKindsMatchTheRegistry is what
// keeps the registry honest about what the validator actually emits.
func TestActivityDiagnostic_EveryRegisteredKindIsMapped(t *testing.T) {
	for _, kind := range parser.KnownActivityProblemKinds() {
		code, fix := ActivityDiagnostic(kind)
		if code == ActivityUnmappedCode {
			t.Errorf("kind %q has no published diagnostic", kind)
		}
		if strings.TrimSpace(code) == "" {
			t.Errorf("kind %q maps to an empty code", kind)
		}
		if strings.TrimSpace(fix) == "" {
			t.Errorf("kind %q maps to an empty fix", kind)
		}
	}
}

// An unknown kind must be loud, never quietly plausible. Mapping it to a
// real published code would produce a mis-routed finding that looks
// correct to everyone downstream.
func TestActivityDiagnostic_UnknownKindIsLoud(t *testing.T) {
	code, fix := ActivityDiagnostic(parser.ActivityProblemKind("invented-later"))
	if code != ActivityUnmappedCode {
		t.Errorf("an unknown kind must report %q, got %q", ActivityUnmappedCode, code)
	}
	if !strings.Contains(fix, "bug in parlay") {
		t.Errorf("the fix should say it is a bug, got %q", fix)
	}
}

// The two remedies that name versions must name the RIGHT ones.
//
// A v1 declaration is valid — it reads through the chain and validates
// clean — so the parse-refusal remedy must not tell its author to
// rewrite. The shape remedy is reached only by a programmatically built
// Activity, since a parsed one has already been migrated, so it names the
// version the tool writes.
func TestActivityRemedies_NameTheRightVersions(t *testing.T) {
	if !strings.Contains(ActivityFixNotParseable, "1 or 2") {
		t.Errorf("the parse remedy must name the supported range, not one version: %q", ActivityFixNotParseable)
	}
	if strings.Contains(ActivityFixNotParseable, "schema_version 2 and") {
		t.Errorf("the parse remedy tells a valid v1 author to rewrite: %q", ActivityFixNotParseable)
	}

	_, shapeFix := ActivityDiagnostic(parser.ProblemSchemaVersion)
	if !strings.Contains(shapeFix, "2") {
		t.Errorf("the shape remedy must name the current authoring version: %q", shapeFix)
	}

	_, untilFix := ActivityDiagnostic(parser.ProblemUntilOnUnparked)
	if strings.Contains(untilFix, "unparked event") {
		t.Errorf("the until remedy fires for any non-parked event and must say so: %q", untilFix)
	}
	if !strings.Contains(untilFix, "non-parked") {
		t.Errorf("the until remedy should name the general case: %q", untilFix)
	}
}
