// parlay-feature: design-loop/vocabulary-validation
// parlay-component: cross-cutting/core-cli-wiring

package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/parlay-tool/parlay/studio/pkg/vocabulary"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// validateVocabularyCmd surfaces the studio/internal/vocabulary library
// as a CLI subcommand. The command resolves @<feature> to its layout
// artifact, reads the componentVocabulary field, resolves the matching
// adapter via the adapter-set, and emits the structured report as JSON
// on stdout. Exit code 0 when no error-severity entries; exit 1
// otherwise. The two stable error codes (vocabulary-missing-from-adapter,
// vocabulary-unknown-adapter) appear verbatim on stderr on resolution
// failure.
var validateVocabularyCmd = &cobra.Command{
	Use:   "validate-vocabulary @<feature>",
	Short: "Validate a feature's layout against the resolved adapter vocabulary (JSON output)",
	Long: `Validate the layout of @<feature> against the closed admissible vocabulary
declared in the resolved adapter's vocabulary: block. Emits the
structured report as JSON on stdout. Exit code 0 means no
error-severity entries; exit code 1 means at least one error entry.

The command imports github.com/parlay-tool/parlay/studio/pkg/vocabulary
directly (in-process; no subprocess) so the CLI report is byte-equivalent
to a library call against the same (layout, adapter) pair.`,
	Args: cobra.ExactArgs(1),
	RunE: runValidateVocabulary,
}

var (
	validateVocabularyNode           string
	validateVocabularyLayoutFile     string
	validateVocabularyAdapterFile    string
	validateVocabularyNonInteractive bool
)

func init() {
	validateVocabularyCmd.Flags().StringVar(&validateVocabularyNode, "node", "",
		"Validate a single layout node by path (read-back classification mode); empty validates the full layout")
	validateVocabularyCmd.Flags().StringVar(&validateVocabularyLayoutFile, "layout", "",
		"Path to the layout YAML to validate (defaults to the feature's *.layout.yaml)")
	validateVocabularyCmd.Flags().StringVar(&validateVocabularyAdapterFile, "adapter", "",
		"Path to a specific adapter YAML to use for resolution (defaults to the active root's adapter-set)")
	validateVocabularyCmd.Flags().BoolVar(&validateVocabularyNonInteractive, "non-interactive", false,
		"Accept the flag for CI compatibility; the CLI has no interactive prompts")
}

// validateVocabularyOutput is the JSON envelope the CLI emits on stdout.
// The Report field is the same shape callers see from the library — same
// JSON tags, same field order. Byte-equivalence of the report is pinned
// by validate_vocabulary_test.go.
type validateVocabularyOutput struct {
	Feature string             `json:"feature"`
	Adapter string             `json:"adapter,omitempty"`
	Node    string             `json:"node,omitempty"`
	Report  []vocabulary.Entry `json:"report"`
}

// layoutShape is the shape we yaml-unmarshal layout files into. It is
// intentionally permissive — the validator uses only Type/Properties/...
// downstream, and any unknown keys are passed through to children.
type layoutShape struct {
	ComponentVocabulary string          `yaml:"componentVocabulary"`
	Root                vocabulary.Node `yaml:"root"`
}

func runValidateVocabulary(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	out := validateVocabularyOutput{
		Feature: slug,
		Node:    validateVocabularyNode,
		Report:  []vocabulary.Entry{},
	}

	// Resolve the layout file.
	layoutPath := validateVocabularyLayoutFile
	if layoutPath == "" {
		// Look for a *.layout.yaml under the feature's spec directory.
		layoutPath = autoDiscoverLayoutFile(cfg.FeaturePath(slug))
	}
	if layoutPath == "" {
		return fmt.Errorf("no layout file found for feature %s; pass --layout <path>", slug)
	}

	layoutData, err := os.ReadFile(layoutPath)
	if err != nil {
		return fmt.Errorf("read layout %s: %w", layoutPath, err)
	}
	var layout layoutShape
	if err := yaml.Unmarshal(layoutData, &layout); err != nil {
		return fmt.Errorf("parse layout %s: %w", layoutPath, err)
	}

	// Resolve the vocabulary.
	vocab, adapterRef, resErr := resolveVocabularyForLayout(cfg, &layout)
	out.Adapter = adapterRef
	if resErr != nil {
		// The two stable codes get surfaced verbatim on stderr; also
		// echoed in the JSON envelope for callers that parse stdout.
		fmt.Fprintln(cmd.ErrOrStderr(), resErr.Error())
		// Emit the JSON envelope with the literal error code in a
		// well-known field so callers don't have to parse stderr.
		errOut := struct {
			Feature string `json:"feature"`
			Error   string `json:"error"`
		}{Feature: slug, Error: resErr.Error()}
		data, _ := json.MarshalIndent(errOut, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		// Exit non-zero on resolution failure.
		return NewExitCodeError(1)
	}

	// Decide between full-layout and single-node mode based on --node.
	var report vocabulary.Report
	if validateVocabularyNode != "" {
		// Locate the named node in the typed tree.
		node, found := findNodeByPath(&layout.Root, validateVocabularyNode)
		if !found {
			return fmt.Errorf("node %q not found in layout %s", validateVocabularyNode, layoutPath)
		}
		report = vocabulary.Validate(context.Background(), node, vocab)
	} else {
		report = vocabulary.Validate(context.Background(), vocabulary.Layout{Root: layout.Root}, vocab)
	}
	out.Report = report.Entries
	if out.Report == nil {
		out.Report = []vocabulary.Entry{}
	}

	// Emit JSON to stdout.
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))

	// Exit code policy: 0 when zero error-severity entries; 1 otherwise.
	if report.HasErrors() {
		return NewExitCodeError(1)
	}
	return nil
}

