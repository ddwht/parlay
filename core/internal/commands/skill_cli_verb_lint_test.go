package commands

import (
	"regexp"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/embedded"
)

// parlayVerbPattern matches "parlay <verb>" occurrences in skill prose —
// used to catch skills that reference a CLI command by a name that was
// renamed, removed, or never existed.
var parlayVerbPattern = regexp.MustCompile(`\bparlay ([a-z][a-z0-9-]*)\b`)

// parlayVerbLintAllowlist holds words that legitimately follow "parlay"
// in prose without naming a CLI command — e.g. "parlay commands are
// non-interactive", "the parlay design pipeline". Extend this list,
// don't loosen parlayVerbPattern, when a new false positive shows up: a
// loosened regex silently stops catching real drift.
var parlayVerbLintAllowlist = map[string]bool{
	"and":      true,
	"commands": true,
	"design":   true,
	"form":     true,
	"marker":   true,
	"project":  true, // "the parlay project root" — from the expanded active-root marker
}

// TestSkillsOnlyReferenceRegisteredCLIVerbs guards against skill prose
// drifting from the actual CLI surface: every `parlay <verb>` mention in
// an embedded skill must name a verb registered on the root cobra
// command. A skill that references a renamed or removed command would
// otherwise only surface as an "unknown command" shell error at agent
// run time, potentially releases after the drift was introduced.
func TestSkillsOnlyReferenceRegisteredCLIVerbs(t *testing.T) {
	registered := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		fields := strings.Fields(c.Use)
		if len(fields) == 0 {
			continue
		}
		registered[fields[0]] = true
	}
	if len(registered) == 0 {
		t.Fatal("no commands registered on rootCmd — init() may not have run")
	}

	skills, err := embedded.ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}

	for _, s := range skills {
		matches := parlayVerbPattern.FindAllStringSubmatch(string(s.Content), -1)
		for _, m := range matches {
			verb := m[1]
			if registered[verb] || parlayVerbLintAllowlist[verb] {
				continue
			}
			t.Errorf("skill %s references `parlay %s`, which is not a registered CLI command and not in parlayVerbLintAllowlist — "+
				"fix the reference if the command was renamed/removed, or add %q to the allowlist if this is legitimate prose",
				s.Name, verb, verb)
		}
	}
}
