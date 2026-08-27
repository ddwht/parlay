// parlay-feature: parlay-tool/schema-consolidation
// parlay-component: validation-mode-dispatch
// parlay-artifact: test

// The schema and the severity table have to agree about which diagnostics are
// warnings, because a reader who believes a warning is a failure changes their
// artifact to avoid something that was never going to block them — and a reader
// who believes a failure is a warning ships and finds out later.
//
// buildfile.schema.md said a non-empty `models:` "fails validation". The
// validator the CLI runs emits it as a warning, and only when the project has a
// resolvable domain-model.yaml. Nothing held the two claims against each other,
// so the schema stated the opposite of the behaviour for as long as it took
// someone to test it by hand.

package agent

import (
	"regexp"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/embedded"
)

// nonFailingMarkers are the parenthesised markers a schema row uses to say a
// diagnostic does not block. Two are in use: "(warning)" and "(info)".
//
// The code has no info severity — only error and warning — so "(info)" resolves
// to warning at runtime. That is a documentation nuance rather than a drift: both
// markers tell a reader the same load-bearing thing, which is that the row will
// not fail their build. What matters is that a row saying nothing is read as a
// failure, so an unmarked row for a non-failing code is the defect this catches.
var nonFailingMarkers = []string{"(warning)", "(info)"}

func marksNonFailing(marker string) bool {
	for _, m := range nonFailingMarkers {
		if strings.Contains(marker, m) {
			return true
		}
	}
	return false
}

// codeRowWithMarker matches a schema code-table row, capturing the code and
// whatever follows it before the next column separator — which is where a
// severity marker lives when a row carries one.
var codeRowWithMarker = regexp.MustCompile(`^\|\s*` + "`" + `([a-z][a-z0-9-]+)` + "`" + `\s*([^|]*)\|`)

// TestWarningSeveritiesAreDocumentedAsWarnings holds ruleSeverityTable against
// the schemas in both directions.
func TestWarningSeveritiesAreDocumentedAsWarnings(t *testing.T) {
	rows := schemaCodeRows(t)
	if len(rows) == 0 {
		t.Fatal("parsed no schema code rows — the table format has drifted from this parser")
	}

	// Direction 1: a code the tool treats as a warning in build mode must not be
	// documented as an unmarked (i.e. failing) row.
	for code, perMode := range ruleSeverityTable {
		if perMode[ModeBuild] != SeverityWarning {
			continue
		}
		if _, graduates := graduatingCodes[code]; graduates {
			// Severity depends on the artifact's declared revision: warning
			// below the graduation version, error at or above it. A row marked
			// (warning) would tell a reader the wrong thing for every current
			// artifact, and an unmarked row would tell them the wrong thing for
			// every legacy one. The schema states the split in prose above the
			// table instead, which is the only place it fits.
			continue
		}
		marker, documented := rows[code]
		if !documented {
			// Documented in prose rather than a table, or not documented at all.
			// The table-only blind spot is recorded in the embedded conformance
			// suite; this test does not restate it.
			continue
		}
		if !marksNonFailing(marker) {
			t.Errorf("%s is a build-mode warning but its schema row carries no %v marker — a reader takes an unmarked row as a failure", code, nonFailingMarkers)
		}
	}

	// Direction 2: a row marked as non-failing must actually resolve to a
	// warning. This is the direction that caught buildfile-models-deprecated's
	// opposite — a row promising not to block while the code blocked would be the
	// worse of the two errors.
	for code, marker := range rows {
		if !marksNonFailing(marker) {
			continue
		}
		if got := RuleSeverity(code, ModeBuild); got != SeverityWarning {
			t.Errorf("%s is documented as non-failing (%s) but RuleSeverity reports %q in build mode", code, strings.TrimSpace(marker), got)
		}
	}
}

// schemaCodeRows returns every code documented in a schema table, mapped to the
// text between the code and the end of its first column.
func schemaCodeRows(t *testing.T) map[string]string {
	t.Helper()
	names, err := embedded.SchemaNames()
	if err != nil {
		t.Fatalf("SchemaNames: %v", err)
	}
	rows := map[string]string{}
	for _, name := range names {
		data, err := embedded.ReadSchema(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if m := codeRowWithMarker.FindStringSubmatch(line); m != nil {
				// A code documented in more than one schema keeps the first
				// marker seen; disagreement between two tables about the same
				// code is a separate problem this test does not adjudicate.
				if _, seen := rows[m[1]]; !seen {
					rows[m[1]] = m[2]
				}
			}
		}
	}
	return rows
}
