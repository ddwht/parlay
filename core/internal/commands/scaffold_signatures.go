package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
)

// scaffoldSignaturesCmd computes and writes a buildfile's
// `source-signatures:` block.
//
// The block is the input to the codegen freshness gate: a content hash per
// source artifact the buildfile consumed, so a buildfile built from stale
// sources refuses to generate. Every value in it is a sha256 of a file that
// is sitting right there — no judgment, no framework knowledge, nothing an
// agent is better at than a hash function.
//
// It was nonetheless authored by hand, and the regression run found what
// that costs: signatures naming artifacts the feature does not have, and
// signatures omitted for artifacts it does. Both fail in the same
// direction. A gate whose recorded inputs are wrong does not fail loudly —
// it passes, and codegen proceeds against sources nobody checked.
//
// This does not move the gate itself into Go. The comparison stays where
// buildfile.schema.md puts it, in the generate-code phase; only the
// recorded values become computed rather than transcribed.
var scaffoldSignaturesCmd = &cobra.Command{
	Use:   "scaffold-signatures @<feature>",
	Short: "Compute and write a buildfile's source-signatures block (JSON output)",
	Args:  cobra.ExactArgs(1),
	RunE:  runScaffoldSignatures,
}

var scaffoldSignaturesAdapter string

func init() {
	scaffoldSignaturesCmd.Flags().StringVar(&scaffoldSignaturesAdapter, "adapter", "",
		"Adapter file to hash for adapter-version (default: the project's registered adapter)")
}

// signatureSource pairs a signature field name with the file it hashes.
// Order is the schema's order, so a regenerated block diffs cleanly against
// a hand-written one.
type signatureSource struct {
	field string
	path  string
}

// featureSignatureSources returns the artifacts whose content feeds a
// feature's signature block. A source that does not exist is skipped, per
// the schema's "when <artifact> exists" column — recording a hash for an
// absent file would make the gate fail forever.
func featureSignatureSources(featureDir, projectRoot string, hasUnits bool) []signatureSource {
	surface := filepath.Join(featureDir, "surface.yaml")
	if !fileExistsAt(surface) {
		surface = filepath.Join(featureDir, "surface.md")
	}
	candidates := []signatureSource{
		{"intents", filepath.Join(featureDir, "intents.md")},
		{"dialogs", filepath.Join(featureDir, "dialogs.md")},
		{"surface", surface},
		{"capabilities", filepath.Join(featureDir, "capabilities.yaml")},
		{"infrastructure", filepath.Join(featureDir, "infrastructure.md")},
		{"domain", filepath.Join(projectRoot, "domain-model.yaml")},
	}
	var out []signatureSource
	for _, c := range candidates {
		if fileExistsAt(c.path) {
			out = append(out, c)
		}
	}
	// Layout files are per-page; hash them collectively so a change to any
	// page's layout invalidates the buildfile that consumed it.
	if layouts, _ := filepath.Glob(filepath.Join(featureDir, "*.layout.yaml")); len(layouts) > 0 {
		out = append(out, signatureSource{"layout", ""}) // path filled by caller
	}
	// Hand-authored units, hashed collectively for the same reason layouts
	// are: one field per artifact kind, not one per file.
	//
	// This is the entry that makes a unit's code block a stale build. The
	// advisory hashes in .baseline.yaml report a changed unit but stop
	// nothing; source-signatures is the gate generate-code actually
	// refuses on. A hand-written engine whose behaviour moved under a
	// feature's fixtures is exactly the case where refusing is right.
	if hasUnits {
		out = append(out, signatureSource{"authored", ""}) // aggregate, filled by caller
	}
	return out
}

