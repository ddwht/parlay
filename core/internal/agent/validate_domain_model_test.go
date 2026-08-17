// parlay-feature: studio-support/domain-model-yaml-migration
// parlay-component: validation-error-reporter
// parlay-artifact: test
//
// Tests for ValidateDomainModel, ValidateDomainModelStructured, and the
// schema-version dispatch (ResolveDomainModelVersion). The closed-set
// resolutions applied here:
//
//   - enum-tone-vocabulary  → closed-set
//   - entity-field-shape    → flat-only

package agent

import (
	"strings"
	"testing"
)

func containsCode(errs []ValidationError, code string) bool {
	for _, e := range errs {
		if e.Code == code {
			return true
		}
	}
	return false
}

func errSummary(errs []ValidationError) string {
	if len(errs) == 0 {
		return "<none>"
	}
	parts := make([]string, len(errs))
	for i, e := range errs {
		parts[i] = e.Code + ": " + e.Message
	}
	return strings.Join(parts, " | ")
}

func TestValidateDomainModel_HappyPath(t *testing.T) {
	yaml := `schema_version: 1
enums:
  - name: OrderStatus
    values:
      - value: pending
        label: Pending
        tone: warning
      - value: paid
        label: Paid
        tone: success
      - value: cancelled
        label: Cancelled
        tone: danger
entities:
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
      - name: status
        type: OrderStatus
        enum: OrderStatus
        required: true
      - name: customer_id
        type: ref
        target: Customer
        required: true
  - name: Customer
    fields:
      - name: id
        type: uuid
        required: true
      - name: name
        type: string
        required: true
relationships:
  - name: customer-orders
    from: Customer
    to: Order
    cardinality: one-to-many
operations:
  - name: cancel-order
    input: [Order.id]
    effects:
      - "set Order.status to cancelled"
`
	errs := ValidateDomainModelStructuredMode("domain-model.yaml", []byte(yaml), ModeAuthoring)

	// This fixture carries a legacy operations: block, which since v0.3 is a
	// hard error in every mode — the field was removed, and only the
	// migrator reads it.
	if n := countCode(errs, "domain-operations-unsupported"); n != 1 {
		t.Errorf("want one domain-operations-unsupported for a model with operations:, got %d: %s", n, errSummary(errs))
	}
	f, _ := findingWithCode(errs, "domain-operations-unsupported")
	if f.Severity != string(SeverityError) {
		t.Errorf("severity = %q, want error — the field is removed, not deprecated", f.Severity)
	}
}

func TestValidateDomainModel_MissingSchemaVersion(t *testing.T) {
	yaml := `entities:
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
`
	errs := ValidateDomainModelStructuredMode("domain-model.yaml", []byte(yaml), ModeAuthoring)
	if !containsCode(errs, "missing-schema-version") {
		t.Errorf("expected missing-schema-version error, got: %s", errSummary(errs))
	}
	// The condition is error-severity, which is what makes the command layer fail
	// on it. ValidateDomainModel used to be asserted here as the Validator-shaped
	// wrapper; it is gone, and the rendering it did lives in
	// commands.validateDomainModelAdapter, where the severity split belongs.
	f, ok := findingWithCode(errs, "missing-schema-version")
	if !ok {
		t.Fatalf("expected missing-schema-version, got: %s", errSummary(errs))
	}
	if f.Severity == string(SeverityWarning) {
		t.Error("missing schema_version must be error-severity — it is not something a build can proceed past")
	}
	if !strings.Contains(f.Message, "schema_version") {
		t.Errorf("message should mention schema_version, got: %v", f.Message)
	}
}

func TestValidateDomainModel_SchemaVersionTooNew(t *testing.T) {
	yaml := `schema_version: 99
entities: []
`
	errs := ValidateDomainModelStructuredMode("domain-model.yaml", []byte(yaml), ModeAuthoring)
	if !containsCode(errs, "schema-version-newer-than-binary") {
		t.Errorf("expected schema-version-newer-than-binary, got: %s", errSummary(errs))
	}
	for _, e := range errs {
		if e.Code == "schema-version-newer-than-binary" &&
			!strings.Contains(e.Message, "newer than this Core release supports") {
			t.Errorf("error message should mention 'newer than this Core release supports'; got: %q", e.Message)
		}
	}
}

func TestValidateDomainModel_UndeclaredEntityReference(t *testing.T) {
	yaml := `schema_version: 1
entities:
  - name: Customer
    fields:
      - name: id
        type: uuid
        required: true
relationships:
  - name: customer-orders
    from: Customer
    to: Order
    cardinality: one-to-many
`
	errs := ValidateDomainModelStructuredMode("domain-model.yaml", []byte(yaml), ModeAuthoring)
	if !containsCode(errs, "undeclared-entity-reference") {
		t.Errorf("expected undeclared-entity-reference, got: %s", errSummary(errs))
	}
	// Message should name the offending entity ("Order").
	found := false
	for _, e := range errs {
		if e.Code == "undeclared-entity-reference" && strings.Contains(e.Message, "Order") {
			found = true
		}
	}
	if !found {
		t.Errorf("undeclared-entity-reference message should name 'Order'; got: %s", errSummary(errs))
	}
}

