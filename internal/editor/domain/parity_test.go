//go:build integration

// parlay-feature: domain-model-editor/domain-model-editor-validation
// parlay-component: cross-cutting/out-of-process-validate-endpoint
// parlay-artifact: test
//
// The parity / drift alarm. Runs a shared fixture corpus through BOTH
// `parlay validate --type domain-model --json` directly AND the
// POST /api/domain-model/validate endpoint, and asserts the two finding sets
// are identical per fixture. Because both paths run the SAME binary, this
// guards the Studio wrapper (request shaping, stdin transport, path mapping),
// not Core's rules — a mismatch means Studio perturbed the model on the way
// through.
//
// Gated behind `//go:build integration` because it requires a real, built
// `parlay` on PATH (or at PARLAY_BIN). The default `go test ./...` excludes
// it, so the unit suite needs no `parlay` installed. Run with:
//
//	cd studio && go test -tags integration ./internal/domain/...
//
// It skips cleanly (never fails) when no `parlay` binary can be located, so a
// CI stage without the binary reports skipped rather than red — but it never
// fakes a pass: when the binary IS present a real per-fixture comparison runs.

package domain

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
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

// directFindings runs the model bytes through `parlay validate` directly and
// returns the normalized (sorted) finding key set.
func directFindings(t *testing.T, bin string, model []byte) []string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), bin, "validate", "--type", "domain-model", "--json", "-")
	cmd.Stdin = bytes.NewReader(model)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("direct parlay validate: %v", err)
	}
	var raw []coreFinding
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("decode direct output: %v\n%s", err, out.String())
	}
	keys := make([]string, 0, len(raw))
	for _, f := range raw {
		keys = append(keys, f.Code+"@"+f.Context+"/"+f.Severity)
	}
	sort.Strings(keys)
	return keys
}

// endpointFindings runs the model bytes through the POST /validate endpoint
// (which shells the same binary) and returns the normalized finding key set.
func endpointFindings(t *testing.T, bin string, modelYAML []byte) []string {
	t.Helper()

	// The endpoint receives the draft as JSON. Parse the fixture YAML into the
	// wire Model and re-marshal to the validate request body; the wrapper then
	// re-serializes it to YAML for the subprocess.
	var model Model
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

	body, err := json.Marshal(validateRequest{Model: model})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	s := New("/project")
	s.parlayBin = bin
	s.validate = func(ctx context.Context, m Model) ([]Finding, error) {
		return Validate(ctx, bin, m)
	}
	r := chi.NewRouter()
	s.Mount(r)

	req := httptest.NewRequest(http.MethodPost, "/api/domain-model/validate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("endpoint status = %d; body=%s", w.Code, w.Body.String())
	}
	var resp validateResponse
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
	bin, err := locateParlayBinary()
	if err != nil || bin == "" {
		t.Skipf("no parlay binary located (%v); skipping parity integration test", err)
	}
	for name, model := range parityCorpus {
		t.Run(name, func(t *testing.T) {
			direct := directFindings(t, bin, []byte(model))
			endpoint := endpointFindings(t, bin, []byte(model))
			if strings.Join(direct, "|") != strings.Join(endpoint, "|") {
				t.Fatalf("parity drift for %q:\n  direct:   %v\n  endpoint: %v", name, direct, endpoint)
			}
		})
	}
}
