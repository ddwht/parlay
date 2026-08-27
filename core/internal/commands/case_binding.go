package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"gopkg.in/yaml.v3"
)

// A state-only decision is a person saying "this case observes state rather
// than what the criterion states, and that is the honest answer HERE." That
// judgment is about one case's actual content. Binding it to the suite and
// case NAME alone lets the body be replaced wholesale — a different
// observation, for different reasons, still citing the same criterion and
// still marked state-only — while the approval keeps matching. The reviewer
// approved something that no longer exists.
//
// CaseFingerprint is the content that judgment was about.

// CaseFingerprint hashes a case's whole declared content.
//
// It is deliberately NOT limited to the observable steps: a case that keeps
// its steps but changes which criterion it cites, or stops being state-only,
// is also not the case that was approved.
func CaseFingerprint(n yaml.Node) (string, error) {
	// Re-encode through yaml rather than hashing source bytes, so that
	// reindentation, comment edits and key reordering do not read as a
	// changed observation. What must change the hash is what the case
	// declares, not how the file is laid out.
	canonical, err := canonicalizeNode(&n)
	if err != nil {
		return "", err
	}
	out, err := yaml.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("fingerprint case: %w", err)
	}
	sum := sha256.Sum256(out)
	return hex.EncodeToString(sum[:]), nil
}

// canonicalizeNode decodes a node into plain Go values with mapping keys
// sorted, so that the marshalled form depends only on content.
func canonicalizeNode(n *yaml.Node) (any, error) {
	var v any
	if err := n.Decode(&v); err != nil {
		return nil, fmt.Errorf("decode case: %w", err)
	}
	return sortKeys(v), nil
}

// sortKeys rewrites every map into a yaml.MapSlice-equivalent with keys in a
// stable order. yaml.v3 marshals Go maps in sorted key order already, so
// decoding to map[string]any is sufficient; this walks the tree to make that
// property explicit and to normalise nested types.
func sortKeys(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, inner := range t {
			out[k] = sortKeys(inner)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, inner := range t {
			out[fmt.Sprint(k)] = sortKeys(inner)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, inner := range t {
			out[i] = sortKeys(inner)
		}
		return out
	default:
		return v
	}
}

// resolvedCase is a case located in testcases.yaml, with everything a decision
// needs to bind to it.
type resolvedCase struct {
	Suite       string
	Name        string
	Coverage    string
	Ref, Text   string
	Fingerprint string
}

// resolveCases parses testcases content into located cases.
func resolveCases(content []byte) ([]resolvedCase, error) {
	var shape struct {
		Suites []struct {
			Name  string      `yaml:"name"`
			Cases []yaml.Node `yaml:"cases"`
		} `yaml:"suites"`
	}
	if err := yaml.Unmarshal(content, &shape); err != nil {
		return nil, fmt.Errorf("parse testcases: %w", err)
	}
	var out []resolvedCase
	for _, su := range shape.Suites {
		for _, node := range su.Cases {
			var meta struct {
				Name      string `yaml:"name"`
				Coverage  string `yaml:"coverage"`
				Criterion struct {
					Ref  string `yaml:"ref"`
					Text string `yaml:"text"`
				} `yaml:"criterion"`
			}
			if err := node.Decode(&meta); err != nil {
				continue
			}
			fp, err := CaseFingerprint(node)
			if err != nil {
				continue
			}
			out = append(out, resolvedCase{
				Suite: su.Name, Name: meta.Name, Coverage: meta.Coverage,
				Ref: meta.Criterion.Ref, Text: meta.Criterion.Text,
				Fingerprint: fp,
			})
		}
	}
	return out, nil
}
