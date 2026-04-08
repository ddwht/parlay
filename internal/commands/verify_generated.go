package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var verifyGeneratedCmd = &cobra.Command{
	Use:   "verify-generated <@feature>",
	Short: "Verify generated code files against the last-known content hashes (JSON output)",
	Long: `Compare each file recorded in .parlay/build/<feature>/.code-hashes.yaml
against its current on-disk content and classify it as stable, modified,
or missing. Used by /parlay-generate-code to detect user edits to files
the tool considers stable, so the agent surfaces conflicts rather than
silently overwriting hand-edited code.`,
	Args: cobra.ExactArgs(1),
	RunE: runVerifyGenerated,
}

type verifyFileEntry struct {
	Path      string `json:"path"`
	Component string `json:"component"`
}

type verifyOutput struct {
	Feature      string            `json:"feature"`
	HasHashes    bool              `json:"has_hashes"`
	Stable       []verifyFileEntry `json:"stable,omitempty"`
	Modified     []verifyFileEntry `json:"modified,omitempty"`
	Missing      []verifyFileEntry `json:"missing,omitempty"`
}

func runVerifyGenerated(cmd *cobra.Command, args []string) error {
	slug := strings.TrimPrefix(args[0], "@")
	output, err := computeVerifyOutput(slug)
	if err != nil {
		return err
	}
	return emitVerifyJSON(output)
}

// computeVerifyOutput loads the code-hashes sidecar for a feature and
// classifies each recorded file as stable / modified / missing. Exposed
// for tests so they can assert on the struct without parsing JSON.
func computeVerifyOutput(slug string) (*verifyOutput, error) {
	stored, err := loadCodeHashes(slug)
	if err != nil {
		return nil, err
	}

	output := &verifyOutput{Feature: slug}
	if stored == nil {
		return output, nil
	}
	output.HasHashes = true

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

		if currentHash == entry.Hash {
			output.Stable = append(output.Stable, fileEntry)
		} else {
			output.Modified = append(output.Modified, fileEntry)
		}
	}

	return output, nil
}

func emitVerifyJSON(output *verifyOutput) error {
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
