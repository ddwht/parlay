// parlay-feature: parlay-tool/multi-adapter
// parlay-component: adapter-kind-discriminator
// parlay-artifact: test

package parser

import (
	"path/filepath"
	"testing"
)

func TestParseAdapterSet_FullStack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "adapter-set.yaml")
	content := []byte(`name: my-app
targets:
  presentation: { adapter: react-antd, root: apps/web }
  transport: { adapter: openapi-rest, root: apps/api }
  application: { adapter: nestjs-application, root: apps/api }
  persistence: { adapter: prisma-postgres, root: apps/api }
links:
  - { from: presentation, relation: calls, to: transport }
  - { from: transport, relation: dispatches, to: application }
  - { from: application, relation: persists, to: persistence }
`)
	as, err := ParseAdapterSetBytes(path, content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if as.Name != "my-app" {
		t.Errorf("name: got %q, want my-app", as.Name)
	}
	if got := len(as.Targets); got != 4 {
		t.Errorf("targets: got %d, want 4", got)
	}
	if got := as.Targets["presentation"].Adapter; got != "react-antd" {
		t.Errorf("presentation adapter: got %q, want react-antd", got)
	}
	if got := len(as.Links); got != 3 {
		t.Errorf("links: got %d, want 3", got)
	}
	if !as.IsMultiTarget() {
		t.Error("IsMultiTarget: got false on full-stack adapter set")
	}
}

func TestParseAdapterSet_PresentationOnly(t *testing.T) {
	as, err := ParseAdapterSetBytes("test.yaml", []byte(`name: presentation-only
targets:
  presentation: { adapter: react-antd, root: src }
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if as.IsMultiTarget() {
		t.Error("IsMultiTarget: got true on presentation-only adapter set")
	}
}

func TestParseAdapterSet_NilReceiver(t *testing.T) {
	var as *AdapterSet
	if as.IsMultiTarget() {
		t.Error("IsMultiTarget on nil: got true, want false")
	}
}
