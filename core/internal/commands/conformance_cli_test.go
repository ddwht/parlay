package commands

import (
	"bytes"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ddwht/parlay/core/internal/embedded"
)

// This file is the WP5 conformance ratchet: three source-and-skill tests that
// turn a class of toolchain self-disagreement into a failure at the commit that
// introduces it, rather than a discovery an agent makes at runtime once per
// build phase.
//
//   - TestSkillsDoNotNameBareInternalSubcommands closes the gap the existing
//     verb lint leaves open: parlayVerbPattern only matches `parlay <verb>`
//     forms, so a bare `check-drift` — an internal subcommand named without its
//     `parlay internal` path — sails past it. That is exactly the shape L-series
//     reported: a skill names a command a shell cannot run.
//   - TestSkillFlagsResolveOnTheirCommand kills the phantom-flag class (the
//     removed `--json` on check-buildfile): every `--flag` a skill writes next
//     to a `parlay <verb> [<sub>]` invocation must actually parse on that
//     command.
//   - TestBuildfileReadersAreAllowlisted defends WP2's reconciliation: every
//     file that unmarshals a buildfile with its own struct must be recorded,
//     with a reason, so a new own-struct reader that bypasses the shared
//     v2-aware resolver cannot land silently.

// -----------------------------------------------------------------------------
// WP5.1 — bare internal-subcommand names in skill prose
// -----------------------------------------------------------------------------

// bareInternalIgnore lists internal-subcommand names that are also ordinary
// English words a skill may legitimately write in backticks without meaning the
// command — `diff` for a textual diff, `parse` for the act of parsing, `serve`
// for serving. Matching these as command references would flood the lint with
// false positives and train people to widen the ignore-list, which is how a
// real drift then slips in. Conservative matching over aggressive matching:
// the cost of missing a bare `diff`-the-command mention is one confused agent;
// the cost of a noisy lint is the whole check getting silenced.
var bareInternalIgnore = map[string]bool{
	"diff":  true,
	"parse": true,
	"serve": true,
}

// backtickSpan matches the text inside a single pair of backticks, with no
// backtick in between — the smallest unit prose uses to set a term as code.
var backtickSpan = regexp.MustCompile("`([^`]+)`")

// TestSkillsDoNotNameBareInternalSubcommands asserts no skill or agent brief
// presents an internal subcommand by its bare name. The commands under
// `parlay internal` emit JSON and are reached as `parlay internal <cmd>`;
// writing just `check-drift` names something a shell cannot run, and the
// existing `parlay <verb>` verb lint never sees it because there is no `parlay `
// prefix to anchor on.
func TestSkillsDoNotNameBareInternalSubcommands(t *testing.T) {
	_, groups := registeredVerbs()
	internalSubs, ok := groups["internal"]
	if !ok || len(internalSubs) == 0 {
		t.Fatal("the `internal` command group has no subcommands — registerInternalCommands() may not have run")
	}

	// "internal-only": a name that lives under `internal` and is not ALSO a
	// top-level command. A name registered in both places is reachable bare and
	// this lint has no business flagging it.
	top, _ := registeredVerbs()
	flagged := map[string]bool{}
	for name := range internalSubs {
		if top[name] {
			continue
		}
		if bareInternalIgnore[name] {
			continue
		}
		flagged[name] = true
	}
	if len(flagged) == 0 {
		t.Fatal("no internal-only subcommands survived the ignore-list — the lint would assert nothing")
	}

	skills, err := embedded.ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}
	agents, err := embedded.ReadAllAgents()
	if err != nil {
		t.Fatalf("ReadAllAgents: %v", err)
	}

	check := func(kind, name, body string) {
		for _, m := range backtickSpan.FindAllStringSubmatch(body, -1) {
			span := strings.TrimSpace(m[1])
			// Exact match only: a full `parlay internal check-drift` invocation
			// has other tokens inside the backticks and is correct as written,
			// so it must not trip the lint. Only a span that is nothing but the
			// bare subcommand name is the mistake.
			if flagged[span] {
				t.Errorf("%s %s writes `%s` bare, but %q is an internal subcommand — "+
					"write the full invocation `parlay internal %s` so the reference names something a shell can run",
					kind, name, span, span, span)
			}
		}
	}

	for _, s := range skills {
		check("skill", s.Name, string(s.Content))
	}
	for _, a := range agents {
		check("agent", a.Name, string(a.Content))
	}
}

