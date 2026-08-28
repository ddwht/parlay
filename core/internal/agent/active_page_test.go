package agent

import (
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

// frag builds a fragment with the fields composition actually reads.
func frag(feature, name, page, region string, order int) parser.Fragment {
	return parser.Fragment{Feature: feature, Name: name, Page: page, Region: region, Order: order}
}

func refsOf(fs []parser.Fragment) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, FragmentRef(f))
	}
	return out
}

func hasActiveCode(errs []ValidationError, code string) bool {
	for _, e := range errs {
		if e.Code == code {
			return true
		}
	}
	return false
}

// The case the whole resolver exists for, and the one a feature-local fold
// cannot see: feature B retires a fragment declared only in feature A, with no
// page manifest anywhere. A resolver that started from A's own surface would
// keep A's fragment and demand a mount assertion for retired output.
func TestCrossFeatureSupersederRemovesOtherFeaturesFragment(t *testing.T) {
	a := frag("catalog", "Viewport", "Customers", "main", 1)
	b := frag("render", "Viewport V2", "Customers", "main", 1)
	b.Supersedes = "@catalog/viewport"

	view := ResolveActiveView([]parser.Fragment{a, b})

	if len(view.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", view.Errors)
	}
	got := refsOf(view.Active)
	if len(got) != 1 || got[0] != "@render/viewport-v2" {
		t.Fatalf("active = %v, want only @render/viewport-v2", got)
	}
	if by := view.Retired["@catalog/viewport"]; by != "@render/viewport-v2" {
		t.Fatalf("retired-by = %q, want @render/viewport-v2", by)
	}

	// The owning feature must be able to learn its contribution left the page
	// even though its own surface is untouched — the fact its source
	// signature cannot carry.
	if own := view.RetiredOwnedBy("catalog"); len(own) != 1 {
		t.Fatalf("RetiredOwnedBy(catalog) = %v, want one entry", own)
	}
	if own := view.RetiredOwnedBy("render"); len(own) != 0 {
		t.Fatalf("RetiredOwnedBy(render) = %v, want none", own)
	}
}

// A -> B -> C: both B and C retire, A is the single head.
func TestTransitiveChainResolvesToOneHead(t *testing.T) {
	c := frag("f1", "C", "P", "main", 1)
	b := frag("f2", "B", "P", "main", 1)
	b.Supersedes = "@f1/c"
	a := frag("f3", "A", "P", "main", 1)
	a.Supersedes = "@f2/b"

	view := ResolveActiveView([]parser.Fragment{c, b, a})

	if len(view.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", view.Errors)
	}
	got := refsOf(view.Active)
	if len(got) != 1 || got[0] != "@f3/a" {
		t.Fatalf("active = %v, want only @f3/a", got)
	}
}

// Two fragments naming the same target fork the composition. The target must
// stay ACTIVE: retiring it on the strength of a contradiction would delete
// content no one chose to remove.
func TestForkIsRefusedAndTargetStaysActive(t *testing.T) {
	target := frag("f1", "Old", "P", "main", 1)
	h1 := frag("f2", "New A", "P", "main", 1)
	h1.Supersedes = "@f1/old"
	h2 := frag("f3", "New B", "P", "main", 1)
	h2.Supersedes = "@f1/old"

	view := ResolveActiveView([]parser.Fragment{target, h1, h2})

	if !hasActiveCode(view.Errors, "surface-supersedes-conflict") && len(view.Errors) == 0 {
		t.Fatalf("fork produced no diagnostic")
	}
	if len(view.Active) != 3 {
		t.Fatalf("active = %v, want all three left standing", refsOf(view.Active))
	}
	if len(view.Retired) != 0 {
		t.Fatalf("retired = %v, want nothing retired on a fork", view.Retired)
	}
}

