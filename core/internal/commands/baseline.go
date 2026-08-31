package commands

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var checkDriftCmd = &cobra.Command{
	Use:   "check-drift <@feature>",
	Short: "Check if intents have changed since the last build (JSON output for agent consumption)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCheckDrift,
}

// Baseline is the stored snapshot of feature content at build time.
//
// Two layers of hashes:
//   - Intents: per-field hashes used by the existing drift detection
//     (parlay check-drift). Granular field-level reporting.
//   - Sources: per-element content hashes for incremental rebuilds
//     (parlay diff). Used to determine which buildfile components
//     are stable / dirty / removed without re-running the agent.
type Baseline struct {
	// SchemaVersion is the baseline format version. Absent (0) means a
	// baseline written before the field existed — see the
	// "missing means unknown" rule on HashedSources.Domain.
	SchemaVersion     int                   `yaml:"schema-version,omitempty"`
	GeneratedAt       string                `yaml:"generated-at"`
	Intents           map[string]IntentHash `yaml:"intents"`
	Sources           *HashedSources        `yaml:"sources,omitempty"`
	BuildfileSections map[string]string     `yaml:"buildfile-sections,omitempty"`

	// LastAppliedAmendment is the highest amendment sequence number that had
	// been applied to the contract artifacts when this baseline was saved.
	// save-build-state runs after a green build, at which point the ledger
	// is by definition fully applied — so this records the ledger's highest
	// sequence at save time. A ledger entry beyond it is the unapplied tail
	// check-drift reports. Zero means "no amendments
	// yet" (or a pre-v3 baseline, which is the same statement).
	LastAppliedAmendment int `yaml:"last-applied-amendment,omitempty"`
}

// BaselineSchemaVersion is the current baseline format version.
//
//	1 — adds Domain and AdapterVersion to HashedSources.
//	2 — adds Authored to HashedSources.
//	3 — adds LastAppliedAmendment and HashedSources.Amendments (the
//	    ledger-and-contract model; both zero-valued while a feature has
//	    no amendments).
const BaselineSchemaVersion = 3

// IntentHash stores hashes of individual intent fields for granular drift detection.
type IntentHash struct {
	ContentHash string `yaml:"content-hash"`
	Goal        string `yaml:"goal-hash"`
	Constraints string `yaml:"constraints-hash"`
	Verify      string `yaml:"verify-hash"`
	Objects     string `yaml:"objects-hash"`
}

