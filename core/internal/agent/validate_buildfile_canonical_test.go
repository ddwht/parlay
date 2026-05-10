// parlay-feature: parlay-tool/multi-adapter
// parlay-component: multi-target-buildfile-schema
// parlay-artifact: test

package agent

import "testing"

func TestValidateBuildfileCanonical_TargetRestatesField(t *testing.T) {
	content := []byte(`feature: f
operations:
  "@f/operation:x":
    kind: command
    errors: [validation-failed]
targets:
  application:
    operations:
      - errors: [validation-failed]
`)
	outcomes := ValidateBuildfileCanonical(ModeBuild, "test", content)
	if !findCode(outcomes, "buildfile-target-restates-canonical") {
		t.Errorf("missing buildfile-target-restates-canonical; got %+v", outcomes)
	}
}

func TestValidateBuildfileCanonical_ComponentsDoubleDeclared(t *testing.T) {
	content := []byte(`feature: f
components:
  c1:
    widget: foo
targets:
  presentation:
    components:
      c2:
        widget: bar
`)
	outcomes := ValidateBuildfileCanonical(ModeBuild, "test", content)
	if !findCode(outcomes, "buildfile-components-double-declared") {
		t.Errorf("missing buildfile-components-double-declared; got %+v", outcomes)
	}
}

func TestValidateBuildfileCanonical_ModelsDeprecated(t *testing.T) {
	content := []byte(`feature: f
models:
  Task:
    properties:
      title: { type: string }
`)
	outcomes := ValidateBuildfileCanonical(ModeBuild, "test", content)
	if !findCode(outcomes, "buildfile-models-deprecated") {
		t.Errorf("missing buildfile-models-deprecated; got %+v", outcomes)
	}
}

func TestValidateBuildfileCanonical_TargetOperationMissing(t *testing.T) {
	content := []byte(`feature: f
operations:
  "@f/operation:x":
    kind: command
targets:
  transport:
    operations:
      "@f/operation:nonexistent":
        exposure: rest-endpoint
`)
	outcomes := ValidateBuildfileCanonical(ModeBuild, "test", content)
	if !findCode(outcomes, "buildfile-target-operation-missing") {
		t.Errorf("missing buildfile-target-operation-missing; got %+v", outcomes)
	}
}

func TestValidateBuildfileCanonical_BindingOperationMissing(t *testing.T) {
	content := []byte(`feature: f
operations:
  "@f/operation:x":
    kind: command
bindings:
  f:
    /home:
      header:
        layout_node: header
        surface_fragment: "@f/header"
        domain_element: "@f/operation:nope"
        confidence: rules
`)
	outcomes := ValidateBuildfileCanonical(ModeBuild, "test", content)
	if !findCode(outcomes, "buildfile-binding-operation-missing") {
		t.Errorf("missing buildfile-binding-operation-missing; got %+v", outcomes)
	}
}

func TestValidateBuildfileCanonical_BindingResolvedRefPasses(t *testing.T) {
	content := []byte(`feature: f
operations:
  "@f/operation:x":
    kind: command
bindings:
  f:
    /home:
      header:
        layout_node: header
        surface_fragment: "@f/header"
        domain_element: "@f/operation:x"
        confidence: rules
`)
	outcomes := ValidateBuildfileCanonical(ModeBuild, "test", content)
	if findCode(outcomes, "buildfile-binding-operation-missing") {
		t.Errorf("resolved binding op-ref incorrectly flagged; got %+v", outcomes)
	}
}

func TestValidateBuildfileCanonical_BareOpRefRejected(t *testing.T) {
	content := []byte(`feature: f
operations:
  "@f/operation:x":
    kind: command
targets:
  transport:
    operations:
      "task.create":
        exposure: rest-endpoint
`)
	outcomes := ValidateBuildfileCanonical(ModeBuild, "test", content)
	if !findCode(outcomes, "buildfile-operation-ref-unnormalized") {
		t.Errorf("missing buildfile-operation-ref-unnormalized for bare ref; got %+v", outcomes)
	}
}

func TestValidateBuildfileCanonical_CleanShape(t *testing.T) {
	content := []byte(`feature: f
operations:
  "@f/operation:x":
    kind: command
    errors: [validation-failed]
targets:
  presentation:
    components:
      c1:
        widget: button
`)
	outcomes := ValidateBuildfileCanonical(ModeBuild, "test", content)
	for _, o := range outcomes {
		if o.Severity == SeverityError {
			t.Errorf("unexpected error: %+v", o)
		}
	}
}
