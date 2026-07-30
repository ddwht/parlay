package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var verifyGeneratedCmd = &cobra.Command{
	Use:   "verify-generated [@feature]",
	Short: "Verify generated code files against the last-known content hashes (JSON output)",
	Long: `Compare each recorded generated file against its current on-disk content
and classify it as stable, modified, or missing. Two modes:

  parlay verify-generated @feature   Per-feature code-hashes.
  parlay verify-generated            Project-level code-hashes
                                     (from .parlay/build/_project/).`,
	Args: cobra.RangeArgs(0, 1),
	RunE: runVerifyGenerated,
}

var verifyGeneratedStrict bool

func init() {
	verifyGeneratedCmd.Flags().BoolVar(&verifyGeneratedStrict, "strict", false,
		"Exit non-zero when any file was changed outside codegen (adopted) or has undeclared provenance")
}

type verifyFileEntry struct {
	Path      string `json:"path"`
	Component string `json:"component"`
}

type verifyOutput struct {
	Feature   string `json:"feature"`
	HasHashes bool   `json:"has_hashes"`
	// SchemaVersion of the snapshot being read. 0 means it predates
	// provenance, which is what makes an empty provenance interpretable
	// rather than alarming.
	SchemaVersion int               `json:"schema_version"`
	Stable        []verifyFileEntry `json:"stable,omitempty"`
	Modified      []verifyFileEntry `json:"modified,omitempty"`
	Missing       []verifyFileEntry `json:"missing,omitempty"`
	// Adopted holds files a save recorded as changed outside codegen. Unlike
	// Modified, this is not an inference from a hash: the file was on disk in
	// a state no emission declared, so something other than codegen wrote it.
	Adopted []verifyFileEntry `json:"adopted,omitempty"`
	// Unknown holds files whose provenance was never declared — chiefly a
	// snapshot written before provenance existed. Reported separately from
	// Stable so a pre-provenance snapshot cannot read as a clean bill of
	// health it never established.
	Unknown []verifyFileEntry `json:"unknown,omitempty"`
}

// classify places a file into the right bucket.
//
// Modified stays honestly ambiguous. Re-emission is only functionally
// deterministic, so a changed hash genuinely could be either a hand-edit or
// an ordinary regeneration, and saying so is more useful than guessing.
// Adopted is different in kind: it is a CONFIRMED hand-edit, established at
// save time by a declaration, not inferred here from bytes.
func (o *verifyOutput) classify(entry CodeHashEntry, fileEntry verifyFileEntry, currentHash string) {
	if currentHash != entry.Hash {
		o.Modified = append(o.Modified, fileEntry)
		return
	}
	switch entry.Provenance {
	case ProvenanceAdopted:
		o.Adopted = append(o.Adopted, fileEntry)
	case ProvenanceGenerated:
		o.Stable = append(o.Stable, fileEntry)
	default:
		o.Unknown = append(o.Unknown, fileEntry)
	}
}

func runVerifyGenerated(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		// Project-level: read from _project code-hashes
		output, err := computeProjectVerifyOutput(cfg)
		if err != nil {
			return err
		}
		return emitVerifyJSON(cmd, output)
	}
	slug := parser.FeatureSlug(args[0])
	output, err := computeVerifyOutput(cfg, slug)
	if err != nil {
		return err
	}
	return emitVerifyJSON(cmd, output)
}

// computeProjectVerifyOutput reads the project-level code-hashes sidecar
// and classifies each recorded file.
func computeProjectVerifyOutput(cfg *config.Context) (*verifyOutput, error) {
	path := projectCodeHashesPath(cfg)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &verifyOutput{Feature: "_project"}, nil
		}
		return nil, err
	}
	var stored CodeHashes
	if err := yaml.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("invalid project code-hashes: %w", err)
	}

	output := &verifyOutput{Feature: "_project", HasHashes: true, SchemaVersion: stored.SchemaVersion}

	paths := make([]string, 0, len(stored.Files))
	for p := range stored.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		entry := stored.Files[p]
		fileEntry := verifyFileEntry{Path: p, Component: entry.Component}

		if _, err := os.Stat(p); err != nil {
			output.Missing = append(output.Missing, fileEntry)
			continue
		}
		currentHash, err := hashFileContent(p)
		if err != nil {
			output.Missing = append(output.Missing, fileEntry)
			continue
		}
		output.classify(entry, fileEntry, currentHash)
	}
	return output, nil
}

// computeVerifyOutput loads the code-hashes sidecar for a feature and
// classifies each recorded file as stable / modified / missing. Exposed
// for tests so they can assert on the struct without parsing JSON.
func computeVerifyOutput(cfg *config.Context, slug string) (*verifyOutput, error) {
	stored, err := loadCodeHashes(cfg, slug)
	if err != nil {
		return nil, err
	}

	output := &verifyOutput{Feature: slug}
	if stored == nil {
		return output, nil
	}
	output.HasHashes = true
	output.SchemaVersion = stored.SchemaVersion

	// Walk in sorted path order for deterministic output.
	paths := make([]string, 0, len(stored.Files))
	for p := range stored.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, path := range paths {
		entry := stored.Files[path]
		fileEntry := verifyFileEntry{Path: path, Component: entry.Component}

		if _, err := os.Stat(path); err != nil {
			output.Missing = append(output.Missing, fileEntry)
			continue
		}

		currentHash, err := hashFileContent(path)
		if err != nil {
			output.Missing = append(output.Missing, fileEntry)
			continue
		}

		output.classify(entry, fileEntry, currentHash)
	}

	return output, nil
}

// emitVerifyJSON prints the report and, under --strict, exits non-zero.
//
// The default exit stays 0 deliberately. This is a JSON reporter whose
// consumer — generate-code.skill.md step 10 — parses the JSON and decides;
// making the reporter decide instead would break that step and put the policy
// in the wrong place. --strict exists for CI, which has no such consumer.
func emitVerifyJSON(cmd *cobra.Command, output *verifyOutput) error {
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	if verifyGeneratedStrict && (len(output.Adopted) > 0 || len(output.Unknown) > 0) {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"--strict: %d adopted, %d of unknown provenance\n", len(output.Adopted), len(output.Unknown))
		return NewExitCodeError(1)
	}
	return nil
}
