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
	canonical := canonicalize(raw)
	out, err := yaml.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("canonical-form marshal: %w", err)
	}
	sum := sha256.Sum256(out)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
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
