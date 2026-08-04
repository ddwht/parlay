// parlay-feature: parlay-tool/hand-authored-units
// parlay-component: authored-unit-validation
//
// Validates spec/intents/<unit>/authored.yaml against authored.schema.md.
// This is the structural pass: shape, required fields, slug agreement with
// the containing directory, and glob containment. It deliberately does not
// resolve the globs against the filesystem — whether a glob matches any
// file, and whether it overlaps generated output, are questions about a
// project rather than about a file, and they belong with the tracking pass
// that already holds the emitted manifest.

package agent

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AuthoredUnitSchemaVersion is the shape this binary writes and reads
// natively. Older versions reach it through a migrator chain (the policy
// authored.schema.md declares) rather than by regeneration: nothing can
// regenerate this file, because what it describes is precisely the code
// the tool does not produce.
const AuthoredUnitSchemaVersion = 1

// AuthoredUnit is the parsed declaration.
type AuthoredUnit struct {
	SchemaVersion int      `yaml:"schema_version"`
	Unit          string   `yaml:"unit"`
	Summary       string   `yaml:"summary"`
	Sources       []string `yaml:"sources"`
	Tests         []string `yaml:"tests"`
	Satisfies     []string `yaml:"satisfies"`
}

// ParseAuthoredUnit unmarshals a declaration without validating it.
// Callers that need a verdict should run ValidateAuthoredUnit first; this
// exists for the readers that have already been handed a valid file.
func ParseAuthoredUnit(content []byte) (*AuthoredUnit, error) {
	var u AuthoredUnit
	if err := yaml.Unmarshal(content, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// ValidateAuthoredUnit checks one authored.yaml. path is used for the
// slug-agreement check and for message context; content is the file's
// bytes.
func ValidateAuthoredUnit(mode ValidationMode, path string, content []byte) []ValidationOutcome {
	var outcomes []ValidationOutcome

	u, err := ParseAuthoredUnit(content)
	if err != nil {
		// A parse failure hides every other rule, so this is the one
		// place that returns early: reporting "unit: is missing" about a
		// file that is not YAML at all sends the author to the wrong line.
		return []ValidationOutcome{NewOutcome(mode, "authored-invalid-yaml",
			fmt.Sprintf("%s does not parse as YAML: %v", path, err))}
	}

	if u.SchemaVersion != AuthoredUnitSchemaVersion {
		outcomes = append(outcomes, NewOutcome(mode, "authored-schema-version-unsupported",
			fmt.Sprintf("schema_version is %d; this binary reads %d — run `parlay upgrade` if the file is newer, or migrate it if older",
				u.SchemaVersion, AuthoredUnitSchemaVersion)))
	}

	if strings.TrimSpace(u.Unit) == "" {
		outcomes = append(outcomes, NewOutcome(mode, "authored-field-missing",
			"unit: is required — it names the unit and must match the containing directory"))
	} else if dir := authoredUnitDirName(path); dir != "" && dir != u.Unit {
		outcomes = append(outcomes, NewOutcome(mode, "authored-unit-slug-mismatch",
			fmt.Sprintf("unit: is %q but the declaration sits in %q — every command addresses this unit by its directory name", u.Unit, dir)))
	}

	if strings.TrimSpace(u.Summary) == "" {
		outcomes = append(outcomes, NewOutcome(mode, "authored-field-missing",
			"summary: is required — the phases that refuse to generate into this unit quote it when they explain why"))
	}

	if len(u.Sources) == 0 {
		outcomes = append(outcomes, NewOutcome(mode, "authored-field-missing",
			"sources: is required and must name at least one glob — a unit owning no files declares nothing"))
	}

	outcomes = append(outcomes, validateAuthoredGlobs(mode, "sources", u.Sources)...)
	outcomes = append(outcomes, validateAuthoredGlobs(mode, "tests", u.Tests)...)

	return outcomes
}

// validateAuthoredGlobs enforces containment: every declared glob is
// root-relative and stays inside the root. An empty entry is caught here
// too — as a glob it would match nothing, but as a manifest line it reads
// as a deliberate declaration, which is the worse of the two failures.
func validateAuthoredGlobs(mode ValidationMode, field string, globs []string) []ValidationOutcome {
	var outcomes []ValidationOutcome
	for _, g := range globs {
		trimmed := strings.TrimSpace(g)
		if trimmed == "" {
			outcomes = append(outcomes, NewOutcome(mode, "authored-field-missing",
				fmt.Sprintf("%s: contains an empty entry — remove the line or give it a glob", field)))
			continue
		}
		if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "~") {
			outcomes = append(outcomes, NewOutcome(mode, "authored-glob-escapes-root",
				fmt.Sprintf("%s: entry %q is an absolute path — globs are resolved against the active root and must be relative to it", field, trimmed)))
			continue
		}
		if hasParentSegment(trimmed) {
			outcomes = append(outcomes, NewOutcome(mode, "authored-glob-escapes-root",
				fmt.Sprintf("%s: entry %q contains a `..` segment — a unit declares ownership of code inside the project, and parlay cannot track files outside the root", field, trimmed)))
		}
	}
	return outcomes
}

// hasParentSegment reports whether a slash-separated glob steps upward.
// Checked segment-wise rather than with strings.Contains(".."), which
// would reject a legitimate "src/foo..bar/**".
func hasParentSegment(glob string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(glob), "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// authoredUnitDirName returns the name of the directory holding the given
// authored.yaml, or "" when the path has no parent to read. For an
// initiative-nested unit this is the leaf segment, which is what the
// qualified identifier's second half is.
func authoredUnitDirName(path string) string {
	dir := filepath.Dir(path)
	if dir == "." || dir == string(filepath.Separator) {
		return ""
	}
	return filepath.Base(dir)
}