// contentHash returns "sha256:<hex>" for a file's bytes — the prefixed,
// full-length form buildfiles already carry, so regenerating a hand-written
// block changes no formatting. Content, never mtime: re-saving an unedited file has to leave
// the signature identical, or the gate cries stale on every checkout.
func contentHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// combinedHash hashes several files as one value, in the order given.
func combinedHash(paths []string) (string, error) {
	h := sha256.New()
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		h.Write([]byte(filepath.Base(p)))
		h.Write(data)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// computeSourceSignatures builds the signature map for one feature.
func computeSourceSignatures(featureDir, projectRoot, adapterPath string, unitHashes map[string]string) (map[string]string, error) {
	sigs := map[string]string{}
	for _, s := range featureSignatureSources(featureDir, projectRoot, len(unitHashes) > 0) {
		if s.field == "authored" {
			sigs["authored"] = combineUnitHashes(unitHashes)
			continue
		}
		if s.field == "layout" {
			layouts, err := filepath.Glob(filepath.Join(featureDir, "*.layout.yaml"))
			if err != nil {
				return nil, err
			}
			sort.Strings(layouts)
			h, err := combinedHash(layouts)
			if err != nil {
				return nil, err
			}
			sigs["layout"] = h
			continue
		}
		h, err := contentHash(s.path)
		if err != nil {
			return nil, err
		}
		sigs[s.field] = h
	}

	// adapter-version is the one required field. Without it the gate cannot
	// tell an adapter upgrade from a no-op, and an adapter upgrade changes
	// every emitted file.
	if adapterPath == "" {
		return nil, fmt.Errorf("no adapter found; pass --adapter <path>")
	}
	h, err := contentHash(adapterPath)
	if err != nil {
		return nil, fmt.Errorf("hash adapter %s: %w", adapterPath, err)
	}
	sigs["adapter-version"] = h
	return sigs, nil
}

// writeSourceSignatures replaces the `source-signatures:` block in a
// buildfile by splicing text, not by re-encoding YAML.
//
// A buildfile is a 700-line human-reviewed document: folded descriptions,
// blank lines grouping related components, comments explaining why a field
// is the way it is. Round-tripping it through a YAML encoder preserves
// every value and destroys all of that — the first version of this
// function silently reflowed submit-expense from 701 lines to 644, which
// makes the next review diff unreadable and is the same complaint filed
// against Studio for dropping hand-written comments on save.
//
// So: find the block, replace exactly those lines, leave every other byte
// untouched.
func writeSourceSignatures(buildfilePath string, sigs map[string]string) error {
	data, err := os.ReadFile(buildfilePath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")

	var block []string
	block = append(block, "source-signatures:")
	for _, field := range signatureFieldOrder {
		if v, ok := sigs[field]; ok {
			block = append(block, fmt.Sprintf("  %s: %q", field, v))
		}
	}

	start, end := findTopLevelBlock(lines, "source-signatures:")
	if start < 0 {
		// Absent: append at the end, after any trailing blank lines.
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, block...)
		lines = append(lines, "")
	} else {
		out := append([]string{}, lines[:start]...)
		out = append(out, block...)
		out = append(out, lines[end:]...)
		lines = out
	}
	return os.WriteFile(buildfilePath, []byte(strings.Join(lines, "\n")), 0644)
}

// findTopLevelBlock locates a top-level `key:` and the line after its last
// indented child. Returns (-1, -1) when the key is absent.
//
// Blank lines inside a block belong to it; a blank line before the next
// top-level key would otherwise be swallowed into the replacement and
// quietly close up the spacing the author chose.
func findTopLevelBlock(lines []string, key string) (int, int) {
	start := -1
	for i, l := range lines {
		if l == key || strings.HasPrefix(l, key+" ") {
			start = i
			break
		}
	}
	if start < 0 {
		return -1, -1
	}
	end := start + 1
	lastContent := end
	for end < len(lines) {
		l := lines[end]
		if strings.TrimSpace(l) == "" {
			end++
			continue
		}
		if !strings.HasPrefix(l, " ") && !strings.HasPrefix(l, "\t") {
			break
		}
		end++
		lastContent = end
	}
	return start, lastContent
}

// signatureFieldOrder pins emission order to the schema's field order, so
// regenerating a hand-written block produces a diff of changed hashes
// rather than a reordering nobody can read.
var signatureFieldOrder = []string{
	"intents", "dialogs", "surface", "capabilities",
	"infrastructure", "domain", "layout", "authored", "adapter-version",
}

// combineUnitHashes folds every unit's aggregate hash into one signature
// value. Sorted by unit id so the result depends on the set, not on map
// iteration order — an unstable value here would rewrite the buildfile's
// signature block on every run and make the gate fire at random.
func combineUnitHashes(unitHashes map[string]string) string {
	units := make([]string, 0, len(unitHashes))
	for u := range unitHashes {
		units = append(units, u)
	}
	sort.Strings(units)

	h := sha256.New()
	for _, u := range units {
		h.Write([]byte(u))
		h.Write([]byte{0})
		h.Write([]byte(unitHashes[u]))
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

type scaffoldSignaturesOutput struct {
	Feature    string            `json:"feature"`
	Buildfile  string            `json:"buildfile"`
	Signatures map[string]string `json:"signatures"`
	Skipped    []string          `json:"skipped_absent_artifacts,omitempty"`
}

func runScaffoldSignatures(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	featureDir := cfg.FeaturePath(slug)
	if !fileExistsAt(filepath.Join(featureDir, "intents.md")) && !dirExists(featureDir) {
		return fmt.Errorf("feature %s: no %s", slug, featureDir)
	}
	buildfilePath := filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")
	if !fileExistsAt(buildfilePath) {
		return fmt.Errorf("feature %s: no buildfile at %s — run the build phase first", slug, buildfilePath)
	}

	adapterPath := scaffoldSignaturesAdapter
	if adapterPath == "" {
		adapterPath = presentationAdapterFile(cfg)
	}

	sigs, err := computeSourceSignatures(featureDir, cfg.RepoRoot(), adapterPath, authoredUnitHashes(cfg))
	if err != nil {
		return err
	}
	if err := writeSourceSignatures(buildfilePath, sigs); err != nil {
		return err
	}

	var skipped []string
	for _, f := range signatureFieldOrder {
		if _, ok := sigs[f]; !ok {
			skipped = append(skipped, f)
		}
	}

	out, _ := json.MarshalIndent(scaffoldSignaturesOutput{
		Feature:    slug,
		Buildfile:  buildfilePath,
		Signatures: sigs,
		Skipped:    skipped,
	}, "", "  ")
	fmt.Fprintln(cmd.OutOrStdout(), string(out))
	return nil
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// presentationAdapterFile resolves the adapter whose vocabulary the
// presentation-scoped commands (signatures, composition/shared-store) need.
// In a multi-target project it returns the presentation slot's adapter from
// adapter-set.yaml, resolved child-first with parent fallback so a child root
// inherits the parent's adapters. Falls back to soleAdapterFile for
// single-target projects with no adapter-set, which refuses to guess when a
// root holds several adapters rather than picking by filename order.
func presentationAdapterFile(cfg *config.Context) string {
	if as, err := parser.ParseAdapterSet(cfg.AdapterSetPath()); err == nil {
		if tgt, ok := as.Targets["presentation"]; ok && tgt.Adapter != "" {
			// ResolveAdapter, not a bare join: in a multi-root project a child
			// inherits the parent's adapters, which is what the generated
			// CLAUDE.md has always told users happens.
			if p, _ := cfg.ResolveAdapter(tgt.Adapter); p != "" {
				return p
			}
		}
	}
	if p, err := soleAdapterFile(cfg); err == nil {
		return p
	}
	return ""
}

// soleAdapterFile resolves "the" adapter for a project that has not pinned an
// adapter-set: the single adapter file at the active root, or — for a child
// root with none of its own — the single one inherited from the parent.
//
// It refuses to choose when several are present. The predecessor
// (firstAdapterFile) returned the lexically-first match, which in a
// react-nest-prisma project is `nestjs-application.adapter.yaml` — a widget-less
// backend adapter silently standing in for the presentation one. A wrong
// adapter produces plausible, wrong paths; an error produces a fix.
func soleAdapterFile(cfg *config.Context) (string, error) {
	dirs := []string{cfg.AdaptersPath()}
	if cfg.Root.Kind == config.RootKindChild && cfg.Root.ParentPath != "" {
		dirs = append(dirs, filepath.Join(cfg.Root.ParentPath, config.ParlayDir, config.AdaptersDir))
	}
	for _, dir := range dirs {
		matches, err := filepath.Glob(filepath.Join(dir, "*.adapter.yaml"))
		if err != nil || len(matches) == 0 {
			continue
		}
		sort.Strings(matches)
		if len(matches) == 1 {
			return matches[0], nil
		}
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, strings.TrimSuffix(filepath.Base(m), ".adapter.yaml"))
		}
		return "", fmt.Errorf("%s holds %d adapters (%s) and the project pins no .parlay/adapter-set.yaml — "+
			"declare which adapter fills which target kind rather than leaving the choice to filename order",
			dir, len(matches), strings.Join(names, ", "))
	}
	return "", fmt.Errorf("no adapter found under %s", cfg.AdaptersPath())
}

