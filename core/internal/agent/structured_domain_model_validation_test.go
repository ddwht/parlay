// parlay-feature: studio-support/structured-domain-model-validation
// parlay-component: cross-cutting/element-path-on-every-finding
// parlay-artifact: test
//
// Tests for the structured domain-model validation feature:
//   - intent 2: machine-usable element path on every finding
//     (dotted paths + the whole-model token, determinism, no blank path)
//   - intent 3: emit domain-operations-deprecated (authoring warning /
//     build error, empty-vs-populated block, read-only detection)
//
// The CLI-level intent 1 behavior (--json, stdin, exit codes, human path)
// lives in internal/commands/validate_domain_model_json_test.go.

package agent

import (
	"bytes"
	"strings"
	"testing"
)

// ---- fixtures (inline domain-model YAML; see the buildfile's fixtures) ----

const dmvCleanModel = `schema_version: 1
entities:
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
`

const dmvTwoViolations = `schema_version: 1
entities:
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
      - name: amount
        type: decimal
        required: true
  - name: Customer
    fields:
      - name: id
        type: uuid
        required: true
relationships:
  - name: order-customer
    from: Order
    to: Customer
    cardinality: sideways
`

const dmvBadFieldType = `schema_version: 1
entities:
  - name: Order
    fields:
      - name: amount
        type: decimal
        required: true
`

const dmvUnknownCardinality = `schema_version: 1
entities:
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
  - name: Customer
    fields:
      - name: id
        type: uuid
        required: true
relationships:
  - name: order-customer
    from: Order
    to: Customer
    cardinality: sideways
`

const dmvMissingSchemaVersion = `entities:
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
`

const dmvWithOperations = `schema_version: 1
entities:
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
operations:
  - name: cancel-order
    input:
      - Order.id
    effects:
      - "set Order.status to cancelled"
`

const dmvEmptyOperations = `schema_version: 1
entities:
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
operations: []
`

// findingWithCode returns the first finding with the given code, or false.
func findingWithCode(errs []ValidationError, code string) (ValidationError, bool) {
	for _, e := range errs {
		if e.Code == code {
			return e, true
		}
	}
	return ValidationError{}, false
}

func countCode(errs []ValidationError, code string) int {
	n := 0
	for _, e := range errs {
		if e.Code == code {
			n++
		}
	}
	return n
}

// ============================================================================
// Intent 2 — machine-usable element path on every finding
// ============================================================================

// bad field type reports a path resolving to that field.
func TestStructuredDMV_BadFieldTypePath(t *testing.T) {
	errs := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvBadFieldType), ModeAuthoring)
	f, ok := findingWithCode(errs, "field-type-outside-closed-set")
	if !ok {
		t.Fatalf("expected field-type-outside-closed-set, got: %s", errSummary(errs))
	}
	if f.Context != "entities.Order.fields.amount.type" {
		t.Errorf("expected path entities.Order.fields.amount.type, got %q", f.Context)
	}
}

// unknown relationship cardinality reports a path resolving to the end.
func TestStructuredDMV_UnknownCardinalityPath(t *testing.T) {
	errs := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvUnknownCardinality), ModeAuthoring)
	f, ok := findingWithCode(errs, "relationship-cardinality-unknown")
	if !ok {
		t.Fatalf("expected relationship-cardinality-unknown, got: %s", errSummary(errs))
	}
	if f.Context != "relationships.order-customer.cardinality" {
		t.Errorf("expected path relationships.order-customer.cardinality, got %q", f.Context)
	}
}

// missing-schema-version reports the distinguished whole-model token.
func TestStructuredDMV_MissingSchemaVersionWholeModelToken(t *testing.T) {
	errs := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvMissingSchemaVersion), ModeAuthoring)
	f, ok := findingWithCode(errs, "missing-schema-version")
	if !ok {
		t.Fatalf("expected missing-schema-version, got: %s", errSummary(errs))
	}
	if f.Context != wholeModelPathToken {
		t.Errorf("expected whole-model token %q, got %q", wholeModelPathToken, f.Context)
	}
	if wholeModelPathToken != "<domain-model>" {
		t.Errorf("whole-model token drifted from <domain-model>: %q", wholeModelPathToken)
	}
}

// invalid-yaml also reports the whole-model token, never the file path.
func TestStructuredDMV_InvalidYAMLWholeModelToken(t *testing.T) {
	errs := ValidateDomainModelStructuredMode("m.yaml", []byte("schema_version: 1\nentities: [oops"), ModeAuthoring)
	f, ok := findingWithCode(errs, "invalid-yaml")
	if !ok {
		t.Fatalf("expected invalid-yaml, got: %s", errSummary(errs))
	}
	if f.Context != wholeModelPathToken {
		t.Errorf("expected whole-model token for invalid-yaml, got %q", f.Context)
	}
}

// paths are deterministic across runs over identical bytes.
func TestStructuredDMV_DeterministicPaths(t *testing.T) {
	a := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvTwoViolations), ModeAuthoring)
	b := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvTwoViolations), ModeAuthoring)
	if len(a) != len(b) {
		t.Fatalf("finding count differs across runs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Context != b[i].Context {
			t.Errorf("path %d differs across runs: %q vs %q", i, a[i].Context, b[i].Context)
		}
	}
}