// HashedSources stores per-element content hashes used by parlay diff
// to compute component-level dirty/stable/removed sets.
//
// Maps are slug → hex-encoded sha256 prefix (16 chars). Surface fragments
// are keyed by Slugify(fragment.Name).
type HashedSources struct {
	Intents          map[string]string `yaml:"intents,omitempty"`
	Dialogs          map[string]string `yaml:"dialogs,omitempty"`
	SurfaceFragments map[string]string `yaml:"surface-fragments,omitempty"`

	// Capabilities, Infrastructure, and SurfaceYAML are whole-file
	// content hashes for capabilities.yaml, infrastructure.md, and
	// surface.yaml — advisory only. There is no per-operation or
	// per-fragment granularity here the way SurfaceFragments has for
	// surface.md. Empty string means the file didn't exist when the
	// baseline was captured.
	//
	// This comment used to say the buildfile schema's source-signatures:
	// mechanism "was found to be aspirational (never implemented)", and that
	// these fields were the real freshness signal "until a hard codegen gate is
	// specced separately". Both halves were wrong, and wrong in a way that
	// invited someone to delete a live mechanism. source-signatures: IS the hard
	// gate: generate-code.skill.md step 11.6 reads it, recomputes each artifact's
	// hash, and refuses to emit for the feature on mismatch with a non-zero exit.
	// It is enforced by the skill rather than by Go because codegen is a skill —
	// `parlay generate-code` is a twenty-line pointer at it — so "no Go code does
	// this" was mistaken for "nothing does this".
	//
	// The two mechanisms are deliberately distinct and buildfile.schema.md says
	// so at length: source-signatures: is a hard per-feature gate that blocks
	// emission, HashedSources is an advisory component-level signal that informs
	// `parlay internal diff` and blocks nothing. Do not unify one away against
	// the other.
	Capabilities   string `yaml:"capabilities,omitempty"`
	Infrastructure string `yaml:"infrastructure,omitempty"`
	SurfaceYAML    string `yaml:"surface-yaml,omitempty"`

	// Domain is a whole-file hash of the project's canonical
	// domain-model.yaml, and AdapterVersion of the resolved adapter file.
	//
	// Both were previously untracked, which made domain-model edits
	// structurally invisible to drift detection: a designer could change
	// the shared model (the whole point of the Studio editor) and every
	// dependent feature would still report has_drift:false, so nothing was
	// ever marked for rebuild.
	//
	// "Missing means unknown, not drifted." A baseline written before
	// SchemaVersion 1 has no domain hash, and comparing "" against a real
	// hash would report every pre-existing project as drifted the moment
	// the binary is upgraded. Readers MUST treat an empty stored value as
	// "no opinion" and skip the comparison; the next save-build-state
	// backfills it.
	Domain         string `yaml:"domain,omitempty"`
	AdapterVersion string `yaml:"adapter-version,omitempty"`

	// Authored maps a hand-authored unit's qualified id to an aggregate
	// hash over its whole declared file set.
	//
	// Project-scoped, alongside Domain and AdapterVersion, and for the same
	// reason: a unit is not a per-feature file, so a change to it dirties
	// every feature that reads it. This deliberately does NOT model which
	// features those are. The plan that introduced units left the choice
	// open between declaring the edge in authored.yaml and declaring it in
	// the consumer; the answer taken here is that neither is needed yet,
	// because the codebase already had a shape for exactly this problem —
	// domain-model.yaml is shared by every feature, is tracked with one
	// hash, and dirties all of them when it moves. Over-approximating in
	// that direction is safe (a spurious rebuild), and the alternative
	// under-approximates when the edge is wrong or stale (a silent stale
	// fixture, which is the bug this whole mechanism exists to catch).
	//
	// A precise per-consumer edge is a real refinement, and lands with
	// per-component baseline scoping rather than ahead of it.
	//
	// Same "missing means unknown, not drifted" rule as Domain: a
	// pre-v2 baseline has no entry, and comparing against one would
	// report every existing project as drifted on upgrade.
	Authored map[string]string `yaml:"authored,omitempty"`

	// Amendments maps ledger filename → whole-file hash, captured at save
	// time. The ledger is append-only, so the only legitimate change to
	// this set over time is NEW entries: a stored entry whose hash moved
	// or whose file vanished is a ledger-integrity violation (an amendment
	// was edited or deleted after being recorded), which check-drift
	// reports. Same "missing means unknown" rule as its siblings for
	// pre-v3 baselines.
	Amendments map[string]string `yaml:"amendments,omitempty"`
}

type driftItem struct {
	Intent        string   `json:"intent"`
	ChangedFields []string `json:"changed_fields"`
}

type driftOutput struct {
	Feature    string      `json:"feature"`
	HasDrift   bool        `json:"has_drift"`
	Drifted    []driftItem `json:"drifted,omitempty"`
	NewIntents []string    `json:"new_intents,omitempty"`
	Removed    []string    `json:"removed_intents,omitempty"`

	// SharedSourcesChanged names project-scoped artifacts that changed
	// since the baseline — currently "domain-model" and "adapter".
	// These are not per-feature files, so a change dirties every feature
	// that reads them. Reported separately from intent drift so callers
	// can tell "this feature's own spec changed" from "something the
	// whole project shares changed underneath it".
	SharedSourcesChanged []string `json:"shared_sources_changed,omitempty"`

	// The founding docs are frozen at first build, so a change to
	// intents.md is not drift to rebuild from — it is a LedgerIntegrity
	// violation to surface. UnappliedAmendments names the ledger tail
	// beyond the baseline's last-applied-amendment: changes that were
	// decided but never applied to the contract. Drifted, NewIntents and
	// Removed above no longer carry founding-doc changes (those all
	// classify as integrity findings); the fields stay for the JSON
	// shape's consumers.
	LedgerIntegrity     []string `json:"ledger_integrity,omitempty"`
	UnappliedAmendments []string `json:"unapplied_amendments,omitempty"`
}

