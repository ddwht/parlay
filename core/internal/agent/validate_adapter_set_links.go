// parlay-feature: parlay-tool/multi-adapter
// parlay-component: adapter-set-link-enforcement
//
// Validates that every cross-kind edge recorded in the buildfile's targets:
// block is authorized by .parlay/adapter-set.yaml's links: block. Authoring
// mode emits warnings; build mode fails. An edge to an unfilled slot fails
// earlier as an unresolved target.

package agent

import (
	"fmt"

	"github.com/ddwht/parlay/core/internal/parser"
)

// linkRelations is the closed set of cross-kind relations.
var linkRelations = map[string]bool{
	"calls":      true,
	"dispatches": true,
	"persists":   true,
}

// CrossKindEdge is the projection of a buildfile edge that crosses kinds.
// The edge is built from a buildfile's targets: block at validation time.
type CrossKindEdge struct {
	From string // adapter-kind slot (e.g., "presentation")
	To   string // adapter-kind slot (e.g., "transport")
	Ref  string // optional: location/operation reference for error context
}

// ValidateAdapterSetLinks walks the supplied edges and rejects any edge
// whose (from, to) pair is not authorized by adapterSet.Links. Returns one
// outcome per violation.
func ValidateAdapterSetLinks(mode ValidationMode, adapterSet *parser.AdapterSet, edges []CrossKindEdge) []ValidationOutcome {
	var outcomes []ValidationOutcome

	if adapterSet == nil {
		// No adapter-set: every cross-kind edge fails as unfilled.
		for _, e := range edges {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-set-link-unfilled-slot",
				fmt.Sprintf("edge %s -> %s targets a slot the adapter-set does not declare%s", e.From, e.To, formatRef(e.Ref))))
		}
		return outcomes
	}

	authorized := authorizedEdgeSet(adapterSet.Links)

	// Validate each declared link.relation is in the closed set.
	for _, link := range adapterSet.Links {
		if !linkRelations[link.Relation] {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-set-link-unknown-relation",
				fmt.Sprintf("links: entry declares relation %q outside the closed set {calls, dispatches, persists}", link.Relation)))
		}
		if _, ok := adapterSet.Targets[link.From]; !ok {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-set-link-unfilled-slot",
				fmt.Sprintf("links: entry from %q targets a slot the adapter-set does not declare", link.From)))
		}
		if _, ok := adapterSet.Targets[link.To]; !ok {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-set-link-unfilled-slot",
				fmt.Sprintf("links: entry to %q targets a slot the adapter-set does not declare", link.To)))
		}
	}

	if len(adapterSet.Links) == 0 && len(edges) > 0 {
		// links: omitted; every edge fails.
		for _, e := range edges {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-set-link-missing",
				fmt.Sprintf("cross-kind edge %s -> %s but adapter-set has no links: block%s", e.From, e.To, formatRef(e.Ref))))
		}
		return outcomes
	}

	for _, e := range edges {
		key := e.From + "->" + e.To
		if !authorized[key] {
			outcomes = append(outcomes, NewOutcome(mode, "adapter-set-link-violated",
				fmt.Sprintf("cross-kind edge %s -> %s is not authorized by links:%s", e.From, e.To, formatRef(e.Ref))))
		}
	}

	return outcomes
}

func authorizedEdgeSet(links []parser.AdapterSetLink) map[string]bool {
	out := make(map[string]bool, len(links))
	for _, l := range links {
		out[l.From+"->"+l.To] = true
	}
	return out
}

func formatRef(ref string) string {
	if ref == "" {
		return ""
	}
	return " (" + ref + ")"
}
