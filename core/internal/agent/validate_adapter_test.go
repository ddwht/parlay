// parlay-feature: parlay-tool/adapter-authoring
// parlay-artifact: test
//
// Rule-level coverage for the complete adapter validator: each rule in both
// polarities, and the kind-conditional split.

package agent

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// validPresentation builds a presentation adapter that passes every rule, so a
// case can introduce exactly one defect. The vocabulary is generated from the
// closed sets, so it cannot drift from what the validator requires.
func validPresentation(name string, extra string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "name: %s\nframework: %s\nversion: \"1\"\nkind: presentation\n", name, name)
	for _, sec := range []struct {
		key   string
		vocab map[string]bool
	}{{"shows", ClosedSetShows}, {"actions", ClosedSetActions}, {"flows", ClosedSetFlows}} {
		var terms []string
		for t := range sec.vocab {
			terms = append(terms, t)
		}
		sort.Strings(terms)
		fmt.Fprintf(&b, "%s:\n", sec.key)
		for _, t := range terms {
			fmt.Fprintf(&b, "  %s:\n    widget: W\n", t)
		}
	}
	b.WriteString("file-conventions:\n  source-root: \"src/\"\n  component-pattern: feature-modules\n  naming: PascalCase\n  entry-point: \"src/main.ts\"\n")
	b.WriteString(extra)
	return []byte(b.String())
}

func adapterCodesOf(out []ValidationOutcome) map[string]bool {
	m := map[string]bool{}
	for _, o := range out {
		m[o.Code] = true
	}
	return m
}

func TestValidateAdapter_ValidPresentationIsClean(t *testing.T) {
	out := ValidateAdapter(ModeBuild, "vue.adapter.yaml", validPresentation("vue", ""))
	if len(out) != 0 {
		t.Fatalf("a complete presentation adapter must validate clean; got %+v", out)
	}
}

func TestValidateAdapter_RulesFireIndividually(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		content []byte
		want    string
	}{
		{"missing name", "x.adapter.yaml", []byte("kind: presentation\n"), "adapter-name-missing"},
		{"slug mismatch", "other.adapter.yaml", validPresentation("vue", ""), "adapter-name-slug-mismatch"},
		{"unknown kind", "vue.adapter.yaml", []byte("name: vue\nkind: frontend\n"), "adapter-kind-unknown"},
		{"presentation declaring supports", "vue.adapter.yaml",
			validPresentation("vue", "supports:\n  steps: [create-one]\n"), "adapter-supports-shape-mismatch"},
		{"backend without supports", "db.adapter.yaml",
			[]byte("name: db\nkind: persistence\nfile-conventions:\n  source-root: s/\n  component-pattern: p\n  naming: kebab-case\n  entry-point: e\n"),
			"adapter-supports-shape-mismatch"},
		{"backend supports outside vocabulary", "db.adapter.yaml",
			[]byte("name: db\nkind: persistence\nsupports:\n  steps: [telepathy]\nfile-conventions:\n  source-root: s/\n  component-pattern: p\n  naming: kebab-case\n  entry-point: e\n"),
			"adapter-supports-unknown-term"},
		{"missing file-conventions", "vue.adapter.yaml", []byte("name: vue\nkind: transport\nsupports:\n  steps: []\n"), "adapter-file-conventions-missing"},
		{"bad naming", "vue.adapter.yaml",
			[]byte("name: vue\nkind: transport\nsupports:\n  steps: []\nfile-conventions:\n  source-root: s/\n  component-pattern: p\n  naming: SCREAMING\n  entry-point: e\n"),
			"adapter-naming-unknown"},
		{"unknown path placeholder", "vue.adapter.yaml",
			validPresentation("vue", "  paths:\n    component: \"f/{widget}.tsx\"\n"), "adapter-path-template-invalid"},
		{"empty packages entry", "vue.adapter.yaml",
			validPresentation("vue", "  packages:\n    utils: \"\"\n"), "adapter-packages-invalid"},
		{"bad design-system source", "vue.adapter.yaml",
			validPresentation("vue", "design-system:\n  color:\n    source: vibes\n"), "adapter-design-system-source-unknown"},
		{"figma design-system source retired", "vue.adapter.yaml",
			// `source: figma` was a VALID value until the design-spec surface
			// retirement (amendment design-spec-surface-retired, 2026-08-31);
			// this pins that the formerly-legal value is now rejected, not
			// merely that arbitrary junk is.
			validPresentation("vue", "design-system:\n  color:\n    source: figma\n"), "adapter-design-system-source-unknown"},
		{"mount strategy without placeholder", "vue.adapter.yaml",
			validPresentation("vue", "mount-strategies:\n  route:\n    detection: \"<Route\"\n    template: \"no placeholder\"\n    description: d\n"),
			"adapter-mount-strategy-invalid"},
		{"composition without wiring", "vue.adapter.yaml",
			validPresentation("vue", "compositions:\n  c:\n    trigger: t\n    description: d\n"), "adapter-composition-invalid"},
		{"convention without applies-to", "vue.adapter.yaml",
			validPresentation("vue", "conventions:\n  c:\n    rule: r\n"), "adapter-convention-invalid"},
		{"container without allowed-children", "vue.adapter.yaml",
			validPresentation("vue", "componentVocabulary:\n  name: vue@3\n  components:\n    - type: v.box\n      category: container\n"),
			"adapter-component-vocabulary-invalid"},
		{"enum without values", "vue.adapter.yaml",
			validPresentation("vue", "componentVocabulary:\n  name: vue@3\n  components:\n    - type: v.b\n      category: leaf\n      properties:\n        - name: size\n          type: enum\n"),
			"adapter-component-vocabulary-invalid"},
		{"tokens missing mode emit-form", "vue.adapter.yaml",
			validPresentation("vue", "tokens:\n  modes: [light, dark]\n  color:\n    - name: surface\n      emit-forms: [\"light:x\"]\n"),
			"adapter-tokens-invalid"},
		{"duplicate spacing order", "vue.adapter.yaml",
			validPresentation("vue", "tokens:\n  modes: [light]\n  spacing:\n    - {name: a, order: 1, emit-form: x}\n    - {name: b, order: 1, emit-form: y}\n"),
			"adapter-tokens-invalid"},
		{"toolchain missing stage", "vue.adapter.yaml",
			validPresentation("vue", "toolchain:\n  skills:\n    - id: rev\n      invoke: \"/rev\"\n      source: community\n      phase: [code]\n      authority: advisory\n      required: false\n      fallback: skip\n"),
			"toolchain-stage-unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := adapterCodesOf(ValidateAdapter(ModeBuild, tc.path, tc.content))
			if !got[tc.want] {
				var have []string
				for c := range got {
					have = append(have, c)
				}
				sort.Strings(have)
				t.Errorf("want %s; got %v", tc.want, have)
			}
		})
	}
}

