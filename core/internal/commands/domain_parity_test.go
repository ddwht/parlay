// parlay-feature: parlay-tool/domain-document-api
// parlay-component: cross-cutting/out-of-process-validate-endpoint
//
// The component name says out-of-process and there has been neither a process
// boundary nor an endpoint for some time. The identifier is a spec anchor read
// by the coverage and drift checks through the marker-to-buildfile
// correspondence; renaming it here alone would break that correspondence, and
// renaming it properly is a spec migration across those artifacts. Recorded so
// the next reader knows the staleness was priced rather than missed.
// parlay-artifact: test
//
// The one-engine alarm.
//
// There is exactly one set of domain-model rules in this repository —
// agent.ValidateDomainModelStructuredMode — and three ways to reach it. This
// suite runs a shared fixture corpus through all three and asserts they return
// the same findings:
//
//	direct  agent.ValidateDomainModelStructuredMode on the fixture bytes
//	seam    domainmodel.Validate through the domainValidator ValidatorFunc,
//	        the path reserved for the future write gate
//	cli     `parlay validate --type domain-model --json` reading the bytes
//	        from stdin
//
// What it guards is not the rules — one engine cannot disagree with itself —
// but the wrappers around it, WITHIN THE LIMITS OF WHAT DIAGNOSTICS SHOW: the
// legs compare (code, path, severity) sets, so a round-trip perturbation is
// caught only when it changes some finding (a dropped operations block, a
// re-typed scalar). A lost VALID field leaves every diagnostic set empty and
// is invisible to this comparison — that property is witnessed separately by
// TestDomainParity_RoundTripPreservesValidFields below, which asserts the
// typed decode+serialize round-trip reproduces the clean fixture's content.
// The CLI leg covers the label and the JSON projection. A mismatch means a
// wrapper changed the model or the finding on the way through.
//
// It replaces a two-leg version whose second leg was an HTTP endpoint in the
// browser editor. The editor is gone; the witness is not, because deleting a
// cross-path check and promising a replacement later is how the paths get to
// disagree unobserved. Phase 4.1 adds the `parlay domain validate` leg to this
// same corpus when that command exists.

package commands

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/domainmodel"
)

// parityCorpus is one draft per closed error code plus a clean fixture. Each is
// validated through every leg; the finding sets must match.
var parityCorpus = map[string]string{
	"clean": `schema_version: 1
entities:
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
`,
	"missing-schema-version": `entities:
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
`,
	"field-type-outside-closed-set": `schema_version: 1
entities:
  - name: Order
    fields:
      - name: qty
        type: quantity
        required: true
`,
	"undeclared-entity-reference": `schema_version: 1
entities:
  - name: Order
    fields:
      - name: status
        type: OrderStatus
        enum: OrderStatus
        required: true
`,
	"relationship-cardinality-unknown": `schema_version: 1
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
`,
	"domain-operations-unsupported": `schema_version: 1
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
`,
}

// findingKey is the normalized comparison form: the closed code, the element
// path the finding is anchored to, and the severity. Everything the three legs
// are allowed to render differently (field names, JSON tags, struct types) is
// dropped; everything they must agree on is kept.
func findingKey(code, path, severity string) string {
	return code + "@" + path + "/" + severity
}

// directFindings runs the fixture bytes straight through Core's validator — the
// same call, label, and mode the CLI's --json mode uses.
func directFindings(t *testing.T, model []byte) []string {
	t.Helper()
	raw := agent.ValidateDomainModelStructuredMode(domainmodel.StdinLabel, model, agent.ModeAuthoring)
	keys := make([]string, 0, len(raw))
	for _, f := range raw {
		keys = append(keys, findingKey(f.Code, f.Context, f.Severity))
	}
	sort.Strings(keys)
	return keys
}

// seamFindings runs the fixture through the ValidatorFunc seam: decode to a
// Model, then let domainmodel.Validate re-serialize it and call domainValidator
// — the exact path a save takes before it writes.
func seamFindings(t *testing.T, modelYAML []byte) []string {
	t.Helper()

	var model domainmodel.Model
	if err := yaml.Unmarshal(modelYAML, &model); err != nil {
		t.Fatalf("parse fixture yaml: %v", err)
	}
	// The deprecated operations block is not part of the typed Model, so carry
	// it across explicitly; a draft that drops it on the way in would validate
	// clean for the wrong reason.
	var opsProbe struct {
		Operations []map[string]any `yaml:"operations"`
	}
	_ = yaml.Unmarshal(modelYAML, &opsProbe)
	model.Operations = opsProbe.Operations

	// The real validator Core wires in production. Injecting one here would
	// test the injection.
	findings, err := domainmodel.Validate(context.Background(), domainValidator, model)
	if err != nil {
		t.Fatalf("validate through the seam: %v", err)
	}
	keys := make([]string, 0, len(findings))
	for _, f := range findings {
		keys = append(keys, findingKey(f.Code, f.Field, f.Severity))
	}
	sort.Strings(keys)
	return keys
}

