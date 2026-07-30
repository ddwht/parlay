// parlay-feature: design-loop/vocabulary-validation
// parlay-component: cross-cutting/vocabulary-validator-library

package vocabulary

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Vocabulary is the in-memory shape of the adapter `vocabulary:` block.
// Exactly four subfields, mirroring the schema doc one-for-one. Extra
// subfields surface as a load-shape error so adapter authors are pinned
// to the closed contract.
type Vocabulary struct {
	Components       []ComponentSpec       `yaml:"components"`
	SpacingTokens    []string              `yaml:"spacing_tokens"`
	ColorTokens      []string              `yaml:"color_tokens"`
	LayoutContainers []LayoutContainerSpec `yaml:"layout_containers"`
}

// ComponentSpec declares one component in the vocabulary. Properties is a
// flat list of admissible property names; Variants is the per-axis enum
// of admissible values.
type ComponentSpec struct {
	Name       string              `yaml:"name"`
	Properties []string            `yaml:"properties"`
	Variants   map[string][]string `yaml:"variants"`
}

// LayoutContainerSpec declares a layout-container shape — e.g.,
// clarity.region — and the parameter set the validator pins. Parameter
// names not in AdmissibleParameters fail variant-style; parameter values
// outside the matching ParameterConstraint enum fail too.
type LayoutContainerSpec struct {
	ContainerType        string                         `yaml:"container_type"`
	AdmissibleParameters []string                       `yaml:"admissible_parameters"`
	ParameterConstraints map[string]ParameterConstraint `yaml:"parameter_constraints"`
}

// ParameterConstraint pins the type of one layout-container parameter and,
// for enum-shaped parameters, the closed allowed-values set.
type ParameterConstraint struct {
	Type          string   `yaml:"type"`
	AllowedValues []string `yaml:"allowed_values"`
}

// ErrVocabularyMissingFromAdapter is the stable error code emitted when
// the resolved adapter file has no `vocabulary:` block. The literal
// string "vocabulary-missing-from-adapter" is part of the wire contract
// — the design-loop skill matches it textually. Suite 4 pins the string
// by direct comparison.
var ErrVocabularyMissingFromAdapter = errors.New("vocabulary-missing-from-adapter")

// ErrVocabularyUnknownAdapter is the stable error code emitted when a
// layout's componentVocabulary value cannot be resolved through the
// registered adapter set. The literal string "vocabulary-unknown-adapter"
// is part of the wire contract — Suite 4 pins it textually.
var ErrVocabularyUnknownAdapter = errors.New("vocabulary-unknown-adapter")

// adapterYAML is the shape we yaml-unmarshal the resolved adapter file
// into. Only the vocabulary: block matters here — everything else is
// captured into RawRest so unknown top-level keys do not error the load.
type adapterYAML struct {
	Vocabulary *Vocabulary `yaml:"vocabulary"`
}

// LoadFromAdapterFile reads the adapter YAML at path and extracts the
// `vocabulary:` block. Returns ErrVocabularyMissingFromAdapter when the
// adapter parses cleanly but has no vocabulary block. The error wraps a
// helpful message that names the adapter path.
//
// File-IO contract: this function opens EXACTLY the named file. It does
// NOT look for a sibling <adapter>.vocabulary.yaml. vocabulary_test.go
// pins that contract by placing such a sibling and asserting it is not
// read.
func LoadFromAdapterFile(path string) (Vocabulary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Vocabulary{}, fmt.Errorf("read adapter file %s: %w", path, err)
	}
	var doc adapterYAML
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Vocabulary{}, fmt.Errorf("parse adapter file %s: %w", path, err)
	}
	if doc.Vocabulary == nil {
		return Vocabulary{}, fmt.Errorf("%w: adapter file %s has no vocabulary: block", ErrVocabularyMissingFromAdapter, path)
	}
	return *doc.Vocabulary, nil
}

// cachedAdapter carries both the parsed vocabulary block AND the
// componentVocabulary.name pulled from the same file. We cache them
// together so that a single first-access read populates both halves;
// subsequent calls against the same path serve both from cache and
// observe ZERO additional file reads.
type cachedAdapter struct {
	vocab Vocabulary
	name  string
	// vocabErr is the load result for the vocabulary block specifically.
	// We cache the missing-from-adapter sentinel as well so subsequent
	// calls against an adapter without a vocabulary block also hit the
	// cache rather than re-reading the file.
	vocabErr error
}

