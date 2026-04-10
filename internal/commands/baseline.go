package commands

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ddwht/parlay/internal/config"
	"github.com/ddwht/parlay/internal/parser"
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
	GeneratedAt       string                `yaml:"generated-at"`
	Intents           map[string]IntentHash `yaml:"intents"`
	Sources           *HashedSources        `yaml:"sources,omitempty"`
	BuildfileSections map[string]string     `yaml:"buildfile-sections,omitempty"`
}

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
}

func baselinePath(slug string) string {
	return filepath.Join(config.BuildPath(slug), ".baseline.yaml")
}

// buildBaseline computes a Baseline struct from the current source files of
// a feature. Best-effort on dialogs and surface fragments — missing files are
// skipped silently. Does not touch disk; callers (typically saveBuildState)
// are responsible for serialization and writing.
func buildBaseline(slug string) (*Baseline, error) {
	featurePath := config.FeaturePath(slug)

	intents, err := parser.ParseIntentsFile(filepath.Join(featurePath, "intents.md"))
	if err != nil {
		return nil, fmt.Errorf("read intents: %w", err)
	}

	baseline := &Baseline{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Intents:     make(map[string]IntentHash),
		Sources:     &HashedSources{Intents: make(map[string]string)},
	}

	for _, intent := range intents {
		baseline.Intents[intent.Slug] = hashIntent(intent)
		baseline.Sources.Intents[intent.Slug] = hashIntentContent(intent)
	}

	if dialogs, err := parser.ParseDialogsFile(filepath.Join(featurePath, "dialogs.md")); err == nil {
		baseline.Sources.Dialogs = make(map[string]string)
		for _, dialog := range dialogs {
			baseline.Sources.Dialogs[dialog.Slug] = hashDialogContent(dialog)
		}
	}

	if fragments, err := parser.ParseSurfaceFile(filepath.Join(featurePath, "surface.md")); err == nil {
		baseline.Sources.SurfaceFragments = make(map[string]string)
		for _, frag := range fragments {
			baseline.Sources.SurfaceFragments[parser.Slugify(frag.Name)] = hashFragmentContent(frag)
		}
	}

	return baseline, nil
}

// marshalBaseline serializes a Baseline to YAML bytes for atomic disk writes.
func marshalBaseline(b *Baseline) ([]byte, error) {
	return yaml.Marshal(b)
}

func runCheckDrift(cmd *cobra.Command, args []string) error {
	slug := strings.TrimPrefix(args[0], "@")
	featurePath := config.FeaturePath(slug)

	output, err := detectDrift(slug, featurePath)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func detectDrift(slug, featurePath string) (*driftOutput, error) {
	output := &driftOutput{Feature: slug}

	// Load baseline
	blPath := baselinePath(slug)
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

	output.HasDrift = len(output.Drifted) > 0 || len(output.NewIntents) > 0 || len(output.Removed) > 0
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
