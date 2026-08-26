// parlay-feature: parlay-tool/feature-retirement
// parlay-component: inbound-reference-inventory
//
// Who still points at this feature?
//
// Deliberately NOT `affected-set`. That probe answers a different question —
// what must be rebuilt if this changes — and answers it by searching BUILT
// buildfiles, skipping any feature whose buildfile it cannot read on the
// reasoning that nothing can depend through an unbuilt feature. Sound for
// rebuild scoping. Wrong here: a feature nobody has built yet can reference the
// retiring one in its specification, and retiring on that answer removes ground
// from underneath precisely the work not yet done. So this reads specifications,
// and never treats unbuilt as independent.
//
// It is also not the per-standing-head scope accounting, which walks OUTBOUND
// references — contract entries inside a feature whose source: names a retired
// intent. Retirement is the inbound question, and about artifacts no feature
// owns at all.

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
)

// InboundReference is one thing still pointing at a feature, reported with
// enough context to verify without repeating the scan: which artifact owns it,
// where inside that artifact, and the reference itself.
type InboundReference struct {
	Owner string `json:"owner"` // the feature or global artifact holding it
	Path  string `json:"path"`  // file, relative to the root
	Field string `json:"field"` // position within the file
	Ref   string `json:"ref"`   // the reference as written
}

func (r InboundReference) String() string {
	return fmt.Sprintf("%s · %s · %s · %s", r.Owner, r.Path, r.Field, r.Ref)
}

// scannedFiles is the CLOSED set of positions that count as pointing at a
// feature. Closed on purpose: a rule that blocks on any occurrence of a name is
// one people learn to route around, and it cannot tell a dependency from a
// sentence mentioning one. Prose, dialogs, source comments and trigger
// provenance are excluded — they record what prompted something, not what needs
// it.
//
// intents.md and dialogs.md are absent for that reason: a founding document
// naming another feature is telling a story about why this one exists.
var scannedFiles = []struct{ name, field string }{
	{"surface.yaml", "surface fragment"},
	{"capabilities.yaml", "operation"},
	{"infrastructure.md", "infrastructure fragment"},
	{"buildfile.yaml", "buildfile reference"},
	{"testcases.yaml", "testcase reference"},
}

// FindInboundReferences reports everything still pointing at target.
//
// Structural rather than substring: buildfiles carry illustrative refs in prose
// that name no real feature, and a substring scan reports those as dependents.
// A reference must appear as a whole @-qualified token whose feature half
// resolves to the target.
func FindInboundReferences(cfg *config.Context, target string) ([]InboundReference, error) {
	features, err := cfg.AllFeatures()
	if err != nil {
		return nil, fmt.Errorf("enumerate features: %w", err)
	}

	pattern, err := featureRefPattern(target)
	if err != nil {
		return nil, err
	}

	var found []InboundReference
	for _, other := range features {
		if sameFeature(other, target) {
			continue
		}
		featDir := cfg.FeaturePath(other)
		for _, sf := range scannedFiles {
			path := filepath.Join(featDir, sf.name)
			if sf.name == "buildfile.yaml" || sf.name == "testcases.yaml" {
				path = filepath.Join(cfg.BuildPath(other), sf.name)
			}
			found = append(found, scanFileForRefs(path, other, sf.field, pattern)...)
		}
		// A feature that has never been built contributes no buildfile, and
		// that absence is NOT evidence of independence — its spec files above
		// were still read. This is the case affected-set misses.
	}

	// Artifacts belonging to no feature. A walk over features alone cannot see
	// these, and a page manifest or the project's shared vocabulary pointing at
	// a retired feature breaks in exactly the same way.
	for _, g := range globalArtifacts(cfg) {
		found = append(found, scanFileForRefs(g.path, g.owner, g.field, pattern)...)
	}

	return found, nil
}

type globalArtifact struct{ path, owner, field string }

func globalArtifacts(cfg *config.Context) []globalArtifact {
	var out []globalArtifact
	out = append(out, globalArtifact{
		path:  cfg.DomainModelPath(),
		owner: "project domain model", field: "domain reference",
	})
	pagesDir := cfg.PagesPath()
	if entries, err := os.ReadDir(pagesDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			out = append(out, globalArtifact{
				path:  filepath.Join(pagesDir, e.Name()),
				owner: "page manifest", field: "page region entry",
			})
		}
	}
	return out
}

// featureRefPattern matches an @-qualified reference whose feature half is the
// target, in any qualification the tree actually uses: bare, feature-qualified
// and initiative-qualified.
func featureRefPattern(target string) (*regexp.Regexp, error) {
	bare := target
	if i := strings.LastIndex(target, "/"); i >= 0 {
		bare = target[i+1:]
	}
	alts := regexp.QuoteMeta(target)
	if bare != target {
		alts += "|" + regexp.QuoteMeta(bare)
	}
	// The trailing delimiter is what keeps @pricing from matching
	// @pricing-v2: a reference names the feature and then addresses something
	// inside it.
	return regexp.Compile(`@(?:` + alts + `)[/:]`)
}

func scanFileForRefs(path, owner, field string, pattern *regexp.Regexp) []InboundReference {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []InboundReference
	for i, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		// trigger: records what prompted a change — provenance, not a
		// dependency. Excluded by the same rule that excludes prose.
		if strings.HasPrefix(trimmed, "trigger:") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if m := pattern.FindString(line); m != "" {
			out = append(out, InboundReference{
				Owner: owner,
				Path:  path,
				Field: fmt.Sprintf("%s (line %d)", field, i+1),
				Ref:   strings.TrimSpace(refToken(line, pattern)),
			})
		}
	}
	return out
}

// refToken lifts the whole reference out of a line, so the report shows what
// was written rather than the fragment the pattern matched.
func refToken(line string, pattern *regexp.Regexp) string {
	loc := pattern.FindStringIndex(line)
	if loc == nil {
		return ""
	}
	end := loc[0]
	for end < len(line) && !strings.ContainsRune(" \t,'\"]}", rune(line[end])) {
		end++
	}
	return line[loc[0]:end]
}