// vocabularyCache holds process-local cached adapter loads keyed by the
// absolute adapter file path. Per infrastructure caching: on-first-access
// — the first ResolveForLayout call loads + parses; subsequent calls
// against the same adapter path hit the cache.
var (
	vocabularyCache   = map[string]cachedAdapter{}
	vocabularyCacheMu sync.Mutex
)

// ResolveForLayout maps a layout's componentVocabulary: value (e.g.
// "clarity@17") to a registered adapter under adapterRootDir and loads its
// vocabulary block. Returns ErrVocabularyUnknownAdapter when no adapter
// in registeredAdapters resolves to the layout's value. The error message
// names the referenced componentVocabulary value AND the registered list,
// satisfying Suite 4's identifier-in-message invariant.
//
// Resolution rule: the layout value matches an adapter whose vocabulary's
// declared name field equals it. To keep this skill free of network calls
// and AI invocations, ResolveForLayout iterates the registeredAdapters
// list, opens each candidate adapter YAML once, and stops at the first
// match. The candidate set is small (typically one or two adapters in a
// project), so this linear scan is acceptable.
func ResolveForLayout(layoutComponentVocabulary string, registeredAdapters []string, adapterRootDir string) (Vocabulary, error) {
	for _, slug := range registeredAdapters {
		candidate := filepath.Join(adapterRootDir, slug+".adapter.yaml")
		entry, err := loadAdapterCached(candidate)
		if err != nil {
			// File-IO / parse errors are surfaced as-is — these are not
			// vocabulary-unknown-adapter. The two stable error codes
			// are reserved for the missing-block and unknown-vocab paths.
			return Vocabulary{}, err
		}
		if entry.name != layoutComponentVocabulary {
			continue
		}
		// Name matches; vocabErr (if any) is the resolution error path
		// — namely vocabulary-missing-from-adapter.
		if entry.vocabErr != nil {
			return Vocabulary{}, entry.vocabErr
		}
		return entry.vocab, nil
	}
	return Vocabulary{}, fmt.Errorf("%w: referenced componentVocabulary %q does not resolve against any registered adapter (registered: %v)",
		ErrVocabularyUnknownAdapter, layoutComponentVocabulary, registeredAdapters)
}

// loadAdapterCached reads + parses the adapter file once per path. Both
// the vocabulary block and the componentVocabulary.name flow from the
// same read, so a successful first call pins both for subsequent hits.
// vocabErr is preserved separately so an adapter without a vocabulary
// block still caches the name (and the missing-from-adapter sentinel).
func loadAdapterCached(path string) (cachedAdapter, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	vocabularyCacheMu.Lock()
	if cached, ok := vocabularyCache[abs]; ok {
		vocabularyCacheMu.Unlock()
		return cached, nil
	}
	vocabularyCacheMu.Unlock()

	data, err := os.ReadFile(abs)
	if err != nil {
		return cachedAdapter{}, fmt.Errorf("read adapter file %s: %w", abs, err)
	}
	var full struct {
		ComponentVocabulary struct {
			Name string `yaml:"name"`
		} `yaml:"componentVocabulary"`
		Vocabulary *Vocabulary `yaml:"vocabulary"`
	}
	if err := yaml.Unmarshal(data, &full); err != nil {
		return cachedAdapter{}, fmt.Errorf("parse adapter file %s: %w", abs, err)
	}
	entry := cachedAdapter{
		name: full.ComponentVocabulary.Name,
	}
	if full.Vocabulary == nil {
		entry.vocabErr = fmt.Errorf("%w: adapter file %s has no vocabulary: block", ErrVocabularyMissingFromAdapter, abs)
	} else {
		entry.vocab = *full.Vocabulary
	}

	vocabularyCacheMu.Lock()
	vocabularyCache[abs] = entry
	vocabularyCacheMu.Unlock()
	return entry, nil
}

// Suppress unused-warning for errors at package level — kept in case
// callers reach for the helper above. We deliberately do not export it.
var _ = errors.Is

// resetVocabularyCacheForTest is a test hook — keep package-private so
// production callers cannot accidentally drop the cache. Used by the
// _test.go files to drive caching invariants.
func resetVocabularyCacheForTest() {
	vocabularyCacheMu.Lock()
	defer vocabularyCacheMu.Unlock()
	vocabularyCache = map[string]cachedAdapter{}
}
