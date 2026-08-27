// parlay-feature: parlay-tool/criterion-authority
// parlay-component: transitional-severity-graduation
// parlay-artifact: test

package agent

import (
	"regexp"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/embedded"
)

// The graduation is worthless if the producers emit a lower version than the
// threshold: every fresh artifact then reads as legacy and the checks stay
// warnings forever. That is not hypothetical — it was the state when the
// thresholds landed, and the validator and the prose had no way to notice they
// disagreed.
//
// So the instruction that WRITES each artifact is pinned to the constant that
// grades it.
func TestConformance_ProducersEmitTheGraduationVersion(t *testing.T) {
	cases := []struct {
		skill    string
		artifact string
		pattern  *regexp.Regexp
		want     int
	}{
		{
			skill: "build-feature", artifact: "testcases.yaml",
			pattern: regexp.MustCompile(`Set ` + "`" + `schema_version: (\d+)` + "`"),
			want:    TestcasesGraduationVersion,
		},
		{
			skill: "create-artifacts", artifact: "capabilities.yaml",
			pattern: regexp.MustCompile(`Set ` + "`" + `schema_version: (\d+)` + "`"),
			want:    CapabilitiesGraduationVersion,
		},
	}

	for _, tc := range cases {
		t.Run(tc.skill, func(t *testing.T) {
			body := skillBody(t, tc.skill)
			m := tc.pattern.FindStringSubmatch(body)
			if m == nil {
				t.Fatalf("%s does not tell the agent which schema_version to write for %s — "+
					"an artifact with no declared version reads as legacy and the checks that grade it stay warnings", tc.skill, tc.artifact)
			}
			if got := m[1]; got != itoa(tc.want) {
				t.Errorf("%s writes schema_version: %s for %s, but it is graded at %d — "+
					"a fresh artifact would read as legacy and never graduate", tc.skill, got, tc.artifact, tc.want)
			}
		})
	}
}

// The schema documenting each artifact must agree too: it is what a person
// reads when deciding what to write by hand.
func TestConformance_SchemasDocumentTheGraduationVersion(t *testing.T) {
	for _, tc := range []struct {
		schema string
		want   int
	}{
		{"testcases", TestcasesGraduationVersion},
		{"capabilities", CapabilitiesGraduationVersion},
	} {
		t.Run(tc.schema, func(t *testing.T) {
			body, err := embedded.ReadSchema(tc.schema + ".schema.md")
			if err != nil {
				t.Fatalf("read %s schema: %v", tc.schema, err)
			}
			want := "schema_version: " + itoa(tc.want)
			if !strings.Contains(string(body), want) {
				t.Errorf("%s.schema.md never shows %q — the document a person writes from should not describe a shape the validator grades as legacy", tc.schema, want)
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

func skillBody(t *testing.T, name string) string {
	t.Helper()
	all, err := embedded.ReadAllSkills()
	if err != nil {
		t.Fatalf("read skills: %v", err)
	}
	for _, s := range all {
		if s.Name == name {
			return string(s.Content)
		}
	}
	t.Fatalf("no skill named %q", name)
	return ""
}
