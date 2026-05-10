// parlay-feature: parlay-tool/multi-adapter
// parlay-component: codegen-read-set-and-layer-pipeline
// parlay-artifact: test

package agent

import "testing"

func TestCodegenInputGuard_SpecIntentsForbidden(t *testing.T) {
	a := AllowedReadPaths{}
	if code := a.CheckRead("spec/intents/task-list/intents.md"); code != "codegen-spec-read-forbidden" {
		t.Errorf("got %q, want codegen-spec-read-forbidden", code)
	}
}

func TestCodegenInputGuard_AbsolutePathSpecIntentsForbidden(t *testing.T) {
	a := AllowedReadPaths{}
	if code := a.CheckRead("/workspace/parlay-dev/core/spec/intents/x/y.md"); code != "codegen-spec-read-forbidden" {
		t.Errorf("got %q, want codegen-spec-read-forbidden", code)
	}
}

func TestCodegenInputGuard_OutOfScope(t *testing.T) {
	a := AllowedReadPaths{
		SourceRoots: []string{"apps/web"},
	}
	if code := a.CheckRead("apps/api/x.ts"); code != "codegen-input-out-of-scope" {
		t.Errorf("got %q, want codegen-input-out-of-scope", code)
	}
}

func TestCodegenInputGuard_AllowedSourceRoot(t *testing.T) {
	a := AllowedReadPaths{
		SourceRoots: []string{"apps/web"},
	}
	if code := a.CheckRead("apps/web/src/Foo.tsx"); code != "" {
		t.Errorf("got %q, want allow", code)
	}
}

func TestCodegenInputGuard_AllowedAdapter(t *testing.T) {
	a := AllowedReadPaths{
		Adapters: []string{".parlay/adapters/react-antd.adapter.yaml"},
	}
	if code := a.CheckRead(".parlay/adapters/react-antd.adapter.yaml"); code != "" {
		t.Errorf("got %q, want allow", code)
	}
}