// -----------------------------------------------------------------------------
// WP5.2 — flags a skill names must parse on the command it names them on
// -----------------------------------------------------------------------------

// flagToken matches a long flag as skill prose writes one: `--source-root`,
// `--type`. Short flags and value tokens are not flags and are not matched.
var flagToken = regexp.MustCompile(`--([a-z][a-z0-9-]*)`)

// TestSkillFlagsResolveOnTheirCommand asserts that every `--flag` a skill or
// agent writes immediately after a `parlay <verb> [<sub>]` invocation is a flag
// that command actually accepts. The removed phantom `--json` on check-buildfile
// (the command emits JSON unconditionally and never registered the flag) is the
// class this closes: a documented flag that silently does nothing, discovered
// only when an agent copies it and sees no effect.
func TestSkillFlagsResolveOnTheirCommand(t *testing.T) {
	if len(rootCmd.Commands()) == 0 {
		t.Fatal("no commands registered on rootCmd — init() may not have run")
	}

	skills, err := embedded.ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}
	agents, err := embedded.ReadAllAgents()
	if err != nil {
		t.Fatalf("ReadAllAgents: %v", err)
	}

	checked := 0
	check := func(kind, name, body string) {
		// Only flags written INSIDE the same backtick span as the invocation
		// are part of the command line. A flag in its own prose span — the
		// literal "there is no `--json` flag" that documents check-buildfile's
		// absence of one — is not a phantom; scanning the whole line would flag
		// exactly the sentence that got the phantom removed. The command line is
		// a single code span, so the span is the correct unit.
		for _, span := range backtickSpan.FindAllStringSubmatch(body, -1) {
			text := span[1]
			// Attribute flags to the nearest preceding parlay invocation within
			// the span: the region from one `parlay <verb>` match to the next
			// (or to the span's end) is that invocation's argument list.
			idx := parlayVerbPattern.FindAllStringSubmatchIndex(text, -1)
			for i, loc := range idx {
				verb := text[loc[2]:loc[3]]
				var sub string
				if loc[4] >= 0 {
					sub = text[loc[4]:loc[5]]
				}
				cmd := resolveCLICommand(verb, sub)
				if cmd == nil {
					// The verb lint owns unknown-command reporting; a flag on a
					// command that does not resolve is its problem, not ours.
					continue
				}
				regionEnd := len(text)
				if i+1 < len(idx) {
					regionEnd = idx[i+1][0]
				}
				region := text[loc[1]:regionEnd]
				for _, fm := range flagToken.FindAllStringSubmatch(region, -1) {
					flag := fm[1]
					checked++
					if !flagResolves(cmd, flag) {
						full := verb
						if sub != "" {
							full += " " + sub
						}
						t.Errorf("%s %s writes `--%s` on `parlay %s`, but that command registers no such flag — "+
							"remove the phantom flag or register it (this is the check-buildfile `--json` class)",
							kind, name, flag, full)
					}
				}
			}
		}
	}

	for _, s := range skills {
		check("skill", s.Name, string(s.Content))
	}
	for _, a := range agents {
		check("agent", a.Name, string(a.Content))
	}

	if checked == 0 {
		t.Fatal("no `parlay <verb> --flag` pairs found in any skill — the extractor has drifted from the prose")
	}
}

// resolveCLICommand walks the cobra tree to the command a `parlay <verb> [<sub>]`
// reference names. When sub is present but is not a registered subcommand (it is
// a positional argument, e.g. `parlay validate authored`), the flags belong to
// the verb command.
func resolveCLICommand(verb, sub string) *cobra.Command {
	child := func(parent *cobra.Command, want string) *cobra.Command {
		for _, c := range parent.Commands() {
			if f := strings.Fields(c.Use); len(f) > 0 && f[0] == want {
				return c
			}
		}
		return nil
	}
	top := child(rootCmd, verb)
	if top == nil {
		return nil
	}
	if sub == "" {
		return top
	}
	if leaf := child(top, sub); leaf != nil {
		return leaf
	}
	return top
}

