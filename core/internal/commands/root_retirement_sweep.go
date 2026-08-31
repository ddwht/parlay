// parlay-feature: parlay-tool/root-retirement
// parlay-component: cross-cutting/project-wide-source-aware-inbound-sweep
//
// Who, anywhere in the project, still stands on the retiring root?
//
// This sweep is deliberately its own engine, distinct from the
// feature-retirement Inventory in inbound_references.go. That inventory
// is closed, single-root and specification-only ON PURPOSE — sound where
// it lives, and left untouched. Root retirement asks a wider question:
// it spans every root in the project (the parent and every registered
// child, not the active root only), and it reads source trees, shipped
// guidance documents (deployed .claude/skills/, .parlay/modules/,
// repo-level CLAUDE.md), schema documents (deployed .parlay/schemas/ and
// the embedded authoring sources), and specifications.
//
// Matching is against the retiring root's whole PATH SPACE, not only its
// enumerated features: any reference into the root's namespace counts,
// including ownership markers naming a feature of the retiring root,
// references to paths under the root that correspond to no enumerated
// feature, and instructions in guidance documents naming something only
// the retiring root provides. The line drawn is what the reference DOES
// if the root goes away: narrative prose that merely mentions the root's
// name does not block.
//
// What this sweep IS, precisely: a line-based LEXICAL scan. It reads
// every eligible file as text and applies a fixed set of line patterns —
// ownership markers, path references into the root's namespace,
// `@feature` refs, plain group-qualified and component-qualified feature
// references, and `--root <name>` instructions. It does not parse Go,
// YAML, markdown or schema documents into syntax trees, and it makes no
// claim to understand structure: a reference is found because it is
// written down, wherever it is written down. A file whose first 8 KiB
// contains a NUL byte is treated as binary and carries no textual
// reference to scan.
//
// Fail-closed, exactly as far as that scope reaches: a file that is
// present but cannot be READ is recorded as a scan failure that refuses
// the retirement — "cannot tell" is never reported as "none" — and so is
// a directory that cannot be listed. There is no separate "unparseable"
// condition, because nothing here parses; claiming one would promise a
// detection this engine does not perform. Every finding carries the
// owning artifact path, the position within it, and the reference as
// written — the same triple the feature-retirement Inventory reports,
// reused as a shape precedent.

package commands

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
)

// Sweep finding kinds — what a reference into the retiring root's
// namespace looks like where it was found.
const (
	// sweepKindOwnershipMarker is a parlay-feature:/parlay-extends:
	// ownership marker on a surviving file naming a feature of the
	// retiring root. Blocks unless a disposition re-homes that feature.
	sweepKindOwnershipMarker = "ownership-marker"
	// sweepKindPathReference is a reference to a path under the root's
	// directory namespace — enumerated feature or not.
	sweepKindPathReference = "path-reference"
	// sweepKindFeatureReference is an @feature ref naming a feature of
	// the retiring root.
	sweepKindFeatureReference = "feature-reference"
	// sweepKindCommandReference is an instruction addressing the root
	// itself (--root <name>) — something only the retiring root can
	// satisfy.
	sweepKindCommandReference = "command-reference"
	// sweepKindGroupQualifiedReference is a plain group-qualified
	// feature reference — `design-loop/design-loop`, or a deeper
	// component-qualified form such as
	// `studio-foundation/studio-deployer/cross-cutting/deploy-step` —
	// written without any marker, @ref or root prefix around it. This
	// is how features are actually named across the corpus: in Go
	// comments, in YAML values, in markdown prose and in generated
	// copies of all three.
	sweepKindGroupQualifiedReference = "group-qualified-reference"
)

// RootSweepFinding is one thing still standing on the retiring root:
// the owning artifact, the position within it, and the reference as
// written.
type RootSweepFinding struct {
	Path     string `json:"path"`
	Position string `json:"position"`
	Ref      string `json:"ref"`
	Kind     string `json:"kind"`
	// Feature names the retiring-root feature concerned, for
	// ownership-marker and feature-reference findings; empty for pure
	// path-space matches.
	Feature string `json:"feature,omitempty"`
	// Blocking is false only for an ownership-marker finding whose
	// feature a disposition re-homes; the re-home readiness check then
	// owns whether the claim has actually moved (decision:
	// rehomed-ownership-nonblocking-at-sweep).
	Blocking bool `json:"blocking"`
}

