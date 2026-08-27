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
// references. Retirement is the inbound question, and about artifacts no feature
// owns at all.
//
// Two properties matter more than coverage breadth. Artifacts are read as
// PARSED documents, because a reference may be written as a folded or block
// scalar spread over lines that do not begin with its key, or inline in a flow
// mapping, or as a mapping KEY — all legal, all invisible to line matching. And
// the scan is FAIL-CLOSED: a file it could not read or parse becomes a recorded
// failure, never silence, because a check whose whole job is to establish that
// nothing points here cannot report clean over a file it failed to read.

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// InboundReference is one thing still pointing at a feature, reported with
// enough context to verify without repeating the scan.
type InboundReference struct {
	Owner string `json:"owner"` // the feature or global artifact holding it
	Path  string `json:"path"`  // file on disk
	Field string `json:"field"` // position within the file
	Ref   string `json:"ref"`   // the reference as written
}

func (r InboundReference) String() string {
	return fmt.Sprintf("%s · %s · %s · %s", r.Owner, r.Path, r.Field, r.Ref)
}

// ScanFailure is an artifact the inventory could not read or parse.
//
// An absent OPTIONAL artifact is absence. Anything else — a permission error, a
// malformed document, a ledger that will not load — is "cannot tell", and
// cannot tell is not none.
type ScanFailure struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

func (f ScanFailure) String() string { return f.Path + ": " + f.Reason }

// Inventory is the result of a whole-project inbound scan.
type Inventory struct {
	References []InboundReference `json:"references"`
	Failures   []ScanFailure      `json:"failures"`
}

// Clean reports whether the scan ESTABLISHED that nothing points at the
// feature. A failure leaves the answer unknown, which is not clean.
func (i Inventory) Clean() bool { return len(i.References) == 0 && len(i.Failures) == 0 }

// artifactRule says where references live in one artifact.
//
// Positions are named per artifact rather than pooled, because the same word
// means different things in different files, and because a global bag of key
// names cannot express the one shape that matters most: a mapping whose KEYS
// are references.
type artifactRule struct {
	name  string // filename within the feature directory
	build bool   // lives under .parlay/build/<feature>/ instead
	field string // how to describe a finding from this artifact

	// valueKeys: mapping keys whose VALUES may hold a reference.
	valueKeys []string
	// keyedMaps: mappings whose KEYS are references. Canonical operations are
	// indexed this way — operations."@feature/operation:id" — and a value walk
	// cannot reach them however many names its allowlist holds.
	keyedMaps []string
	// mdFields: for markdown artifacts, the field prefixes that carry
	// references. The distinction is not "is this a structured field" but WHICH
	// field: infrastructure's Source cites features, its Behavior describes
	// them.
	mdFields []string
}

// scannedArtifacts is the CLOSED set of positions that count.
//
// Closed on purpose: a rule that blocks on any occurrence of a name is one
// people learn to route around, and it cannot tell a dependency from a sentence
// mentioning one. intents.md and dialogs.md are absent because a founding
// document naming another feature is telling a story about why this one exists.
var scannedArtifacts = []artifactRule{
	{name: "surface.yaml", field: "surface fragment", valueKeys: []string{"source", "supersedes"}},
	{name: "capabilities.yaml", field: "operation", valueKeys: []string{"source"}},
	{name: "infrastructure.md", field: "infrastructure fragment", mdFields: []string{"**Source**:"}},
	{
		name: "buildfile.yaml", build: true, field: "buildfile reference",
		valueKeys: []string{
			"source", "sources", "supersedes", "operation", "binding", "feature",
			"surface_fragment", "domain_element", "component", "fixture", "flow",
			// A designer-confidence binding records candidates as ref: entries.
			"ref",
		},
		keyedMaps: []string{"operations"},
	},
	{
		name: "testcases.yaml", build: true, field: "testcase reference",
		valueKeys: []string{
			"source_refs", "operation", "ref", "feature", "component",
			// A v1 suite cites the intent it validates.
			"intent",
		},
	},
}

