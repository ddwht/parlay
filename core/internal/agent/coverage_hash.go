// parlay-feature: parlay-tool/multi-adapter
// parlay-component: coverage-review-gate
//
// Canonical-form serializer + SHA-256 hasher used by the coverage-review
// gate. Hashes are computed over a normalized form (sorted keys, normalized
// whitespace, stable list ordering) so that cosmetic edits do not invalidate
// the recorded review.

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// CanonicalFormHash returns the sha256 hash of the canonical-form
// serialization of the supplied YAML content. Inputs that fail to parse
// return a non-nil error; callers should treat that as a buildfile/testcases
// authoring problem, not a hashing problem.
func CanonicalFormHash(content []byte) (string, error) {
	var raw interface{}
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return "", fmt.Errorf("canonical-form parse: %w", err)
	}
	return canonicalHashOf(raw)
}

// canonicalHashOf hashes an already-parsed YAML value in canonical form. It is
// the shared tail of CanonicalFormHash and SuiteHashes: the former hashes a
// whole file, the latter hashes one suite subtree, and both must agree byte
// for byte on the same content so a suite's hash is stable whether it is read
// from the file or lifted out of it.
func canonicalHashOf(raw interface{}) (string, error) {
	canonical := canonicalize(raw)
	out, err := yaml.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("canonical-form marshal: %w", err)
	}
	sum := sha256.Sum256(out)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// SuiteHashes returns a canonical-form hash for every suite in testcases.yaml,
// keyed the way the review gate keys required suites: by the suite's `id` when
// present, otherwise its `name`. A suite carrying neither is skipped — the gate
// cannot demand an identity it has no way to name, the same rule
// requiredSuiteIDs applies.
//
// This is what lets coverage-review staleness be per-suite rather than
// whole-file: editing one suite changes only its entry here, so a review that
// approved the others stays valid for them.
func SuiteHashes(testcasesContent []byte) (map[string]string, error) {
	var doc struct {
		Suites []map[string]interface{} `yaml:"suites"`
	}
	if err := yaml.Unmarshal(testcasesContent, &doc); err != nil {
		return nil, fmt.Errorf("canonical-form parse: %w", err)
	}
	out := make(map[string]string, len(doc.Suites))
	for _, s := range doc.Suites {
		key := suiteHashKey(s)
		if key == "" {
			continue
		}
		h, err := canonicalHashOf(s)
		if err != nil {
			return nil, err
		}
		out[key] = h
	}
	return out, nil
}

// suiteHashKey returns a suite's id, or its name when it has no id. It mirrors
// requiredSuiteIDs so the keys in SuiteHashes line up with the suites the gate
// asks about; a mismatch here would make every suite read as unhashed.
func suiteHashKey(s map[string]interface{}) string {
	if id, ok := s["id"].(string); ok && id != "" {
		return id
	}
	if name, ok := s["name"].(string); ok && name != "" {
		return name
	}
	return ""
}

// PerSuiteStale returns the subset of approved suites whose recorded hash is
// missing or no longer matches the current hash — the suites that were blessed
// and have since changed, and therefore need re-review. Suites that were never
// approved are not stale; they are unapproved, which the gate reports
// separately. When recorded is empty the review predates per-suite hashing and
// this returns nothing, leaving the whole-file fallback to decide.
func PerSuiteStale(recorded, now map[string]string, approved []string) []string {
	if len(recorded) == 0 {
		return nil
	}
	var stale []string
	for _, suite := range approved {
		was, hadRecord := recorded[suite]
		if !hadRecord || was != now[suite] {
			stale = append(stale, suite)
		}
	}
	return stale
}

// canonicalize walks an arbitrary parsed-YAML value and returns a structurally
// equivalent value with deterministic ordering: every map is converted into a
// sorted-key ordered structure (yaml.Node MappingNode); slices are recursed
// element-wise but their element order is preserved, since most schemas treat
// list order as semantically meaningful.
func canonicalize(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		node := &yaml.Node{Kind: yaml.MappingNode}
		for _, k := range keys {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: k}
			valNode := nodeFor(canonicalize(t[k]))
			node.Content = append(node.Content, keyNode, valNode)
		}
		return node
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, e := range t {
			out[i] = canonicalize(e)
		}
		return out
	default:
		return v
	}
}

// nodeFor wraps a canonicalized value into a yaml.Node so MappingNodes can
// nest other MappingNodes.
func nodeFor(v interface{}) *yaml.Node {
	if n, ok := v.(*yaml.Node); ok {
		return n
	}
	out := &yaml.Node{}
	if err := out.Encode(v); err != nil {
		// Fall back to a string-encoded representation. Hashing is best-effort
		// here; a parse-equivalent value that fails to encode is a yaml.v3 bug,
		// not an authoring problem.
		out = &yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("%v", v)}
	}
	return out
}
