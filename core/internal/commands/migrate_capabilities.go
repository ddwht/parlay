// parlay-feature: parlay-tool/multi-adapter
// parlay-component: capabilities-migration-operations-extraction
// parlay-extends: parlay-tool/architectural-prose-artifact/partial-migration-semantics-in-migrate-capabilities
//
// Walks each feature's infrastructure.md and moves operation-shaped
// fragments into capabilities.yaml while leaving architectural-prose
// fragments (boundaries, probes, allowlists, dependency pins, and other
// non-operation-shaped concerns) in place. Partial migration is the
// success case: it is normal and expected for some fragments to be
// extracted and others retained, and a feature whose infrastructure.md
// contains only architectural prose is a successful all-retained run
// with exit code zero.
//
// The runtime output is partitioned per feature into three sections:
// "Extracted to capabilities.yaml", "Retained in infrastructure.md", and
// (when applicable) a "Deleted: infrastructure.md (was empty after
// extraction)" line. Empty partitions are printed explicitly, not
// suppressed, so a developer reading the output understands the
// partition shape regardless of which side was empty.
//
// --dry-run prints the same partition output with a "(dry run — no
// files written)" header and leaves the feature folder byte-identical
// to before the run.

package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/spf13/cobra"
)

var migrateCapabilitiesDryRun bool

var migrateCapabilitiesCmd = &cobra.Command{
	Use:   "migrate-capabilities",
	Short: "Move operation-shaped fragments from infrastructure.md into capabilities.yaml; retain architectural-prose fragments in place",
	Long: `Move operation-shaped fragments from infrastructure.md into
capabilities.yaml. Architectural-prose fragments (boundaries, probes,
allowlists, dependency pins, and other non-operation-shaped concerns)
are retained in infrastructure.md.

Partial migration is the success case. For each feature the command
prints two lists:

  Extracted to capabilities.yaml:
    - <feature-local fragment id> -> <new operation id>
  Retained in infrastructure.md:
    - <fragment ## heading>

A feature whose infrastructure.md contains no operation-shaped fragments
is a successful all-retained run with exit code zero; a feature where
every fragment is operation-shaped extracts everything and deletes the
now-empty infrastructure.md, printing a "Deleted: infrastructure.md
(was empty after extraction)" line. Empty partitions are printed
explicitly so the partition shape is always visible.

Use --dry-run to preview the partition without writing anything; the
feature folder is left byte-identical to before the dry run.`,
	Args: cobra.NoArgs,
	RunE: runMigrateCapabilities,
}

func init() {
	migrateCapabilitiesCmd.Flags().BoolVar(&migrateCapabilitiesDryRun, "dry-run", false,
		"Print the would-be partition for every feature; touch nothing on disk")
}

// fragment represents one `## `-delimited block inside an
// infrastructure.md file, captured verbatim so retained fragments can
// be written back without lexical drift.
type fragment struct {
	heading string // e.g. "Task storage boundary"
	body    string // verbatim block including the "## " heading line
}

// featurePartition is the per-feature result of classifying every
// fragment in its infrastructure.md.
type featurePartition struct {
	feature    string
	featDir    string
	infraPath  string
	capPath    string
	header     string // optional preamble before the first `---` / `## `
	separator  string // "\n\n---\n\n" or whatever joined the original fragments
	extracted  []extractedOp
	retained   []fragment
	skipped    bool // capabilities.yaml already existed
	skipReason string
	readErr    error
	unrouted   []agent.ClassificationResult // classifier output for retained fragments
}

type extractedOp struct {
	fragmentID string // feature-local fragment id (slug of the heading)
	opID       string // capabilities.yaml operation id (also the slug)
	stub       string // YAML stub body
}

