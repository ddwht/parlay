// parlay-feature: parlay-tool/structured-domain-model-validation
// parlay-component: cross-cutting/element-path-on-every-finding
// parlay-artifact: test
//
// Tests for the structured domain-model validation feature:
//   - intent 2: machine-usable element path on every finding
//     (dotted paths + the whole-model token, determinism, no blank path)
//   - intent 3: emit domain-operations-unsupported (authoring warning /
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
// Intent 3 — emit domain-operations-unsupported
// ============================================================================

// A populated operations block errors in EVERY mode since v0.3 — the field
// was removed, and the fix names the migrator out.
func TestStructuredDMV_OperationsUnsupportedError(t *testing.T) {
	for _, mode := range []ValidationMode{ModeAuthoring, ModeBuild} {
		errs := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvWithOperations), mode)
		if n := countCode(errs, "domain-operations-unsupported"); n != 1 {
			t.Fatalf("%s: expected exactly 1 domain-operations-unsupported, got %d: %s", mode, n, errSummary(errs))
		}
		f, _ := findingWithCode(errs, "domain-operations-unsupported")
		if f.Severity != string(SeverityError) {
			t.Errorf("%s: expected error severity (field removed), got %q", mode, f.Severity)
		}
		if !strings.Contains(f.Fix, "parlay migrate-domain-operations") {
			t.Errorf("fix should name parlay migrate-domain-operations, got %q", f.Fix)
		}
		if f.Context != wholeModelPathToken {
			t.Errorf("expected whole-model token, got %q", f.Context)
		}
	}
}

// model with no operations block emits no finding.
func TestStructuredDMV_NoOperationsNoFinding(t *testing.T) {
	errs := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvCleanModel), ModeAuthoring)
	if countCode(errs, "domain-operations-unsupported") != 0 {
		t.Errorf("clean model must not emit domain-operations-unsupported: %s", errSummary(errs))
	}
}

// model with an empty operations block emits no finding.
func TestStructuredDMV_EmptyOperationsNoFinding(t *testing.T) {
	errs := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvEmptyOperations), ModeAuthoring)
	if countCode(errs, "domain-operations-unsupported") != 0 {
		t.Errorf("empty operations block must not emit domain-operations-unsupported: %s", errSummary(errs))
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
// The legacy 2-arg entry point is gone, along with the boolean that made it
// different. It suppressed domain-operations-unsupported so that
// ValidateDomainModel — which failed on any finding regardless of severity —
// would not reject every project carrying a legacy operations: block. The
// severity filter moved to the command layer, so the suppression has nothing left
// to protect and the two CLI paths no longer disagree about what the model holds.
// ============================================================================

// TestStructuredDMV_DeprecationEmittedOnEveryPath replaces
// TestStructuredDMV_LegacyEntryPointNoDeprecation, which asserted the
// suppression this change removes. The property worth holding is its inverse:
// there is no path that reads a populated operations: block and says nothing.
func TestStructuredDMV_DeprecationEmittedOnEveryPath(t *testing.T) {
	for _, mode := range []ValidationMode{ModeAuthoring, ModeBuild} {
		errs := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvWithOperations), mode)
		if countCode(errs, "domain-operations-unsupported") != 1 {
			t.Errorf("mode %q: want exactly one domain-operations-unsupported, got: %s", mode, errSummary(errs))
		}
	}

	// Since v0.3 the severity is error in EVERY mode — the field is removed,
	// not deprecated, so authoring gets the same hard stop the build does.
	authoring, _ := findingWithCode(
		ValidateDomainModelStructuredMode("m.yaml", []byte(dmvWithOperations), ModeAuthoring),
		"domain-operations-unsupported")
	if authoring.Severity != string(SeverityError) {
		t.Errorf("authoring severity = %q, want error", authoring.Severity)
	}
	build, _ := findingWithCode(
		ValidateDomainModelStructuredMode("m.yaml", []byte(dmvWithOperations), ModeBuild),
		"domain-operations-unsupported")
	if build.Severity != string(SeverityError) {
		t.Errorf("build severity = %q, want error", build.Severity)
	}
}

func TestStructuredDMV_WholeModelTokenCarried(t *testing.T) {
	errs := ValidateDomainModelStructuredMode("m.yaml", []byte(dmvMissingSchemaVersion), ModeAuthoring)
	f, ok := findingWithCode(errs, "missing-schema-version")
	if !ok {
		t.Fatalf("expected missing-schema-version, got: %s", errSummary(errs))
	}
	if f.Context != wholeModelPathToken {
		t.Errorf("legacy entry point should carry whole-model token, got %q", f.Context)
	}
}
