// parlay-feature: parlay-tool/ledger-and-contract
// parlay-component: amendment-ledger-check
//
// Ledger-level validation for a feature's amendments/ directory, plus the
// declared dirty set. Single-file shape problems are `parlay validate
// --type amendment`'s job; this command checks what only the whole ledger
// and the contract artifacts can answer: sequence integrity, supersedes
// resolution, whether every affects: ref names a contract entry that
// exists, and — as JSON for the skills — which entries the ledger's
// unapplied tail says are dirty.

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var checkAmendmentsCmd = &cobra.Command{
	Use:   "check-amendments <@feature>",
	Short: "Validate a feature's amendment ledger and emit the declared dirty set (JSON)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCheckAmendments,
}

type amendmentIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type amendmentEntry struct {
	Seq        int      `json:"seq"`
	Slug       string   `json:"slug"`
	Date       string   `json:"date,omitempty"`
	Affects    []string `json:"affects"`
	Supersedes []string `json:"supersedes,omitempty"`
}

type checkAmendmentsOutput struct {
	Feature    string           `json:"feature"`
	Amendments []amendmentEntry `json:"amendments"`
	// DirtySet is the resolvable affects: refs of the UNAPPLIED TAIL only —
	// amendments whose Seq exceeds the feature baseline's
	// last-applied-amendment. That is the set a rebuild must actually touch:
	// everything at or below the baseline's last-applied sequence was already
	// folded into the generated code when the baseline was saved. Scoping it
	// this way (L7) is what makes dirty_set agree with what
	// `parlay internal diff` infers by hashing — the two disagreed when
	// dirty_set was the cumulative union, because the union kept naming
	// long-applied refs as dirty forever. Deduplicated in first-seen order.
	DirtySet []string `json:"dirty_set"`
	// AllAffects is the cumulative union of EVERY amendment's resolvable
	// affects: refs, deduplicated in first-seen order — the whole ledger's
	// footprint, regardless of what has been applied. This is the former
	// dirty_set semantics, kept under an honest name for consumers that want
	// the full history (audit, cross-feature pressure surveys) rather than the
	// rebuild-scoping tail.
	AllAffects []string `json:"all_affects"`
	// SupersededBy is the computed reverse of every amendment's supersedes:
	// forward links keyed by the superseded slug, valued by the slugs of the
	// later amendments that supersede it. The amendment files are immutable
	// once written, so a "who replaced me" link cannot live in the earlier
	// file; computing it here gives read-time forward navigation without
	// touching the ledger. Always present (possibly empty) so consumers can
	// index it unconditionally.
	SupersededBy map[string][]string `json:"superseded_by"`
	Ready        bool                `json:"ready"`
	Issues       []amendmentIssue    `json:"issues"`
}

