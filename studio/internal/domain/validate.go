// parlay-feature: domain-model-editor/domain-model-editor-validation
// parlay-component: cross-cutting/out-of-process-validate-endpoint

package domain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// WholeModelPath is the distinguished element path Core emits for a finding
// that names the whole model rather than a specific entity/field/enum/
// relationship (e.g. missing-schema-version, invalid-yaml). Studio passes it
// through verbatim; the editor treats it as "highlight nothing".
const WholeModelPath = "<domain-model>"

// Finding is one validation finding as surfaced to the editor. It is a
// verbatim projection of a Core finding: the closed error Code, the element
// path (Field) anchoring it, the Core-emitted Severity (domain-operations-
// deprecated is the sole warning; every other code is an error), and the
// human Message plus the actionable Fix. Studio invents, renames, and
// reclassifies nothing.
type Finding struct {
	Field    string `json:"field"`
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Fix      string `json:"fix,omitempty"`
}

// IsError reports whether this finding is error-severity — the gate predicate.
// Only findings Core classifies as error block a save; warnings never do.
func (f Finding) IsError() bool { return f.Severity == "error" }

// coreFinding mirrors the JSON shape Core emits from
// `parlay validate --type domain-model --json`: a bare array of
// {code, message, context, fix, severity}. Context carries the element path
// (or the whole-model token); severity is the authoring-mode severity.
type coreFinding struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Context  string `json:"context"`
	Fix      string `json:"fix"`
	Severity string `json:"severity"`
}

// execValidateJSON is the seam through which Validate reaches the out-of-
// process validator. It defaults to the real subprocess call; unit tests swap
// it for a fake so the default `go test ./...` needs no `parlay` on PATH.
var execValidateJSON = realExecValidateJSON

// Validate obtains findings for a draft ONLY by shelling out to Core's
// `parlay validate --type domain-model --json` — Studio imports no Core
// package and reimplements no rule. It is a pure function over the submitted
// draft: it reads nothing from disk, mutates nothing, and returns findings
// computed from the draft bytes alone, independent of any on-disk model.
//
// The draft is serialized to YAML (including a populated deprecated
// operations block, so the validator sees exactly what was submitted) and
// piped on the subprocess's stdin. Each Core finding is mapped to a Finding
// with the closed code, element path, severity, and message left UNCHANGED as
// Core emits them.
func Validate(ctx context.Context, parlayBin string, model Model) ([]Finding, error) {
	yamlBytes, err := marshalForValidation(model)
	if err != nil {
		return nil, fmt.Errorf("domain: serialize draft for validation: %w", err)
	}

	out, err := execValidateJSON(ctx, parlayBin, yamlBytes)
	if err != nil {
		return nil, err
	}

	var raw []coreFinding
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("domain: parse validator output: %w", err)
	}

	findings := make([]Finding, 0, len(raw))
	for _, c := range raw {
		findings = append(findings, Finding{
			Field:    c.Context,
			Code:     c.Code,
			Severity: c.Severity,
			Message:  c.Message,
			Fix:      c.Fix,
		})
	}
	return findings, nil
}

// HasErrorFinding reports whether any finding in the set is error-severity —
// the save-gate predicate. Warnings (domain-operations-deprecated) never make
// this true, alone or alongside errors.
func HasErrorFinding(findings []Finding) bool {
	for _, f := range findings {
		if f.IsError() {
			return true
		}
	}
	return false
}

// validationDraft is the marshaling shape used to reproduce the submitted
// draft as YAML for the validator. Unlike the persistence serializer (which
// splices the on-disk operations block back byte-for-byte via rawOperations),
// this shape renders the wire draft's parsed Operations directly, so a draft
// submitted over JSON with a deprecated operations block is validated with
// that block present. schema_version is omitted when zero/absent so a draft
// that never carried one reproduces the missing-schema-version condition.
type validationDraft struct {
	SchemaVersion int              `yaml:"schema_version,omitempty"`
	Enums         []Enum           `yaml:"enums,omitempty"`
	Entities      []Entity         `yaml:"entities,omitempty"`
	Relationships []Relationship   `yaml:"relationships,omitempty"`
	Operations    []map[string]any `yaml:"operations,omitempty"`
}

// marshalForValidation renders the in-memory draft to the deterministic YAML
// the validator consumes on stdin.
func marshalForValidation(model Model) ([]byte, error) {
	draft := validationDraft{
		SchemaVersion: model.SchemaVersion,
		Enums:         model.Enums,
		Entities:      model.Entities,
		Relationships: model.Relationships,
		Operations:    model.Operations,
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(draft); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// realExecValidateJSON runs `<parlayBin> validate --type domain-model --json -`
// with the draft YAML on stdin and returns the emitted JSON finding array.
// The CLI exits 0 for --json even when the finding list is non-empty (a
// finding list is a query result), so a non-empty list is not an exec error.
func realExecValidateJSON(ctx context.Context, parlayBin string, stdin []byte) ([]byte, error) {
	if parlayBin == "" {
		return nil, errors.New("domain: parlay binary not located; cannot validate")
	}
	cmd := exec.CommandContext(ctx, parlayBin, "validate", "--type", "domain-model", "--json", "-")
	cmd.Stdin = bytes.NewReader(stdin)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("domain: run parlay validate: %w (stderr: %s)", err, errBuf.String())
	}
	return out.Bytes(), nil
}

// locateParlayBinary resolves the `parlay` executable ONCE, mirroring Core's
// parlay-studio detection (core/internal/config/studio.go): an explicit
// PARLAY_BIN env gate, else a PATH lookup, in both cases confirmed by an
// executable-bit stat check. It returns a non-empty path only when a runnable
// binary is found; callers store the result at construction and reuse it for
// every request. A resolution failure is non-fatal at construction — the
// stored path is empty and the real validate path surfaces a server-error if
// invoked, while unit tests bypass the subprocess via execValidateJSON.
func locateParlayBinary() (string, error) {
	// Gate 1: explicit PARLAY_BIN. An empty value is an explicit suppression.
	if raw, ok := os.LookupEnv("PARLAY_BIN"); ok {
		if raw == "" {
			return "", errors.New("domain: PARLAY_BIN set empty; parlay validation suppressed")
		}
		return classifyParlayCandidate(raw)
	}
	// Gate 2: PATH lookup.
	resolved, err := exec.LookPath("parlay")
	if err != nil || resolved == "" {
		return "", fmt.Errorf("domain: parlay not found on PATH: %w", err)
	}
	return classifyParlayCandidate(resolved)
}

// classifyParlayCandidate stats a candidate path and confirms it is a runnable
// file (executable bit set), returning its absolute path.
func classifyParlayCandidate(path string) (string, error) {
	abs := path
	if !filepath.IsAbs(path) {
		if a, err := filepath.Abs(path); err == nil {
			abs = a
		}
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("domain: stat parlay candidate %q: %w", abs, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("domain: parlay candidate %q is a directory", abs)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("domain: parlay candidate %q is not executable", abs)
	}
	return abs, nil
}
