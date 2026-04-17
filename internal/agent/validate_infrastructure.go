// parlay-feature: infrastructure-layer
// parlay-component: InfrastructureValidationResult

package agent

import (
	"fmt"
	"strings"

	"github.com/ddwht/parlay/internal/parser"
)

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
func ValidateInfrastructureDeep(path string) []ValidationError {
	var errors []ValidationError

	fragments, err := parser.ParseInfrastructureFile(path)
	if err != nil {
		return []ValidationError{{
			Code:    "infrastructure-not-readable",
			Message: fmt.Sprintf("cannot parse infrastructure.md: %s", err),
			Context: path,
			Fix:     "ensure the file exists and is valid markdown with ## fragment headings",
		}}
	}

	if len(fragments) == 0 {
		return []ValidationError{{
			Code:    "no-fragments",
			Message: "infrastructure.md has no fragment blocks",
			Context: path,
			Fix:     "add at least one ## fragment with Behavior, Source, and Modifies or Introduces",
		}}
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

		if frag.Behavior == "" {
			errors = append(errors, ValidationError{
				Code:    "missing-behavior",
				Message: fmt.Sprintf("fragment %q has no Behavior field", frag.Name),
				Context: fmt.Sprintf("infrastructure.md ## %s", frag.Name),
				Fix:     "add **Behavior**: describing what the change does",
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

		if len(frag.Modifies) == 0 && len(frag.Introduces) == 0 {
			errors = append(errors, ValidationError{
				Code:    "no-modifies-or-introduces",
				Message: fmt.Sprintf("fragment %q has neither Modifies nor Introduces", frag.Name),
				Context: fmt.Sprintf("infrastructure.md ## %s", frag.Name),
				Fix:     "add **Modifies**: (existing code to change) or **Introduces**: (new code to add), or both",
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

	return errors
}