// no finding carries a blank element path.
func TestStructuredDMV_NoBlankPath(t *testing.T) {
	errs := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvTwoViolations), ModeAuthoring)
	if len(errs) == 0 {
		t.Fatalf("expected findings for the two-violation fixture")
	}
	for i, e := range errs {
		if e.Context == "" {
			t.Errorf("finding %d [%s] carries a blank element path", i, e.Code)
		}
	}
}

// the two-violation fixture yields exactly two findings (one per violation,
// never collapsed) — the CLI array shape depends on this.
func TestStructuredDMV_TwoViolationsExactlyTwo(t *testing.T) {
	errs := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvTwoViolations), ModeAuthoring)
	if len(errs) != 2 {
		t.Fatalf("expected exactly 2 findings, got %d: %s", len(errs), errSummary(errs))
	}
	if !containsCode(errs, "field-type-outside-closed-set") ||
		!containsCode(errs, "relationship-cardinality-unknown") {
		t.Errorf("expected both distinct violation codes, got: %s", errSummary(errs))
	}
}

// ============================================================================
// Intent 3 — emit domain-operations-deprecated
// ============================================================================

// populated operations block in authoring mode emits one warning finding,
// whose fix names parlay migrate-domain-operations.
func TestStructuredDMV_OperationsAuthoringWarning(t *testing.T) {
	errs := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvWithOperations), ModeAuthoring)
	if n := countCode(errs, "domain-operations-deprecated"); n != 1 {
		t.Fatalf("expected exactly 1 domain-operations-deprecated, got %d: %s", n, errSummary(errs))
	}
	f, _ := findingWithCode(errs, "domain-operations-deprecated")
	if f.Severity != string(SeverityWarning) {
		t.Errorf("expected warning severity in authoring mode, got %q", f.Severity)
	}
	if !strings.Contains(f.Fix, "parlay migrate-domain-operations") {
		t.Errorf("fix should name parlay migrate-domain-operations, got %q", f.Fix)
	}
	if f.Context != wholeModelPathToken {
		t.Errorf("expected whole-model token, got %q", f.Context)
	}
}

// same model in build mode emits the code at error severity (fails the build).
func TestStructuredDMV_OperationsBuildError(t *testing.T) {
	errs := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvWithOperations), ModeBuild)
	f, ok := findingWithCode(errs, "domain-operations-deprecated")
	if !ok {
		t.Fatalf("expected domain-operations-deprecated in build mode, got: %s", errSummary(errs))
	}
	if f.Severity != string(SeverityError) {
		t.Errorf("expected error severity in build mode (fails the build), got %q", f.Severity)
	}
}

// model with no operations block emits no finding.
func TestStructuredDMV_NoOperationsNoFinding(t *testing.T) {
	errs := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvCleanModel), ModeAuthoring)
	if countCode(errs, "domain-operations-deprecated") != 0 {
		t.Errorf("clean model must not emit domain-operations-deprecated: %s", errSummary(errs))
	}
}

// model with an empty operations block emits no finding.
func TestStructuredDMV_EmptyOperationsNoFinding(t *testing.T) {
	errs := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvEmptyOperations), ModeAuthoring)
	if countCode(errs, "domain-operations-deprecated") != 0 {
		t.Errorf("empty operations block must not emit domain-operations-deprecated: %s", errSummary(errs))
	}
}

// detection is read-only — the input bytes are byte-for-byte identical
// after validation runs.
func TestStructuredDMV_ReadOnlyDetection(t *testing.T) {
	content := []byte(dmvWithOperations)
	before := make([]byte, len(content))
	copy(before, content)
	_ = ValidateDomainModelStructuredMode("m.yaml", content, ModeBuild)
	if !bytes.Equal(before, content) {
		t.Errorf("validation mutated the input bytes; detection must be read-only")
	}
}

// ============================================================================
// Backward-compatibility: the legacy 2-arg entry point does NOT emit the
// deprecation finding (preserves its finding set for existing callers) but
// still carries the whole-model token + severity.
// ============================================================================

func TestStructuredDMV_LegacyEntryPointNoDeprecation(t *testing.T) {
	errs := ValidateDomainModelStructured("m.yaml", []byte(dmvWithOperations))
	if countCode(errs, "domain-operations-deprecated") != 0 {
		t.Errorf("legacy 2-arg entry point must not emit domain-operations-deprecated: %s", errSummary(errs))
	}
}

func TestStructuredDMV_LegacyEntryPointStillCarriesToken(t *testing.T) {
	errs := ValidateDomainModelStructured("m.yaml", []byte(dmvMissingSchemaVersion))
	f, ok := findingWithCode(errs, "missing-schema-version")
	if !ok {
		t.Fatalf("expected missing-schema-version, got: %s", errSummary(errs))
	}
	if f.Context != wholeModelPathToken {
		t.Errorf("legacy entry point should carry whole-model token, got %q", f.Context)
	}
}