func runMigrateCapabilities(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}

	intentsRoot := filepath.Join(cfg.Root.Path, "spec", "intents")
	infraFiles, err := walkInfraFiles(intentsRoot)
	if err != nil {
		return fmt.Errorf("walk intents tree: %w", err)
	}

	out := cmd.OutOrStdout()

	if migrateCapabilitiesDryRun {
		fmt.Fprintln(out, "(dry run — no files written)")
	}

	totalExtracted, totalRetained, totalDeleted := 0, 0, 0
	for _, infraPath := range infraFiles {
		feature, _ := filepath.Rel(intentsRoot, filepath.Dir(infraPath))
		featDir := filepath.Dir(infraPath)
		capPath := filepath.Join(featDir, "capabilities.yaml")

		part := &featurePartition{
			feature:   feature,
			featDir:   featDir,
			infraPath: infraPath,
			capPath:   capPath,
		}

		if _, err := os.Stat(capPath); err == nil {
			part.skipped = true
			part.skipReason = "capabilities.yaml already exists"
			fmt.Fprintf(out, "  %s — %s; skipping\n", feature, part.skipReason)
			continue
		}

		header, separator, fragments, err := readFragments(infraPath)
		if err != nil {
			part.readErr = err
			fmt.Fprintf(out, "  %s — read failed: %v\n", feature, err)
			continue
		}
		part.header = header
		part.separator = separator

		for _, f := range fragments {
			if fragmentIsOperationShaped(f) {
				op := extractOperationFromFragment(f)
				part.extracted = append(part.extracted, op)
			} else {
				part.retained = append(part.retained, f)
				// Classifier output is informational only; it does not
				// route the fragment anywhere automatically.
				part.unrouted = append(part.unrouted, agent.ClassifyPatternFragment(f.body))
			}
		}

		printPartition(out, part)

		if !migrateCapabilitiesDryRun {
			if err := applyPartition(part); err != nil {
				return err
			}
		}

		totalExtracted += len(part.extracted)
		totalRetained += len(part.retained)
		if !migrateCapabilitiesDryRun && len(part.extracted) > 0 && len(part.retained) == 0 {
			totalDeleted++
		}
	}

	fmt.Fprintf(out, "Operations extracted: %d total\nFragments retained: %d total\nInfrastructure files deleted (empty after extraction): %d total\n",
		totalExtracted, totalRetained, totalDeleted)
	fmt.Fprintln(out, "The migrator moves operation-shaped content between two co-equal artifacts; non-operation-shaped fragments stay in infrastructure.md by design.")
	return nil
}

// printPartition emits the per-feature partition block in the canonical
// shape. Empty lists are printed explicitly with "(none)" so the
// partition shape is always visible, and the all-retained case prints
// a stable "no operation-shaped fragments to migrate" line.
func printPartition(out interface{ Write(p []byte) (int, error) }, part *featurePartition) {
	fmt.Fprintf(out, "  %s\n", part.feature)

	if len(part.extracted) == 0 && len(part.retained) == 0 {
		// No fragments at all — the file is empty or whitespace-only.
		fmt.Fprintln(out, "    Extracted to capabilities.yaml: (none)")
		fmt.Fprintln(out, "    Retained in infrastructure.md: (none)")
		return
	}

	if len(part.extracted) == 0 {
		fmt.Fprintln(out, "    no operation-shaped fragments to migrate; infrastructure.md left in place")
	} else {
		fmt.Fprintln(out, "    Extracted to capabilities.yaml:")
		for _, op := range part.extracted {
			fmt.Fprintf(out, "      - %s -> %s\n", op.fragmentID, op.opID)
		}
	}

	if len(part.retained) == 0 {
		fmt.Fprintln(out, "    Retained in infrastructure.md: (none)")
	} else {
		fmt.Fprintln(out, "    Retained in infrastructure.md:")
		for _, f := range part.retained {
			fmt.Fprintf(out, "      - %s\n", f.heading)
		}
	}

	if !migrateCapabilitiesDryRun && len(part.extracted) > 0 && len(part.retained) == 0 {
		fmt.Fprintln(out, "    Deleted: infrastructure.md (was empty after extraction)")
	}
}

// applyPartition writes the partition to disk: emits capabilities.yaml
// when at least one fragment was extracted, rewrites infrastructure.md
// to retain only the retained fragments, and deletes infrastructure.md
// when every fragment was extracted.
func applyPartition(part *featurePartition) error {
	if len(part.extracted) > 0 {
		stubs := make([]string, 0, len(part.extracted))
		for _, op := range part.extracted {
			stubs = append(stubs, op.stub)
		}
		capContent := buildCapabilitiesYAML(part.feature, stubs)
		if err := os.WriteFile(part.capPath, []byte(capContent), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", part.capPath, err)
		}
	}

	if len(part.extracted) > 0 && len(part.retained) == 0 {
		// Every fragment migrated out — delete the now-empty file rather
		// than leaving a zero-byte stub behind.
		if err := os.Remove(part.infraPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete %s: %w", part.infraPath, err)
		}
		return nil
	}

	if len(part.extracted) > 0 && len(part.retained) > 0 {
		// Rewrite infrastructure.md with only the retained fragments,
		// preserving the original header and separator style.
		newContent := writeRetainedInfra(part.header, part.separator, part.retained)
		if err := os.WriteFile(part.infraPath, []byte(newContent), 0o644); err != nil {
			return fmt.Errorf("rewrite %s: %w", part.infraPath, err)
		}
	}

	// All-retained: leave the file as-is byte-for-byte. No write needed.
	return nil
}

