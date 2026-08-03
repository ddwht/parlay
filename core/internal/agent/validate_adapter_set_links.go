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
// reads: each backend target's per-operation step OWNERSHIP (owns:), and the
// presentation components' actions (an action whose target is an operation ref
// is the UI calling that operation).
type crossKindEdgeShape struct {
	Targets map[string]struct {
		Operations map[string]struct {
			Owns []string `yaml:"owns"`
		} `yaml:"operations"`
		Components map[string]struct {
			Actions []struct {
				Target string `yaml:"target"`
			} `yaml:"actions"`
		} `yaml:"components"`
	} `yaml:"targets"`
}

// ExtractCrossKindEdges derives a v2 buildfile's cross-kind edges from step
// OWNERSHIP rather than co-projection. The model: every operation is entered at
// the orchestrator layer (the lowest-edgeKindRank backend kind present — the UI
// calls it), and the orchestrator delegates each step owned by a DIFFERENT
// layer to that layer across a cross-kind edge. So for an operation the UI
// calls, whose return-* steps the application owns and whose create-one step
// persistence owns, the edges are presentation→application (UI calls the op) and
// application→persistence (orchestrator delegates the owned write). The owned-
// step→owner relationship IS the cross-kind link — no separate heuristic.
//
// Deterministic and side-effect free; the result feeds ValidateAdapterSetLinks.
// Ownership comes from targets.<kind>.operations.<opRef>.owns, materialized by
// build-feature via `parlay internal scaffold-operations`.
func ExtractCrossKindEdges(content []byte) []CrossKindEdge {
	var bf crossKindEdgeShape
	if err := yaml.Unmarshal(content, &bf); err != nil {
		return nil
	}

	// Orchestrator = the entry backend layer = the lowest-rank backend kind
	// that carries operations (transport if present, else application, else
	// persistence).
	orchestrator := ""
	orchRank := int(^uint(0) >> 1)
	for kind, t := range bf.Targets {
		if kind == "presentation" || len(t.Operations) == 0 {
			continue
		}
		if r := edgeKindRank[kind]; r < orchRank {
			orchRank, orchestrator = r, kind
		}
	}

	// Per operation, the backend kinds that own at least one of its steps.
	opOwners := map[string]map[string]bool{}
	for kind, t := range bf.Targets {
		if kind == "presentation" {
			continue
		}
		for opRef, entry := range t.Operations {
			if !isOperationRef(opRef) || len(entry.Owns) == 0 {
				continue
			}
			if opOwners[opRef] == nil {
				opOwners[opRef] = map[string]bool{}
			}
			opOwners[opRef][kind] = true
		}
	}

	// Operations the UI invokes (presentation action → operation ref).
	uiCalls := map[string]bool{}
	if pres, ok := bf.Targets["presentation"]; ok {
		for _, comp := range pres.Components {
			for _, a := range comp.Actions {
				if isOperationRef(a.Target) {
					uiCalls[a.Target] = true
				}
			}
		}
	}

	// Deterministic op ordering: every op that has owners or is UI-called.
	opSet := map[string]bool{}
	for opRef := range opOwners {
		opSet[opRef] = true
	}
	for opRef := range uiCalls {
		opSet[opRef] = true
	}
	var ops []string
	for opRef := range opSet {
		ops = append(ops, opRef)
	}
	sort.Strings(ops)

	seen := map[string]bool{}
	var edges []CrossKindEdge
	addEdge := func(from, to, ref string) {
		if from == "" || to == "" || from == to {
			return
		}
		key := from + "->" + to
		if seen[key] {
			return
		}
		seen[key] = true
		edges = append(edges, CrossKindEdge{From: from, To: to, Ref: ref})
	}

	for _, opRef := range ops {
		if uiCalls[opRef] {
			addEdge("presentation", orchestrator, opRef)
		}
		var owners []string
		for owner := range opOwners[opRef] {
			owners = append(owners, owner)
		}
		sort.Slice(owners, func(i, j int) bool { return edgeKindRank[owners[i]] < edgeKindRank[owners[j]] })
		for _, owner := range owners {
			addEdge(orchestrator, owner, opRef)
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
