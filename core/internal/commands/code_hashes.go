package commands

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"gopkg.in/yaml.v3"
)

// CodeHashesFile is the sidecar filename inside .parlay/build/<feature>/.
const CodeHashesFile = ".code-hashes.yaml"

// CodeHashes is the on-disk schema for tracking generated-file content
// hashes. Used by parlay verify-generated to detect user edits to files
// that the tool considers "stable" (i.e., would otherwise be skipped
// during incremental code generation).
//
// The map key is the file path relative to the project root, exactly as
// recorded by parlay save-code-hashes when the file was last generated.
type CodeHashes struct {
	// SchemaVersion is what makes an empty Provenance interpretable. Without
	// it, "" means both "this snapshot predates provenance" and "this file
	// has no provenance in a provenance-aware snapshot" — and those call for
	// opposite treatment. Absent (0) is the pre-provenance snapshot.
	SchemaVersion int                      `yaml:"schema-version,omitempty"`
	GeneratedAt   string                   `yaml:"generated-at"`
	Files         map[string]CodeHashEntry `yaml:"files"`
}

// CodeHashesSchemaVersion is the current on-disk version. Bumped when the
// meaning of an existing field changes, not when a field is added.
const CodeHashesSchemaVersion = 1

// Provenance values. The zero value is deliberately not "generated": reading
// an unknown file as generated is exactly the silent blessing this field
// exists to stop, and it would preserve today's bug straight through the
// upgrade.
const (
	ProvenanceGenerated = "generated"
	ProvenanceAdopted   = "adopted"
	ProvenanceUnknown   = "" // no declaration covers this file
)

// CodeHashEntry pairs a generated file's owning component with the content
// hash captured at generation time, and records who wrote it.
//
// The hash alone cannot answer that question. buildfile.schema.md draws the
// consequence itself: parlay guarantees *functional* determinism, not
// byte-identity, so "a content hash cannot by itself distinguish 'a human
// edited this file' from 'we regenerated it'." Every design that infers
// provenance from hash stability assumes byte-stable re-emission and
// misclassifies every legitimate regeneration.
//
// So provenance is DECLARED. The emitter says what it wrote, and the
// governing invariant is that no comparison in this design is ever between
// two codegen emissions of the same file.
type CodeHashEntry struct {
	Component string `yaml:"component"`
	Hash      string `yaml:"hash"`
	// Provenance is one of ProvenanceGenerated, ProvenanceAdopted, or
	// ProvenanceUnknown.
	Provenance string `yaml:"provenance,omitempty"`
	// EmittedAt is when codegen last declared writing this file. Empty for
	// anything parlay did not see emitted.
	EmittedAt string `yaml:"emitted-at,omitempty"`
}

// codeHashesPath returns the canonical sidecar location for a feature.
func codeHashesPath(cfg *config.Context, slug string) string {
	return filepath.Join(cfg.BuildPath(slug), CodeHashesFile)
}

// loadCodeHashes reads the sidecar file for a feature. Returns nil (no
// error) when the file does not exist — that's the first-generation case.
func loadCodeHashes(cfg *config.Context, slug string) (*CodeHashes, error) {
	data, err := os.ReadFile(codeHashesPath(cfg, slug))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var hashes CodeHashes
	if err := yaml.Unmarshal(data, &hashes); err != nil {
		return nil, fmt.Errorf("invalid code-hashes file: %w", err)
	}
	return &hashes, nil
}

// loadProjectCodeHashes reads the project-level code-hashes sidecar
// (.parlay/build/_project/.code-hashes.yaml). Returns nil (no error)
// when the file does not exist — the first-generation case, or a
// project that has never run save-build-state.
func loadProjectCodeHashes(cfg *config.Context) (*CodeHashes, error) {
	data, err := os.ReadFile(projectCodeHashesPath(cfg))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var hashes CodeHashes
	if err := yaml.Unmarshal(data, &hashes); err != nil {
		return nil, fmt.Errorf("invalid project code-hashes file: %w", err)
	}
	return &hashes, nil
}

// filesDroppedBySourceRootNarrowing compares a previous project-level
// CodeHashes snapshot against a freshly computed one and returns every
// file the old snapshot tracked that the new one does not — restricted
// to files that still exist on disk. A file legitimately deleted since
// the last run is not a narrowing signal (it's supposed to disappear
// from tracking); a file that still exists on disk but fell out of
// tracking means the --source-root passed this run doesn't cover
// ground the previous run's source-root did, which silently shrinks
// what verify-generated can ever check again unless caught here.
func filesDroppedBySourceRootNarrowing(previous, current *CodeHashes) []string {
	if previous == nil {
		return nil
	}
	var dropped []string
	for path := range previous.Files {
		if _, stillTracked := current.Files[path]; stillTracked {
			continue
		}
		if _, statErr := os.Stat(path); statErr == nil {
			dropped = append(dropped, path)
		}
	}
	sort.Strings(dropped)
	return dropped
}

