// parlay-feature: parlay-tool/multi-adapter
// parlay-component: adapter-kind-discriminator
//
// Validates .parlay/adapter-set.yaml against the closed-vocabulary kind
// discriminator and the per-slot adapter-existence rules.

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/parser"
)

// adapterKindClosedSet is the canonical closed set of adapter kinds.
var adapterKindClosedSet = map[string]bool{
	"presentation": true,
	"transport":    true,
	"application":  true,
	"persistence":  true,
}

// adapterFileShape is the minimal projection of an adapter file used for
// kind-mismatch validation. We only need the kind: field; everything else
// stays in the framework-vocabulary validation path.
type adapterFileShape struct {
	Kind string `yaml:"kind"`
}

// ValidateAdapterSet validates the structural shape of .parlay/adapter-set.yaml.
// It checks: closed-vocabulary kinds, no duplicate slots, every referenced
// adapter file exists, and the adapter's declared kind matches the slot it
// occupies.
func ValidateAdapterSet(mode ValidationMode, path string, content []byte) []ValidationOutcome {
	var outcomes []ValidationOutcome

	as, err := parser.ParseAdapterSetBytes(path, content)
	if err != nil {
		outcomes = append(outcomes, NewOutcome(mode, "adapter-set-invalid-yaml", err.Error()))
		return outcomes
	}

	// Walk targets: closed-vocabulary kind + adapter existence + kind match.
	for slotKind, target := range as.Targets {
		if !adapterKindClosedSet[slotKind] {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-kind-unknown",
				fmt.Sprintf("targets key %q is outside the closed set {presentation, transport, application, persistence}", slotKind)))
			continue
		}
		if target.Adapter == "" {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-set-adapter-missing",
				fmt.Sprintf("targets.%s.adapter is empty", slotKind)))
			continue
		}

		// Resolve the adapter file relative to the adapter-set's project root
		// (parent of .parlay/).
		adapterPath := resolveAdapterPath(path, target.Adapter)
		adapterContent, err := os.ReadFile(adapterPath)
		if err != nil {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-set-adapter-missing",
				fmt.Sprintf("targets.%s.adapter %q has no .parlay/adapters/%s.adapter.yaml", slotKind, target.Adapter, target.Adapter)))
			continue
		}

		var shape adapterFileShape
		if err := yaml.Unmarshal(adapterContent, &shape); err != nil {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-set-adapter-missing",
				fmt.Sprintf("targets.%s.adapter %q failed to parse: %v", slotKind, target.Adapter, err)))
			continue
		}

		// Absent kind: defaults to presentation.
		actualKind := shape.Kind
		if actualKind == "" {
			actualKind = "presentation"
		}
		if actualKind != slotKind {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-set-kind-mismatch",
				fmt.Sprintf("targets.%s references adapter %q whose kind is %q (slot expects %q)", slotKind, target.Adapter, actualKind, slotKind)))
		}
	}

	return outcomes
}

// resolveAdapterPath turns an adapter slug into the on-disk path relative
// to the adapter-set file's project root (its parent's parent — i.e.,
// .parlay/adapter-set.yaml -> ./.parlay/adapters/<slug>.adapter.yaml).
func resolveAdapterPath(adapterSetPath, slug string) string {
	parlayDir := filepath.Dir(adapterSetPath)
	return filepath.Join(parlayDir, "adapters", slug+".adapter.yaml")
}

// FilterMode trims warnings that the active mode classifies as silent.
// Authoring keeps every outcome; build mode keeps only error-severity
// outcomes (warnings flow through reporting but don't gate the build).
func FilterMode(mode ValidationMode, outcomes []ValidationOutcome) []ValidationOutcome {
	if mode == ModeAuthoring {
		return outcomes
	}
	out := outcomes[:0]
	for _, o := range outcomes {
		if o.Severity == SeverityError {
			out = append(out, o)
		}
	}
	return out
}

// _ ensures strings is referenced even if the compiler dead-codes
// resolveAdapterPath in some build configuration. Avoids "imported and not
// used" surprises during incremental edits.
var _ = strings.TrimSpace
