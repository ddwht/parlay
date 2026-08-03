// parlay-feature: parlay-tool/multi-adapter
// parlay-artifact: test
//
// A mutating toolchain tool's writes are authorized by its declared write-set,
// not the plan — so check-write-set admits files inside that region rather than
// false-flagging them codegen-wrote-outside-plan.

package commands

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

func TestWriteSetRegion(t *testing.T) {
	cases := map[string]string{
		"src/**":        "src",
		"src/app/**":    "src/app",
		"src":           "src",
		"cmd/gen/*":     "cmd/gen",
		"./src/**":      "src",
		"apps/web/src/": "apps/web/src",
	}
	for in, want := range cases {
		if got := writeSetRegion(in); got != want {
			t.Errorf("writeSetRegion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWithinAnyRegion(t *testing.T) {
	regions := []string{"apps/web/src"}
	if !withinAnyRegion("apps/web/src/features/Notes/NoteForm.tsx", regions) {
		t.Error("a file under the region should be admitted")
	}
	if !withinAnyRegion("apps/web/src", regions) {
		t.Error("the region root itself should be admitted")
	}
	if withinAnyRegion("apps/api/src/notes/notes.service.ts", regions) {
		t.Error("a file outside the region must not be admitted")
	}
	if withinAnyRegion("apps/web/srcfoo", regions) {
		t.Error("a sibling with a shared prefix but no path boundary must not be admitted")
	}
}

// The fixture's mutating MCP scaffolder declares write-set src/** on the
// presentation adapter; multi-target rebases it under the presentation root
// (apps/web), and the advisory skill's empty write-set contributes nothing.
func TestToolchainWriteSetRegions_Fixture(t *testing.T) {
	abs, err := filepath.Abs(filepath.Join("testdata", "multitarget"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Name: "multitarget", Path: abs, Kind: config.RootKindStandalone},
		Source:     config.SourceCwdWalkUp,
	}, nil)
	got := toolchainWriteSetRegions(cfg)
	want := []string{"apps/web/src"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("regions = %v, want %v", got, want)
	}
}
