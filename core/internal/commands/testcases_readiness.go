// parlay-feature: parlay-tool/criterion-authority
// parlay-component: mechanical-testcase-readiness
//
// The mechanical half of the redesign, on a path that runs.
//
// The bargain replacing the blanket human gate is: a person approves the
// standard, and the deterministic middle — does every criterion have a case
// discharging it, does every case cite something real, does an operation suite
// exist — is checked mechanically. That half was written, graduated to error on
// current artifacts, and reachable from nothing.
//
// `validate --type testcases` hardcodes ModeAuthoring, so the graduated
// severities never applied on the only CLI path that emitted them. gateCode
// checked buildfile validity, composition, freshness, ledger state, criterion
// authority and exceptions — and never the testcases. So removing the human
// gate would have left the middle advisory: exactly deleted-before-built.
//
// Fail-closed throughout. An unreadable testcases file, an unreadable contract
// artifact or an unreadable exception ledger is "cannot establish readiness",
// never "nothing to check" — the distinction the rest of this release keeps
// having to relearn.

package commands

import (
	"fmt"

	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// TestcasesReadiness is what the boundary needs to know.
type TestcasesReadiness struct {
	// Blockers stop the boundary. Errors under the artifact's own revision.
	Blockers []string
	// Warnings are reported and do not stop it.
	Warnings []string
}

// CheckTestcasesReadiness runs the criterion and coverage walkers in BUILD mode
// against a feature's real testcases, contract and exception ledger.
//
// Deliberately not testcasesCoverageInputs: that builder is written for the
// authoring path and treats an unresolvable contract as a partial or empty
// subject, which is the missing-subject failure this release exists to stop
// shipping. Here a declared artifact that cannot be read is a refusal.
func CheckTestcasesReadiness(cfg *config.Context, slug string) TestcasesReadiness {
	var r TestcasesReadiness

	featureDir := cfg.FeaturePath(slug)

	tcPath := filepath.Join(cfg.BuildPath(slug), "testcases.yaml")
	content, err := os.ReadFile(tcPath)
	if err != nil && !os.IsNotExist(err) {
		r.Blockers = append(r.Blockers, fmt.Sprintf("testcases for %s cannot be read: %v — readiness cannot be established over a file that was not read", slug, err))
		return r
	}
	if os.IsNotExist(err) {
		// Absence is judged against the SUBJECT, not waved through. An earlier
		// version returned success here on the belief that the buildfile checks
		// report a missing testcases.yaml. They do not — computeCheckBuildfile
		// validates only buildfile.yaml, and the code that emits
		// testcases-not-found is computeReviewGate, which is the thing being
		// removed. So a feature with an approved criterion set, a valid
		// buildfile and no tests at all would have passed.
		criteria, cErr := CurrentCriteria(cfg, slug)
		if cErr != nil {
			r.Blockers = append(r.Blockers, fmt.Sprintf("%s has no testcases and its contract cannot be read to tell whether it needs any: %v", slug, cErr))
			return r
		}
		if len(criteria) > 0 {
			r.Blockers = append(r.Blockers, fmt.Sprintf(
				"%s declares %d criteria and has no testcases.yaml — nothing discharges them", slug, len(criteria)))
		}
		// A genuinely criterion-free feature may legitimately have none.
		return r
	}

	// Parse here rather than relying on the validator to report it:
	// ValidateTestcasesV2 returns NO outcomes on a YAML error, on the reasoning
	// that an upstream validator handles parse failures — and no upstream
	// validator runs at this boundary. So an unparseable testcases file
	// contributed nothing and the boundary read that as readiness.
	var probe map[string]any
	if err := yaml.Unmarshal(content, &probe); err != nil {
		r.Blockers = append(r.Blockers, fmt.Sprintf("testcases for %s cannot be parsed: %v — readiness cannot be established over a file nothing could read", slug, err))
		return r
	}

	in := agent.TestcasesV2Input{Path: tcPath, Content: content}

	// Contract: capabilities supplies canonical operations and their criteria;
	// surface supplies its fragments'. A declared artifact that will not parse
	// leaves the standard unknown, and an unknown standard cannot be checked
	// against — the walkers would simply find nothing to require.
	capsPath := filepath.Join(featureDir, "capabilities.yaml")
	statErr := statArtifact(capsPath)
	if statErr != nil {
		// Anything other than absence — a permission or I/O failure — is not
		// an absent artifact, and treating it as one silently drops the whole
		// operation contract from the subject being checked.
		r.Blockers = append(r.Blockers, fmt.Sprintf("capabilities for %s cannot be reached: %v", slug, statErr))
		return r
	}
	if artifactExists(capsPath) {
		caps, capErr := parser.ParseCapabilities(capsPath)
		if capErr != nil {
			r.Blockers = append(r.Blockers, fmt.Sprintf("capabilities for %s cannot be read: %v", slug, capErr))
			return r
		}
		if caps.Feature != "" {
			in.ContractResolved = true
			in.Revisions.Capabilities = caps.SchemaVersion
			for _, op := range caps.Operations {
				if op.ID == "" {
					continue
				}
				ref := parser.NormalizeOperationID(caps.Feature, op.ID)
				in.CanonicalOperations = append(in.CanonicalOperations, ref)
				for _, v := range op.Verify {
					if text := agent.CanonicalCriterionText(v); text != "" {
						in.Criteria = append(in.Criteria, agent.CriterionRef{Ref: ref, Text: text})
					}
				}
			}
		}
	}

	if surfacePath := parser.ResolveSurfacePath(featureDir); surfacePath != "" {
		fragments, fErr := parser.ParseSurfaceFile(surfacePath)
		if fErr != nil {
			r.Blockers = append(r.Blockers, fmt.Sprintf("surface for %s cannot be read: %v", slug, fErr))
			return r
		}
		in.ContractResolved = true
		for _, f := range fragments {
			if f.Name == "" || f.Feature == "" {
				continue
			}
			ref := fmt.Sprintf("@%s/fragment:%s", f.Feature, f.Name)
			for _, v := range f.Verify {
				if text := agent.CanonicalCriterionText(v); text != "" {
					in.Criteria = append(in.Criteria, agent.CriterionRef{Ref: ref, Text: text})
				}
			}
		}
	}

	// Exceptions excuse criteria, so a ledger that cannot be honoured must not
	// silently excuse nothing and let the coverage walk report the difference
	// as ordinary uncovered criteria. gateCoverageExceptions reports the
	// ledger's own problems; here its failure means readiness is unknown.
	ev := CheckCoverageExceptions(cfg, slug)
	if len(ev.Blockers) > 0 {
		r.Blockers = append(r.Blockers, fmt.Sprintf("coverage exceptions for %s are not in a state that can be applied, so testcase readiness cannot be established", slug))
		return r
	}
	in.ExemptCriteria = ev.Exempt

	for _, o := range agent.ValidateTestcasesV2(agent.ModeBuild, in) {
		switch o.Severity {
		case agent.SeverityError:
			r.Blockers = append(r.Blockers, fmt.Sprintf("[%s] %s", o.Code, o.Message))
		case agent.SeverityWarning:
			r.Warnings = append(r.Warnings, fmt.Sprintf("[%s] %s", o.Code, o.Message))
		}
	}
	return r
}

// statArtifact returns nil for both a present artifact and a genuinely absent
// one, and an error for anything else. Absence is a fact about the project;
// a permission or I/O failure is a fact about this run, and conflating them
// drops real content out of whatever is being checked.
func statArtifact(path string) error {
	if _, err := os.Stat(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func artifactExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