// Missing a surface term is different from mapping it to not-applicable: the
// latter is a real answer, the former leaves codegen with no widget.
func TestValidateAdapter_VocabularyCoverage(t *testing.T) {
	full := validPresentation("vue", "")
	trimmed := []byte(strings.Replace(string(full), "  data-tree:\n    widget: W\n", "", 1))
	if !adapterCodesOf(ValidateAdapter(ModeBuild, "vue.adapter.yaml", trimmed))["adapter-vocabulary-incomplete"] {
		t.Error("a missing Show term must be reported")
	}

	notApplicable := []byte(strings.Replace(string(full), "  data-chart:\n    widget: W\n",
		"  data-chart:\n    widget: not-applicable\n    description: CLIs cannot draw\n", 1))
	if adapterCodesOf(ValidateAdapter(ModeBuild, "vue.adapter.yaml", notApplicable))["adapter-vocabulary-incomplete"] {
		t.Error("widget: not-applicable is a valid mapping, not a missing term")
	}

	unknown := append(full, []byte("\n")...)
	unknown = []byte(strings.Replace(string(unknown), "shows:\n", "shows:\n  hologram:\n    widget: W\n", 1))
	if !adapterCodesOf(ValidateAdapter(ModeBuild, "vue.adapter.yaml", unknown))["adapter-vocabulary-unknown-term"] {
		t.Error("a term outside the surface vocabulary must be reported")
	}
}

// Every finding is reported in one pass. The predecessor returned on the first
// error, so an author needed one round-trip per defect.
func TestValidateAdapter_ReportsEveryFindingAtOnce(t *testing.T) {
	broken := []byte("name: mystery\nkind: frontend\nfile-conventions:\n  naming: SCREAMING\n")
	got := ValidateAdapter(ModeBuild, "broken.adapter.yaml", broken)
	if len(got) < 4 {
		t.Fatalf("expected several findings in one pass, got %d: %+v", len(got), got)
	}
}
