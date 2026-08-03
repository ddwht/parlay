// parlay-feature: parlay-tool/multi-adapter
// parlay-artifact: test
//
// Ownership derivation: each step is owned by the backend layer whose adapter
// lists it. Exercised against the committed multitarget fixture.

package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

func TestScaffoldOperations_DerivesOwnershipFromFixture(t *testing.T) {
	abs, err := filepath.Abs(filepath.Join("testdata", "multitarget"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Name: "multitarget", Path: abs, Kind: config.RootKindStandalone},
		Source:     config.SourceCwdWalkUp,
	}, nil)
	cmd := testCommandWithContext(t, cfg)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runScaffoldOperations(cmd, []string{"notes"}); err != nil {
		t.Fatalf("scaffold-operations: %v", err)
	}

	var out scaffoldOperationsOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("parse output: %v\n%s", err, buf.String())
	}

	cn := out.Operations["@notes/operation:notes.create-note"]
	// Persistence owns the data step; application owns validation + return
	// shaping (sorted).
	if !reflect.DeepEqual(cn.Owns["persistence"], []string{"create-one"}) {
		t.Errorf("persistence should own [create-one], got %v", cn.Owns["persistence"])
	}
	if !reflect.DeepEqual(cn.Owns["application"], []string{"return-one", "validate-input"}) {
		t.Errorf("application should own [return-one validate-input], got %v", cn.Owns["application"])
	}

	ln := out.Operations["@notes/operation:notes.list-notes"]
	if !reflect.DeepEqual(ln.Owns["persistence"], []string{"read-many"}) {
		t.Errorf("persistence should own [read-many], got %v", ln.Owns["persistence"])
	}
	if !reflect.DeepEqual(ln.Owns["application"], []string{"return-many"}) {
		t.Errorf("application should own [return-many], got %v", ln.Owns["application"])
	}
}