func runCheckAmendments(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	featDir := cfg.FeaturePath(slug)

	out := checkAmendmentsOutput{
		Feature:      slug,
		Amendments:   []amendmentEntry{},
		DirtySet:     []string{},
		AllAffects:   []string{},
		SupersededBy: map[string][]string{},
		Issues:       []amendmentIssue{},
	}

	// The unapplied tail is defined against the feature baseline's
	// last-applied-amendment: any amendment beyond it has not yet been folded
	// into generated code. A missing/unreadable baseline (never built, or
	// pre-v3) reads as 0, so every amendment counts as unapplied — the
	// conservative reading, matching a from-scratch build.
	lastApplied := 0
	if blData, readErr := os.ReadFile(baselinePath(cfg, slug)); readErr == nil {
		var baseline Baseline
		if yaml.Unmarshal(blData, &baseline) == nil {
			lastApplied = baseline.LastAppliedAmendment
		}
	}

	amendments, err := parser.LoadFeatureAmendments(featDir)
	if err != nil {
		out.Issues = append(out.Issues, amendmentIssue{
			Severity: "error", Code: "amendment-not-parseable", Message: err.Error(),
		})
		return emitCheckAmendmentsJSON(cmd, out)
	}

	// Files in the ledger directory that match no accepted name are worth
	// naming: a mis-numbered file silently absent from the ledger reads as
	// "never happened".
	reportStrayAmendmentFiles(featDir, &out)

	slugs := map[string]bool{}
	seqSeen := map[int]string{}
	prevSeq := 0
	// Accumulated in sequence order for scope-overlap detection: each earlier
	// amendment's file slug plus the canonical set of contract entries it
	// declares in affects:. A later amendment editing an entry an earlier one
	// also edits, without naming the earlier in its supersedes:, is two
	// unordered writers on the same contract entry — the L15/F18 hazard.
	type priorScope struct {
		fileSlug string
		affects  map[string]bool
	}
	var priors []priorScope
	for _, a := range amendments {
		entry := amendmentEntry{Seq: a.Seq, Slug: a.Slug, Date: a.Date, Affects: a.Affects, Supersedes: a.Supersedes}
		out.Amendments = append(out.Amendments, entry)

		// Single-file shape problems surface here too, so one command
		// answers "is the ledger healthy" without a per-file walk.
		content, readErr := os.ReadFile(a.Path)
		if readErr == nil {
			for _, o := range agent.ValidateAmendment(agent.ModeBuild, a.Path, content) {
				out.Issues = append(out.Issues, amendmentIssue{
					Severity: string(o.Severity), Code: o.Code, Message: fmt.Sprintf("%03d-%s: %s", a.Seq, a.FileSlug, o.Message),
				})
			}
		}

		if a.Slug != "" && a.Slug != a.FileSlug {
			out.Issues = append(out.Issues, amendmentIssue{
				Severity: "error", Code: "amendment-slug-mismatch",
				Message: fmt.Sprintf("%s: frontmatter amendment: %q disagrees with the filename slug %q — the file may not lie about its own identity", filepath.Base(a.Path), a.Slug, a.FileSlug),
			})
		}
		if other, dup := seqSeen[a.Seq]; dup {
			out.Issues = append(out.Issues, amendmentIssue{
				Severity: "error", Code: "amendment-out-of-sequence",
				Message: fmt.Sprintf("sequence %03d used by both %q and %q — renumber the later one", a.Seq, other, a.FileSlug),
			})
		}
		seqSeen[a.Seq] = a.FileSlug
		if prevSeq > 0 && a.Seq > prevSeq+1 {
			out.Issues = append(out.Issues, amendmentIssue{
				Severity: "warning", Code: "amendment-sequence-gap",
				Message: fmt.Sprintf("sequence jumps %03d -> %03d — expected after compaction, otherwise a numbering mistake", prevSeq, a.Seq),
			})
		}
		prevSeq = a.Seq

		for _, sup := range a.Supersedes {
			if !slugs[sup] {
				out.Issues = append(out.Issues, amendmentIssue{
					Severity: "error", Code: "amendment-supersedes-unknown",
					Message: fmt.Sprintf("%03d-%s supersedes %q, which is no EARLIER amendment in this ledger", a.Seq, a.FileSlug, sup),
				})
			}
		}
		slugs[a.FileSlug] = true

		// Canonical scope of THIS amendment: every affects: ref that parses,
		// keyed by its normalized @feature/kind:name form so two spellings of
		// the same entry collide. Built regardless of on-disk resolvability —
		// scope overlap is about declared intent, and an unresolvable ref is
		// reported on its own line below.
		affectsCanon := map[string]bool{}
		for _, raw := range a.Affects {
			ref, parseErr := parser.ParseAmendmentRef(raw)
			if parseErr != nil {
				continue // already reported by ValidateAmendment as malformed
			}
			affectsCanon[canonicalAmendmentRef(ref)] = true
			if resolveErr := resolveAmendmentRef(cfg, ref); resolveErr != nil {
				out.Issues = append(out.Issues, amendmentIssue{
					Severity: "error", Code: "amendment-affects-unresolved",
					Message: fmt.Sprintf("%03d-%s: %s", a.Seq, a.FileSlug, resolveErr.Error()),
				})
				continue
			}
			// Every resolvable ref joins the cumulative footprint; only the
			// unapplied tail joins the rebuild-scoping dirty set.
			out.AllAffects = appendUniqueRef(out.AllAffects, raw)
			if a.Seq > lastApplied {
				out.DirtySet = appendUniqueRef(out.DirtySet, raw)
			}
		}

		// Scope overlap against every earlier amendment this one does not
		// supersede. Naming the earlier amendment in supersedes: is exactly the
		// declaration that the later change replaces it, so an overlap there is
		// intended and silent; an overlap without it is two writers with no
		// ordering between them.
		supersedesSet := map[string]bool{}
		for _, sup := range a.Supersedes {
			supersedesSet[sup] = true
		}
		for _, prior := range priors {
			if supersedesSet[prior.fileSlug] {
				continue
			}
			var overlap []string
			for ref := range affectsCanon {
				if prior.affects[ref] {
					overlap = append(overlap, ref)
				}
			}
			if len(overlap) == 0 {
				continue
			}
			sort.Strings(overlap)
			out.Issues = append(out.Issues, amendmentIssue{
				Severity: "warning", Code: "amendment-scope-overlap",
				Message: fmt.Sprintf("%03d-%s edits %s, which earlier %q also edits and this amendment does not supersede — two amendments change the same contract entry with no ordering between them", a.Seq, a.FileSlug, strings.Join(overlap, ", "), prior.fileSlug),
			})
		}
		priors = append(priors, priorScope{fileSlug: a.FileSlug, affects: affectsCanon})

		// Forward link: this amendment supersedes each named earlier slug, so
		// record it as the "superseded by" of that slug.
		for _, sup := range a.Supersedes {
			out.SupersededBy[sup] = append(out.SupersededBy[sup], a.FileSlug)
		}
	}

	return emitCheckAmendmentsJSON(cmd, out)
}

