// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/deterministic-serialization-and-operations-passthrough

package domain

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// Serialize renders a model to deterministic YAML bytes. Serializing the same
// in-memory model twice produces byte-identical output (and therefore the
// same etag): a stable key order per the struct's field order, with the
// declaration order of enums, entities, fields, and values preserved exactly
// as arranged by the designer.
//
// The deprecated operations block is carried through unchanged: it is spliced
// back verbatim from the bytes captured at load time (model.rawOperations),
// so a save that touched only an unrelated entity leaves the operations block
// byte-for-byte identical. Unset enum-value label/tone are omitted (the
// struct tags use omitempty), never written as empty strings.
func Serialize(model Model) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(model); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}

	out := buf.Bytes()
	if len(model.rawOperations) > 0 {
		// The typed encoder output always ends with a newline; append the
		// verbatim operations block directly after it.
		out = append(out, model.rawOperations...)
	}
	return out, nil
}

// captureOperationsBlock extracts the verbatim bytes of the top-level
// `operations:` block from a raw model file, from the `operations:` key line
// through the byte just before the next top-level key (or end of file). It
// returns nil when the file has no operations block.
//
// Byte-for-byte fidelity is the point: the extracted bytes are spliced back
// unchanged on serialize, so no code path mutates, reorders, or drops the
// deprecated operations entries.
func captureOperationsBlock(raw []byte) []byte {
	lines := bytes.SplitAfter(raw, []byte("\n"))

	start := -1
	for i, line := range lines {
		if isTopLevelKey(line, "operations") {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}

	end := len(lines)
	for j := start + 1; j < len(lines); j++ {
		if isTopLevelKeyLine(lines[j]) {
			end = j
			break
		}
	}

	var block []byte
	for _, line := range lines[start:end] {
		block = append(block, line...)
	}
	return block
}

// isTopLevelKey reports whether line is a top-level mapping key named key
// (i.e. `key:` at column 0, no leading whitespace).
func isTopLevelKey(line []byte, key string) bool {
	trimmed := bytes.TrimRight(line, "\r\n")
	prefix := key + ":"
	return string(trimmed) == prefix || bytes.HasPrefix(trimmed, []byte(prefix))
}

// isTopLevelKeyLine reports whether line begins a top-level mapping key: a
// non-space, non-comment, non-list character at column 0 followed somewhere
// by a colon. Used to find the boundary after the operations block.
func isTopLevelKeyLine(line []byte) bool {
	if len(line) == 0 {
		return false
	}
	c := line[0]
	if c == ' ' || c == '\t' || c == '#' || c == '-' || c == '\n' || c == '\r' {
		return false
	}
	return bytes.Contains(line, []byte(":"))
}