// FindInboundReferences reports everything still pointing at target, plus every
// artifact it could not read.
func FindInboundReferences(cfg *config.Context, target string) (Inventory, error) {
	features, err := cfg.AllFeatures()
	if err != nil {
		return Inventory{}, fmt.Errorf("enumerate features: %w", err)
	}
	pattern, err := featureRefPattern(target)
	if err != nil {
		return Inventory{}, err
	}

	var inv Inventory
	for _, other := range features {
		if sameFeature(other, target) {
			continue
		}
		featDir := cfg.FeaturePath(other)
		for _, rule := range scannedArtifacts {
			dir := featDir
			if rule.build {
				dir = cfg.BuildPath(other)
			}
			inv.add(scanArtifact(filepath.Join(dir, rule.name), other, rule, pattern))
		}
		// A ledger is a directory of files, so a filename-keyed table cannot
		// reach it. affects: may name another feature's contract entry.
		inv.add(scanAmendments(featDir, other, pattern))

		// A feature that has never been built contributes no buildfile, and
		// that absence is NOT evidence of independence — its spec files above
		// were still read. This is the case affected-set misses.
	}

	// Artifacts belonging to no feature. A walk over features cannot see these,
	// and a page manifest or the shared vocabulary pointing at a retired
	// feature breaks in exactly the same way.
	inv.add(scanDomainModel(cfg, pattern))
	inv.add(scanPageManifests(cfg, pattern))

	return inv, nil
}

func (i *Inventory) add(refs []InboundReference, fails []ScanFailure) {
	i.References = append(i.References, refs...)
	i.Failures = append(i.Failures, fails...)
}

func scanArtifact(path, owner string, rule artifactRule, pattern *regexp.Regexp) ([]InboundReference, []ScanFailure) {
	if len(rule.mdFields) > 0 {
		return scanMarkdownFields(path, owner, rule.field, pattern, rule.mdFields)
	}
	return scanYAML(path, owner, rule.field, pattern, rule.valueKeys, rule.keyedMaps)
}

// scanYAML walks a parsed document and inspects only the positions a reference
// may occupy.
func scanYAML(path, owner, field string, pattern *regexp.Regexp, valueKeys, keyedMaps []string) ([]InboundReference, []ScanFailure) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // an optional artifact simply absent
		}
		return nil, []ScanFailure{{Path: path, Reason: err.Error()}}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		// A present artifact that will not parse leaves the answer unknown. A
		// raw pass may surface visible tokens as a hint, but it cannot turn
		// unknown into clean, so the failure is recorded either way.
		refs, _ := scanRawLines(path, owner, field, pattern, nil)
		return refs, []ScanFailure{{Path: path, Reason: "cannot parse: " + err.Error()}}
	}

	wantValue := toSet(valueKeys)
	wantKeyed := toSet(keyedMaps)

	var out []InboundReference
	var walk func(n *yaml.Node, key string)
	walk = func(n *yaml.Node, key string) {
		if n == nil {
			return
		}
		switch n.Kind {
		case yaml.DocumentNode, yaml.SequenceNode:
			for _, c := range n.Content {
				walk(c, key)
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				k, v := n.Content[i], n.Content[i+1]
				if wantKeyed[key] && pattern.MatchString(k.Value) {
					out = append(out, InboundReference{
						Owner: owner, Path: path,
						Field: fmt.Sprintf("%s · %s key (line %d)", field, key, k.Line),
						Ref:   strings.TrimSpace(k.Value),
					})
				}
				walk(v, k.Value)
			}
		case yaml.ScalarNode:
			if wantValue[key] && pattern.MatchString(n.Value) {
				out = append(out, InboundReference{
					Owner: owner, Path: path,
					Field: fmt.Sprintf("%s · %s (line %d)", field, key, n.Line),
					Ref:   strings.TrimSpace(n.Value),
				})
			}
		}
	}
	walk(&doc, "")
	return out, nil
}

// scanMarkdownFields reads a markdown artifact's reference-carrying fields.
func scanMarkdownFields(path, owner, field string, pattern *regexp.Regexp, fields []string) ([]InboundReference, []ScanFailure) {
	return scanRawLines(path, owner, field, pattern, fields)
}

