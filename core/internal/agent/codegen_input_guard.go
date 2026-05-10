// parlay-feature: parlay-tool/multi-adapter
// parlay-component: codegen-read-set-and-layer-pipeline
//
// Pin generate-code's allowed input set to: per-feature buildfile.yaml /
// testcases.yaml / coverage-review.yaml; the project's adapter-set.yaml,
// referenced adapter files, blueprint.yaml, config.yaml, domain-model.yaml;
// and the source tree under each adapter's declared root. Forbid reads of
// spec/intents/**.

package agent

import (
	"fmt"
	"path/filepath"
	"strings"
)

// AllowedReadPaths describes the read-set codegen is permitted to consult
// for a single project pass. Built from the loaded buildfiles + adapter-set.
type AllowedReadPaths struct {
	BuildArtifacts []string // .parlay/build/<feature>/{buildfile,testcases,coverage-review}.yaml
	ParlayConfig   []string // .parlay/{config,blueprint,adapter-set}.yaml
	Adapters       []string // .parlay/adapters/<slug>.adapter.yaml
	DomainModels   []string // .parlay/domain-model.yaml + spec/intents/<feat>/domain-model.yaml
	SourceRoots    []string // declared roots under each adapter's targets.<kind>.root
}

// CheckRead reports whether the supplied path is allowed under the read-set.
// Returns "" on allow; on deny, returns the validation code that should be
// surfaced (codegen-spec-read-forbidden or codegen-input-out-of-scope).
func (a *AllowedReadPaths) CheckRead(path string) (code string) {
	clean := filepath.ToSlash(filepath.Clean(path))

	// Hard deny: spec/intents/** is the source-of-truth boundary codegen
	// must never cross.
	if strings.Contains(clean, "/spec/intents/") || strings.HasPrefix(clean, "spec/intents/") {
		return "codegen-spec-read-forbidden"
	}

	for _, p := range a.BuildArtifacts {
		if filepath.ToSlash(filepath.Clean(p)) == clean {
			return ""
		}
	}
	for _, p := range a.ParlayConfig {
		if filepath.ToSlash(filepath.Clean(p)) == clean {
			return ""
		}
	}
	for _, p := range a.Adapters {
		if filepath.ToSlash(filepath.Clean(p)) == clean {
			return ""
		}
	}
	for _, p := range a.DomainModels {
		if filepath.ToSlash(filepath.Clean(p)) == clean {
			return ""
		}
	}
	for _, root := range a.SourceRoots {
		r := filepath.ToSlash(filepath.Clean(root))
		if r == "" {
			continue
		}
		if clean == r || strings.HasPrefix(clean, r+"/") {
			return ""
		}
	}

	return "codegen-input-out-of-scope"
}

// CheckReadOutcome wraps CheckRead and returns a ValidationOutcome ready to
// surface to the user.
func (a *AllowedReadPaths) CheckReadOutcome(mode ValidationMode, path string) (ValidationOutcome, bool) {
	code := a.CheckRead(path)
	if code == "" {
		return ValidationOutcome{}, true
	}
	return NewOutcome(mode, code, fmt.Sprintf("codegen attempted to read %q which is outside the allowed input set", path)), false
}
