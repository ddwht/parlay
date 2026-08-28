package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/parser"
)

// Parity between the page the product SHOWS and the page the resolver says is
// active.
//
// This is the check whose absence let the defect live: `supersedes:` was
// parsed, stored and validated for forks, while view-page never referenced it
// and codegen never saw it. Every per-file test passed, because no test ever
// asked the two halves the same question. A parity assertion is the only shape
// that catches "the field is understood and never applied" — it compares a
// derived answer against the rendered one instead of checking each alone.
func TestViewPageAgreesWithResolverOnActiveSet(t *testing.T) {
	cfg, root := newPageTestContext(t)
	chdirForTest(t, root)

	// alpha's panel is retired by beta's, cross-feature, no manifest.
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: Panel\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 1\n")
	writeFeatureSurface(t, root, "beta",
		"fragments:\n  - name: Panel V2\n    shows: read-collection\n    source: \"@beta/intent\"\n    page: dashboard\n    region: main\n    order: 1\n    supersedes: \"@alpha/panel\"\n")
	writeFeatureSurface(t, root, "gamma",
		"fragments:\n  - name: Side\n    shows: read-collection\n    source: \"@gamma/intent\"\n    page: dashboard\n    region: sidebar\n    order: 1\n")

	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runViewPage(cmd, []string{"dashboard"}); err != nil {
		t.Fatalf("view-page: %v", err)
	}
	rendered := out.String()

	fragments, err := parser.ScanAllSurfaces(specDirOf(root))
	if err != nil {
		t.Fatal(err)
	}
	view := agent.ResolveActiveView(fragments)

	// Every ref the resolver calls active on this page must appear in the
	// rendered view.
	for _, f := range view.ActiveOnPage("dashboard") {
		ref := agent.FragmentRef(f)
		if !strings.Contains(rendered, ref) {
			t.Errorf("resolver says %s is active but view-page did not render it:\n%s", ref, rendered)
		}
	}

	// And every ref it retired must NOT appear as a rendered region entry.
	// Checked against the region block specifically, because the retirement
	// notice legitimately names the same ref further down.
	regionBlock := rendered
	if idx := strings.Index(rendered, "Retired by supersession"); idx >= 0 {
		regionBlock = rendered[:idx]
	}
	for ref := range view.Retired {
		if strings.Contains(regionBlock, ref) {
			t.Errorf("resolver retired %s but view-page still renders it in the page body:\n%s", ref, regionBlock)
		}
	}

	// The specific regression, asserted directly rather than only via parity:
	// before the resolver was wired in, both panels rendered and raced to
	// mount the same slot.
	if strings.Contains(regionBlock, "@alpha/panel") {
		t.Error("superseded @alpha/panel is still rendered — supersedes: is being parsed and not applied")
	}
	if !strings.Contains(rendered, "@beta/panel-v2") {
		t.Error("superseding @beta/panel-v2 is missing from the view")
	}
	if !strings.Contains(rendered, "Retired by supersession") {
		t.Error("view-page did not report the retirement it applied")
	}
}

// A refused composition must render as refused. Leaving a fork silent would
// make an uncomposed page look identical to one with nothing to compose,
// which is the ambiguity the whole change is removing.
func TestViewPageReportsCompositionRefusal(t *testing.T) {
	cfg, root := newPageTestContext(t)
	chdirForTest(t, root)

	writeFeatureSurface(t, root, "base",
		"fragments:\n  - name: Slot\n    shows: read-collection\n    source: \"@base/intent\"\n    page: dashboard\n    region: main\n    order: 1\n")
	writeFeatureSurface(t, root, "alpha",
		"fragments:\n  - name: A\n    shows: read-collection\n    source: \"@alpha/intent\"\n    page: dashboard\n    region: main\n    order: 1\n    supersedes: \"@base/slot\"\n")
	writeFeatureSurface(t, root, "beta",
		"fragments:\n  - name: B\n    shows: read-collection\n    source: \"@beta/intent\"\n    page: dashboard\n    region: main\n    order: 1\n    supersedes: \"@base/slot\"\n")

	cmd := testCommandWithContext(t, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runViewPage(cmd, []string{"dashboard"}); err != nil {
		t.Fatalf("view-page: %v", err)
	}
	rendered := out.String()

	if !strings.Contains(rendered, "Composition refusals") {
		t.Errorf("a two-headed fork rendered without saying supersession was not applied:\n%s", rendered)
	}
	if !strings.Contains(rendered, "surface-supersedes-conflict") {
		t.Errorf("refusal did not name the code:\n%s", rendered)
	}
	// The contested target stays on the page: refusing to compose must not
	// delete content.
	if !strings.Contains(rendered, "@base/slot") {
		t.Errorf("contested target vanished from a refused composition:\n%s", rendered)
	}
}
