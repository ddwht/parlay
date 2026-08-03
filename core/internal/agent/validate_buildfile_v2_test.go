// parlay-feature: parlay-tool/multi-adapter
// parlay-artifact: test
//
// Acceptance-gate + cross-kind-edge tests for the multi-target (v2) buildfile
// shape. The gate's contract is asymmetric: v2 must be accepted when an
// adapter-set names a resolvable presentation adapter, but v1 must be provably
// untouched and the legacy error string must survive byte-for-byte.

package agent

import (
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

func TestValidateBuildfile_V1StillAccepted(t *testing.T) {
	// A top-level adapter: is the frozen v1 shape. It must never reach the v2
	// branch, so it validates exactly as before multi-target existed.
	content := []byte("feature: f\nadapter: react-antd\n")
	if err := ValidateBuildfile("buildfile.yaml", content); err != nil {
		t.Fatalf("v1 buildfile must still validate: %v", err)
	}
}

func TestValidateBuildfile_V2Accepted(t *testing.T) {
	content := []byte("feature: f\nadapter-set: stack\ntargets:\n  presentation:\n    adapter: react-antd\n")
	if err := ValidateBuildfile("buildfile.yaml", content); err != nil {
		t.Fatalf("v2 buildfile with a resolvable presentation adapter must validate: %v", err)
	}
}

func TestValidateBuildfile_NeitherRejectedWithLegacyString(t *testing.T) {
	// The schema doc and downstream tooling match this string literally, so a
	// buildfile with neither shape must fail with it unchanged.
	content := []byte("feature: f\ncomponents: {}\n")
	err := ValidateBuildfile("buildfile.yaml", content)
	if err == nil {
		t.Fatal("a buildfile with neither adapter: nor adapter-set: must be rejected")
	}
	if !strings.Contains(err.Error(), "buildfile missing 'adapter' field") {
		t.Fatalf("expected the frozen legacy error string, got: %v", err)
	}
}

func TestValidateBuildfile_V2WithoutPresentationRejected(t *testing.T) {
	// adapter-set: present but no presentation slot with an adapter — not
	// enough to accept; keep the legacy error rather than half-accepting.
	content := []byte("feature: f\nadapter-set: stack\ntargets:\n  application:\n    adapter: nestjs-application\n")
	err := ValidateBuildfile("buildfile.yaml", content)
	if err == nil || !strings.Contains(err.Error(), "buildfile missing 'adapter' field") {
		t.Fatalf("v2 without a presentation adapter must be rejected with the legacy string, got: %v", err)
	}
}

func TestExtractCrossKindEdges_AuthorizedByLinks(t *testing.T) {
	// Edges derive from step OWNERSHIP: the UI calls the operation
	// (presentation→orchestrator), the application orchestrates and owns
	// return-one, and persistence owns create-one — so the orchestrator
	// (application) delegates the persistence-owned step across
	// application→persistence.
	content := []byte(`adapter-set: stack
operations:
  "@notes/operation:create": { kind: command }
targets:
  presentation:
    components:
      form:
        actions:
          - { name: submit, effect: call, target: "@notes/operation:create" }
  application:
    operations:
      "@notes/operation:create": { http: "POST /notes", owns: [return-one] }
  persistence:
    operations:
      "@notes/operation:create": { model: Note, owns: [create-one] }
`)
	edges := ExtractCrossKindEdges(content)
	got := map[string]bool{}
	for _, e := range edges {
		got[e.From+"->"+e.To] = true
	}
	if !got["presentation->application"] || !got["application->persistence"] {
		t.Fatalf("expected UI->application->persistence edges, got %+v", edges)
	}

	authorized := &parser.AdapterSet{
		Targets: map[string]parser.AdapterSetTarget{
			"presentation": {Adapter: "react-antd", Root: "apps/web"},
			"application":  {Adapter: "nestjs-application", Root: "apps/api"},
			"persistence":  {Adapter: "prisma-postgres", Root: "apps/api"},
		},
		Links: []parser.AdapterSetLink{
			{From: "presentation", Relation: "calls", To: "application"},
			{From: "application", Relation: "persists", To: "persistence"},
		},
	}
	if out := ValidateAdapterSetLinks(ModeBuild, authorized, edges); len(out) != 0 {
		t.Fatalf("fully-authorized edges must pass, got %+v", out)
	}

	// Drop the application->persistence link: the edge is now unauthorized.
	broken := *authorized
	broken.Links = authorized.Links[:1]
	out := ValidateAdapterSetLinks(ModeBuild, &broken, edges)
	if !findCode(out, "adapter-set-link-violated") {
		t.Fatalf("a missing link must produce adapter-set-link-violated, got %+v", out)
	}
}
