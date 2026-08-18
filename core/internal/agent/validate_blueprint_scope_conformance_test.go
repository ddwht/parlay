// parlay-feature: parlay-tool/multi-adapter
// parlay-component: blueprint-scope-and-precedence
// parlay-artifact: test
//
// Ties blueprint's closed key set to the schema section that documents it.
//
// The schema's "Owned scope" section says, in the line right below the list,
// "Key names in this section are read by code — they must match the body."
// They did not. The list was corrected to {app, shells, navigation,
// authorization, data, errors, state, platform} and blueprintAllowedScope was
// left holding the pre-correction set, so `validate --project` rejected
// `shells:` and `authorization:` while `validate --type blueprint` accepted
// them. A prose instruction that code must match prose is not enforcement;
// this is.

package agent

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// ownedScopeLine matches the schema sentence that enumerates the closed set,
// e.g. "Blueprint's owned scope is closed to: `app`, `shells`, …".
var ownedScopeLine = regexp.MustCompile(`(?m)^Blueprint's owned scope is closed to: (.+?)\. `)

// backtickedKey pulls `app` out of "`app`,".
var backtickedKey = regexp.MustCompile("`([a-z-]+)`")

const blueprintSchemaPath = "../embedded/schemas/blueprint.schema.md"

func TestConformance_BlueprintScopeMatchesSchema(t *testing.T) {
	data, err := os.ReadFile(blueprintSchemaPath)
	if err != nil {
		t.Fatalf("read %s: %v", blueprintSchemaPath, err)
	}

	m := ownedScopeLine.FindSubmatch(data)
	if m == nil {
		t.Fatalf("could not find the \"Blueprint's owned scope is closed to:\" sentence in %s.\n"+
			"Either the section was reworded or removed. It is the documented source of this "+
			"list — if it moved, point this test at the new home rather than deleting the tie.",
			blueprintSchemaPath)
	}

	documented := map[string]bool{}
	for _, k := range backtickedKey.FindAllSubmatch(m[1], -1) {
		documented[string(k[1])] = true
	}
	if len(documented) == 0 {
		t.Fatalf("parsed zero keys out of the owned-scope sentence: %q", m[1])
	}

	var onlyInSchema, onlyInCode []string
	for k := range documented {
		if !blueprintAllowedScope[k] {
			onlyInSchema = append(onlyInSchema, k)
		}
	}
	for k := range blueprintAllowedScope {
		if !documented[k] {
			onlyInCode = append(onlyInCode, k)
		}
	}
	sort.Strings(onlyInSchema)
	sort.Strings(onlyInCode)

	if len(onlyInSchema) > 0 {
		t.Errorf("blueprint.schema.md documents %d key(s) the validator rejects: %s\n"+
			"A blueprint written from the schema body fails validation.",
			len(onlyInSchema), strings.Join(onlyInSchema, ", "))
	}
	if len(onlyInCode) > 0 {
		t.Errorf("blueprintAllowedScope permits %d key(s) the schema does not document: %s\n"+
			"Either document them or stop accepting them — an undocumented accepted key is one "+
			"nobody can discover except by reading the validator.",
			len(onlyInCode), strings.Join(onlyInCode, ", "))
	}
}
