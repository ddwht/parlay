// parlay-feature: infrastructure-layer
// parlay-component: InfrastructureValidationResult
// parlay-extends: infrastructure-layer/portability-lint

package agent

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ddwht/parlay/internal/parser"
)

type PortabilityWarning struct {
	Fragment   string `json:"fragment"`
	Field      string `json:"field"`
	Content    string `json:"content"`
	Suggestion string `json:"suggestion"`
}

// ValidateInfrastructure checks infrastructure.md has valid fragment structure.
func ValidateInfrastructure(path string, content []byte) error {
	if !strings.Contains(string(content), "## ") {
		return fmt.Errorf("infrastructure.md has no fragment headings (## )")
	}
	if !strings.Contains(string(content), "**Behavior**:") {
		return fmt.Errorf("infrastructure.md has no **Behavior**: fields")
	}
	return nil
}

// ValidateInfrastructureDeep performs full schema validation on an infrastructure.md file.
func ValidateInfrastructureDeep(path string) ([]ValidationError, []PortabilityWarning) {
	var errors []ValidationError

	fragments, err := parser.ParseInfrastructureFile(path)
	if err != nil {
		return []ValidationError{{
			Code:    "infrastructure-not-readable",
			Message: fmt.Sprintf("cannot parse infrastructure.md: %s", err),
			Context: path,
			Fix:     "ensure the file exists and is valid markdown with ## fragment headings",
		}}, nil
	}

	if len(fragments) == 0 {
		return []ValidationError{{
			Code:    "no-fragments",
			Message: "infrastructure.md has no fragment blocks",
			Context: path,
			Fix:     "add at least one ## fragment with Affects, Behavior, and Source",
		}}, nil
	}

	seen := make(map[string]bool)
	for _, frag := range fragments {
		if seen[frag.Name] {
			errors = append(errors, ValidationError{
				Code:    "duplicate-fragment-name",
				Message: fmt.Sprintf("fragment %q appears more than once", frag.Name),
				Context: fmt.Sprintf("infrastructure.md ## %s", frag.Name),
				Fix:     "rename one of the duplicate fragments to be unique",
			})
		}
		seen[frag.Name] = true

		if frag.Affects == "" {
			errors = append(errors, ValidationError{
				Code:    "missing-affects",
				Message: fmt.Sprintf("fragment %q has no Affects field", frag.Name),
				Context: fmt.Sprintf("infrastructure.md ## %s", frag.Name),
				Fix:     "add **Affects**: describing what area of the system this capability touches",
			})
		}

		if frag.Behavior == "" {
			errors = append(errors, ValidationError{
				Code:    "missing-behavior",
				Message: fmt.Sprintf("fragment %q has no Behavior field", frag.Name),
				Context: fmt.Sprintf("infrastructure.md ## %s", frag.Name),
				Fix:     "add **Behavior**: describing what the capability does",
			})
		}

		if frag.Source == "" {
			errors = append(errors, ValidationError{
				Code:    "missing-source",
				Message: fmt.Sprintf("fragment %q has no Source reference", frag.Name),
				Context: fmt.Sprintf("infrastructure.md ## %s", frag.Name),
				Fix:     "add **Source**: @feature/intent-slug to trace back to the source intent",
			})
		}

		if frag.BackwardCompatible != "" &&
			frag.BackwardCompatible != "yes" &&
			frag.BackwardCompatible != "no" {
			errors = append(errors, ValidationError{
				Code:    "invalid-backward-compatible",
				Message: fmt.Sprintf("fragment %q has Backward-Compatible value %q — must be 'yes' or 'no'", frag.Name, frag.BackwardCompatible),
				Context: fmt.Sprintf("infrastructure.md ## %s", frag.Name),
				Fix:     "set **Backward-Compatible**: to 'yes' or 'no'",
			})
		}
	}

	warnings := lintPortability(fragments)
	return errors, warnings
}

var (
	funcSigPattern  = regexp.MustCompile(`\w+\([^)]*\s+\w+`)
	fileExtPattern  = regexp.MustCompile(`\b\w+\.(go|py|ts|js|rs|java|rb|swift|kt)\b`)
	langKeywords    = regexp.MustCompile(`\b(func|def|class|interface|struct|impl|enum|trait|module)\b`)
	importPathPattern = regexp.MustCompile(`\w+/\w+\.\w+`)
)

func lintPortability(fragments []parser.InfraFragment) []PortabilityWarning {
	var warnings []PortabilityWarning

	for _, frag := range fragments {
		warnings = append(warnings, lintField(frag.Name, "Affects", frag.Affects)...)
		warnings = append(warnings, lintField(frag.Name, "Behavior", frag.Behavior)...)
	}

	return warnings
}

func lintField(fragName, fieldName, content string) []PortabilityWarning {
	var warnings []PortabilityWarning

	if m := funcSigPattern.FindString(content); m != "" {
		warnings = append(warnings, PortabilityWarning{
			Fragment:   fragName,
			Field:      fieldName,
			Content:    m,
			Suggestion: "describe the capability without naming the function or its signature",
		})
	}

	if m := fileExtPattern.FindString(content); m != "" {
		warnings = append(warnings, PortabilityWarning{
			Fragment:   fragName,
			Field:      fieldName,
			Content:    m,
			Suggestion: "use an abstract scope label instead of a file path",
		})
	}

	if m := langKeywords.FindString(content); m != "" {
		warnings = append(warnings, PortabilityWarning{
			Fragment:   fragName,
			Field:      fieldName,
			Content:    m,
			Suggestion: fmt.Sprintf("'%s' is a language keyword — describe the behavior, not the implementation", m),
		})
	}

	return warnings
}