func baselinePath(cfg *config.Context, slug string) string {
	return filepath.Join(cfg.BuildPath(slug), ".baseline.yaml")
}

// buildBaseline computes a Baseline struct from the current source files of
// a feature. Best-effort on dialogs and surface fragments — missing files are
// skipped silently. Does not touch disk; callers (typically saveBuildState)
// are responsible for serialization and writing.
func buildBaseline(cfg *config.Context, slug string) (*Baseline, error) {
	featurePath := cfg.FeaturePath(slug)

	intentsPath := filepath.Join(featurePath, "intents.md")
	intents, err := parser.ParseIntentsFile(intentsPath)
	if err != nil {
		return nil, fmt.Errorf("read intents: %w", err)
	}
	if len(intents) == 0 {
		return nil, fmt.Errorf("baseline refuses empty artifact: %s exists but has no intent blocks", intentsPath)
	}

	baseline := &Baseline{
		SchemaVersion: BaselineSchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Intents:       make(map[string]IntentHash),
		Sources:       &HashedSources{Intents: make(map[string]string)},
	}

	for _, intent := range intents {
		baseline.Intents[intent.Slug] = hashIntent(intent)
		baseline.Sources.Intents[intent.Slug] = hashIntentContent(intent)
	}

	// A missing dialogs.md is fine — dialogs may not be authored yet, and
	// the source-hash is simply omitted. A dialogs.md that exists but
	// parses to zero dialog blocks is treated the same way: for a
	// CLI/backend feature it is the intentional terminal stub ("no
	// interactive dialog turns"), which check-readiness and build-feature
	// already accept as a complete dialogs phase (dialogs are recommended,
	// not required). Refusing here would strand any feature that
	// legitimately has no dialogs at the final save-build-state step, so a
	// zero-block dialogs.md simply contributes no dialogs source-hash
	// section rather than erroring. (The empty-intents guard above stays:
	// intents, unlike dialogs, are required for every feature.)
	dialogsPath := filepath.Join(featurePath, "dialogs.md")
	if dialogs, err := parser.ParseDialogsFile(dialogsPath); err == nil && len(dialogs) > 0 {
		baseline.Sources.Dialogs = make(map[string]string)
		for _, dialog := range dialogs {
			baseline.Sources.Dialogs[dialog.Slug] = hashDialogContent(dialog)
		}
	}

	if surfacePath := parser.ResolveSurfacePath(featurePath); surfacePath != "" {
		if fragments, err := parser.ParseSurfaceFile(surfacePath); err == nil {
			baseline.Sources.SurfaceFragments = make(map[string]string)
			for _, frag := range fragments {
				baseline.Sources.SurfaceFragments[parser.Slugify(frag.Name)] = hashFragmentContent(frag)
			}
		}
	}

	// Advisory whole-file hashes for the three artifacts the buildfile
	// schema's fictional source-signatures: mechanism was supposed to
	// cover. All three are optional — a feature may have any subset of
	// surface.md/surface.yaml, capabilities.yaml, infrastructure.md per
	// the four-co-equal-artifact model.
	if hash, ok := hashWholeFile(filepath.Join(featurePath, "capabilities.yaml")); ok {
		baseline.Sources.Capabilities = hash
	}
	if hash, ok := hashWholeFile(filepath.Join(featurePath, "infrastructure.md")); ok {
		baseline.Sources.Infrastructure = hash
	}
	if hash, ok := hashWholeFile(filepath.Join(featurePath, "surface.yaml")); ok {
		baseline.Sources.SurfaceYAML = hash
	}

	// Project-scoped advisory hashes. Unlike the three above, these are not
	// per-feature files — every feature in the project shares them, so a
	// change to either dirties every feature that reads it.
	if hash, ok := hashWholeFile(cfg.DomainModelPath()); ok {
		baseline.Sources.Domain = hash
	}
	// The adapter this feature builds against is named by its own
	// buildfile, so reuse the same discovery check-buildfile uses rather
	// than re-deriving it from prototype-framework.
	if path := autoDiscoverAdapter(cfg, filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")); path != "" {
		if hash, ok := hashWholeFile(path); ok {
			baseline.Sources.AdapterVersion = hash
		}
	}
	// Hand-authored units, tracked exactly like the two above: shared,
	// whole-artifact, advisory. Recorded even for a feature that consumes
	// no unit, because "which features consume it" is deliberately not
	// modelled — see HashedSources.Authored.
	if unitHashes := authoredUnitHashes(cfg); len(unitHashes) > 0 {
		baseline.Sources.Authored = unitHashes
	}

	// The amendment ledger. Recorded unconditionally (a feature with no
	// amendments/ directory simply records nothing): the file
	// hashes feed the integrity check, and the highest sequence becomes
	// LastAppliedAmendment — save-build-state runs after a green build, at
	// which point the ledger is by definition fully applied.
	if amendments, err := parser.LoadFeatureAmendments(featurePath); err == nil && len(amendments) > 0 {
		baseline.Sources.Amendments = make(map[string]string)
		for _, a := range amendments {
			if hash, ok := hashWholeFile(a.Path); ok {
				baseline.Sources.Amendments[filepath.Base(a.Path)] = hash
			}
			if a.Seq > baseline.LastAppliedAmendment {
				baseline.LastAppliedAmendment = a.Seq
			}
		}
	}

	return baseline, nil
}

