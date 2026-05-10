// parlay-feature: parlay-tool/multi-adapter
// parlay-component: capabilities-artifact
// parlay-artifact: test

package parser

import "testing"

func TestParseCapabilities_TaskCreate(t *testing.T) {
	content := []byte(`schema_version: 1
feature: task-list

operations:
  - id: task.create
    kind: command
    subject: { entity: Task }
    input: { type: CreateTaskInput }
    output: { shape: one, entity: Task }
    errors: [validation-failed, conflict]
    policies: [transaction-required]
    steps:
      - { type: validate-input }
      - { type: create-one, entity: Task, identity: generated }
      - { type: return-one, entity: Task }
`)
	caps, err := ParseCapabilitiesBytes("test", content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if caps.SchemaVersion != 1 {
		t.Errorf("schema_version: got %d", caps.SchemaVersion)
	}
	if caps.Feature != "task-list" {
		t.Errorf("feature: got %q", caps.Feature)
	}
	if got := len(caps.Operations); got != 1 {
		t.Fatalf("operations: got %d, want 1", got)
	}
	op := caps.Operations[0]
	if op.ID != "task.create" {
		t.Errorf("id: got %q", op.ID)
	}
	if op.Kind != "command" {
		t.Errorf("kind: got %q", op.Kind)
	}
	if got := len(op.Steps); got != 3 {
		t.Errorf("steps: got %d, want 3", got)
	}
	if op.Steps[1].Entity != "Task" || op.Steps[1].Identity != "generated" {
		t.Errorf("step[1]: got %+v", op.Steps[1])
	}
}

func TestNormalizeOperationID(t *testing.T) {
	got := NormalizeOperationID("task-list", "task.create")
	want := "@task-list/operation:task.create"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
