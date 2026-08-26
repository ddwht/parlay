// parlay-feature: parlay-tool/multi-adapter
// parlay-component: coverage-review-gate
//
// Parser for .parlay/build/<feature>/coverage-review.yaml — the file that
// records human approval of a feature's testcases.yaml and gates
// `parlay generate-code` on multi-target projects.

package parser

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// CoverageReview is the parsed shape of coverage-review.yaml.
type CoverageReview struct {
	Path           string             `yaml:"-"`
	Feature        string             `yaml:"feature"`
	ReviewedAt     string             `yaml:"reviewed_at"`
	ReviewedBy     string             `yaml:"reviewed_by"`
	ReviewMethod   string             `yaml:"review_method"`
	BuildfileHash  string             `yaml:"buildfile_hash"`
	TestcasesHash  string             `yaml:"testcases_hash"`
	// SuiteHashes records a canonical-form hash per suite so the gate can tell
	// which suites drifted rather than invalidating the whole review when any
	// one changes. Absent in reviews written before per-suite staleness
	// existed; the gate falls back to TestcasesHash when it is empty.
	SuiteHashes    map[string]string  `yaml:"suite_hashes,omitempty"`
	ApprovedSuites []string           `yaml:"approved_suites"`
	Exemptions     []CoverageExemption `yaml:"exemptions,omitempty"`
}

// CoverageExemption documents why a required term has no covering case.
type CoverageExemption struct {
	Suite  string `yaml:"suite"`
	Item   string `yaml:"item"`
	// CriterionText narrows an exemption to a single verify: bullet on Item.
	//
	// Optional, and its absence is load-bearing rather than a default: an
	// exemption with no criterion_text: is entry-wide, which is the only way to
	// read one written before criterion coverage was counted per bullet. Every
	// exemption in existence omits it and none could have recorded it, so
	// reading absence as "excuses nothing" would silently revoke exemptions a
	// human already granted.
	CriterionText string `yaml:"criterion_text,omitempty"`
	Reason string `yaml:"reason"`
}

// ParseCoverageReview reads coverage-review.yaml from disk and parses it.
func ParseCoverageReview(path string) (*CoverageReview, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read coverage-review %s: %w", path, err)
	}
	return ParseCoverageReviewBytes(path, data)
}

// ParseCoverageReviewBytes parses coverage-review.yaml content already in
// memory.
func ParseCoverageReviewBytes(path string, content []byte) (*CoverageReview, error) {
	var cr CoverageReview
	if err := yaml.Unmarshal(content, &cr); err != nil {
		return nil, fmt.Errorf("parse coverage-review %s: %w", path, err)
	}
	cr.Path = path
	return &cr, nil
}