func TestValidateDomainModel_OperationInputFieldNotFound(t *testing.T) {
	yaml := `schema_version: 1
entities:
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
operations:
  - name: cancel-order
    input: [Order.placed_at]
`
	errs := ValidateDomainModelStructuredMode("domain-model.yaml", []byte(yaml), ModeAuthoring)
	if !containsCode(errs, "operation-input-field-not-found") {
		t.Errorf("expected operation-input-field-not-found, got: %s", errSummary(errs))
	}
}

func TestValidateDomainModel_FieldTypeOutsideClosedSet(t *testing.T) {
	yaml := `schema_version: 1
entities:
  - name: Order
    fields:
      - name: amount
        type: decimal
        required: true
`
	errs := ValidateDomainModelStructuredMode("domain-model.yaml", []byte(yaml), ModeAuthoring)
	if !containsCode(errs, "field-type-outside-closed-set") {
		t.Errorf("expected field-type-outside-closed-set, got: %s", errSummary(errs))
	}
	// Error should mention the closed-set list as a fix suggestion.
	for _, e := range errs {
		if e.Code == "field-type-outside-closed-set" {
			if !strings.Contains(e.Fix, "uuid") || !strings.Contains(e.Fix, "named-enum") {
				t.Errorf("fix should list closed-set primitives and named-enum; got: %q", e.Fix)
			}
		}
	}
}

func TestValidateDomainModel_FlatOnlyRejectsNestedField(t *testing.T) {
	// Inline object literal — flat-only resolution rejects this.
	yaml := `schema_version: 1
entities:
  - name: Order
    fields:
      - name: shipping_address
        type:
          street: string
          city: string
        required: true
`
	errs := ValidateDomainModelStructuredMode("domain-model.yaml", []byte(yaml), ModeAuthoring)
	if !containsCode(errs, "field-type-outside-closed-set") {
		t.Errorf("expected field-type-outside-closed-set for nested literal, got: %s", errSummary(errs))
	}
	// Fix should point at the "lift into a separate entity" pattern.
	for _, e := range errs {
		if e.Code == "field-type-outside-closed-set" {
			if !strings.Contains(e.Fix, "lift") || !strings.Contains(e.Fix, "ref") {
				t.Errorf("fix for nested literal should mention 'lift' and 'ref'; got: %q", e.Fix)
			}
		}
	}
}

func TestValidateDomainModel_RelationshipCardinalityUnknown(t *testing.T) {
	yaml := `schema_version: 1
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
  - name: weird
    from: Customer
    to: Order
    cardinality: many-to-self
`
	errs := ValidateDomainModelStructuredMode("domain-model.yaml", []byte(yaml), ModeAuthoring)
	if !containsCode(errs, "relationship-cardinality-unknown") {
		t.Errorf("expected relationship-cardinality-unknown, got: %s", errSummary(errs))
	}
}

func TestValidateDomainModel_EnumToneOutsideClosedSet(t *testing.T) {
	// closed-set tone resolution: only neutral|info|warning|danger|success.
	yaml := `schema_version: 1
enums:
  - name: OrderStatus
    values:
      - value: pending
        tone: ominous
`
	errs := ValidateDomainModelStructuredMode("domain-model.yaml", []byte(yaml), ModeAuthoring)
	if !containsCode(errs, "enum-tone-outside-closed-set") {
		t.Errorf("expected enum-tone-outside-closed-set for unknown tone, got: %s", errSummary(errs))
	}
	for _, e := range errs {
		if e.Code == "enum-tone-outside-closed-set" {
			// Closed-set list should appear in the message or fix for
			// designer recoverability.
			haystack := e.Message + " " + e.Fix
			for _, tone := range []string{"neutral", "info", "warning", "danger", "success"} {
				if !strings.Contains(haystack, tone) {
					t.Errorf("error should list every closed-set tone; missing %q in %q", tone, haystack)
					break
				}
			}
		}
	}
}

func TestValidateDomainModel_EnumToneInClosedSet(t *testing.T) {
	yaml := `schema_version: 1
enums:
  - name: OrderStatus
    values:
      - value: pending
        tone: warning
      - value: paid
        tone: success
      - value: cancelled
        tone: danger
      - value: archived
        tone: neutral
      - value: backordered
        tone: info
`
	errs := ValidateDomainModelStructuredMode("domain-model.yaml", []byte(yaml), ModeAuthoring)
	for _, e := range errs {
		if e.Code == "enum-tone-outside-closed-set" {
			t.Errorf("known tones must pass; got error: %s", errSummary(errs))
		}
	}
}