// flagResolves reports whether name parses on cmd, including persistent flags
// inherited from any ancestor (--root, --verbose and --ambiguity-as-signal are
// registered on rootCmd and reachable from every subcommand). cmd.Flags() folds
// in the command's own persistent flags; walking the parent chain covers the
// inherited ones, which are not populated until Execute.
func flagResolves(cmd *cobra.Command, name string) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Flags().Lookup(name) != nil {
			return true
		}
		if c.PersistentFlags().Lookup(name) != nil {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// WP5.3 — every own-struct buildfile reader is allowlisted with a reason
// -----------------------------------------------------------------------------

// buildfileYAMLTag matches a struct field tagged with a top-level buildfile key.
// These are the keys a struct declares when it decodes a buildfile directly
// rather than routing through the shared v2-aware resolver.
var buildfileYAMLTag = regexp.MustCompile(`yaml:"(components|targets|presentation|plan|wiring|fixtures|models|operations|routes|source-signatures|adapter-set)"`)

// buildfileReaderAllowlist records every production file that unmarshals a
// buildfile with its own struct, and why that is allowed to exist alongside the
// canonical resolver. The reasons carry WP2.1's per-site audit forward: a reader
// is fine here only if it reads a section the v2 shape leaves in place, or it IS
// the resolver, or it is a shape probe that picks the v1/v2 path. A new reader of
// the RELOCATED sections (components:, routes:) that is not in this list — the
// BP1 regression — fails the test and has to justify itself or route through
// agent.ResolveBuildfileComponents / ResolveBuildfileRoutes instead.
//
// The matrix `excused` discipline (matrix_test.go): an entry that no longer
// reproduces — a file that stopped reading a buildfile, or was deleted — fails
// too, so the list cannot carry a stale excuse.
//
// The canonical resolver (agent/validate.go's deepBuildfile, which
// agent.ResolveBuildfileComponents/ResolveBuildfileRoutes wrap) is deliberately
// absent: it decodes buildfile CONTENT handed to it, never the buildfile.yaml
// path, so it falls outside this disk-reader enumeration by construction — and
// it is the target every entry below either routes through or is excused from.
// The detection anchors on the buildfile.yaml path literal precisely so the
// resolver is not something a new reader can be mistaken for.
var buildfileReaderAllowlist = map[string]string{
	"agent/composition_seed.go": "seedDesignationBuildfile reads only top-level fixtures:, which stays top-level in v2 (WP2.1 audit: v1-only-correct). No relocated section is touched.",

	"commands/check_composition.go": "fixtureBuildfile reads only top-level fixtures:, which stays top-level in v2 (WP2.1 audit: v1-only-correct).",

	"commands/emission_groups.go": "planBuildfile reads plan.creates/modifies, which stays top-level in v2 — per-target rows aggregate into it (WP2.1 audit: v1-only-correct).",

	"commands/composition_flow.go": "route ownership routes through agent.ResolveBuildfileRoutes after WP2.1; the remaining own struct reads only plan:, which stays top-level in v2.",

	"commands/check_write_set.go": "reads plan.creates/modifies only — the write-set allowlist the buildfile's plan declares, top-level and v2-safe.",

	"commands/scaffold_plan.go": "the plan scaffolder: it derives and writes plan rows, reading adapter-set:/targets: to project per-target creates. It authors the buildfile rather than validating fragments against a possibly-wrong section.",

	"commands/scaffold_operations.go": "multi-target scaffolder; reads adapter-set:/targets: to gate on the v2 shape before scaffolding operations. Shape probe, not a component reader.",

	"commands/scaffold_seed.go": "reads only top-level fixtures: to scaffold seed data; v2-safe for the same reason as composition_seed.",

	"commands/toolchain_plan.go": "reads adapter-set: to decide whether multi-target toolchain steps apply. Shape probe, not a fragment reader.",

	"commands/check_buildfile.go": "autoDiscoverAdapter is a thin adapter/adapter-set/targets shape probe used to pick the v1-vs-v2 validation path; the buildfile body itself is handed to the agent validator (agent/validate.go), the canonical reader above.",

	"commands/merged_routes.go": "the own struct here decodes the BLUEPRINT's navigation.routes for the shell/guard join, not a buildfile — the detector matches it only because the same file names buildfile.yaml. Every buildfile route it emits comes from agent.ResolveBuildfileRoutes, and the plan:-presence field it feeds comes from agent.BuildfileDeclaresPlan; this file has no buildfile struct of its own.",
}

// TestBuildfileReadersAreAllowlisted enumerates every production Go file that
// unmarshals a buildfile with its own struct and asserts each is recorded in
// buildfileReaderAllowlist. WP2 routed the broken readers through one v2-aware
// resolver; without this ratchet, a new own-struct reader of the relocated
// components:/routes: sections would reintroduce the BP1 self-disagreement
// (check-coverage blessing a multi-target feature that check-buildfile calls
// broken) with no test to catch it.
func TestBuildfileReadersAreAllowlisted(t *testing.T) {
	root := coreSourceRoot(t)

	// Comment-stripped, so a struct shown in a doc comment or a commented-out
	// reader does not register as a live one. Same reasoning as the embedded
	// conformance corpus: a `//` example is documentation, not a decode site.
	found := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "testdata" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src := stripGoComments(t, path)
		// Two signals, both required: the file reads a buildfile from disk
		// (the buildfile.yaml path literal) AND declares a struct with a
		// top-level buildfile key. Adapter docs, page manifests and the
		// adapter-set file reuse tags like `components:`/`targets:` but never
		// name buildfile.yaml, so requiring both keeps this scoped to true
		// buildfile readers without an ever-growing exclusion list.
		if !strings.Contains(src, "buildfile.yaml") {
			return nil
		}
		if !buildfileYAMLTag.MatchString(src) {
			return nil
		}
		found[relSitePath(root, path)] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(found) == 0 {
		t.Fatal("found zero own-struct buildfile readers — the detection heuristic has drifted from the source")
	}

	var unlisted []string
	for site := range found {
		if _, ok := buildfileReaderAllowlist[site]; !ok {
			unlisted = append(unlisted, site)
		}
	}
	sort.Strings(unlisted)
	if len(unlisted) > 0 {
		t.Errorf("%d file(s) unmarshal a buildfile with their own struct but are not in buildfileReaderAllowlist:\n  %s\n\n"+
			"Route the reader through agent.ResolveBuildfileComponents / ResolveBuildfileRoutes, or — if it reads only a "+
			"section the v2 shape leaves top-level (fixtures:, models:, plan:) — add it here with that reason. A new reader "+
			"of components:/routes: with its own struct is the BP1 self-disagreement WP2 removed.",
			len(unlisted), strings.Join(unlisted, "\n  "))
	}

	// The list cannot rot: an entry whose file no longer reads a buildfile (or
	// no longer exists) fails, so nobody carries a stale excuse forward.
	for site, why := range buildfileReaderAllowlist {
		if !found[site] {
			t.Errorf("buildfileReaderAllowlist lists %q but it no longer unmarshals a buildfile with its own struct — "+
				"remove the entry. It was recorded because: %s", site, why)
		}
	}
}

// coreSourceRoot returns the core/ directory, derived from this test file's
// compiled-in path so the walk does not depend on the working directory (every
// command fixture chdirs into a temp tree).
func coreSourceRoot(t *testing.T) string {
	t.Helper()
	// packageSourceDir is core/internal/commands; two parents up is core.
	return filepath.Dir(filepath.Dir(packageSourceDir(t)))
}

// relSitePath renders a walked path as "<pkg>/<file>.go" — the shape the
// allowlist keys use, stable regardless of where core/ sits on disk.
func relSitePath(root, path string) string {
	rel, err := filepath.Rel(filepath.Join(root, "internal"), path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}

// stripGoComments returns path's source with comments removed, via the parser
// and printer rather than a regex — a `//` inside a string literal must not
// delete real code. On a parse error it falls back to raw bytes (fails noisy,
// not silent).
func stripGoComments(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, 0)
	if err != nil {
		return string(data)
	}
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, file); err != nil {
		return string(data)
	}
	return buf.String()
}
