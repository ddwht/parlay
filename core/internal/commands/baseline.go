package commands

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
}

// BaselineSchemaVersion is the current baseline format version.
//
//	1 — adds Domain and AdapterVersion to HashedSources.
const BaselineSchemaVersion = 1

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
	Intents             map[string]string `yaml:"intents,omitempty"`
	Dialogs             map[string]string `yaml:"dialogs,omitempty"`
	SurfaceFragments    map[string]string `yaml:"surface-fragments,omitempty"`
	DesignSpecFragments map[string]string `yaml:"design-spec-fragments,omitempty"`
	DesignSpecShared    string            `yaml:"design-spec-shared,omitempty"`

	// Capabilities, Infrastructure, and SurfaceYAML are whole-file
	// content hashes for capabilities.yaml, infrastructure.md, and
	// surface.yaml — advisory only. There is no per-operation or
	// per-fragment granularity here the way SurfaceFragments has for
	// surface.md; the buildfile schema's own source-signatures:
	// mechanism was found to be aspirational (never implemented), and
	// these three fields are the real advisory freshness signal for
	// those artifacts until a hard codegen gate is specced separately.
	// Empty string means the file didn't exist when the baseline was
	// captured.
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

	// Hash design-spec fragments (optional — missing file is not an error).
	designSpecPath := filepath.Join(cfg.BuildPath(slug), "design-spec.yaml")
	if dsFragments, dsShared, err := hashDesignSpecFragments(designSpecPath); err == nil {
		if len(dsFragments) > 0 || dsShared != "" {
			baseline.Sources.DesignSpecFragments = dsFragments
			baseline.Sources.DesignSpecShared = dsShared
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

	currentSlugs := make(map[string]bool)
	for _, intent := range intents {
		currentSlugs[intent.Slug] = true
		oldHash, exists := baseline.Intents[intent.Slug]
		if !exists {
			output.NewIntents = append(output.NewIntents, intent.Title)
			continue
		}

		newHash := hashIntent(intent)
		if changed := diffHashes(oldHash, newHash); len(changed) > 0 {
			output.Drifted = append(output.Drifted, driftItem{
				Intent:        intent.Title,
				ChangedFields: changed,
			})
		}
	}

	// Detect removed intents
	for slug := range baseline.Intents {
		if !currentSlugs[slug] {
			output.Removed = append(output.Removed, slug)
		}
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
	}

	output.HasDrift = len(output.Drifted) > 0 || len(output.NewIntents) > 0 ||
		len(output.Removed) > 0 || len(output.SharedSourcesChanged) > 0
	return output, nil
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
		if section, ok := raw[key]; ok {
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

// hashDesignSpecFragments parses a design-spec.yaml and returns per-fragment
// content hashes plus a hash of the shared section. Fragment keys are
// slugified for consistency with surface fragment slugs. Returns nil maps
// if the file doesn't exist.
func hashDesignSpecFragments(designSpecPath string) (fragments map[string]string, sharedHash string, err error) {
	data, err := os.ReadFile(designSpecPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", err
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, "", err
	}

	fragments = make(map[string]string)

	// Hash the shared section (affects all fragments).
	if shared, ok := raw["shared"]; ok {
		sharedBytes, err := yaml.Marshal(shared)
		if err == nil {
			sharedHash = sha256Hex(string(sharedBytes))
		}
	}

	// Hash each fragment entry.
	if fragsRaw, ok := raw["fragments"]; ok {
		if fragsMap, ok := fragsRaw.(map[string]interface{}); ok {
			for name, value := range fragsMap {
				fragBytes, err := yaml.Marshal(value)
				if err != nil {
					continue
				}
				fragments[parser.Slugify(name)] = sha256Hex(string(fragBytes))
			}
		}
	}

	return fragments, sharedHash, nil
}