// saveCodeHashes writes the sidecar file for a feature. Creates the
// parent directory if needed.
func saveCodeHashes(cfg *config.Context, slug string, hashes *CodeHashes) error {
	path := codeHashesPath(cfg, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(hashes)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// hashFileContent returns a 16-char hex sha256 prefix of the file at path.
// Matches the granularity used by baseline.go's sha256Hex helper so that
// hash strings are visually consistent across the tool.
func hashFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:8]), nil
}

// buildCodeHashes scans a source root for parlay markers, hashes each file,
// and returns a CodeHashes struct ready for serialization. Markers belonging
// to a different feature are skipped (returned as the second value). Does
// not touch disk; callers (typically saveBuildState) are responsible for
// writing. cfg is currently unused inside this helper but accepted to keep
// the signature consistent with the surrounding migration.
func buildCodeHashes(cfg *config.Context, slug, sourceRoot string) (*CodeHashes, int, error) {
	return buildCodeHashesWithProvenance(cfg, slug, sourceRoot, nil, nil)
}

// emissionDeclaration is what codegen said it wrote this run. A nil
// declaration means no manifest was supplied at all, which is a different
// state from an empty one: an empty manifest is a run that emitted nothing,
// a nil one is a run that did not say.
type emissionDeclaration struct {
	Paths map[string]bool
}

// buildCodeHashesWithProvenance is buildCodeHashes plus the classification.
//
// The table, and why each row compares what it does:
//
//	path is declared emitted   → generated. Compares NOTHING — it is a
//	                             declaration, not an inference. This is the
//	                             row that keeps byte-identity out of the
//	                             design.
//	not declared, previous
//	entry exists, hash equal   → carry the previous entry forward verbatim.
//	                             Disk now vs disk then with no emission in
//	                             between: nobody touched it.
//	not declared, hash differs
//	(or no previous entry)     → adopted. Disk now vs the last declared
//	                             emission, with no emission in between —
//	                             something changed this file and it was not
//	                             codegen. This is the silent-clobber case.
//	no declaration at all      → unknown, for every entry.
func buildCodeHashesWithProvenance(cfg *config.Context, slug, sourceRoot string, emitted *emissionDeclaration, previous *CodeHashes) (*CodeHashes, int, error) {
	_ = cfg
	markers, err := parser.ScanGenerated(sourceRoot)
	if err != nil {
		return nil, 0, fmt.Errorf("scan failed: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	hashes := &CodeHashes{
		SchemaVersion: CodeHashesSchemaVersion,
		GeneratedAt:   now,
		Files:         make(map[string]CodeHashEntry, len(markers)),
	}

	skipped := 0
	for _, marker := range markers {
		// When slug is non-empty (per-feature mode): only record files
		// belonging to this feature or feature-less files. When slug is
		// empty (project-level mode): accept all files regardless of feature.
		if slug != "" && marker.Feature != "" && marker.Feature != slug {
			skipped++
			continue
		}
		hash, err := hashFileContent(marker.Path)
		if err != nil {
			return nil, 0, fmt.Errorf("hash failed for %s: %w", marker.Path, err)
		}
		entry := CodeHashEntry{Component: marker.Component, Hash: hash}

		switch {
		case emitted == nil:
			// Nothing declared. Unknown rather than generated — see the
			// Provenance constants for why the zero value must not read as
			// "we wrote this".
			entry.Provenance = ProvenanceUnknown

		case emitted.Paths[normalizeWriteSetPath(marker.Path)]:
			entry.Provenance = ProvenanceGenerated
			entry.EmittedAt = now

		default:
			prev, had := previousEntry(previous, marker.Path)
			switch {
			case had && prev.Hash == hash:
				// Unchanged since the last save and not emitted since. Carry
				// the earlier verdict forward rather than re-deciding it: a
				// file adopted three runs ago is still adopted, and a file
				// generated three runs ago that nobody has touched is still
				// generated.
				entry.Provenance = prev.Provenance
				entry.EmittedAt = prev.EmittedAt
			default:
				entry.Provenance = ProvenanceAdopted
			}
		}

		hashes.Files[marker.Path] = entry
	}

	return hashes, skipped, nil
}

// previousEntry looks a path up in an earlier snapshot, tolerating a nil one.
func previousEntry(previous *CodeHashes, path string) (CodeHashEntry, bool) {
	if previous == nil || previous.Files == nil {
		return CodeHashEntry{}, false
	}
	e, ok := previous.Files[path]
	return e, ok
}

// marshalCodeHashes serializes a CodeHashes struct to YAML bytes for atomic
// disk writes. Symmetric with marshalBaseline.
func marshalCodeHashes(h *CodeHashes) ([]byte, error) {
	return yaml.Marshal(h)
}
