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
var parlayVerbPattern = regexp.MustCompile(`\bparlay ([a-z][a-z0-9-]*)(?: ([a-z][a-z0-9-]*))?\b`)

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
	registered, groups := registeredVerbs()
	if len(registered) == 0 {
		t.Fatal("no commands registered on rootCmd — init() may not have run")
	}

	skills, err := embedded.ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}

	check := func(kind, name, body string) {
		for _, m := range parlayVerbPattern.FindAllStringSubmatch(body, -1) {
			verb, sub := m[1], m[2]

			// A group name followed by a subcommand — `parlay internal
			// check-buildfile` — must resolve at both levels. Checking only
			// the first token would let every reference under a group go
			// unvalidated, which is exactly the coverage the namespacing
			// change would otherwise have quietly removed.
			if subs, ok := groups[verb]; ok && sub != "" {
				if !subs[sub] {
					t.Errorf("%s %s references `parlay %s %s`, but %q has no subcommand %q",
						kind, name, verb, sub, verb, sub)
				}
				continue
			}

			if registered[verb] || parlayVerbLintAllowlist[verb] {
				continue
			}
			t.Errorf("%s %s references `parlay %s`, which is not a registered CLI command and not in parlayVerbLintAllowlist — "+
				"fix the reference if the command was renamed/removed, or add %q to the allowlist if this is legitimate prose",
				kind, name, verb, verb)
		}
	}

	for _, s := range skills {
		check("skill", s.Name, string(s.Content))
	}

	// Agent briefs name CLI commands as freely as skills do, and drift
	// there is harder to notice: nobody opens .claude/agents/ to read.
	agents, err := embedded.ReadAllAgents()
	if err != nil {
		t.Fatalf("ReadAllAgents: %v", err)
	}
	for _, a := range agents {
		check("agent", a.Name, string(a.Content))
	}
}

// registeredVerbs returns the top-level command names and, for any command
// that has subcommands, the set of names beneath it.
func registeredVerbs() (top map[string]bool, groups map[string]map[string]bool) {
	top = map[string]bool{}
	groups = map[string]map[string]bool{}
	for _, c := range rootCmd.Commands() {
		fields := strings.Fields(c.Use)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		top[name] = true
		if subs := c.Commands(); len(subs) > 0 {
			set := map[string]bool{}
			for _, sc := range subs {
				sf := strings.Fields(sc.Use)
				if len(sf) > 0 {
					set[sf[0]] = true
				}
			}
			groups[name] = set
		}
	}
	return top, groups
}