func (f RootSweepFinding) String() string {
	return fmt.Sprintf("%s · %s · %s (%s)", f.Path, f.Position, f.Ref, f.Kind)
}

// RootSweepResult aggregates the sweep, fail-closed: findings block the
// retirement while they stand, and any failure refuses it outright.
type RootSweepResult struct {
	Findings []RootSweepFinding `json:"findings"`
	Failures []ScanFailure      `json:"failures"`
}

// BlockingFindings returns the findings that stand in the way of the
// retirement — everything except ownership markers whose feature a
// disposition re-homes.
func (r RootSweepResult) BlockingFindings() []RootSweepFinding {
	var out []RootSweepFinding
	for _, f := range r.Findings {
		if f.Blocking {
			out = append(out, f)
		}
	}
	return out
}

// markerLineRe matches an ownership-marker header line in any comment
// style (// for Go/TS, # for YAML/shell, <!-- --> for markdown/HTML).
var markerLineRe = regexp.MustCompile(`parlay-(feature|extends):\s*([^\s*>-]\S*)`)

// markerFeature extracts the feature named by an ownership-marker line.
// For parlay-extends the value is feature/component; the caller matches
// by prefix since feature slugs may themselves carry an initiative
// segment. Returns the raw marker value and whether the line is a
// marker at all.
func markerFeature(line string) (string, bool) {
	m := markerLineRe.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return m[2], true
}

// markerNamesRetiringFeature reports which retiring feature (if any) a
// marker value names: an exact parlay-feature match, or a
// parlay-extends value of the form <feature>/<component>.
func markerNamesRetiringFeature(value string, retiring map[string]bool) (string, bool) {
	if retiring[value] {
		return value, true
	}
	for f := range retiring {
		if strings.HasPrefix(value, f+"/") {
			return f, true
		}
	}
	return "", false
}

// groupQualifiedFeatureRes builds one regex per retiring feature for the
// way features are ORDINARILY written down: bare and group-qualified.
//
// Markers, `@feature` refs and `--root <name>` are the decorated forms,
// and matching only those misses the majority of real references —
// `design-loop/design-loop` in a Go comment, a YAML value naming
// `studio-foundation/studio-deployer`, prose in a shipped skill naming
// `<group>/<feature>/cross-cutting/<component>`. Each of those keeps
// pointing at the retiring root after it is gone.
//
// Two rules keep the match from firing on ordinary words:
//
//   - Word boundaries on both ends. The leading position must not be a
//     word character, a dot or a hyphen, so `redesign-loop/x` does not
//     match `design-loop/x`; the trailing position must not continue
//     the path or the word. A slash IS allowed to lead, so a reference
//     embedded in a longer path (`spec/intents/<group>/<feature>`)
//     still matches — a longer path containing the slug is a reference
//     to it, not a coincidence.
//   - A path-ish context check. A feature slug that is itself
//     group-qualified (it contains a slash) is path-ish as written and
//     matches on its own. A single-segment slug is a bare word that
//     could be anything, so it counts only when followed by at least
//     one more path segment. This is the difference between reporting
//     `alpha/tools` and reporting the English word "alpha".
//
// Anything that survives both rules is reported. The sweep does not
// silently drop a plausible reference on suspicion of coincidence: a
// person reading the preview dismisses a false positive in a moment,
// while a missed reference is discovered only after the root is gone.
func groupQualifiedFeatureRes(features []string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, f := range features {
		q := regexp.QuoteMeta(f)
		// A path segment is a word run, optionally dot-separated
		// (`deploy-step`, `notes.md`). Written that way rather than as
		// `[\w.-]+`, a sentence-ending period after a reference stays
		// outside the match — the finding carries the reference as
		// written, and "deploy-step." is not what was written.
		segment := `(?:/[\w-]+(?:\.[\w-]+)*)`
		tail := segment + `*` // component-qualified and deeper forms
		if !strings.Contains(f, "/") {
			// A bare word only counts in a path-ish context.
			tail = segment + `+`
		}
		out = append(out, regexp.MustCompile(`(?:^|[^\w.-])(`+q+tail+`)(?:$|[^\w/-])`))
	}
	return out
}

