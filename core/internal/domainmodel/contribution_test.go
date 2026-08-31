// parlay-feature: parlay-tool/domain-document-api
// parlay-component: cross-cutting/contribution-diff-and-apply
// parlay-artifact: test

package domainmodel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func model(entities ...Entity) Model {
	return Model{SchemaVersion: CurrentSchemaVersion, Enums: []Enum{}, Entities: entities}
}

func entity(name string, fields ...Field) Entity {
	return Entity{Name: name, Fields: fields}
}

func paths(els []Element) []string {
	out := make([]string, 0, len(els))
	for _, e := range els {
		out = append(out, e.Path)
	}
	return out
}

func TestAnEntityTheRootLacksIsAnAddition(t *testing.T) {
	root := model(entity("ExpenseReport", Field{Name: "id", Type: "uuid"}))
	contribution := model(entity("Approval", Field{Name: "id", Type: "uuid"}))

	d := Diff(root, contribution)
	if got := paths(d.Additions); len(got) != 1 || got[0] != "entities.Approval" {
		t.Errorf("additions = %v, want [entities.Approval]", got)
	}
	if d.HasConflicts() || len(d.Redundant) != 0 {
		t.Errorf("unexpected conflicts %#v or redundant %#v", d.Conflicts, d.Redundant)
	}
}

func TestANewFieldOnAnExistingEntityIsAnAddition(t *testing.T) {
	root := model(entity("ExpenseReport", Field{Name: "id", Type: "uuid"}))
	contribution := model(entity("ExpenseReport",
		Field{Name: "id", Type: "uuid"},
		Field{Name: "settledAt", Type: "datetime"}))

	d := Diff(root, contribution)
	if got := paths(d.Additions); len(got) != 1 || got[0] != "entities.ExpenseReport.fields.settledAt" {
		t.Errorf("additions = %v", got)
	}
	if got := paths(d.Redundant); len(got) != 1 || got[0] != "entities.ExpenseReport.fields.id" {
		t.Errorf("redundant = %v — a field already declared identically is redundant, not an addition", got)
	}
	if d.Additions[0].Entity != "ExpenseReport" {
		t.Errorf("a field addition must carry the entity it lands on: %#v", d.Additions[0])
	}
}

// The same field described two ways is the case that must not merge. Picking
// one silently is how a model stops meaning what its author thought.
func TestTheSameFieldDescribedDifferentlyIsAConflict(t *testing.T) {
	root := model(entity("ExpenseReport", Field{Name: "total", Type: "float"}))
	contribution := model(entity("ExpenseReport", Field{Name: "total", Type: "string"}))

	d := Diff(root, contribution)
	if len(d.Conflicts) != 1 {
		t.Fatalf("want one conflict, got %#v", d.Conflicts)
	}
	c := d.Conflicts[0]
	if c.Path != "entities.ExpenseReport.fields.total" {
		t.Errorf("unexpected path: %s", c.Path)
	}
	// Both descriptions have to be in the finding: "these disagree" alone
	// leaves the reader to go and look up what the root says.
	if c.Root != "type: float" || c.Proposed != "type: string" {
		t.Errorf("conflict must carry both descriptions: root %q, proposed %q", c.Root, c.Proposed)
	}
}

// A feature adding a value to an existing enum is the common case. Treating
// the enum as one unit would report it as a conflict.
func TestANewValueOnAnExistingEnumIsAnAddition(t *testing.T) {
	root := Model{SchemaVersion: 1, Enums: []Enum{{Name: "Status", Values: []EnumValue{{Value: "draft"}}}}}
	contribution := Model{SchemaVersion: 1, Enums: []Enum{{Name: "Status", Values: []EnumValue{
		{Value: "draft"}, {Value: "settled"},
	}}}}

	d := Diff(root, contribution)
	if got := paths(d.Additions); len(got) != 1 || got[0] != "enums.Status.values.settled" {
		t.Errorf("additions = %v", got)
	}
	if d.HasConflicts() {
		t.Errorf("adding a value to an enum is not a conflict: %#v", d.Conflicts)
	}
}

func TestAnEnumValueRelabelledIsAConflict(t *testing.T) {
	root := Model{SchemaVersion: 1, Enums: []Enum{{Name: "Status", Values: []EnumValue{{Value: "draft", Label: "Draft"}}}}}
	contribution := Model{SchemaVersion: 1, Enums: []Enum{{Name: "Status", Values: []EnumValue{{Value: "draft", Label: "Unsent"}}}}}

	if d := Diff(root, contribution); len(d.Conflicts) != 1 {
		t.Errorf("want one conflict, got %#v", d)
	}
}

func TestRelationshipsDiffByName(t *testing.T) {
	root := Model{SchemaVersion: 1, Relationships: []Relationship{
		{Name: "report-items", From: "ExpenseReport", To: "LineItem", Cardinality: "one-to-many"},
	}}
	contribution := Model{SchemaVersion: 1, Relationships: []Relationship{
		{Name: "report-items", From: "ExpenseReport", To: "LineItem", Cardinality: "one-to-many"},
		{Name: "report-approval", From: "ExpenseReport", To: "Approval", Cardinality: "one-to-one"},
	}}

	d := Diff(root, contribution)
	if got := paths(d.Additions); len(got) != 1 || got[0] != "relationships.report-approval" {
		t.Errorf("additions = %v", got)
	}
	if got := paths(d.Redundant); len(got) != 1 {
		t.Errorf("redundant = %v", got)
	}
}

