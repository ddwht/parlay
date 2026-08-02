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
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/parser"
)

// edgeKindRank orders adapter kinds from UI down to storage. A cross-kind edge
// runs between two consecutive layers that both touch the same operation.
var edgeKindRank = map[string]int{
	"presentation": 0,
	"transport":    1,
	"application":  2,
	"persistence":  3,
}

// crossKindEdgeShape projects the parts of a v2 buildfile the edge extractor
// reads: each backend target's projected operation refs, and the presentation
// components' actions (an action that calls an operation ties the UI layer to
// whichever backend layer projects that operation).
type crossKindEdgeShape struct {
	Targets map[string]struct {
		Operations map[string]yaml.Node `yaml:"operations"`
		Components  map[string]struct {
			Actions []struct {
				Target string `yaml:"target"`
			} `yaml:"actions"`
		} `yaml:"components"`
	} `yaml:"targets"`
}

// ExtractCrossKindEdges projects a v2 buildfile's cross-kind edges. For each
// operation, the layers that touch it — presentation via a component action
// that calls it, a backend kind via its targets.<kind>.operations projection —
// are ordered UI→storage by edgeKindRank and an edge is emitted between each
// consecutive pair. So an operation the UI calls, the application orchestrates,
// and persistence stores yields presentation→application and
// application→persistence. Deterministic and side-effect free; the result
// feeds ValidateAdapterSetLinks.
//
// This is the edge producer whose absence kept ValidateAdapterSetLinks
// unreachable: the type existed but nothing built a CrossKindEdge from a real
// buildfile.
func ExtractCrossKindEdges(content []byte) []CrossKindEdge {
	var bf crossKindEdgeShape
	if err := yaml.Unmarshal(content, &bf); err != nil {
		return nil
	}

	touched := map[string]map[string]bool{}
	touch := func(op, kind string) {
		if touched[op] == nil {
			touched[op] = map[string]bool{}
		}
		touched[op][kind] = true
	}
	for kind, t := range bf.Targets {
		if kind == "presentation" {
			for _, comp := range t.Components {
				for _, a := range comp.Actions {
					if isOperationRef(a.Target) {
						touch(a.Target, "presentation")
					}
				}
			}
			continue
		}
		for opRef := range t.Operations {
			if isOperationRef(opRef) {
				touch(opRef, kind)
			}
		}
	}

	var ops []string
	for op := range touched {
		ops = append(ops, op)
	}
	sort.Strings(ops)

	seen := map[string]bool{}
	var edges []CrossKindEdge
	for _, op := range ops {
		var kinds []string
		for k := range touched[op] {
			kinds = append(kinds, k)
		}
		sort.Slice(kinds, func(i, j int) bool { return edgeKindRank[kinds[i]] < edgeKindRank[kinds[j]] })
		for i := 0; i+1 < len(kinds); i++ {
			from, to := kinds[i], kinds[i+1]
			key := from + "->" + to
			if seen[key] {
				continue
			}
			seen[key] = true
			edges = append(edges, CrossKindEdge{From: from, To: to, Ref: op})
		}
	}
	return edges
}

func isOperationRef(s string) bool {
	return strings.HasPrefix(s, "@") && strings.Contains(s, "/operation:")
}

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