// autoDiscoverLayoutFile finds a *.layout.yaml under the feature's spec
// directory. Returns the first match in lexicographic order. Returns ""
// when no layout file exists.
func autoDiscoverLayoutFile(featureDir string) string {
	entries, err := os.ReadDir(featureDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) > len(".layout.yaml") && name[len(name)-len(".layout.yaml"):] == ".layout.yaml" {
			return filepath.Join(featureDir, name)
		}
	}
	return ""
}

// resolveVocabularyForLayout finds the adapter whose componentVocabulary
// name matches the layout's componentVocabulary field, then loads its
// vocabulary block. Returns vocabulary-unknown-adapter when no adapter
// resolves, and vocabulary-missing-from-adapter when the resolved
// adapter has no vocabulary block. The returned adapterRef is the path
// to the adapter that successfully resolved (empty on failure).
func resolveVocabularyForLayout(cfg *config.Context, layout *layoutShape) (vocabulary.Vocabulary, string, error) {
	// --adapter override: read the adapter directly.
	if validateVocabularyAdapterFile != "" {
		v, err := vocabulary.LoadFromAdapterFile(validateVocabularyAdapterFile)
		if err != nil {
			return vocabulary.Vocabulary{}, validateVocabularyAdapterFile, err
		}
		return v, validateVocabularyAdapterFile, nil
	}

	// Discover registered adapters under the active root's
	// .parlay/adapters/ directory.
	adaptersDir := cfg.AdaptersPath()
	entries, err := os.ReadDir(adaptersDir)
	if err != nil {
		return vocabulary.Vocabulary{}, "", fmt.Errorf("read adapters dir %s: %w", adaptersDir, err)
	}
	var slugs []string
	for _, e := range entries {
		name := e.Name()
		const suffix = ".adapter.yaml"
		if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
			slugs = append(slugs, name[:len(name)-len(suffix)])
		}
	}
	v, err := vocabulary.ResolveForLayout(layout.ComponentVocabulary, slugs, adaptersDir)
	if err != nil {
		// Wire-contract: stable error codes flow through unchanged.
		return vocabulary.Vocabulary{}, "", err
	}
	return v, layout.ComponentVocabulary, nil
}

// findNodeByPath descends the typed tree looking for a node whose Path
// matches the requested path. Returns the node + true on hit; an
// empty node + false on miss.
func findNodeByPath(n *vocabulary.Node, path string) (vocabulary.Node, bool) {
	if n == nil {
		return vocabulary.Node{}, false
	}
	if n.Path == path {
		return *n, true
	}
	for i := range n.Children {
		if found, ok := findNodeByPath(&n.Children[i], path); ok {
			return found, true
		}
	}
	return vocabulary.Node{}, false
}

// IsVocabularyMissingError exposes the package-level Is helpers so test
// code in this package can match against the stable error codes
// without re-importing vocabulary. Keeps the wire-contract surface
// concentrated in this file.
func IsVocabularyMissingError(err error) bool {
	return errors.Is(err, vocabulary.ErrVocabularyMissingFromAdapter)
}

func IsVocabularyUnknownAdapterError(err error) bool {
	return errors.Is(err, vocabulary.ErrVocabularyUnknownAdapter)
}