func scanRawLines(path, owner, field string, pattern *regexp.Regexp, fields []string) ([]InboundReference, []ScanFailure) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []ScanFailure{{Path: path, Reason: err.Error()}}
	}
	var out []InboundReference
	for i, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		// trigger: records what prompted a change — provenance, not need.
		if strings.HasPrefix(trimmed, "trigger:") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if len(fields) > 0 && !hasAnyPrefix(trimmed, fields) {
			continue
		}
		if pattern.MatchString(line) {
			out = append(out, InboundReference{
				Owner: owner, Path: path,
				Field: fmt.Sprintf("%s (line %d)", field, i+1),
				Ref:   refToken(line, pattern),
			})
		}
	}
	return out, nil
}

// scanAmendments reads a feature's ledger through the loader rather than by
// recognizing the spellings of affects: entries.
func scanAmendments(featDir, owner string, pattern *regexp.Regexp) ([]InboundReference, []ScanFailure) {
	dir := filepath.Join(featDir, "amendments")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}
	amendments, err := parser.LoadFeatureAmendments(featDir)
	if err != nil {
		// A ledger that will not load is not a ledger with nothing in it.
		return nil, []ScanFailure{{Path: dir, Reason: err.Error()}}
	}
	var out []InboundReference
	for _, a := range amendments {
		for _, ref := range a.Affects {
			if !pattern.MatchString(ref) {
				continue
			}
			out = append(out, InboundReference{
				Owner: owner, Path: a.Path,
				Field: fmt.Sprintf("amendment %03d-%s · affects:", a.Seq, a.FileSlug),
				Ref:   strings.TrimSpace(ref),
			})
		}
	}
	return out, nil
}

func scanDomainModel(cfg *config.Context, pattern *regexp.Regexp) ([]InboundReference, []ScanFailure) {
	return scanYAML(cfg.DomainModelPath(), "project domain model", "domain reference", pattern,
		[]string{"source", "sources", "feature", "owner", "ref"}, nil)
}

// scanPageManifests reads each manifest through the page parser. Recognizing
// list markers "1." through "9." silently lost item 10 onward, and the parser
// already exposes each region's components.
func scanPageManifests(cfg *config.Context, pattern *regexp.Regexp) ([]InboundReference, []ScanFailure) {
	entries, err := os.ReadDir(cfg.PagesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []ScanFailure{{Path: cfg.PagesPath(), Reason: err.Error()}}
	}
	var refs []InboundReference
	var fails []ScanFailure
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(cfg.PagesPath(), e.Name())
		page, err := parser.ParsePageFile(path)
		if err != nil {
			fails = append(fails, ScanFailure{Path: path, Reason: err.Error()})
			continue
		}
		for _, region := range page.Regions {
			for _, c := range region.Components {
				if pattern.MatchString(c) {
					refs = append(refs, InboundReference{
						Owner: "page manifest", Path: path,
						Field: fmt.Sprintf("region %s · component", region.Name),
						Ref:   strings.TrimSpace(c),
					})
				}
			}
		}
	}
	return refs, fails
}

// featureRefPattern matches an @-qualified reference whose feature half is the
// target, in any qualification the tree uses: bare, feature-qualified and
// initiative-qualified.
func featureRefPattern(target string) (*regexp.Regexp, error) {
	bare := target
	if i := strings.LastIndex(target, "/"); i >= 0 {
		bare = target[i+1:]
	}
	alts := regexp.QuoteMeta(target)
	if bare != target {
		alts += "|" + regexp.QuoteMeta(bare)
	}
	// The trailing delimiter keeps @pricing from matching @pricing-v2: a
	// reference names the feature and then addresses something inside it.
	return regexp.Compile(`@(?:` + alts + `)[/:]`)
}

func refToken(line string, pattern *regexp.Regexp) string {
	loc := pattern.FindStringIndex(line)
	if loc == nil {
		return ""
	}
	end := loc[0]
	for end < len(line) && !strings.ContainsRune(" \t,'\"]}", rune(line[end])) {
		end++
	}
	return strings.TrimSpace(line[loc[0]:end])
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func toSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, i := range items {
		out[i] = true
	}
	return out
}