func walkInfraFiles(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) == "infrastructure.md" {
			out = append(out, path)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return out, err
}

// readFragments splits infrastructure.md into `## `-delimited fragments
// preserving each fragment's verbatim body. A leading top-level `# `
// title and any preamble before the first fragment are returned as
// `header`. Fragments are separated by lines containing only `---`.
func readFragments(path string) (header, separator string, fragments []fragment, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", nil, err
	}
	defer f.Close()

	var headerLines []string
	var current []string
	var currentHeading string
	separator = "\n\n---\n\n"

	flush := func() {
		if currentHeading == "" && len(current) == 0 {
			return
		}
		if currentHeading == "" {
			// Preamble before the first fragment becomes part of header.
			headerLines = append(headerLines, current...)
			current = nil
			return
		}
		body := strings.TrimRight(strings.Join(current, "\n"), "\n")
		fragments = append(fragments, fragment{heading: currentHeading, body: body})
		current = nil
		currentHeading = ""
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "## ") && currentHeading == "" && len(current) == 0 {
			currentHeading = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			current = append(current, line)
			continue
		}
		if strings.HasPrefix(line, "## ") {
			// Adjacent fragment without a `---` separator.
			flush()
			currentHeading = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			current = append(current, line)
			continue
		}
		current = append(current, line)
	}
	flush()
	if err := sc.Err(); err != nil {
		return "", "", nil, err
	}

	header = strings.TrimRight(strings.Join(headerLines, "\n"), "\n")
	return header, separator, fragments, nil
}

// fragmentIsOperationShaped classifies a fragment by inspecting its
// verbatim body with the existing closed-vocabulary verb detector. The
// detection logic is unchanged; this feature only changes the
// surrounding partition behavior.
func fragmentIsOperationShaped(f fragment) bool {
	return isOperationShaped(f.body)
}

// fragmentSlug derives a feature-local id from a fragment heading by
// lowercasing and replacing non-alphanumerics with dashes.
func fragmentSlug(heading string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(heading) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	s := b.String()
	s = strings.TrimRight(s, "-")
	if s == "" {
		return "unknown"
	}
	return s
}

func extractOperationFromFragment(f fragment) extractedOp {
	fragID := fragmentSlug(f.heading)
	opID := fragID
	stub := fmt.Sprintf(`  - id: %s
    kind: unknown
    subject:
      entity: TODO
    steps:
      - { type: validate-input }
    notes: |
%s
`, opID, indentBlock(f.body, "      "))
	return extractedOp{fragmentID: fragID, opID: opID, stub: stub}
}

// writeRetainedInfra reconstructs an infrastructure.md containing only
// the retained fragments, preserving the original header and separator
// style so retaining-only edits do not drift the file beyond what the
// fragment partition requires.
func writeRetainedInfra(header, separator string, retained []fragment) string {
	var parts []string
	if strings.TrimSpace(header) != "" {
		parts = append(parts, header)
	}
	for _, f := range retained {
		parts = append(parts, f.body)
	}
	joined := strings.Join(parts, separator)
	if !strings.HasSuffix(joined, "\n") {
		joined += "\n"
	}
	return joined
}

// readParagraphs is preserved for callers that want paragraph-level
// classification. The migrator itself now operates on fragments.
func readParagraphs(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var paragraphs []string
	var current []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				paragraphs = append(paragraphs, strings.Join(current, "\n"))
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		paragraphs = append(paragraphs, strings.Join(current, "\n"))
	}
	return paragraphs, sc.Err()
}

// isOperationShaped is a conservative detector: paragraphs naming a verb
// from the closed step set plus a domain entity. Detection logic is
// unchanged by the partial-migration-semantics feature.
func isOperationShaped(text string) bool {
	lower := strings.ToLower(text)
	verbs := []string{"validate-input", "create-one", "update-one", "delete-one", "read-one", "read-many", "search"}
	for _, v := range verbs {
		if strings.Contains(lower, v) {
			return true
		}
	}
	return false
}

func indentBlock(text, indent string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = indent + l
	}
	return strings.Join(lines, "\n")
}

func buildCapabilitiesYAML(feature string, operations []string) string {
	return fmt.Sprintf(`# parlay-feature: parlay-tool/multi-adapter
# parlay-component: capabilities-migration-operations-extraction
#
# Auto-extracted from infrastructure.md. Each operation has kind: unknown
# until a designer fills in the real value (command or query).

schema_version: 1
feature: %s

operations:
%s`, feature, strings.Join(operations, "\n"))
}