// cliFindings runs the fixture through `validate --type domain-model --json`
// with the bytes on stdin and decodes the finding array it prints.
func cliFindings(t *testing.T, modelYAML []byte) []string {
	t.Helper()

	cmd, out := newDMVCmd(string(modelYAML))
	// A blocking finding makes this return an ExitCodeError, which is the
	// contract, not a failure of the run — the findings are on stdout either
	// way and comparing them is the point.
	_ = runValidateDomainModelJSON(cmd, "-")

	var findings []agent.ValidationError
	if err := json.Unmarshal(out.Bytes(), &findings); err != nil {
		t.Fatalf("decode CLI output: %v\n%s", err, out.String())
	}
	keys := make([]string, 0, len(findings))
	for _, f := range findings {
		keys = append(keys, findingKey(f.Code, f.Context, f.Severity))
	}
	sort.Strings(keys)
	return keys
}

// TestValidateParityAcrossReachPaths is the one-engine alarm.
func TestValidateParityAcrossReachPaths(t *testing.T) {
	for name, model := range parityCorpus {
		t.Run(name, func(t *testing.T) {
			direct := directFindings(t, []byte(model))
			seam := seamFindings(t, []byte(model))
			cli := cliFindings(t, []byte(model))

			if strings.Join(direct, "|") != strings.Join(seam, "|") {
				t.Errorf("seam disagrees with the validator for %q:\n  direct: %v\n  seam:   %v", name, direct, seam)
			}
			if strings.Join(direct, "|") != strings.Join(cli, "|") {
				t.Errorf("CLI disagrees with the validator for %q:\n  direct: %v\n  cli:    %v", name, direct, cli)
			}
			// A corpus entry that provokes nothing on every leg would pass the
			// comparisons while testing nothing. Only "clean" may be empty.
			if name != "clean" && len(direct) == 0 {
				t.Fatalf("fixture %q produced no findings; it no longer exercises the code it names", name)
			}
		})
	}
}

// TestValidatesWithNoBinary is what replaces
// TestBinaryLocatedOnceAtConstruction, which asserted the editor resolved a
// `parlay` executable once at construction and reused it. There is no
// executable to resolve now; the claim worth keeping is that validation
// through the seam depends on nothing outside the process.
//
// PATH is emptied rather than trusted to be clean. This machine has a real
// parlay installed, so a test that merely ran here would pass just as happily
// against the subprocess it is meant to prove gone.
func TestValidatesWithNoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("PARLAY_BIN", "")

	// A draft with no schema_version, so Core has something to say about it.
	// Asserting a specific code confirms the real rules ran — an empty result
	// would be equally consistent with a validator that did nothing at all.
	got := seamFindings(t, []byte("entities:\n  - name: Order\n    fields:\n      - name: id\n        type: uuid\n        required: true\n"))
	if !slices.ContainsFunc(got, func(k string) bool { return strings.HasPrefix(k, "missing-schema-version@") }) {
		t.Fatalf("want missing-schema-version from the real validator with no binary on PATH, got %v", got)
	}
}

// TestDomainParity_RoundTripPreservesValidFields witnesses the property the
// diagnostic comparison above cannot: a VALID field survives the typed
// decode+serialize round-trip. Diagnostics stay empty whether or not a valid
// field is lost, so preservation has to be asserted on the content itself. A
// richer clean fixture exercises every field the wire model carries — enum
// with label and tone, ref with target, enum-typed field with its companion
// key, a relationship with a field.relationship back-reference, and the
// required flag in both states.
func TestDomainParity_RoundTripPreservesValidFields(t *testing.T) {
	fixture := `schema_version: 1
enums:
  - name: OrderStatus
    values:
      - value: open
        label: Open
        tone: info
      - value: closed
entities:
  - name: Customer
    fields:
      - name: id
        type: uuid
        required: true
  - name: Order
    fields:
      - name: id
        type: uuid
        required: true
      - name: customer
        type: ref
        target: Customer
        relationship: order-customers
        required: true
      - name: status
        type: OrderStatus
        enum: OrderStatus
        required: false
relationships:
  - name: order-customers
    from: Order
    to: Customer
    cardinality: many-to-one
`
	var first domainmodel.Model
	if err := yaml.Unmarshal([]byte(fixture), &first); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	out, err := domainmodel.Serialize(first)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	var second domainmodel.Model
	if err := yaml.Unmarshal(out, &second); err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("round-trip changed the model:\nfirst:  %+v\nsecond: %+v\nserialized:\n%s", first, second, out)
	}
	// And the serialized form still holds every distinctive scalar the
	// fixture set, so a symmetric loss (dropped by both decodes) is caught.
	for _, want := range []string{"label: Open", "tone: info", "target: Customer", "relationship: order-customers", "enum: OrderStatus", "cardinality: many-to-one", "required: false"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("serialized model lost %q:\n%s", want, out)
		}
	}
}
