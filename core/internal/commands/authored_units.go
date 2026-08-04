// parlay-feature: parlay-tool/hand-authored-units
// parlay-component: authored-unit-ingestion
//
// Resolves hand-authored unit declarations into the concrete file set
// parlay tracks for them, and projects that file set into .parlay/ so
// codegen can honour it without reading spec/intents/**.
//
// This is the second ingestion path. The first — parser.ScanGenerated —
// admits a file on the strength of a generation marker, which is exactly
// the property hand-authored code does not have and must never acquire.
// So the two paths cannot be unified: one is marker-keyed and the other
// is declaration-keyed, and the declaration is the whole point.

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"gopkg.in/yaml.v3"
)

// AuthoredProjectionFile is where the resolved unit file set is projected,
// under .parlay/build/_project/.
//
// The projection exists because of a rule that has no exceptions: codegen
// may not read spec/intents/**. A unit's declaration lives there, and
// codegen needs it — it must refuse to write into a unit's files. Carving
// out a filename exemption would make the isolation rule negotiable, which
// generate-code calls "the load-bearing test for whether the buildfile is
// doing its job." Projecting instead keeps the rule absolute and treats the
// unit declaration the way capabilities are already treated: compiled out
// of the spec tree into the build tree, where codegen is allowed to look.
const AuthoredProjectionFile = "authored-files.yaml"

// The resolution-pass diagnostics, named rather than inlined into their
// format strings. Conformance scans Go source for each documented code as a
// standalone quoted literal, and a code spliced into a longer message
// ("authored-glob-empty: %s...") reads to that scan as never emitted — which
// is how a documented diagnostic ends up looking implemented while nothing
// can produce it.
const (
	CodeAuthoredGlobEmpty             = "authored-glob-empty"
	CodeAuthoredGlobOverlapsGenerated = "authored-glob-overlaps-generated"
	CodeAuthoredInvalidYAML           = "authored-invalid-yaml"
)

// AuthoredUnitFiles is one unit's resolved file set.
type AuthoredUnitFiles struct {
	Unit    string   `yaml:"unit"`
	Summary string   `yaml:"summary,omitempty"`
	Sources []string `yaml:"sources"`
	Tests   []string `yaml:"tests,omitempty"`
}

// AuthoredProjection is the whole projected set, ordered for a stable file.
type AuthoredProjection struct {
	GeneratedAt string              `yaml:"generated-at"`
	Units       []AuthoredUnitFiles `yaml:"units"`
}

// authoredDeclaration is the resolved, path-keyed form the tracking paths
// consume: every file some unit claims, mapped to the unit that claims it.
//
// Path-keyed rather than unit-keyed because every consumer asks the same
// question — "is this path hand-authored, and by whom" — and asking it of a
// per-unit slice would mean a linear scan per file.
type authoredDeclaration struct {
	// Owner maps a normalized root-relative path to its owning unit id.
	Owner map[string]string
	// Units is the unit id list, sorted, for stable messages.
	Units []string
}

// hasUnits reports whether any unit is declared. A nil declaration and an
// empty one are the same thing here — unlike the emission manifest, where
// "did not say" and "said nothing" are genuinely different states, a
// project with no units and a project that was not asked about units both
// have no hand-authored files to track.
func (d *authoredDeclaration) hasUnits() bool {
	return d != nil && len(d.Owner) > 0
}

func (d *authoredDeclaration) ownerOf(path string) (string, bool) {
	if d == nil {
		return "", false
	}
	unit, ok := d.Owner[normalizeWriteSetPath(path)]
	return unit, ok
}

