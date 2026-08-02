// parlay-feature: parlay-tool/multi-adapter
// parlay-artifact: test
//
// Per-target plan derivation for multi-target (adapter-set) buildfiles: each
// slot derives from its natural inputs and lands under its own root, and the
// derivation agrees with the committed testdata/multitarget fixture's authored
// plan.targets (the golden emission).

package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"gopkg.in/yaml.v3"
)

func mustAdapterForPlan(t *testing.T, y string) adapterForPlan {
	t.Helper()
	var ad adapterForPlan
	if err := yaml.Unmarshal([]byte(y), &ad); err != nil {
		t.Fatal(err)
	}
	return ad
}

func TestDerivePlanTargets_ThreeTargets(t *testing.T) {
	as := &parser.AdapterSet{Targets: map[string]parser.AdapterSetTarget{
		"presentation": {Adapter: "react", Root: "apps/web"},
		"application":  {Adapter: "nest", Root: "apps/api"},
		"persistence":  {Adapter: "prisma", Root: "apps/api"},
	}}
	adapters := map[string]adapterForPlan{
		"presentation": mustAdapterForPlan(t, "file-conventions:\n  naming: PascalCase\n  paths:\n    component: \"src/features/{feature}/{Name}.tsx\"\n    test: \"src/features/{feature}/{Name}.test.tsx\"\n"),
		"application":  mustAdapterForPlan(t, "file-conventions:\n  naming: kebab-case\n  paths:\n    service: \"src/{feature}/{feature}.service.ts\"\n    controller: \"src/{feature}/{feature}.controller.ts\"\n    module: \"src/{feature}/{feature}.module.ts\"\n"),
		"persistence":  mustAdapterForPlan(t, "file-conventions:\n  naming: kebab-case\n  paths:\n    model: \"prisma/schema.prisma\"\n"),
	}

	got := derivePlanTargets("notes", []string{"note-form", "note-list"}, true, []string{"Note"}, as, adapters)

	pathsOf := func(kind string) map[string]bool {
		out := map[string]bool{}
		for _, e := range got[kind].Creates {
			out[e.Path] = true
		}
		return out
	}

	pres := pathsOf("presentation")
	for _, want := range []string{
		"apps/web/src/features/Notes/NoteForm.tsx",
		"apps/web/src/features/Notes/NoteForm.test.tsx",
		"apps/web/src/features/Notes/NoteList.tsx",
		"apps/web/src/features/Notes/NoteList.test.tsx",
	} {
		if !pres[want] {
			t.Errorf("presentation plan missing %q; got %v", want, pres)
		}
	}
	// Entities belong to the persistence target, so no schema/model row leaks
	// into presentation.
	for p := range pres {
		if filepath.Base(p) == "schema.prisma" {
			t.Errorf("presentation target must not emit the persistence schema: %q", p)
		}
	}

	app := pathsOf("application")
	for _, want := range []string{
		"apps/api/src/notes/notes.service.ts",
		"apps/api/src/notes/notes.controller.ts",
		"apps/api/src/notes/notes.module.ts",
	} {
		if !app[want] {
			t.Errorf("application plan missing %q; got %v", want, app)
		}
	}

	persist := pathsOf("persistence")
	if !persist["apps/api/prisma/schema.prisma"] || len(persist) != 1 {
		t.Errorf("persistence plan should be the single shared schema, got %v", persist)
	}
}

// A feature with no operations gets no backend files.
func TestDeriveApplicationPlan_NoOperationsNoFiles(t *testing.T) {
	ad := mustAdapterForPlan(t, "file-conventions:\n  naming: kebab-case\n  source-root: apps/api\n  paths:\n    service: \"src/{feature}/{feature}.service.ts\"\n")
	if got := deriveApplicationPlan("notes", false, ad); len(got.Creates) != 0 {
		t.Fatalf("a feature with no operations must derive no backend files, got %v", got.Creates)
	}
}

// The committed fixture's authored plan.targets must agree with the live
// derivation — the golden-emission contract for `scaffold-plan --compare`.
func TestScaffoldPlanMultiTarget_AgreesWithFixture(t *testing.T) {
	abs, err := filepath.Abs(filepath.Join("testdata", "multitarget"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Name: "multitarget", Path: abs, Kind: config.RootKindStandalone},
		Source:     config.SourceCwdWalkUp,
	}, nil)
	cmd := testCommandWithContext(t, cfg)

	scaffoldPlanCompare = true
	defer func() { scaffoldPlanCompare = false }()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runScaffoldPlan(cmd, []string{"notes"}); err != nil {
		t.Fatalf("scaffold-plan: %v", err)
	}

	var out scaffoldPlanOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("parse output: %v\n%s", err, buf.String())
	}
	if out.Agrees == nil || !*out.Agrees {
		t.Fatalf("derived plan must agree with the fixture's authored plan.targets;\n only_in_derived=%v\n only_in_authored=%v",
			out.OnlyDerived, out.OnlyAuthored)
	}
	// Sanity: all three targets are present in the derivation.
	for _, kind := range []string{"presentation", "application", "persistence"} {
		if len(out.Targets[kind].Creates) == 0 {
			t.Errorf("expected derived rows for target %q", kind)
		}
	}
}
