// parlay-feature: annotations
// parlay-component: annotation-boundary-gate
//
// No build and no emission reads a file with a thread in it.
//
// That is the whole of §7's answer to the hashing problem, and it is stronger
// than it first looks. Threads are review state; a build is what happens after
// review. Because a thread never survives into a build, no signature is ever
// taken over comment bytes, and clearing a thread can never stale a buildfile
// for a change in nothing. The alternative the design rejected — accept the
// byte drift and explain it at the gate — produced a spurious stale-buildfile
// on every clear, one rebuild per thread, forever.
//
// So EVERY state blocks, open and answered alike. An answered thread is a
// reply the reviewer has not read, and advancing over it builds on a review
// that is not finished. Closing is one entry the reviewer writes, an act they
// can see themselves take.

package commands

import (
	"fmt"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// Readiness and gate codes for review threads (§8).
const (
	openAnnotationsCode     = "open-annotations"
	answeredAnnotationsCode = "answered-annotations"
	closedAnnotationsCode   = "closed-annotations"
)

// annotationReadinessIssues reports the threads standing between a feature and
// the next boundary.
//
// Ordered before drift findings by its caller, deliberately: a reviewer who
// has just annotated a signed file will otherwise meet `stale-buildfile` first
// and go looking for a phantom edit. Their own comment is the cause, and it
// should be the first thing they read.
func annotationReadinessIssues(cfg *config.Context, slug string) []readinessIssue {
	scans, err := collectFeatureAnnotations(cfg, slug)
	if err == nil {
		// Project sources count too. The buildfile's hard gate signs the root
		// domain model, the layout and the adapter alongside the feature's own
		// files, so a thread in any of them is a thread in something this
		// build reads — and two feature gates could both pass while one sat
		// unread in a shared file.
		//
		// CONSERVATIVE for v1: every project source blocks every feature,
		// because there is no per-feature dependency map to scope it by. A
		// feature that genuinely does not read a given adapter is blocked by a
		// thread in it, which is the safe direction of wrong.
		var project []annotationFileScan
		project, err = collectProjectAnnotations(cfg)
		scans = append(scans, project...)
	}
	if err != nil {
		// Fail closed. "Cannot tell whether this feature is under review" is
		// not "it is not under review": the second answer would advance a
		// boundary on a file nobody could read.
		return []readinessIssue{{
			Severity: "error",
			Code:     openAnnotationsCode,
			Message:  fmt.Sprintf("cannot read this feature's review threads: %s", err),
			Fix:      "fix the unreadable file, then re-run",
		}}
	}
	refuseAnnotationsInAppliedRecords(scans)

	var issues []readinessIssue
	counts := countAnnotations(scans)

	if counts.Open > 0 {
		issues = append(issues, readinessIssue{
			Severity: "error",
			Code:     openAnnotationsCode,
			Message:  fmt.Sprintf("%d open review thread(s), first in %s", counts.Open, firstFileWithState(scans, "open")),
			Fix:      "run /parlay-resolve @{feature}; the resolver answers each thread in place",
		})
	}
	if counts.Answered > 0 {
		issues = append(issues, readinessIssue{
			Severity: "error",
			Code:     answeredAnnotationsCode,
			Message:  fmt.Sprintf("%d answered review thread(s), first in %s", counts.Answered, firstFileWithState(scans, "answered")),
			Fix:      "read each reply in place, then write `close` under the ones you accept or a new request under the ones you do not",
		})
	}
	if counts.Closed > 0 {
		// A blocker, not a note. The design first called this informational,
		// reasoning that the build and code skills sweep before their gates so
		// a closed thread is gone by the time the check runs — true of the
		// skills, and §6.4 already insists the rule hold for the DIRECT
		// commands too. Codex found the hole: on a first build there is no
		// prior signature for stale-buildfile to catch the comment bytes with,
		// so a direct gate could pass while reading a file that still has a
		// thread in it. The sweep is one command and it is named here.
		issues = append(issues, readinessIssue{
			Severity: "error",
			Code:     closedAnnotationsCode,
			Message:  fmt.Sprintf("%d closed review thread(s) still in the files", counts.Closed),
			Fix:      "parlay annotations clear @{feature} — removes closed threads and nothing else",
		})
	}

	// A malformed annotation is not a thread and cannot be counted, but it is
	// still someone trying to say something, and the boundary is where they
	// find out it did not land.
	for _, scan := range scans {
		for _, f := range scan.Findings {
			issues = append(issues, readinessIssue{
				Severity: "error",
				Code:     f.Code,
				Message:  fmt.Sprintf("%s:%d %s", f.File, f.Line, f.Message),
				Fix:      f.Fix,
			})
		}
	}
	return issues
}

func firstFileWithState(scans []annotationFileScan, state string) string {
	for _, scan := range scans {
		for _, thread := range scan.Threads {
			if thread.State == state {
				return fmt.Sprintf("%s:%d", scan.Rel, thread.Line)
			}
		}
	}
	return "?"
}

// annotationValidationErrors renders the scanner's findings for a file in
// `parlay validate`'s own finding shape.
//
// Only the malformed ones. A well-formed thread in any state is not a
// validation finding: the file is correct, someone is reviewing it, and
// validate has no opinion about that. The boundary gates do.
func annotationValidationErrors(path string, content []byte) []agent.ValidationError {
	if parser.AnnotationHostFor(path) == "" {
		return nil
	}
	var out []agent.ValidationError
	for _, f := range parser.ScanAnnotations(path, content).Findings {
		out = append(out, agent.ValidationError{
			Code:    f.Code,
			Message: f.Message,
			Context: fmt.Sprintf("%s:%d", f.File, f.Line),
			Fix:     f.Fix,
		})
	}
	return out
}