// hashWholeFile returns a content hash for the entire file at path, and
// false when the file doesn't exist (any other read error is also
// treated as "no hash" — advisory hashing must never fail a baseline
// build over a file it doesn't strictly need).
func hashWholeFile(path string) (hash string, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return sha256Hex(string(data)), true
}

// marshalBaseline serializes a Baseline to YAML bytes for atomic disk writes.
func marshalBaseline(b *Baseline) ([]byte, error) {
	return yaml.Marshal(b)
}

// baselineContentUnchanged reports whether the freshly-built baseline differs
// from the one already on disk in nothing but its generated-at timestamp.
//
// GeneratedAt is the struct's only wall-clock field — every other field is a
// content hash or a count derived from the source, so re-running the build
// over an unedited feature reproduces them exactly. Zeroing that one field on
// both sides and comparing the marshaled bytes isolates "same content, fresh
// stamp" (skip the write, keep the old blame) from any real change (write).
//
// Returns (false, nil) when no baseline exists yet — a first save always
// writes. A read or unmarshal error other than not-exist is returned so the
// caller fails loudly rather than defaulting to skip-or-rewrite on a corrupt
// baseline.
func baselineContentUnchanged(path string, fresh *Baseline) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var prior Baseline
	if err := yaml.Unmarshal(data, &prior); err != nil {
		return false, fmt.Errorf("read existing baseline %s: %w", path, err)
	}

	priorCopy := prior
	freshCopy := *fresh
	priorCopy.GeneratedAt = ""
	freshCopy.GeneratedAt = ""

	priorBytes, err := yaml.Marshal(priorCopy)
	if err != nil {
		return false, err
	}
	freshBytes, err := yaml.Marshal(freshCopy)
	if err != nil {
		return false, err
	}
	return string(priorBytes) == string(freshBytes), nil
}