// resolveAuthoredUnits enumerates the active root's units, parses each
// declaration and expands its globs against the filesystem.
//
// A declaration that fails to parse is reported rather than skipped: a unit
// whose file is malformed silently reverts to "parlay may write here",
// which is the one failure this whole mechanism exists to prevent.
func resolveAuthoredUnits(cfg *config.Context) (*authoredDeclaration, *AuthoredProjection, error) {
	unitIDs, err := cfg.AllUnits()
	if err != nil {
		// A tree that cannot be enumerated is not a tree with no units.
		return nil, nil, fmt.Errorf("enumerate hand-authored units: %w", err)
	}
	sort.Strings(unitIDs)

	decl := &authoredDeclaration{Owner: map[string]string{}, Units: unitIDs}
	projection := &AuthoredProjection{}

	for _, id := range unitIDs {
		declPath := filepath.Join(cfg.FeaturePath(id), config.AuthoredFile)
		content, readErr := os.ReadFile(declPath)
		if readErr != nil {
			return nil, nil, fmt.Errorf(CodeAuthoredInvalidYAML+": cannot read %s: %w", declPath, readErr)
		}
		unit, parseErr := agent.ParseAuthoredUnit(content)
		if parseErr != nil {
			return nil, nil, fmt.Errorf(CodeAuthoredInvalidYAML+": %s does not parse: %w", declPath, parseErr)
		}

		files := AuthoredUnitFiles{Unit: id, Summary: unit.Summary}

		sources, srcErr := expandAuthoredGlobs(cfg.Root.Path, unit.Sources)
		if srcErr != nil {
			return nil, nil, fmt.Errorf("unit %s: %w", id, srcErr)
		}
		tests, testErr := expandAuthoredGlobs(cfg.Root.Path, unit.Tests)
		if testErr != nil {
			return nil, nil, fmt.Errorf("unit %s: %w", id, testErr)
		}
		files.Sources = sources
		files.Tests = tests

		for _, p := range append(append([]string{}, sources...), tests...) {
			key := normalizeWriteSetPath(p)
			if other, taken := decl.Owner[key]; taken && other != id {
				return nil, nil, fmt.Errorf(CodeAuthoredGlobOverlapsGenerated+": %s is claimed by both unit %q and unit %q — a file has exactly one owner", p, other, id)
			}
			decl.Owner[key] = id
		}
		projection.Units = append(projection.Units, files)
	}

	return decl, projection, nil
}

// expandAuthoredGlobs turns declared globs into the concrete, sorted,
// root-relative file list they match.
//
// Each glob is walked from its own directory prefix rather than by walking
// the root and filtering: a unit's sources are a handful of directories in
// a tree that also holds .git, node_modules and every build artifact, and
// the difference between "walk what was declared" and "walk everything and
// discard" is the difference between milliseconds and seconds on every
// save-build-state.
func expandAuthoredGlobs(root string, globs []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string

	for _, glob := range globs {
		g := strings.TrimSpace(filepath.ToSlash(glob))
		if g == "" {
			continue
		}
		prefix := writeSetRegion(g)
		walkFrom := root
		if prefix != "" {
			walkFrom = filepath.Join(root, filepath.FromSlash(prefix))
		}

		matched := 0
		walkErr := filepath.WalkDir(walkFrom, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				// A prefix that does not exist is reported below as an
				// empty glob, which is a better message than the raw
				// ENOENT — the author's mistake is the glob, not the walk.
				return nil
			}
			if d.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(root, p)
			if relErr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if !matchAuthoredGlob(g, rel) {
				return nil
			}
			matched++
			if !seen[rel] {
				seen[rel] = true
				out = append(out, rel)
			}
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("expanding %q: %w", glob, walkErr)
		}
		if matched == 0 {
			// An empty glob is not a harmless no-op. It reads as "this unit
			// owns these files" while tracking none of them, so the unit
			// looks declared and behaves undeclared — the exact state that
			// leaves hand-authored code invisible.
			return nil, fmt.Errorf(CodeAuthoredGlobEmpty+": glob %q matches no file — correct the path or remove the entry", glob)
		}
	}

	sort.Strings(out)
	return out, nil
}

// matchAuthoredGlob matches a slash-separated glob against a
// slash-separated relative path, with `**` matching zero or more path
// segments and every other segment matched by filepath.Match.
//
// Written out rather than reduced to a directory-prefix test (the coarser
// form check-write-set uses for authorization regions) because this decides
// which files parlay records as hand-authored. Over-matching there would
// hand a generated file a hand-authored provenance and permanently exempt
// it from drift detection, which is worse than any convenience gained.
func matchAuthoredGlob(glob, path string) bool {
	return matchSegments(strings.Split(glob, "/"), strings.Split(path, "/"))
}

func matchSegments(pattern, parts []string) bool {
	if len(pattern) == 0 {
		return len(parts) == 0
	}
	if pattern[0] == "**" {
		// Zero or more segments, uniformly — including in trailing
		// position, so `a/**` matches `a/b`, `a/b/c` and `a` itself.
		// Ingestion only ever tests files, so the last case is moot there;
		// it is kept rather than special-cased because a caller asking
		// "is this directory inside the unit" wants yes, and a positional
		// exception to `**` is harder to reason about than the extra match.
		for i := 0; i <= len(parts); i++ {
			if matchSegments(pattern[1:], parts[i:]) {
				return true
			}
		}
		return false
	}
	if len(parts) == 0 {
		return false
	}
	ok, err := filepath.Match(pattern[0], parts[0])
	if err != nil || !ok {
		return false
	}
	return matchSegments(pattern[1:], parts[1:])
}