// isSeparateCheckout reports whether dir is the root of a git checkout
// of its own — a linked worktree, a submodule, a vendored clone.
//
// Git marks all three the same way: a .git entry at the top of the
// checkout, a directory for an ordinary clone and a file pointing at the
// real git directory for a worktree or a submodule. Either answers the
// only question that matters here, which is whether the files below are
// this project's content or another commit's copy of it.
//
// The distinction is not pedantic. A linked worktree under the project
// holds whatever the features were called at the commit it has checked
// out, and those markers read exactly like live inbound references to a
// retiring root — reported as blocking, and impossible for the operator
// to resolve, since editing a checkout of an old commit into agreement
// with the present one is not a thing anyone should do.
func isSeparateCheckout(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

// sweepRootRetirement runs the project-wide, source-aware inbound sweep
// for the retiring root. It walks the parent root's whole tree —
// covering the parent and every registered child, since children live
// under the parent — skipping only the retiring root itself, archives
// of previously retired roots, and version-control internals. The sweep
// runs to completion before any mutation.
func sweepRootRetirement(parentPath string, target config.Root, dispositions *DispositionRecord) (RootSweepResult, error) {
	result := RootSweepResult{}
	if err := retirementEvent("sweep"); err != nil {
		return result, err
	}

	// The retiring root's enumerated features, for marker and @ref
	// matching. Path-space matching below is deliberately wider: a
	// reference to a path under the root that corresponds to no
	// enumerated feature still counts.
	features, err := enumerateRetiringFeatures(target.Path)
	if err != nil {
		// Cannot enumerate: recorded as a scan failure, not passed
		// over — the sweep cannot establish what the markers may name.
		result.Failures = append(result.Failures, ScanFailure{
			Path: target.Path, Reason: fmt.Sprintf("enumerate retiring features: %v", err),
		})
		features = nil
	}
	retiring := map[string]bool{}
	for _, f := range features {
		retiring[f] = true
	}
	rehomed := map[string]bool{}
	if dispositions != nil {
		for _, d := range dispositions.Dispositions {
			if d.Term == dispositionAuthorityReHomedTo {
				rehomed[d.Feature] = true
			}
		}
	}

	// The root's namespace tokens: its registered relative path and, when
	// different, its short name. A path INTO the namespace (token + "/")
	// is a reference; the bare name in prose is not.
	tokens := []string{filepath.ToSlash(target.RelativePath)}
	if target.Name != tokens[0] {
		tokens = append(tokens, target.Name)
	}
	var pathRes []*regexp.Regexp
	for _, tok := range tokens {
		if tok == "" || tok == "." {
			continue
		}
		pathRes = append(pathRes, regexp.MustCompile(`(?:^|[^\w./-])(`+regexp.QuoteMeta(tok)+`/[\w./-]+)`))
	}
	rootFlagRe := regexp.MustCompile(`(--root[= ]` + regexp.QuoteMeta(target.Name) + `)(?:$|[^\w-])`)
	var featureRes []*regexp.Regexp
	for _, f := range features {
		featureRes = append(featureRes, regexp.MustCompile(`(@`+regexp.QuoteMeta(f)+`)(?:$|[^\w/-])`))
	}
	groupRes := groupQualifiedFeatureRes(features)

	retiredDir := retiredRootsDir(parentPath)
	targetAbs := filepath.Clean(target.Path)
	exemptPath := ""
	if dispositions != nil && dispositions.Path != "" {
		exemptPath = filepath.Clean(dispositions.Path)
	}

	walkErr := filepath.WalkDir(parentPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Present but unlistable is "cannot tell", and cannot tell
			// is not none.
			result.Failures = append(result.Failures, ScanFailure{Path: path, Reason: err.Error()})
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			clean := filepath.Clean(path)
			switch {
			case clean == targetAbs:
				// The retiring root's own contents leave with it —
				// internal references are not inbound.
				return filepath.SkipDir
			case clean == retiredDir:
				// Archives of previously retired roots are history, not
				// live standers-on.
				return filepath.SkipDir
			case name == ".git" || name == "node_modules":
				return filepath.SkipDir
			case clean != filepath.Clean(parentPath) && isSeparateCheckout(path):
				// A nested git checkout — a linked worktree, a
				// submodule, a vendored clone — is another checkout of
				// history that happens to sit inside this directory
				// tree. Its files are not project content: they are some
				// other commit's version of it, and the markers in them
				// name whatever the features were called then. Reading
				// them reports a retiring root as still referenced by
				// its own past, which is both wrong and unfixable — the
				// operator cannot edit a checkout of an old commit into
				// agreement with the present one.
				return filepath.SkipDir
			case strings.HasPrefix(name, ".") && name != config.ParlayDir && name != ".claude" && clean != filepath.Clean(parentPath):
				// Other dot-directories are tool internals — but .parlay
				// (modules, schemas, build state) and .claude (deployed
				// skills) are exactly where shipped guidance lives.
				return filepath.SkipDir
			}
			return nil
		}
		// The operator's own disposition record necessarily names every
		// feature in the retiring root — that is what it is for — so
		// scanning it reports the operator's answers back as evidence
		// against them. Exactly the resolved path in use is exempt, and
		// nothing broader: another file that merely looks like a
		// disposition record is scanned like anything else.
		if exemptPath != "" && filepath.Clean(path) == exemptPath {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			result.Failures = append(result.Failures, ScanFailure{Path: path, Reason: readErr.Error()})
			return nil
		}
		if bytes.IndexByte(data[:min(len(data), 8192)], 0) != -1 {
			// Binary content carries no textual reference to scan.
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			pos := fmt.Sprintf("line %d", i+1)
			if value, ok := markerFeature(line); ok {
				if feat, hit := markerNamesRetiringFeature(value, retiring); hit {
					result.Findings = append(result.Findings, RootSweepFinding{
						Path: path, Position: pos, Ref: strings.TrimSpace(line),
						Kind: sweepKindOwnershipMarker, Feature: feat,
						Blocking: !rehomed[feat],
					})
					continue
				}
			}
			matched := false
			for _, re := range pathRes {
				if m := re.FindStringSubmatch(line); m != nil {
					result.Findings = append(result.Findings, RootSweepFinding{
						Path: path, Position: pos, Ref: m[1],
						Kind: sweepKindPathReference, Blocking: true,
					})
					matched = true
					break
				}
			}
			if matched {
				continue
			}
			for j, re := range featureRes {
				if m := re.FindStringSubmatch(line); m != nil {
					result.Findings = append(result.Findings, RootSweepFinding{
						Path: path, Position: pos, Ref: m[1],
						Kind: sweepKindFeatureReference, Feature: features[j], Blocking: true,
					})
					matched = true
					break
				}
			}
			if matched {
				continue
			}
			for j, re := range groupRes {
				if m := re.FindStringSubmatch(line); m != nil {
					result.Findings = append(result.Findings, RootSweepFinding{
						Path: path, Position: pos, Ref: m[1],
						Kind: sweepKindGroupQualifiedReference, Feature: features[j], Blocking: true,
					})
					matched = true
					break
				}
			}
			if matched {
				continue
			}
			if m := rootFlagRe.FindStringSubmatch(line); m != nil {
				result.Findings = append(result.Findings, RootSweepFinding{
					Path: path, Position: pos, Ref: m[1],
					Kind: sweepKindCommandReference, Blocking: true,
				})
			}
		}
		return nil
	})
	if walkErr != nil {
		return result, fmt.Errorf("sweep project tree: %w", walkErr)
	}
	sort.Slice(result.Findings, func(i, j int) bool {
		if result.Findings[i].Path != result.Findings[j].Path {
			return result.Findings[i].Path < result.Findings[j].Path
		}
		return result.Findings[i].Position < result.Findings[j].Position
	})
	return result, nil
}
