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
// kind-mismatch and root-override validation.
type adapterFileShape struct {
	Kind            string `yaml:"kind"`
	FileConventions struct {
		SourceRoot string `yaml:"source-root"`
	} `yaml:"file-conventions"`
}

// normalizeRoot makes two spellings of the same directory comparable.
// "src/", "src", and "./src" are one place; the adapters and presets that ship
// today disagree about the trailing slash, and the adapter-set schema does not
// pin it.
func normalizeRoot(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "./")
	p = strings.Trim(p, "/")
	if p == "" {
		return "."
	}
	return p
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

		outcomes = append(outcomes, checkRootOverrideIsLossless(mode, slotKind, target, shape)...)
	}

	return outcomes
}

// checkRootOverrideIsLossless catches a target root that silently discards the
// framework's source directory.
//
// `targets.<kind>.root` REPLACES the adapter's own `source-root` during plan
// derivation. That is lossless only when `source-root` names a project
// location, which is how the backend adapters use it: nestjs declares
// `source-root: apps/api` and carries `src/` inside its path templates, so
// swapping the project location leaves the framework's directory intact.
//
// The presentation adapters do the opposite. react-antd declares
// `source-root: "src/"` — the framework's directory, not a project location —
// and its templates start at `features/…`. Replacing that with `apps/web`
// deletes the `src/` from every derived path, so the flagship preset emits
// React components to `apps/web/features/…` when the app builds from
// `apps/web/src/`. tsconfig's `include: ["src"]` means the files are not
// merely misplaced: they are outside the TypeScript project, so nothing type
// checks them and the build stays green by not seeing them at all.
//
// One field carries two incompatible meanings and nothing forced a choice.
// Fixing that is a model change; this is the diagnostic that stops the silent
// version happening in the meantime, and it fires on exactly the broken case —
// a root that differs from the source-root it replaces. Where the two agree
// (every backend slot; react-antd-only, whose `root: src` happens to match)
// the substitution loses nothing and this stays quiet.
func checkRootOverrideIsLossless(mode ValidationMode, slotKind string, target parser.AdapterSetTarget, shape adapterFileShape) []ValidationOutcome {
	sourceRoot := normalizeRoot(shape.FileConventions.SourceRoot)
	root := normalizeRoot(target.Root)
	if sourceRoot == "." || root == "." || sourceRoot == root {
		return nil
	}
	return []ValidationOutcome{NewOutcome(mode, "adapter-root-override-lossy",
		fmt.Sprintf("targets.%s.root %q replaces adapter %q's source-root %q, which is a different directory — "+
			"every derived path loses %q. If %q is the framework's source directory rather than a project location, "+
			"the templates that assume it will emit outside the build. Move the framework directory into the adapter's "+
			"path templates, or set root: to the same directory the adapter declares.",
			slotKind, target.Root, target.Adapter, shape.FileConventions.SourceRoot,
			shape.FileConventions.SourceRoot, shape.FileConventions.SourceRoot))}
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
