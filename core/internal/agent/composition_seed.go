package agent

// Which of a feature's fixtures the prototype boots from.
//
// This is the one part of the composed seed that is not derivable, and it is
// deliberately a single declared bit rather than a heuristic over all the
// fixtures. A feature has several fixtures and they are SUPPOSED to disagree
// — an empty state and a populated one are different scenarios over the same
// ids, which is the documented intra-feature rule. Unioning all of them would
// manufacture exactly the contradictions the composed seed exists to detect.
//
// The rule lives here rather than beside the derivation because two callers
// need the same answer: `validate --project` reports when it cannot be
// determined, and `internal scaffold-seed` uses it to pick contributors. Two
// copies of a designation rule is how the two sides come to disagree about
// which fixture is the real one.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type seedDesignationBuildfile struct {
	Fixtures map[string]struct {
		Composes bool `yaml:"composes"`
	} `yaml:"fixtures"`
}

type seedDesignationTestcases struct {
	Suites []struct {
		Scope   string `yaml:"scope"`
		Fixture string `yaml:"fixture"`
	} `yaml:"suites"`
}

// ComposingFixture reports which fixture in buildDir the prototype boots from.
//
// It returns ("", "") when the feature declares no fixtures at all — there is
// nothing to designate and nothing to report. Otherwise it returns either the
// fixture name, or an empty name and a reason the designation is ambiguous,
// suitable as a composition-seed-ambiguous message body.
//
// An explicit `composes: true` wins. Failing that, the fixture named by the
// feature's `scope: route` suite is the deterministic answer: a route suite is
// by definition "everything this route renders", which is the same question
// the seed asks. Zero or several disagreeing route suites is a real design
// question and is handed back rather than guessed.
func ComposingFixture(buildDir string) (string, string) {
	data, err := os.ReadFile(filepath.Join(buildDir, "buildfile.yaml"))
	if err != nil {
		return "", ""
	}
	var bf seedDesignationBuildfile
	if err := yaml.Unmarshal(data, &bf); err != nil {
		return "", ""
	}
	if len(bf.Fixtures) == 0 {
		return "", ""
	}

	var declared []string
	for name, fx := range bf.Fixtures {
		if fx.Composes {
			declared = append(declared, name)
		}
	}
	sort.Strings(declared)
	switch {
	case len(declared) == 1:
		return declared[0], ""
	case len(declared) > 1:
		return "", fmt.Sprintf("%d fixtures are marked `composes: true` (%s); exactly one fixture boots the app",
			len(declared), strings.Join(declared, ", "))
	}

	tcData, err := os.ReadFile(filepath.Join(buildDir, "testcases.yaml"))
	if err != nil {
		return "", "no fixture is marked `composes: true` and there is no testcases.yaml to infer one from"
	}
	var tc seedDesignationTestcases
	if err := yaml.Unmarshal(tcData, &tc); err != nil {
		return "", fmt.Sprintf("no fixture is marked `composes: true` and testcases.yaml cannot be parsed to infer one: %v", err)
	}

	seen := map[string]bool{}
	var names []string
	for _, s := range tc.Suites {
		if s.Scope == "route" && s.Fixture != "" && !seen[s.Fixture] {
			seen[s.Fixture] = true
			names = append(names, s.Fixture)
		}
	}
	sort.Strings(names)

	switch len(names) {
	case 1:
		if _, ok := bf.Fixtures[names[0]]; !ok {
			return "", fmt.Sprintf("the route suite names fixture %q, which the buildfile does not declare", names[0])
		}
		return names[0], ""
	case 0:
		return "", "no fixture is marked `composes: true` and there is no `scope: route` suite to infer one from; " +
			"mark the fixture the prototype should boot with"
	default:
		return "", fmt.Sprintf("the `scope: route` suites name %d different fixtures (%s); "+
			"mark one `composes: true` to say which boots the prototype",
			len(names), strings.Join(names, ", "))
	}
}
