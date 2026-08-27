// parlay-feature: parlay-tool/criterion-authority
// parlay-component: transitional-severity-graduation
//
// When does a transitional warning become an error?
//
// Eight diagnostics sit at warning severity in build mode on one shared
// argument: every artifact in existence predates the field they check, so
// erroring would fail every project at once over a fact none of them could have
// recorded. The severity, as their comments say, "states the direction of
// travel" and "graduates to error once projects have rebuilt".
//
// Nothing encoded when that had happened. `schema_version` stays 2 whether a
// file was written before the field existed or by a current generator that
// simply omitted it, so the validator could not tell a legacy artifact from a
// new deficient one — and the promised graduation could never arrive. A fresh
// build leaving every criterion undischarged passed with warnings, forever.
//
// The discriminator is the artifact's own declared revision, which already
// exists and was inert: testcasesV2Shape decoded schema_version and nothing
// read it, v1-versus-v2 being discriminated by per-suite `kind:` instead. So
// this wires a field the format already had rather than inventing a parallel
// marker beside an unused one. It also follows the documented house rule:
// schema-versioning.schema.md assigns both artifacts the Regenerate policy,
// where an older version means REBUILD — which is exactly the remedy.

package agent

// Artifact revisions at which the transitional checks become binding.
//
// A file at or above these is one a current generator produced, so the facts
// these codes check are ones it could have recorded. Below, the warning stands
// and the remedy is to rebuild.
const (
	// TestcasesGraduationVersion is the shape carrying criterion identity on
	// every case and a file: on every suite.
	TestcasesGraduationVersion = 3
	// CapabilitiesGraduationVersion is the shape carrying source: on every
	// operation.
	CapabilitiesGraduationVersion = 2
)

// graduatingCode names an artifact and the revision at which one transitional
// diagnostic starts blocking.
type graduatingCode struct {
	artifact string
	minimum  int
}

// graduatingCodes is the CLOSED set, table-driven rather than matched on
// anything about the codes themselves.
//
// The set is not identifiable by comment phrasing — one member says "for the
// same reason" where the others name their rationale explicitly, and matching
// on wording would have missed it. It is not identifiable by severity either:
// twenty-three codes are warnings in build mode and most are warnings for
// entirely different reasons.
//
// Deliberately absent, and they must stay absent:
//   - surface-fragment-no-criteria and capability-operation-no-criteria are
//     judgment calls. A single fragment carrying no criteria may be structural
//     or assembly-only, which is why only the aggregate blocks.
//   - verify-criterion-duplicate reports a defect in the CONTRACT, not in the
//     file being validated, so it warns rather than failing a file whose author
//     may not own the problem.
var graduatingCodes = map[string]graduatingCode{
	"testcases-file-missing":                     {"testcases", TestcasesGraduationVersion},
	"testcases-case-criterion-missing":           {"testcases", TestcasesGraduationVersion},
	"verify-criterion-uncovered":                 {"testcases", TestcasesGraduationVersion},
	"testcases-criterion-ref-unknown":            {"testcases", TestcasesGraduationVersion},
	"testcases-criterion-text-missing":           {"testcases", TestcasesGraduationVersion},
	"testcases-criterion-text-drift":             {"testcases", TestcasesGraduationVersion},
	"testcases-cross-kind-criterion-unexercised": {"testcases", TestcasesGraduationVersion},
	"capabilities-source-missing":                {"capabilities", CapabilitiesGraduationVersion},
}

// ArtifactRevisions carries the declared revisions of the artifacts in play.
// A zero means "not declared", which reads as legacy — the conservative
// direction, since it keeps a warning rather than failing a file that may
// predate the field.
type ArtifactRevisions struct {
	Testcases    int
	Capabilities int
}

func (r ArtifactRevisions) revisionOf(artifact string) int {
	switch artifact {
	case "testcases":
		return r.Testcases
	case "capabilities":
		return r.Capabilities
	}
	return 0
}

// GraduatedSeverity resolves a code's severity for artifacts at these
// revisions.
//
// Only build mode graduates. Authoring is where a file is being written and a
// half-finished one is the normal state; the boundary that matters is the one
// where the artifact is handed downstream.
func GraduatedSeverity(code string, mode ValidationMode, revs ArtifactRevisions) Severity {
	base := RuleSeverity(code, mode)
	if mode != ModeBuild || base != SeverityWarning {
		return base
	}
	g, ok := graduatingCodes[code]
	if !ok {
		return base
	}
	if revs.revisionOf(g.artifact) >= g.minimum {
		return SeverityError
	}
	return base
}

// GraduatingCodes exposes the closed set for reporting and conformance.
func GraduatingCodes() map[string]int {
	out := make(map[string]int, len(graduatingCodes))
	for code, g := range graduatingCodes {
		out[code] = g.minimum
	}
	return out
}

// NewGraduatedOutcome builds an outcome whose severity respects the artifact's
// declared revision.
//
// Used at every site emitting one of the eight. A plain NewOutcome there would
// resolve severity from the static table and the graduation would exist while
// nothing consulted it — the failure this codebase has shipped repeatedly, in
// checks whose whole purpose was refusing what they let through.
func NewGraduatedOutcome(mode ValidationMode, revs ArtifactRevisions, code, message string) ValidationOutcome {
	out := NewOutcome(mode, code, message)
	out.Severity = GraduatedSeverity(code, mode, revs)
	return out
}
