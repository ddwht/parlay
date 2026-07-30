package commands

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const angularPathsYAML = `
file-conventions:
  source-root: "src/app/"
  naming: kebab-case
  paths:
    component: "features/{feature}/{name}/{name}.component.ts"
    component-extras:
      - "features/{feature}/{name}/{name}.component.html"
    test: "features/{feature}/{name}/{name}.component.spec.ts"
    model: "core/domain/{entity}.ts"
    feature-routes: "features/{feature}/{feature}.routes.ts"
    routes: "app.routes.ts"
`

func angularAdapter(t *testing.T) adapterForPlan {
	t.Helper()
	var ad adapterForPlan
	if err := yaml.Unmarshal([]byte(angularPathsYAML), &ad); err != nil {
		t.Fatal(err)
	}
	return ad
}

// The derived paths must match what codegen actually emitted in the
// regression run. A template that produces plausible-but-wrong paths is
// worse than no template: those paths go into plan.creates, which codegen
// treats as an authorization to write.
func TestDerivedPathsMatchRealEmittedLayout(t *testing.T) {
	got := derivePlanCreates("approval-history",
		[]string{"decided-reports-history-table"}, nil, angularAdapter(t))

	want := []string{
		"src/app/features/approval-history/decided-reports-history-table/decided-reports-history-table.component.ts",
		"src/app/features/approval-history/decided-reports-history-table/decided-reports-history-table.component.spec.ts",
		"src/app/features/approval-history/decided-reports-history-table/decided-reports-history-table.component.html",
		"src/app/features/approval-history/approval-history.routes.ts",
	}
	var paths []string
	for _, e := range got.Creates {
		paths = append(paths, e.Path)
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("derived paths do not match the emitted layout\n got: %#v\nwant: %#v", paths, want)
	}
	if len(got.Undecidable) != 0 {
		t.Errorf("unexpected undecidable entries: %v", got.Undecidable)
	}
}

