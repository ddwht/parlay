// parlay-feature: parlay-tool/page-manifest
// parlay-artifact: test
package parser

import "testing"

// TestParsePageReadsTheDocumentedMarkdownForm is the regression for a writer
// and a reader that had never agreed on a format.
//
// page.schema.md's Template section documents the markdown form — "# <Page
// Name>", **Owner**/**Status** lines, "## <Region Name>" headings with
// numbered @feature/fragment lists — and that is exactly what lock-page
// writes. ParsePageFile only read YAML frontmatter and the ## Layout block,
// so a conforming manifest decoded to a zero-valued Page with no name, no
// status and NO REGIONS. That is why `validate --type page` returned OK on a
// manifest naming a page, a region and a fragment that all did not exist:
// there was nothing decoded to check.
func TestParsePageReadsTheDocumentedMarkdownForm(t *testing.T) {
	manifest := `# Dashboard

> Everything an employee sees on sign-in

**Owner**: platform-team
**Status**: locked

## main

1. @dashboard/status-tiles
2. @dashboard/recent-reports

## sidebar

1. @dashboard/queue-length
`
	p, err := parsePageContent("dashboard.page.md", []byte(manifest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Name != "Dashboard" {
		t.Errorf("Name = %q, want Dashboard", p.Name)
	}
	if p.Owner != "platform-team" {
		t.Errorf("Owner = %q, want platform-team", p.Owner)
	}
	if p.Status != "locked" {
		t.Errorf("Status = %q, want locked", p.Status)
	}
	if p.Description != "Everything an employee sees on sign-in" {
		t.Errorf("Description = %q", p.Description)
	}
	if len(p.Regions) != 2 {
		t.Fatalf("want 2 regions, got %d: %#v", len(p.Regions), p.Regions)
	}
	if p.Regions[0].Name != "main" || len(p.Regions[0].Components) != 2 {
		t.Errorf("region 0 = %#v", p.Regions[0])
	}
	// Order is the manifest's whole purpose — it overrides surface order — so
	// list position is meaningful.
	if p.Regions[0].Components[0] != "@dashboard/status-tiles" {
		t.Errorf("first fragment = %q, want @dashboard/status-tiles", p.Regions[0].Components[0])
	}
	if p.Regions[1].Name != "sidebar" || len(p.Regions[1].Components) != 1 {
		t.Errorf("region 1 = %#v", p.Regions[1])
	}
}

// The ## Layout section is a fenced YAML block owned by
// extractLayoutSection. Treating it as a region would invent one named
// "Layout" carrying no fragments.
func TestLayoutSectionIsNotParsedAsARegion(t *testing.T) {
	manifest := "# P\n\n## main\n\n1. @f/a\n\n## Layout\n\n```yaml\ncomponentVocabulary: clarity@17\nschema_version: 1\nnodes: []\n```\n"
	p, err := parsePageContent("p.page.md", []byte(manifest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, r := range p.Regions {
		if r.Name == "Layout" {
			t.Fatalf("the Layout section was parsed as a region: %#v", p.Regions)
		}
	}
	if len(p.Regions) != 1 || p.Regions[0].Name != "main" {
		t.Errorf("regions = %#v", p.Regions)
	}
}

// A page authored in the YAML frontmatter form keeps working; the markdown
// scan only fills fields the frontmatter left empty.
func TestFrontmatterStillWins(t *testing.T) {
	manifest := "---\nname: FromFrontmatter\nstatus: reviewed\n---\n\n# FromHeading\n\n## main\n\n1. @f/a\n"
	p, err := parsePageContent("p.page.md", []byte(manifest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Name != "FromFrontmatter" {
		t.Errorf("Name = %q, want the frontmatter value to win", p.Name)
	}
	if p.Status != "reviewed" {
		t.Errorf("Status = %q", p.Status)
	}
	if len(p.Regions) != 1 {
		t.Errorf("markdown regions should still be read: %#v", p.Regions)
	}
}
