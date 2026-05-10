// parlay-feature: parlay-tool/multi-adapter
// parlay-component: adapter-set-link-enforcement
// parlay-artifact: test

package agent

import (
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

func fullStackAdapterSet() *parser.AdapterSet {
	return &parser.AdapterSet{
		Name: "my-app",
		Targets: map[string]parser.AdapterSetTarget{
			"presentation": {Adapter: "react-antd", Root: "apps/web"},
			"transport":    {Adapter: "openapi-rest", Root: "apps/api"},
			"application":  {Adapter: "nestjs-application", Root: "apps/api"},
			"persistence":  {Adapter: "prisma-postgres", Root: "apps/api"},
		},
		Links: []parser.AdapterSetLink{
			{From: "presentation", Relation: "calls", To: "transport"},
			{From: "transport", Relation: "dispatches", To: "application"},
			{From: "application", Relation: "persists", To: "persistence"},
		},
	}
}

func TestValidateAdapterSetLinks_PresentationToPersistenceViolated(t *testing.T) {
	as := fullStackAdapterSet()
	edges := []CrossKindEdge{{From: "presentation", To: "persistence", Ref: "rule-x"}}
	outcomes := ValidateAdapterSetLinks(ModeBuild, as, edges)
	if !findCode(outcomes, "adapter-set-link-violated") {
		t.Errorf("missing adapter-set-link-violated; got %+v", outcomes)
	}
}

func TestValidateAdapterSetLinks_AuthorizedEdgePasses(t *testing.T) {
	as := fullStackAdapterSet()
	edges := []CrossKindEdge{{From: "presentation", To: "transport"}}
	outcomes := ValidateAdapterSetLinks(ModeBuild, as, edges)
	if findCode(outcomes, "adapter-set-link-violated") {
		t.Errorf("authorized edge incorrectly violated; got %+v", outcomes)
	}
}

func TestValidateAdapterSetLinks_OmittedLinksRejectsEveryEdge(t *testing.T) {
	as := fullStackAdapterSet()
	as.Links = nil // omit links entirely
	edges := []CrossKindEdge{{From: "presentation", To: "transport"}}
	outcomes := ValidateAdapterSetLinks(ModeBuild, as, edges)
	if !findCode(outcomes, "adapter-set-link-missing") {
		t.Errorf("missing adapter-set-link-missing; got %+v", outcomes)
	}
}

func TestValidateAdapterSetLinks_UnfilledSlot(t *testing.T) {
	as := fullStackAdapterSet()
	as.Links = append(as.Links, parser.AdapterSetLink{From: "transport", Relation: "calls", To: "background"})
	outcomes := ValidateAdapterSetLinks(ModeBuild, as, nil)
	if !findCode(outcomes, "adapter-set-link-unfilled-slot") {
		t.Errorf("missing adapter-set-link-unfilled-slot; got %+v", outcomes)
	}
}

func TestValidateAdapterSetLinks_NilAdapterSetFailsEveryEdge(t *testing.T) {
	edges := []CrossKindEdge{{From: "presentation", To: "transport"}}
	outcomes := ValidateAdapterSetLinks(ModeBuild, nil, edges)
	if !findCode(outcomes, "adapter-set-link-unfilled-slot") {
		t.Errorf("missing adapter-set-link-unfilled-slot; got %+v", outcomes)
	}
}