func runCheckDrift(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	featurePath := cfg.FeaturePath(slug)

	output, err := detectDrift(cfg, slug, featurePath)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

func detectDrift(cfg *config.Context, slug, featurePath string) (*driftOutput, error) {
	output := &driftOutput{Feature: slug}

	// Load baseline
	blPath := baselinePath(cfg, slug)
	blData, err := os.ReadFile(blPath)
	if err != nil {
		// No baseline = no drift to detect
		return output, nil
	}

	var baseline Baseline
	if err := yaml.Unmarshal(blData, &baseline); err != nil {
		return nil, fmt.Errorf("invalid baseline: %w", err)
	}

	// Load current intents
	intents, err := parser.ParseIntentsFile(filepath.Join(featurePath, "intents.md"))
	if err != nil {
		return nil, fmt.Errorf("failed to read intents: %w", err)
	}

	// Founding docs are frozen at first build (the baseline IS the freeze
	// point), so any change to intents.md is an integrity finding, not
	// rebuild-drift. A pre-v0.4 edit made when editing was legal dissolves
	// via `parlay migrate-ledger`, which re-stamps the founding hashes.
	currentSlugs := make(map[string]bool)
	for _, intent := range intents {
		currentSlugs[intent.Slug] = true
		oldHash, exists := baseline.Intents[intent.Slug]
		if !exists {
			output.LedgerIntegrity = append(output.LedgerIntegrity,
				"intents.md: intent \""+intent.Slug+"\" added after freeze — new ground goes through /parlay-loop, changes through an amendment (/parlay-refine); for a pre-v0.4 edit the freeze shouldn't count, run parlay migrate-ledger")
			continue
		}

		newHash := hashIntent(intent)
		if changed := diffHashes(oldHash, newHash); len(changed) > 0 {
			output.LedgerIntegrity = append(output.LedgerIntegrity,
				"intents.md: intent \""+intent.Slug+"\" changed after freeze — record the change as an amendment (/parlay-refine) and restore the founding text; for a pre-v0.4 edit the freeze shouldn't count, run parlay migrate-ledger")
		}
	}

	// Detect removed intents
	for slug := range baseline.Intents {
		if !currentSlugs[slug] {
			output.LedgerIntegrity = append(output.LedgerIntegrity,
				"intents.md: intent \""+slug+"\" removed after freeze — a dead decision is superseded by an amendment, not erased")
		}
	}

	// Dialogs are frozen too, and the amendment ledger itself has
	// integrity and an unapplied tail to report.
	if baseline.Sources != nil {
		detectLedgerFindings(&baseline, featurePath, output)
	}

	// Project-scoped shared sources. Previously untracked entirely, which
	// meant editing the canonical domain-model.yaml — the whole point of
	// the Studio editor — left every dependent feature reporting
	// has_drift:false, so nothing was ever marked for rebuild.
	//
	// Pre-v1 baselines carry no stored hash for these; "missing means
	// unknown, not drifted" so that upgrading the binary does not report
	// every existing project as drifted. The next save-build-state
	// backfills them.
	if baseline.Sources != nil {
		for _, s := range []struct {
			name       string
			path       string
			storedHash string
		}{
			{"domain-model", cfg.DomainModelPath(), baseline.Sources.Domain},
			{"adapter", autoDiscoverAdapter(cfg, filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")), baseline.Sources.AdapterVersion},
		} {
			if s.path == "" || s.storedHash == "" {
				continue
			}
			if current, ok := hashWholeFile(s.path); ok && current != s.storedHash {
				output.SharedSourcesChanged = append(output.SharedSourcesChanged, s.name)
			}
		}

		// Hand-authored units. This is the comparison the whole of Part A
		// is for: two commits to a hand-written engine invalidated fixture
		// numbers in two dependent buildfiles, and nothing anywhere
		// reported it, because the engine's files were not merely
		// untracked — no ingestion path existed that could return them.
		//
		// Iterating the STORED map, not the current one: a unit that
		// appeared since the baseline was written has no stored hash to
		// compare against, and calling that drift would flag every project
		// the moment it declared its first unit. A unit that has since been
		// removed is caught below.
		if len(baseline.Sources.Authored) > 0 {
			current := authoredUnitHashes(cfg)
			for unit, storedHash := range baseline.Sources.Authored {
				currentHash, stillPresent := current[unit]
				if !stillPresent {
					output.SharedSourcesChanged = append(output.SharedSourcesChanged, "unit:"+unit+" (removed)")
					continue
				}
				if currentHash != storedHash {
					output.SharedSourcesChanged = append(output.SharedSourcesChanged, "unit:"+unit)
				}
			}
			sort.Strings(output.SharedSourcesChanged)
		}
	}

	output.HasDrift = len(output.Drifted) > 0 || len(output.NewIntents) > 0 ||
		len(output.Removed) > 0 || len(output.SharedSourcesChanged) > 0 ||
		len(output.LedgerIntegrity) > 0 || len(output.UnappliedAmendments) > 0
	return output, nil
}

// detectLedgerFindings adds the ledger findings to a drift output:
// frozen-dialog edits, mutated or deleted amendment files, and the unapplied
// ledger tail. Intents are handled inline in detectDrift, where the per-slug
// comparison already exists.
func detectLedgerFindings(baseline *Baseline, featurePath string, output *driftOutput) {
	// Dialogs freeze. Same per-slug comparison parlay diff uses for
	// scoping, reinterpreted: any change is an integrity finding.
	if len(baseline.Sources.Dialogs) > 0 {
		current := map[string]string{}
		if dialogs, err := parser.ParseDialogsFile(filepath.Join(featurePath, "dialogs.md")); err == nil {
			for _, d := range dialogs {
				current[d.Slug] = hashDialogContent(d)
			}
		}
		for slug, stored := range baseline.Sources.Dialogs {
			cur, present := current[slug]
			switch {
			case !present:
				output.LedgerIntegrity = append(output.LedgerIntegrity,
					"dialogs.md: dialog \""+slug+"\" removed after freeze")
			case cur != stored:
				output.LedgerIntegrity = append(output.LedgerIntegrity,
					"dialogs.md: dialog \""+slug+"\" changed after freeze")
			}
		}
		for slug := range current {
			if _, was := baseline.Sources.Dialogs[slug]; !was {
				output.LedgerIntegrity = append(output.LedgerIntegrity,
					"dialogs.md: dialog \""+slug+"\" added after freeze")
			}
		}
	}

	// Amendment ledger: stored files must still exist byte-identical
	// (append-only means the only legitimate change is new files), and the
	// tail beyond last-applied is the unapplied set.
	amendments, err := parser.LoadFeatureAmendments(featurePath)
	if err != nil {
		output.LedgerIntegrity = append(output.LedgerIntegrity, "amendments: "+err.Error())
		return
	}
	currentByName := map[string]string{}
	for _, a := range amendments {
		if hash, ok := hashWholeFile(a.Path); ok {
			currentByName[filepath.Base(a.Path)] = hash
		}
		if a.Seq > baseline.LastAppliedAmendment {
			output.UnappliedAmendments = append(output.UnappliedAmendments, filepath.Base(a.Path))
		}
	}
	for name, stored := range baseline.Sources.Amendments {
		cur, present := currentByName[name]
		switch {
		case !present:
			output.LedgerIntegrity = append(output.LedgerIntegrity,
				"amendments/"+name+" removed from the ledger — history is retained, not erased")
		case cur != stored:
			output.LedgerIntegrity = append(output.LedgerIntegrity,
				"amendments/"+name+" mutated after being recorded — an amendment is written once; a correction is a new amendment")
		}
	}
	sort.Strings(output.LedgerIntegrity)
	sort.Strings(output.UnappliedAmendments)
}

func hashIntent(intent parser.Intent) IntentHash {
	return IntentHash{
		ContentHash: sha256Hex(fmt.Sprintf("%s|%s|%s|%s|%v|%v|%v",
			intent.Goal, intent.Persona, intent.Context, intent.Action,
			intent.Objects, intent.Constraints, intent.Verify)),
		Goal:        sha256Hex(intent.Goal),
		Constraints: sha256Hex(fmt.Sprintf("%v", intent.Constraints)),
		Verify:      sha256Hex(fmt.Sprintf("%v", intent.Verify)),
		Objects:     sha256Hex(fmt.Sprintf("%v", intent.Objects)),
	}
}

func diffHashes(old, new IntentHash) []string {
	var changed []string
	if old.Goal != new.Goal {
		changed = append(changed, "Goal")
	}
	if old.Constraints != new.Constraints {
		changed = append(changed, "Constraints")
	}
	if old.Verify != new.Verify {
		changed = append(changed, "Verify")
	}
	if old.Objects != new.Objects {
		changed = append(changed, "Objects")
	}
	return changed
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:8]) // 16-char hex, enough for drift detection
}

// hashIntentContent returns a content hash for an entire intent — used by
// parlay diff to detect intent changes at the source-element level.
// Distinct from hashIntent (above), which produces per-field hashes for
// granular drift detection.
func hashIntentContent(intent parser.Intent) string {
	return sha256Hex(fmt.Sprintf("%s|%s|%s|%s|%s|%s|%v|%v|%v|%v",
		intent.Title, intent.Goal, intent.Persona, intent.Priority,
		intent.Context, intent.Action,
		intent.Objects, intent.Constraints, intent.Verify, intent.Questions))
}

// hashDialogContent returns a content hash for an entire dialog including
// all its turns and options. Used by parlay diff.
func hashDialogContent(dialog parser.Dialog) string {
	var b strings.Builder
	b.WriteString(dialog.Title)
	b.WriteString("|")
	b.WriteString(dialog.Trigger)
	for _, turn := range dialog.Turns {
		b.WriteString("|")
		b.WriteString(turn.Speaker)
		b.WriteString(":")
		b.WriteString(turn.Type)
		b.WriteString(":")
		b.WriteString(turn.Condition)
		b.WriteString(":")
		b.WriteString(turn.Content)
		for _, opt := range turn.Options {
			b.WriteString("/")
			b.WriteString(opt.Letter)
			b.WriteString(":")
			b.WriteString(opt.Desc)
		}
	}
	return sha256Hex(b.String())
}

// resolveBuildfileSectionNode returns the raw YAML value for a hashed
// buildfile section, resolving the v2 relocation of routes: into
// targets.presentation.routes:. models: and fixtures: stay top-level in both
// the v1 and v2 shapes, so only routes: needs the fallback.
//
// It returns the RAW node (not a re-typed struct) so a v1 buildfile hashes
// byte-identically to before this fallback existed: raw[key] is checked first
// and short-circuits for every v1 file. The fallback fires only for a v2
// buildfile whose top-level routes: is absent — which previously produced an
// empty routes hash for every multi-target project, so its cross-cutting
// regeneration signal never saw a route change. Making it v2-aware changes
// those projects' section hashes once; see WP2.1 re-baseline note.
func resolveBuildfileSectionNode(raw map[string]interface{}, key string) (interface{}, bool) {
	if section, ok := raw[key]; ok {
		return section, true
	}
	if key == "routes" {
		if targets, ok := raw["targets"].(map[string]interface{}); ok {
			if pres, ok := targets["presentation"].(map[string]interface{}); ok {
				if routes, ok := pres["routes"]; ok {
					return routes, true
				}
			}
		}
	}
	return nil, false
}

// hashBuildfileSections reads a buildfile.yaml and returns per-section
// content hashes for the major sections (models, routes, fixtures). Used
// by save-build-state to track which buildfile sections changed between
// generations, enabling the diff command to report section-level changes
// for cross-cutting files.
//
// Returns nil (no error) if the buildfile doesn't exist yet.
func hashBuildfileSections(buildfilePath string) (map[string]string, error) {
	data, err := os.ReadFile(buildfilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	sections := make(map[string]string)
	for _, key := range []string{"models", "routes", "fixtures"} {
		if section, ok := resolveBuildfileSectionNode(raw, key); ok {
			// Re-serialize the section for a stable hash (map key ordering
			// in YAML is deterministic per the yaml.v3 library).
			sectionBytes, err := yaml.Marshal(section)
			if err != nil {
				continue
			}
			sections[key] = sha256Hex(string(sectionBytes))
		}
	}
	return sections, nil
}

// hashFragmentContent returns a content hash for a surface fragment.
// Used by parlay diff.
func hashFragmentContent(frag parser.Fragment) string {
	return sha256Hex(fmt.Sprintf("%s|%s|%s|%s|%s|%s|%d|%v",
		frag.Name, frag.Shows, frag.Actions, frag.Source,
		frag.Page, frag.Region, frag.Order, frag.Notes))
}
