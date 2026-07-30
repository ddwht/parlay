// parlay-feature: domain-model-editor/domain-model-editor-validation
// parlay-component: cross-cutting/out-of-process-validate-endpoint
// parlay-artifact: test
//
// The parity / drift alarm, and the editor's no-binary proof.
//
// The alarm runs a shared fixture corpus through BOTH Core's domain-model
// validator directly AND the POST /api/domain-model/validate endpoint, and
// asserts the two finding sets are identical per fixture. Because both paths now
// run the SAME in-process validator, it guards the editor's wrapper — the YAML a
// draft is re-serialized to, the wire Model round-trip, the element-path and
// severity mapping — and not Core's rules. A mismatch means the editor perturbed
// the model on the way through. That was always the suite's purpose; its own
// comment said so while the two paths were two processes.
//
// Two things changed with the merge.
//
// It used to sit behind `//go:build integration` and skip unless a built `parlay`
// was on PATH or at PARLAY_BIN, because the "direct" side shelled out to one.
// Neither the tag nor the skip gates anything now — the direct side is a function
// call — so it runs on the default `go test ./...` like any other test. A drift
// alarm that only rings in a CI stage somebody remembered to configure is most of
// a drift alarm missing.
//
// And it moved here from internal/editor/domain. The direct side needs
// core/internal/agent, which is importable only from under core/; the endpoint
// side needs the editor's subsystem, which is importable from anywhere. This
// package is the only one that can see both. The corpus below is carried over
// unchanged.

package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/internal/editor/domain"
)

// parityCorpus is one draft per closed error code plus a clean fixture. Each is
// validated through both paths; the finding sets must match.
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
	"domain-operations-deprecated": `schema_version: 1
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

// directFindings runs the fixture bytes straight through Core's validator — the
// same call, label, and mode the CLI's --json mode uses — and returns the
// normalized (sorted) finding key set.
func directFindings(t *testing.T, model []byte) []string {
	t.Helper()
	raw := agent.ValidateDomainModelStructuredMode(domain.StdinLabel, model, agent.ModeAuthoring)
	keys := make([]string, 0, len(raw))
	for _, f := range raw {
		keys = append(keys, f.Code+"@"+f.Context+"/"+f.Severity)
	}
	sort.Strings(keys)
	return keys
}

// parityValidateRequest and parityValidateResponse are the wire shapes of the
// validate endpoint, declared here rather than reused from the editor package
// because they are unexported there. Restating them is the point: the suite
// exercises the endpoint the way the browser does, over JSON, and a change to
// either shape should surface here as a decode failure rather than be absorbed
// by sharing a struct with the code under test.
type parityValidateRequest struct {
	Model domain.Model `json:"model"`
}

type parityValidateResponse struct {
	Fields []struct {
		Field    string `json:"field"`
		Code     string `json:"code"`
		Severity string `json:"severity"`
	} `json:"fields"`
}

// endpointFindings runs the model bytes through the POST /validate endpoint
// (which calls the same validator) and returns the normalized finding key set.
func endpointFindings(t *testing.T, modelYAML []byte) []string {
	t.Helper()

	// The endpoint receives the draft as JSON. Parse the fixture YAML into the
	// wire Model and re-marshal to the validate request body; the wrapper then
	// re-serializes it to YAML for the validator.
	var model domain.Model
	if err := yaml.Unmarshal(modelYAML, &model); err != nil {
		t.Fatalf("parse fixture yaml: %v", err)
	}
	// Carry the deprecated operations block onto the wire model so the endpoint
	// sees it (mirrors how the browser draft would).
	var opsProbe struct {
		Operations []map[string]any `yaml:"operations"`
	}
	_ = yaml.Unmarshal(modelYAML, &opsProbe)
	model.Operations = opsProbe.Operations

	body, err := json.Marshal(parityValidateRequest{Model: model})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	// The real subsystem with the real validator Core wires in production.
	// Injecting one here would test the injection.
	s := domain.New("/project", domainValidator)
	r := chi.NewRouter()
	s.Mount(r)

	req := httptest.NewRequest(http.MethodPost, "/api/domain-model/validate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("endpoint status = %d; body=%s", w.Code, w.Body.String())
	}
	var resp parityValidateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode endpoint output: %v\n%s", err, w.Body.String())
	}
	keys := make([]string, 0, len(resp.Fields))
	for _, f := range resp.Fields {
		keys = append(keys, f.Code+"@"+f.Field+"/"+f.Severity)
	}
	sort.Strings(keys)
	return keys
}

// TestValidateParityAcrossEndpointAndDirectCLI is the drift alarm.
func TestValidateParityAcrossEndpointAndDirectCLI(t *testing.T) {
	for name, model := range parityCorpus {
		t.Run(name, func(t *testing.T) {
			direct := directFindings(t, []byte(model))
			endpoint := endpointFindings(t, []byte(model))
			if strings.Join(direct, "|") != strings.Join(endpoint, "|") {
				t.Fatalf("parity drift for %q:\n  direct:   %v\n  endpoint: %v", name, direct, endpoint)
			}
			// A corpus entry that provokes nothing on either side would pass
			// this comparison while testing nothing. Only "clean" is allowed to
			// be empty.
			if name != "clean" && len(direct) == 0 {
				t.Fatalf("fixture %q produced no findings; it no longer exercises the code it names", name)
			}
		})
	}
}

// TestEditorValidatesWithNoBinary is what replaces
// TestBinaryLocatedOnceAtConstruction, which asserted the editor resolved a
// `parlay` executable once at construction and reused it. There is no executable
// to resolve now; the claim worth keeping is that validation depends on nothing
// outside the process.
//
// PATH is emptied rather than trusted to be clean. This machine has a real parlay
// installed, so a test that merely ran here would pass just as happily against
// the subprocess it is meant to prove gone.
func TestEditorValidatesWithNoBinary(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("PARLAY_BIN", "")

	// A draft with no schema_version, so Core has something to say about it.
	// Asserting a specific code confirms the real rules ran — an empty result
	// would be equally consistent with a validator that did nothing at all.
	got := endpointFindings(t, []byte("entities:\n  - name: Order\n    fields:\n      - name: id\n        type: uuid\n        required: true\n"))
	if !slices.ContainsFunc(got, func(k string) bool { return strings.HasPrefix(k, "missing-schema-version@") }) {
		t.Fatalf("want missing-schema-version from the real validator with no binary on PATH, got %v", got)
	}
}
