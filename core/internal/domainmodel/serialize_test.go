// parlay-feature: parlay-tool/domain-document-api
// parlay-component: cross-cutting/deterministic-serialization-and-operations-passthrough
// parlay-artifact: test

package domainmodel

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSerializeByteIdentical asserts serializing the same in-memory model twice
// produces byte-identical output and therefore the same etag.
func TestSerializeByteIdentical(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	model, _, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	out1, err := Serialize(model)
	if err != nil {
		t.Fatalf("Serialize #1: %v", err)
	}
	out2, err := Serialize(model)
	if err != nil {
		t.Fatalf("Serialize #2: %v", err)
	}
	if !bytes.Equal(out1, out2) {
		t.Fatalf("two serializations differ:\n--- #1 ---\n%s\n--- #2 ---\n%s", out1, out2)
	}
	if computeEtag(out1) != computeEtag(out2) {
		t.Fatal("byte-identical output produced different etags")
	}
}

// TestUnsetLabelAndToneOmitted asserts a value with no label and no tone
// serializes without empty-string label/tone keys, while a populated value
// keeps them.
func TestUnsetLabelAndToneOmitted(t *testing.T) {
	model := Model{
		SchemaVersion: 1,
		Enums: []Enum{{
			Name: "OrderStatus",
			Values: []EnumValue{
				{Value: "paid"}, // no label, no tone
				{Value: "pending", Label: "Pending", Tone: "warning"}, // both set
			},
		}},
		Entities: []Entity{},
	}
	out, err := Serialize(model)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	s := string(out)
	if strings.Contains(s, `label: ""`) || strings.Contains(s, `tone: ""`) {
		t.Fatalf("empty-string label/tone must never be written:\n%s", s)
	}
	// The one populated value contributes exactly one label: and one tone: key.
	if got := strings.Count(s, "label:"); got != 1 {
		t.Fatalf("label: key count = %d, want 1 (only the populated value)\n%s", got, s)
	}
	if got := strings.Count(s, "tone:"); got != 1 {
		t.Fatalf("tone: key count = %d, want 1 (only the populated value)\n%s", got, s)
	}
}

// TestOperationsBlockRoundTripsUntouched asserts a save touching only an
// unrelated entity leaves the deprecated operations block byte-for-byte
// identical to the on-disk block at load time.
func TestOperationsBlockRoundTripsUntouched(t *testing.T) {
	root := writeTempModel(t, testPopulatedYAML)
	model, etag, err := Load(context.Background(), root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// An edit that does not touch the operations block: add a field to Customer.
	model.Entities[0].Fields = append(model.Entities[0].Fields, Field{
		Name: "created_at", Type: "timestamp", Required: false,
	})

	if _, err := Save(context.Background(), root, model, etag); err != nil {
		t.Fatalf("Save: %v", err)
	}

	after, err := os.ReadFile(filepath.Join(root, modelFileName))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	origOps := captureOperationsBlock([]byte(testPopulatedYAML))
	afterOps := captureOperationsBlock(after)
	if len(origOps) == 0 {
		t.Fatal("fixture has no operations block to preserve")
	}
	if !bytes.Equal(origOps, afterOps) {
		t.Fatalf("operations block changed across a save:\n--- before ---\n%s\n--- after ---\n%s", origOps, afterOps)
	}
	// And the unrelated edit did land.
	if !strings.Contains(string(after), "created_at") {
		t.Fatal("the unrelated edit did not persist")
	}
}