func TestValidateDomainModel_RefMissingTarget(t *testing.T) {
	yaml := `schema_version: 1
entities:
  - name: Order
    fields:
      - name: customer_id
        type: ref
        required: true
`
	errs := ValidateDomainModelStructuredMode("domain-model.yaml", []byte(yaml), ModeAuthoring)
	if !containsCode(errs, "ref-missing-target") {
		t.Errorf("expected ref-missing-target, got: %s", errSummary(errs))
	}
}

func TestValidateDomainModel_AggregatesMultipleErrors(t *testing.T) {
	// Every unresolved reference must be reported, not just the first.
	yaml := `schema_version: 1
entities:
  - name: Customer
    fields:
      - name: id
        type: uuid
        required: true
relationships:
  - name: r1
    from: Customer
    to: Order
    cardinality: many-to-self
  - name: r2
    from: Ghost
    to: Customer
    cardinality: one-to-one
`
	errs := ValidateDomainModelStructuredMode("domain-model.yaml", []byte(yaml), ModeAuthoring)
	// Expect at least: undeclared 'Order', unknown cardinality 'many-to-self',
	// undeclared 'Ghost'. Three distinct problems → three errors.
	if len(errs) < 3 {
		t.Errorf("expected at least 3 aggregated errors, got %d: %s", len(errs), errSummary(errs))
	}
}

// --- ResolveDomainModelVersion ---

func TestResolveDomainModelVersion_Equal(t *testing.T) {
	res := ResolveDomainModelVersion(DomainModelBinaryVersion)
	if res.Outcome != DomainModelVersionEqual {
		t.Errorf("expected Equal outcome, got %d", res.Outcome)
	}
	if len(res.Chain) != 0 {
		t.Errorf("expected empty migrator chain on equal version, got %d migrators", len(res.Chain))
	}
}

func TestResolveDomainModelVersion_TooNew(t *testing.T) {
	res := ResolveDomainModelVersion(DomainModelBinaryVersion + 5)
	if res.Outcome != DomainModelVersionTooNew {
		t.Errorf("expected TooNew outcome, got %d", res.Outcome)
	}
	if len(res.Chain) != 0 {
		t.Errorf("TooNew should have no migrator chain; got %d", len(res.Chain))
	}
}

func TestResolveDomainModelVersion_OlderUnreachable(t *testing.T) {
	// No migrators are registered in this build (binary version is 1,
	// the first released value), so any older version is unreachable.
	res := ResolveDomainModelVersion(DomainModelBinaryVersion - 1)
	if res.Outcome != DomainModelVersionUnreachable {
		t.Errorf("expected Unreachable outcome (no migrators registered for v0→v1), got %d", res.Outcome)
	}
}

// fakeMigrator lets the migrator-chain test register a v0→v1 step
// without depending on a real migrator implementation. The registry is
// package-level; the test takes the simplest defensible approach by
// registering, exercising, and clearing.
type fakeMigrator struct {
	from, to int
}

func (m *fakeMigrator) FromVersion() int { return m.from }
func (m *fakeMigrator) ToVersion() int   { return m.to }
func (m *fakeMigrator) Migrate(in map[string]interface{}) (map[string]interface{}, error) {
	return in, nil
}

func TestResolveDomainModelVersion_OlderWithChain(t *testing.T) {
	// Snapshot and restore the global registry so this test doesn't
	// leak into the production code path.
	saved := domainModelMigrators
	domainModelMigrators = map[int]DomainModelMigrator{}
	defer func() { domainModelMigrators = saved }()

	RegisterDomainModelMigrator(&fakeMigrator{from: 0, to: DomainModelBinaryVersion})

	res := ResolveDomainModelVersion(0)
	if res.Outcome != DomainModelVersionMigrate {
		t.Errorf("expected Migrate outcome with registered chain, got %d", res.Outcome)
	}
	if len(res.Chain) != 1 {
		t.Errorf("expected 1 migrator in chain, got %d", len(res.Chain))
	}
}

// closed-set vocabularies appear verbatim in the docstrings/error
// messages — guard against accidental drift.
func TestClosedSetMessages(t *testing.T) {
	if !strings.Contains(closedFieldTypeList, "uuid") || !strings.Contains(closedFieldTypeList, "named-enum") {
		t.Errorf("closedFieldTypeList lost a primitive: %q", closedFieldTypeList)
	}
	if !strings.Contains(closedCardinalityList, "many-to-many") {
		t.Errorf("closedCardinalityList lost many-to-many: %q", closedCardinalityList)
	}
	if !strings.Contains(closedEnumToneList, "neutral") || !strings.Contains(closedEnumToneList, "success") {
		t.Errorf("closedEnumToneList lost a tone: %q", closedEnumToneList)
	}
}