// authoredProjectionPath is where the resolved set lands.
func authoredProjectionPath(cfg *config.Context) string {
	return filepath.Join(cfg.ProjectBuildPath(), AuthoredProjectionFile)
}

// writeAuthoredProjection materializes the resolved set under .parlay/.
//
// Written even when there are no units, and removed in that case, so its
// absence is never ambiguous: a codegen run that finds no projection knows
// the project has no units, rather than having to guess whether the file is
// missing or merely stale.
func writeAuthoredProjection(cfg *config.Context, projection *AuthoredProjection) error {
	path := authoredProjectionPath(cfg)
	if projection == nil || len(projection.Units) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale authored projection: %w", err)
		}
		return nil
	}
	projection.GeneratedAt = nowRFC3339()
	data, err := yaml.Marshal(projection)
	if err != nil {
		return fmt.Errorf("marshal authored projection: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create project build dir: %w", err)
	}
	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("write authored projection: %w", err)
	}
	return nil
}

// loadAuthoredProjection reads the projected set. Returns nil (no error)
// when absent — a project with no units.
func loadAuthoredProjection(cfg *config.Context) (*AuthoredProjection, error) {
	data, err := os.ReadFile(authoredProjectionPath(cfg))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var p AuthoredProjection
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf(CodeAuthoredInvalidYAML+": %s does not parse: %w", authoredProjectionPath(cfg), err)
	}
	return &p, nil
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// CodeUnitNotAFeature is raised by the commands whose whole operation is a
// pipeline step a unit has no place in.
//
// Refusing is the right answer for these rather than a no-op success:
// `create-dialogs` on a unit would write a dialogs.md INSIDE the unit
// directory, and the migration commands would offer operations into a
// unit's capabilities.yaml. Both leave the unit looking like a half-built
// feature, which is the state it was declared to stop being.
const CodeUnitNotAFeature = "unit-not-a-feature"

// refuseOnUnit returns a refusal when slug names a hand-authored unit.
// because is the tail of the message, stating what the command would
// otherwise have done and why it must not.
func refuseOnUnit(cfg *config.Context, slug, because string) error {
	if !config.IsAuthoredUnit(cfg.FeaturePath(slug)) {
		return nil
	}
	return fmt.Errorf("%s: %s is a hand-authored unit — %s", CodeUnitNotAFeature, slug, because)
}

// authoredUnitHashes returns one aggregate content hash per unit, over the
// unit's whole declared file set.
//
// Aggregate rather than per-file because a unit is one thing. Its sources
// are an implementation detail of the boundary it draws — six files or
// sixty, the question a consumer asks is "has the engine changed", and
// per-file granularity here would invite consumers to depend on individual
// files inside a unit, which is the coupling the unit exists to prevent.
//
// Best-effort by design: callers that need the resolution error already
// have it from resolveAuthoredUnits, and drift tracking is advisory. A
// project whose units will not resolve has a louder problem than a missing
// advisory hash, and it is already being reported elsewhere.
func authoredUnitHashes(cfg *config.Context) map[string]string {
	_, projection, err := resolveAuthoredUnits(cfg)
	if err != nil || projection == nil || len(projection.Units) == 0 {
		return nil
	}

	out := make(map[string]string, len(projection.Units))
	for _, u := range projection.Units {
		paths := append(append([]string{}, u.Sources...), u.Tests...)
		sort.Strings(paths)

		var joined strings.Builder
		for _, p := range paths {
			h, hashErr := hashFileContent(p)
			if hashErr != nil {
				// A file that vanished between resolution and hashing is a
				// change in its own right; record the absence rather than
				// dropping it, so the aggregate still moves.
				h = "missing"
			}
			// The path is in the digest as well as the content: renaming a
			// file without editing it changes the unit, and hashing content
			// alone would call that stable.
			joined.WriteString(p)
			joined.WriteString(":")
			joined.WriteString(h)
			joined.WriteString("\n")
		}
		out[u.Unit] = sha256Hex(joined.String())
	}
	return out
}