// Every row cites what produced it, so validate --deep's "every component
// has a plan row referencing it" check passes by construction rather than by
// the agent having remembered.
func TestEveryComponentIsCitedBySomeRow(t *testing.T) {
	comps := []string{"alpha-list", "beta-detail", "gamma-form"}
	got := derivePlanCreates("f", comps, nil, angularAdapter(t))
	for _, c := range comps {
		found := false
		for _, e := range got.Creates {
			for _, s := range e.Sources {
				if s == "component/"+c {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("no plan row cites component/%s", c)
		}
	}
}

// Same inputs, same output, every time — including across map-iteration
// randomization, which an earlier version of this code was subject to.
func TestDerivationIsDeterministic(t *testing.T) {
	comps := []string{"zeta", "alpha", "mu", "beta"}
	ents := []string{"Report", "Approval", "LineItem"}
	first := derivePlanCreates("f", comps, ents, angularAdapter(t))
	for i := 0; i < 50; i++ {
		if !reflect.DeepEqual(derivePlanCreates("f", comps, ents, angularAdapter(t)), first) {
			t.Fatalf("derivation varied between runs on iteration %d", i)
		}
	}
	// And is order-insensitive in its input.
	shuffled := derivePlanCreates("f", []string{"mu", "beta", "zeta", "alpha"}, []string{"LineItem", "Report", "Approval"}, angularAdapter(t))
	if !reflect.DeepEqual(shuffled, first) {
		t.Fatal("derivation depends on input ordering")
	}
}

// An adapter with no paths block must report that it cannot derive, not
// guess. A guessed path in plan.creates reads as an authorized write target.
func TestNoPathsBlockReportsUndecidableRatherThanGuessing(t *testing.T) {
	var bare adapterForPlan
	yaml.Unmarshal([]byte("file-conventions:\n  source-root: \"src/\"\n  naming: kebab-case\n"), &bare)
	got := derivePlanCreates("f", []string{"a", "b"}, nil, bare)
	if len(got.Creates) != 0 {
		t.Fatalf("derived %d rows from an adapter with no templates: %#v", len(got.Creates), got.Creates)
	}
	if len(got.Undecidable) == 0 {
		t.Fatal("silently derived nothing without saying why")
	}
	// One fact about the adapter, not one per component.
	for _, u := range got.Undecidable {
		count := 0
		for _, v := range got.Undecidable {
			if v == u {
				count++
			}
		}
		if count > 1 {
			t.Errorf("undecidable reason repeated %d times: %q", count, u)
		}
	}
}

// Naming conventions have to reach the placeholders, or a snake_case project
// gets kebab-case paths that exist nowhere.
func TestNamingConventionApplies(t *testing.T) {
	cases := map[string]string{
		"kebab-case": "expense-wizard-details-step",
		"snake_case": "expense_wizard_details_step",
		"PascalCase": "ExpenseWizardDetailsStep",
		"camelCase":  "expenseWizardDetailsStep",
	}
	for naming, want := range cases {
		if got := applyNaming("expense-wizard-details-step", naming); got != want {
			t.Errorf("applyNaming(%s) = %q, want %q", naming, got, want)
		}
		// Input shape must not matter — camelCase input yields the same result.
		if got := applyNaming("expenseWizardDetailsStep", naming); got != want {
			t.Errorf("applyNaming(%s) on camelCase input = %q, want %q", naming, got, want)
		}
	}
}

// An unresolved placeholder must be reported, never emitted: a path with a
// literal brace in it is not a path.
func TestUnresolvedPlaceholderIsAnError(t *testing.T) {
	if _, err := expandTemplate("features/{feature}/{unknown}/x.ts", "f", "n", "", "kebab-case"); err == nil {
		t.Fatal("expected an error for an unrecognized placeholder")
	}
	if _, err := expandTemplate("features/{feature}/{name}.ts", "f", "n", "", "kebab-case"); err != nil {
		t.Fatalf("unexpected error on a valid template: %v", err)
	}
}

// Entity rows are attributed to the section, not to a component: the merged
// model layer belongs to no single feature, which is why nothing owned these
// rows before and codegen wrote them outside the allowlist.
func TestEntityRowsAreAttributedToTheModelsSection(t *testing.T) {
	got := derivePlanCreates("f", nil, []string{"ExpenseReport"}, angularAdapter(t))
	if len(got.Creates) != 1 {
		t.Fatalf("want 1 row, got %#v", got.Creates)
	}
	if got.Creates[0].Path != "src/app/core/domain/expense-report.ts" {
		t.Errorf("entity path = %q", got.Creates[0].Path)
	}
	if !reflect.DeepEqual(got.Creates[0].Sources, []string{"section/models"}) {
		t.Errorf("entity sources = %v, want [section/models]", got.Creates[0].Sources)
	}
}

// The composed seed is one file for the whole project, sourced section/seed.
// It is gated on the entity set — the seed is that data — so a feature with
// no entities gets no seed row, exactly as it gets no model rows.
func TestSeedRowIsDerivedFromTheDeclaredTemplate(t *testing.T) {
	ad := angularAdapter(t)
	ad.FileConventions.Paths.Seed = "core/fixtures/seed.data.ts"

	got := derivePlanCreates("f", nil, []string{"ExpenseReport"}, ad)

	var seedRows []planEntry
	for _, c := range got.Creates {
		for _, s := range c.Sources {
			if s == "section/seed" {
				seedRows = append(seedRows, c)
			}
		}
	}
	if len(seedRows) != 1 {
		t.Fatalf("want exactly 1 section/seed row, got %#v", seedRows)
	}
	if seedRows[0].Path != "src/app/core/fixtures/seed.data.ts" {
		t.Errorf("seed path = %q", seedRows[0].Path)
	}

	// No entities, no seed: there would be nothing in it.
	if bare := derivePlanCreates("f", nil, nil, ad); len(bare.Creates) != 0 {
		t.Errorf("a feature with no entities should derive no seed row, got %#v", bare.Creates)
	}
}

// An adapter that declares no seed template gets no seed row and no
// complaint. Most frameworks have no single boot-time dataset — a CLI reads a
// file per invocation, a static site has no runtime — so demanding one would
// be parlay asserting framework knowledge it does not have. Absence is not an
// error, and it must not show up as undecidable either: "cannot derive this"
// is a different claim from "this framework has no such thing".
func TestNoSeedTemplateDerivesNoSeedRowAndNoComplaint(t *testing.T) {
	got := derivePlanCreates("f", nil, []string{"ExpenseReport"}, angularAdapter(t))
	for _, c := range got.Creates {
		for _, s := range c.Sources {
			if s == "section/seed" {
				t.Fatalf("derived a seed row from an adapter that declares none: %#v", c)
			}
		}
	}
	for _, u := range got.Undecidable {
		if strings.Contains(u, "seed") {
			t.Errorf("absence of paths.seed must not be reported as undecidable: %q", u)
		}
	}
}
