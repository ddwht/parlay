// parlay-feature: parlay-tool/multi-adapter
// parlay-component: cli-and-deployer-registration
// parlay-artifact: test

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// setupProject creates a project tree for the multi-target walker to walk.
// adapterSet, adapters, and capabilities each map relative path → content.
func setupProject(t *testing.T, adapterSet string, adapters, capabilities map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	parlay := filepath.Join(dir, ".parlay")
	if err := os.MkdirAll(filepath.Join(parlay, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if adapterSet != "" {
		if err := os.WriteFile(filepath.Join(parlay, "adapter-set.yaml"), []byte(adapterSet), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for slug, content := range adapters {
		if err := os.WriteFile(filepath.Join(parlay, "adapters", slug+".adapter.yaml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for relPath, content := range capabilities {
		full := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestValidateProjectMultiTarget_PresentationOnlyShortCircuits(t *testing.T) {
	root := setupProject(t,
		`name: only
targets:
  presentation: { adapter: react-antd, root: src }
`,
		map[string]string{"react-antd": "name: react-antd\nkind: presentation\n"},
		nil,
	)
	outcomes := ValidateProjectMultiTarget(ModeBuild, root)
	for _, o := range outcomes {
		if o.Severity == SeverityError {
			t.Errorf("presentation-only should produce no error outcomes; got %+v", o)
		}
	}
}

func TestValidateProjectMultiTarget_NoAdapterSetIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".parlay"), 0o755); err != nil {
		t.Fatal(err)
	}
	outcomes := ValidateProjectMultiTarget(ModeBuild, dir)
	if len(outcomes) > 0 {
		t.Errorf("no adapter-set should yield zero outcomes; got %+v", outcomes)
	}
}

func TestValidateProjectMultiTarget_SupportsGateFires(t *testing.T) {
	root := setupProject(t,
		`name: full-stack
targets:
  presentation: { adapter: react-antd, root: apps/web }
  application:  { adapter: nestjs-application, root: apps/api }
links:
  - { from: presentation, relation: calls, to: application }
`,
		map[string]string{
			"react-antd":         "name: react-antd\nkind: presentation\n",
			"nestjs-application": "name: nestjs-application\nkind: application\nsupports:\n  operation_kinds: [command]\n  steps: [validate-input]\n  policies: []\n  errors: [validation-failed]\n",
		},
		map[string]string{
			"spec/intents/task-list/capabilities.yaml": `schema_version: 1
feature: task-list
operations:
  - id: task.search
    kind: query
    subject: { entity: Task }
    steps: [{ type: telepathy }]
`,
		},
	)
	outcomes := ValidateProjectMultiTarget(ModeBuild, root)
	if !findCode(outcomes, "adapter-supports-missing-step") {
		t.Errorf("expected adapter-supports-missing-step for telepathy; got %+v", outcomes)
	}
}

// Two filled backend slots (application + transport) that both list the same
// step get a WARNING about contested ownership — not an error. The bundled
// stacks never trip this (they fill one backend slot per step); it is the guard
// for a future multi-backend project.
func TestValidateProjectMultiTarget_AmbiguousStepOwnerWarns(t *testing.T) {
	root := setupProject(t,
		`name: stack
targets:
  presentation: { adapter: react-antd, root: apps/web }
  application:  { adapter: nestjs-application, root: apps/api }
  transport:    { adapter: openapi-rest, root: apps/api }
`,
		map[string]string{
			"react-antd":         "name: react-antd\nkind: presentation\n",
			"nestjs-application": "name: nestjs-application\nkind: application\nsupports:\n  operation_kinds: [command]\n  steps: [validate-input, return-one]\n  policies: []\n  errors: []\n",
			"openapi-rest":       "name: openapi-rest\nkind: transport\nsupports:\n  operation_kinds: [command]\n  steps: [validate-input, return-one]\n  policies: []\n  errors: []\n",
		},
		nil,
	)
	outcomes := ValidateProjectMultiTarget(ModeBuild, root)
	if !findCode(outcomes, "adapter-supports-step-ambiguous-owner") {
		t.Errorf("expected ambiguous-owner warning for contested validate-input/return-one; got %+v", outcomes)
	}
	for _, o := range outcomes {
		if o.Code == "adapter-supports-step-ambiguous-owner" && o.Severity != SeverityWarning {
			t.Errorf("ambiguous-owner must be a warning, got severity %q", o.Severity)
		}
	}
}

// prototype-framework: was removed in v0.3. A config still declaring it gets
// a hard error pointing at migrate-config — nothing reads the key anymore, so
// a project relying on it is silently unconfigured without this signal.
func TestValidateProjectMultiTarget_PrototypeFrameworkUnsupportedError(t *testing.T) {
	dir := t.TempDir()
	parlay := filepath.Join(dir, ".parlay")
	if err := os.MkdirAll(parlay, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parlay, "config.yaml"),
		[]byte("sdd-framework: GitHub SpecKit\nprototype-framework: Go CLI\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outcomes := ValidateProjectMultiTarget(ModeAuthoring, dir)
	if !findCode(outcomes, "prototype-framework-unsupported") {
		t.Errorf("expected prototype-framework-unsupported; got %+v", outcomes)
	}
	for _, o := range outcomes {
		if o.Code == "prototype-framework-unsupported" && o.Severity != SeverityError {
			t.Errorf("expected error severity; got %q", o.Severity)
		}
	}
}

func TestValidateProjectMultiTarget_BlueprintScopeFires(t *testing.T) {
	root := setupProject(t,
		`name: stack
targets:
  presentation: { adapter: react-antd, root: apps/web }
  application:  { adapter: nestjs-application, root: apps/api }
links:
  - { from: presentation, relation: calls, to: application }
`,
		map[string]string{
			"react-antd":         "name: react-antd\nkind: presentation\n",
			"nestjs-application": "name: nestjs-application\nkind: application\nsupports:\n  operation_kinds: []\n  steps: []\n  policies: []\n  errors: []\n",
		},
		nil,
	)
	// Inject a blueprint with a topology block — must fire blueprint-topology-not-allowed.
	if err := os.WriteFile(filepath.Join(root, ".parlay", "blueprint.yaml"),
		[]byte("targets:\n  presentation: { adapter: foo, root: bar }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outcomes := ValidateProjectMultiTarget(ModeBuild, root)
	if !findCode(outcomes, "blueprint-topology-not-allowed") {
		t.Errorf("expected blueprint-topology-not-allowed; got %+v", outcomes)
	}
}