func TestCycleIsRefusedAndNothingRetires(t *testing.T) {
	a := frag("f1", "A", "P", "main", 1)
	a.Supersedes = "@f2/b"
	b := frag("f2", "B", "P", "main", 1)
	b.Supersedes = "@f1/a"

	view := ResolveActiveView([]parser.Fragment{a, b})

	if !hasActiveCode(view.Errors, "surface-supersedes-cycle") {
		t.Fatalf("no cycle diagnostic; errors = %+v", view.Errors)
	}
	if len(view.Active) != 2 {
		t.Fatalf("active = %v, want both left standing", refsOf(view.Active))
	}
	// One cycle is one defect, however many refs you start the walk from.
	cycles := 0
	for _, e := range view.Errors {
		if e.Code == "surface-supersedes-cycle" {
			cycles++
		}
	}
	if cycles != 1 {
		t.Fatalf("cycle reported %d times, want 1", cycles)
	}
}

func TestMissingSupersedesTargetIsRefused(t *testing.T) {
	a := frag("f1", "A", "P", "main", 1)
	a.Supersedes = "@ghost/nothing"

	view := ResolveActiveView([]parser.Fragment{a})

	if !hasActiveCode(view.Errors, "surface-supersedes-target-unknown") {
		t.Fatalf("no target-unknown diagnostic; errors = %+v", view.Errors)
	}
	if len(view.Active) != 1 {
		t.Fatalf("active = %v, want the fragment left standing", refsOf(view.Active))
	}
}

// supersedes: replaces a fragment in the SAME (page, region). Honouring it
// across slots would retire a fragment on a page the superseder never reaches.
func TestSupersedesAcrossDifferentSlotsIsRefused(t *testing.T) {
	target := frag("f1", "Old", "PageA", "main", 1)
	head := frag("f2", "New", "PageB", "main", 1)
	head.Supersedes = "@f1/old"

	view := ResolveActiveView([]parser.Fragment{target, head})

	if !hasActiveCode(view.Errors, "surface-supersedes-slot-mismatch") {
		t.Fatalf("no slot-mismatch diagnostic; errors = %+v", view.Errors)
	}
	if len(view.Retired) != 0 {
		t.Fatalf("retired = %v, want nothing retired across slots", view.Retired)
	}
}

// An empty region and an explicit "main" are the same slot. If these disagreed,
// a legitimate supersession would be refused as a slot mismatch.
func TestEmptyRegionEqualsMain(t *testing.T) {
	target := frag("f1", "Old", "P", "", 1)
	head := frag("f2", "New", "P", "main", 1)
	head.Supersedes = "@f1/old"

	view := ResolveActiveView([]parser.Fragment{target, head})

	if len(view.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", view.Errors)
	}
	if _, retired := view.Retired["@f1/old"]; !retired {
		t.Fatalf("empty region did not match main; retired = %v", view.Retired)
	}
}

// ActiveOnPage then OwnedBy is the required order. The retired fragment must
// not reappear when the view is narrowed to the feature that owned it.
func TestOwnedByAppliesAfterResolution(t *testing.T) {
	a := frag("catalog", "Table", "Customers", "main", 1)
	old := frag("catalog", "Legacy Table", "Customers", "main", 2)
	head := frag("render", "New Table", "Customers", "main", 2)
	head.Supersedes = "@catalog/legacy-table"

	view := ResolveActiveView([]parser.Fragment{a, old, head})
	own := OwnedBy(view.ActiveOnPage("Customers"), "catalog")

	got := refsOf(own)
	if len(got) != 1 || got[0] != "@catalog/table" {
		t.Fatalf("catalog's active contribution = %v, want only @catalog/table", got)
	}
}

// The resolver feeds a composition signature, so an unchanged tree must
// produce a byte-identical answer however the input happens to be ordered.
func TestResolutionIsOrderIndependent(t *testing.T) {
	a := frag("f1", "A", "P", "main", 1)
	b := frag("f2", "B", "P", "main", 1)
	b.Supersedes = "@f1/a"
	c := frag("f3", "C", "P", "main", 2)

	first := ResolveActiveView([]parser.Fragment{a, b, c})
	second := ResolveActiveView([]parser.Fragment{c, b, a})

	if len(first.Retired) != len(second.Retired) {
		t.Fatalf("retired sets differ: %v vs %v", first.Retired, second.Retired)
	}
	for k, v := range first.Retired {
		if second.Retired[k] != v {
			t.Fatalf("retired[%s] = %q vs %q", k, v, second.Retired[k])
		}
	}
}