// reportStrayAmendmentFiles names files in amendments/ that the loader
// ignores because their name matches no NNN-<slug>.md shape.
func reportStrayAmendmentFiles(featDir string, out *checkAmendmentsOutput) {
	dir := parser.AmendmentsDir(featDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			// archive/ is the one expected subdirectory (compaction moves
			// the old ledger there); anything else is still not an error —
			// the loader only reads well-named files either way.
			continue
		}
		if !amendmentFileNameOK(e.Name()) {
			out.Issues = append(out.Issues, amendmentIssue{
				Severity: "error", Code: "amendment-out-of-sequence",
				Message: fmt.Sprintf("%s does not match NNN-<slug>.md and is invisible to the ledger — rename it or move it out", e.Name()),
			})
		}
	}
}

func amendmentFileNameOK(name string) bool {
	return parser.AmendmentFileNameValid(name)
}

// resolveAmendmentRef checks that a parsed affects: ref names a contract
// entry that exists on disk. The ref carries its own feature, so an
// amendment in one feature may declare effects on another's contract —
// cross-feature pressure is exactly what trigger:/affects: exist to record.
func resolveAmendmentRef(cfg *config.Context, ref parser.AmendmentRef) error {
	featDir := cfg.FeaturePath(ref.Feature)
	switch ref.Kind {
	case "operation":
		capPath := filepath.Join(featDir, "capabilities.yaml")
		caps, err := parser.ParseCapabilities(capPath)
		if err != nil {
			return fmt.Errorf("affects %s: cannot read %s: %v", ref.Raw, capPath, err)
		}
		for _, op := range caps.Operations {
			if op.ID == ref.Name {
				return nil
			}
		}
		return fmt.Errorf("affects %s: no operation %q in %s", ref.Raw, ref.Name, capPath)
	case "surface":
		surfacePath := parser.ResolveSurfacePath(featDir)
		if surfacePath == "" {
			return fmt.Errorf("affects %s: feature has no surface artifact", ref.Raw)
		}
		frags, err := parser.ParseSurfaceFile(surfacePath)
		if err != nil {
			return fmt.Errorf("affects %s: cannot read %s: %v", ref.Raw, surfacePath, err)
		}
		for _, f := range frags {
			if parser.Slugify(f.Name) == ref.Name {
				return nil
			}
		}
		return fmt.Errorf("affects %s: no fragment slugged %q in %s", ref.Raw, ref.Name, surfacePath)
	case "infrastructure":
		infraPath := filepath.Join(featDir, "infrastructure.md")
		_, _, fragments, err := readFragments(infraPath)
		if err != nil {
			return fmt.Errorf("affects %s: cannot read %s: %v", ref.Raw, infraPath, err)
		}
		for _, f := range fragments {
			if fragmentSlug(f.heading) == ref.Name {
				return nil
			}
		}
		return fmt.Errorf("affects %s: no infrastructure fragment slugged %q in %s", ref.Raw, ref.Name, infraPath)
	case "domain":
		// The domain model is root-scoped, not feature-scoped: the ref's
		// feature part records who is asking, the entity resolves against
		// the active root's model.
		dm, err := cfg.LoadDomainModel()
		if err != nil {
			return fmt.Errorf("affects %s: cannot load domain model: %v", ref.Raw, err)
		}
		for _, e := range dm.Entities {
			if e.Name == ref.Name {
				return nil
			}
		}
		return fmt.Errorf("affects %s: no entity %q in the root domain model", ref.Raw, ref.Name)
	default:
		return fmt.Errorf("affects %s: unknown kind %q", ref.Raw, ref.Kind)
	}
}

// canonicalAmendmentRef normalizes a parsed affects: ref to a stable
// @feature/kind:name key so two spellings of the same contract entry compare
// equal in the scope-overlap check. The raw text can vary (surrounding
// whitespace); the parsed fields cannot.
func canonicalAmendmentRef(ref parser.AmendmentRef) string {
	return fmt.Sprintf("@%s/%s:%s", ref.Feature, ref.Kind, ref.Name)
}

func appendUniqueRef(list []string, v string) []string {
	for _, e := range list {
		if e == v {
			return list
		}
	}
	return append(list, v)
}

func emitCheckAmendmentsJSON(cmd *cobra.Command, out checkAmendmentsOutput) error {
	hasError := false
	for _, i := range out.Issues {
		if i.Severity == "error" {
			hasError = true
		}
	}
	out.Ready = !hasError
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	if hasError {
		return NewExitCodeError(1)
	}
	return nil
}