// Two features contributing the same entity compatibly must both land.
func TestCompatibleContributionsMerge(t *testing.T) {
	root := model(entity("ExpenseReport", Field{Name: "id", Type: "uuid"}))
	first := model(entity("ExpenseReport",
		Field{Name: "id", Type: "uuid"},
		Field{Name: "settledAt", Type: "datetime"}))
	second := model(entity("ExpenseReport",
		Field{Name: "id", Type: "uuid"},
		Field{Name: "settledBy", Type: "ref", Target: "Employee"}))

	afterFirst, err := Merge(root, first)
	if err != nil {
		t.Fatalf("first merge: %v", err)
	}
	afterSecond, err := Merge(afterFirst, second)
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}

	var names []string
	for _, f := range afterSecond.Entities[0].Fields {
		names = append(names, f.Name)
	}
	want := []string{"id", "settledAt", "settledBy"}
	if len(names) != len(want) {
		t.Fatalf("fields = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("fields = %v, want %v — additions append in contribution order", names, want)
		}
	}
}

// Incompatible contributions refuse, and the refusal names both descriptions
// so the person deciding has what they need.
func TestIncompatibleContributionRefusesNamingBoth(t *testing.T) {
	root := model(entity("ExpenseReport", Field{Name: "total", Type: "float"}))
	contribution := model(entity("ExpenseReport", Field{Name: "total", Type: "string"}))

	_, err := Merge(root, contribution)
	var conflictErr *ErrContributionConflicts
	if !errors.As(err, &conflictErr) {
		t.Fatalf("want ErrContributionConflicts, got %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"entities.ExpenseReport.fields.total", "float", "string"} {
		if !containsStr(msg, want) {
			t.Errorf("refusal must name %q: %s", want, msg)
		}
	}
}

// Merge must not modify the model it was handed. The caller still holds the
// pre-merge root — the impact report is computed against it.
func TestMergeDoesNotMutateTheRoot(t *testing.T) {
	root := model(entity("ExpenseReport", Field{Name: "id", Type: "uuid"}))
	contribution := model(entity("ExpenseReport",
		Field{Name: "id", Type: "uuid"},
		Field{Name: "settledAt", Type: "datetime"}))

	if _, err := Merge(root, contribution); err != nil {
		t.Fatal(err)
	}
	if len(root.Entities[0].Fields) != 1 {
		t.Errorf("Merge mutated the root model: %#v", root.Entities[0].Fields)
	}
}

func writeModelFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The apply path writes through the same serializer Save uses, so accepting a
// contribution produces the same bytes any other write of the same document
// would.
func TestApplyContributionWritesThroughTheSavePath(t *testing.T) {
	root := t.TempDir()
	writeModelFile(t, root, "domain-model.yaml", `schema_version: 1
entities:
  - name: ExpenseReport
    fields:
      - name: id
        type: uuid
`)

	contribution := model(entity("Approval", Field{Name: "id", Type: "uuid"}))
	if _, err := ApplyContribution(context.Background(), root, contribution); err != nil {
		t.Fatalf("apply: %v", err)
	}

	after, _, err := Load(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Entities) != 2 || after.Entities[1].Name != "Approval" {
		t.Fatalf("contribution did not land: %#v", after.Entities)
	}

	// The same merge serialized directly must produce identical bytes —
	// that is what "one writer" means in practice.
	onDisk, err := os.ReadFile(filepath.Join(root, "domain-model.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	direct, err := Serialize(after)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != string(direct) {
		t.Errorf("apply wrote bytes the serializer would not produce:\n--- on disk ---\n%s\n--- serialized ---\n%s", onDisk, direct)
	}
}

// A conflicting apply writes nothing. Half-applying a proposal is worse than
// refusing it — the file would then be in a state no one authored.
func TestApplyOnAConflictWritesNothing(t *testing.T) {
	root := t.TempDir()
	const original = `schema_version: 1
entities:
  - name: ExpenseReport
    fields:
      - name: total
        type: float
`
	path := writeModelFile(t, root, "domain-model.yaml", original)

	contribution := model(entity("ExpenseReport", Field{Name: "total", Type: "string"}))
	if _, err := ApplyContribution(context.Background(), root, contribution); err == nil {
		t.Fatal("a conflicting contribution must not apply")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != original {
		t.Errorf("the file changed despite the refusal:\n%s", after)
	}
}

// A project with no contribution behaves exactly as before: LoadFile says so
// with a sentinel rather than an error a caller has to pattern-match on.
func TestAMissingContributionIsASentinelNotAFailure(t *testing.T) {
	_, err := LoadFile(filepath.Join(t.TempDir(), "domain-model.yaml"))
	if !errors.Is(err, ErrNoContribution) {
		t.Errorf("want ErrNoContribution, got %v", err)
	}
}

// A contribution reads through the same decoder as the root model, so the
// schema-version gate applies to it identically.
func TestAContributionGoesThroughTheSameSchemaVersionGate(t *testing.T) {
	dir := t.TempDir()
	path := writeModelFile(t, dir, "domain-model.yaml", "entities: []\n")
	if _, err := LoadFile(path); !errors.Is(err, ErrMissingSchemaVersion) {
		t.Errorf("want ErrMissingSchemaVersion, got %v", err)
	}
}

func containsStr(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
