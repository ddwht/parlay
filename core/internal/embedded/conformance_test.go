package embedded

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// Conformance tests bind the schema documentation to the implementation.
//
// Every contradiction found in the ExpenseFlow regression run was the same
// shape: a schema documented a rule the code did not implement, or
// implemented the opposite, and nothing checked. Concretely —
// buildfile-models-deprecated is documented in four places and fires
// nowhere, while the validator *requires* the field the docs say is
// forbidden; and the v2 buildfile shape is called "primary" while the
// validator accepts only v1. Both were discovered only by an agent
// reverse-engineering the validator at runtime, once per build phase.
//
// These tests make the docs falsifiable.

// codeTableHeader matches the canonical error-code table header used across
// the schemas: "| Code | When it fires |".
var codeTableHeader = regexp.MustCompile(`^\|\s*Code\s*\|`)

// codeRow matches a table row whose first cell is a backticked code.
var codeRow = regexp.MustCompile("^\\|\\s*`([a-z][a-z0-9-]+)`\\s*\\|")

// knownUnimplementedCodes are documented error codes that no Go source
// currently emits. Each entry is a real gap, not an exemption to be added
// to casually: it means the schema promises a diagnostic the tool never
// produces. Shrink this list; do not grow it.
//
// A code listed here that HAS become implemented also fails the test, so
// the list cannot silently rot.
var knownUnimplementedCodes = map[string]string{
	// These belong to the multi-target / adapter-set machinery, which no
	// regression run has yet exercised end to end. Documented ahead of the
	// implementation.
	"adapter-set-duplicate-kind":  "adapter-set slot-topology validation is not implemented; no code path inspects duplicate kinds.",
	"blueprint-override-conflict": "the blueprint > adapter-set > adapter precedence resolver is not implemented, so no conflict can be detected.",
	"buildfile-routes-ambiguous":  "multi-target route disambiguation is not implemented; single-target projects cannot reach the ambiguous case.",
	"error-no-mapping":            "canonical operation-error mapping across adapter layers is not implemented.",

	// vocabulary.schema.md documents the adapter-vocabulary resolution
	// diagnostics; the resolver reports vocabulary problems through the
	// layout/validate-vocabulary path without these two specific codes.
	"vocabulary-missing-from-adapter": "adapter-vocabulary resolution does not emit this distinct code; the condition surfaces through validate-vocabulary's generic path.",
	"vocabulary-unknown-adapter":      "same resolver gap as vocabulary-missing-from-adapter.",
}

// buildfile-models-deprecated is no longer allowlisted and no longer needs
// to be: the deep validator the CLI actually uses now emits it (as a
// warning) and resolves entities from domain-model.yaml, so the documented
// shape validates. It was previously implemented only in
// ValidateBuildfileCanonical, which no command calls — two validators with
// opposite contracts and the CLI wired to the wrong one. That is the class
// TestConformance_CanonicalValidatorsAreReachable exists to catch.

// Scope note: this test asserts a documented code is *reachable in source*,
// not that it fires on the right input. A code emitted behind a condition
// that never matches still passes here — e.g. blueprint-strategy-unknown is
// present in the source yet does not fire for an out-of-vocabulary
// data.fetching value. Behavioural coverage belongs in the per-validator
// tests; this is the cheap structural floor beneath them.

func repoSchemaCodes(t *testing.T) map[string][]string {
	t.Helper()
	byFile := map[string][]string{}
	entries, err := fs.ReadDir(schemasFS, "schemas")
	if err != nil {
		t.Fatalf("read embedded schemas: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := schemasFS.ReadFile(filepath.Join("schemas", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		lines := strings.Split(string(data), "\n")
		inTable := false
		for _, line := range lines {
			switch {
			case codeTableHeader.MatchString(line):
				inTable = true
			case inTable && strings.HasPrefix(line, "|---"),
				inTable && strings.HasPrefix(line, "| ---"):
				// separator row, stay in table
			case inTable && strings.HasPrefix(line, "|"):
				if m := codeRow.FindStringSubmatch(line); m != nil {
					byFile[e.Name()] = append(byFile[e.Name()], m[1])
				}
			default:
				inTable = false
			}
		}
	}
	return byFile
}

// goSourceRoot returns the core/ directory, derived from this test file's
// own location so the test does not depend on the working directory.
func goSourceRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// .../core/internal/embedded/conformance_test.go -> .../core
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}

func goSourceCorpus(t *testing.T) string {
	t.Helper()
	var sb strings.Builder
	root := goSourceRoot(t)
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
		// Production sources only. Test files must be excluded or the
		// corpus matches this file's own allowlist strings and every
		// entry reports itself as "emitted". More importantly, a code
		// that appears only in a test is not emitted by the tool.
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		sb.Write(data)
		sb.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return sb.String()
}

// TestConformance_DocumentedErrorCodesAreEmitted asserts that every error
// code a schema documents appears as a literal in Go source. A documented
// code that no code path can produce is a promise the tool does not keep.
func TestConformance_DocumentedErrorCodesAreEmitted(t *testing.T) {
	byFile := repoSchemaCodes(t)
	if len(byFile) == 0 {
		t.Fatal("parsed zero error-code tables — the table-detection heuristic has drifted from the schema format")
	}
	corpus := goSourceCorpus(t)

	var missing []string
	seen := map[string]bool{}
	for file, codes := range byFile {
		for _, code := range codes {
			if seen[code] {
				continue
			}
			seen[code] = true
			emitted := strings.Contains(corpus, `"`+code+`"`)
			_, known := knownUnimplementedCodes[code]

			if !emitted && !known {
				missing = append(missing, code+"  (documented in "+file+")")
			}
			if emitted && known {
				t.Errorf("%s is in knownUnimplementedCodes but IS now emitted — remove it from the allowlist", code)
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("%d documented error code(s) are never emitted by any Go source.\n"+
			"Either implement the diagnostic or remove it from the schema — a documented code "+
			"that cannot fire is how agents end up reverse-engineering the validator at runtime:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

// TestConformance_AllowlistIsHonest keeps the allowlist meaningful: every
// entry must carry a reason, and must still be a code some schema documents.
func TestConformance_AllowlistIsHonest(t *testing.T) {
	byFile := repoSchemaCodes(t)
	documented := map[string]bool{}
	for _, codes := range byFile {
		for _, c := range codes {
			documented[c] = true
		}
	}
	for code, reason := range knownUnimplementedCodes {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%s has no reason recorded", code)
		}
		if !documented[code] {
			t.Errorf("%s is allowlisted but no schema documents it — stale entry", code)
		}
	}
}

// TestConformance_CanonicalValidatorsAreReachable catches dead validators:
// an exported Validate* entry point in the agent package that no command
// ever calls.
//
// This is the check that would have caught the models: contradiction. The
// documented deprecation IS implemented, in ValidateBuildfileCanonical —
// but nothing calls it, so the CLI runs a different validator that requires
// the field the canonical one forbids. An unreachable validator is worse
// than a missing one: it makes the contract look implemented while the tool
// enforces the opposite.
func TestConformance_CanonicalValidatorsAreReachable(t *testing.T) {
	root := goSourceRoot(t)
	agentDir := filepath.Join(root, "internal", "agent")
	commandsDir := filepath.Join(root, "internal", "commands")

	exported := regexp.MustCompile(`(?m)^func (Validate[A-Za-z0-9_]+)\(`)
	var entryPoints []string
	err := filepath.WalkDir(agentDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for _, m := range exported.FindAllStringSubmatch(string(data), -1) {
			entryPoints = append(entryPoints, m[1])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk agent dir: %v", err)
	}
	if len(entryPoints) == 0 {
		t.Fatal("found no exported Validate* entry points — the detection regex has drifted")
	}

	// Corpus of everything outside the agent package that could call them.
	var callers strings.Builder
	for _, dir := range []string{commandsDir} {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if data, readErr := os.ReadFile(path); readErr == nil {
				callers.Write(data)
				callers.WriteByte('\n')
			}
			return nil
		})
	}
	corpus := callers.String()

	// Validators reachable only via another agent-package function are
	// fine; this checks for ones no command reaches at all, directly or
	// through a wrapper in the same package.
	agentCorpus := goSourceCorpus(t)

	var unreachable []string
	for _, fn := range entryPoints {
		if knownUnreachableValidators[fn] != "" {
			continue
		}
		// Match the name as a word, not as a call: the CLI assigns
		// several validators as function values (validator =
		// agent.ValidateSurface), which a "fn(" probe misses entirely
		// and reports as unreachable.
		ref := regexp.MustCompile(`\b` + regexp.QuoteMeta(fn) + `\b`)
		if ref.MatchString(corpus) {
			continue
		}
		// Reached indirectly via another agent-package function? Any
		// reference beyond its own definition counts.
		if len(ref.FindAllString(agentCorpus, -1)) > 1 {
			continue
		}
		unreachable = append(unreachable, fn)
	}
	sort.Strings(unreachable)
	if len(unreachable) > 0 {
		t.Errorf("%d exported validator(s) are never called from any command:\n  %s\n\n"+
			"An unreachable validator makes a documented contract look implemented while the tool "+
			"enforces something else. Either wire it up or delete it.",
			len(unreachable), strings.Join(unreachable, "\n  "))
	}
}

// knownUnreachableValidators records validators deliberately not wired up
// yet, with the reason. Same discipline as knownUnimplementedCodes.
var knownUnreachableValidators = map[string]string{
	"ValidateBuildfileCanonical": "duplicate of the models:-deprecation logic now implemented in the deep validator (which the CLI actually uses). Kept for the multi-target canonical shape; delete it once v2 validation lands, rather than maintaining two implementations of the same rule.",
}

// phaseModules are the skills that run inside a parlay-loop subagent
// (parlay-designer / parlay-build / parlay-code). On the Claude Code
// adapter a subagent has no AskUserQuestion, so a prompt authored in one of
// these files is not merely unreliable — it is silently skipped, and the
// phase then answers its own question. That is how a regression run crossed
// three phase boundaries with zero confirmations and never once saw the
// artifact-set override menu.
//
// The contract: phase modules return a `parlay-decision` block; the driver
// prompts. This test holds the line, because the failure mode leaves no
// trace at runtime — a skipped confirmation looks exactly like a granted
// one.
var phaseModules = []string{
	"add-feature",
	"scaffold-dialogs",
	"create-artifacts",
	"build-feature",
	"generate-code",
}

// driverSkills own user interaction and may prompt freely: the loop driver
// itself, plus the two entry points a user invokes directly.
var driverSkills = []string{"loop", "doctor", "onboard"}

// askUserMention finds an instruction to call the interactive tool. The
// negation in generate-code ("never calls AskUserQuestion") is the one
// legitimate use inside a phase module, so mentions are only a failure when
// they read as a directive.
var askUserDirective = regexp.MustCompile(`(?i)(via|using|use|call|through)\s+AskUserQuestion|AskUserQuestion\s+to\s+(collect|ask|present|confirm)`)

func TestConformance_PhaseModulesDoNotPrompt(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}

	byName := map[string][]byte{}
	for _, s := range skills {
		byName[s.Name] = s.Content
	}

	for _, name := range phaseModules {
		body, ok := byName[name]
		if !ok {
			t.Errorf("phase module %q is not in the embedded skill set — "+
				"if it was renamed or retired, update phaseModules", name)
			continue
		}
		for i, line := range strings.Split(string(body), "\n") {
			if askUserDirective.MatchString(line) {
				t.Errorf("%s.skill.md:%d instructs a phase module to prompt:\n  %s\n"+
					"Phase modules run in subagents where AskUserQuestion does not exist; "+
					"the prompt is silently skipped and the phase answers itself. "+
					"Return a `parlay-decision` block instead.",
					name, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// TestConformance_DriverDocumentsTheDecisionProtocol pins the other half of
// the contract. Phase modules emitting decision requests is only safe if
// something is on the other end reading them.
func TestConformance_DriverDocumentsTheDecisionProtocol(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}

	var driver []byte
	for _, s := range skills {
		if s.Name == "loop" {
			driver = s.Content
		}
	}
	if driver == nil {
		t.Fatal("the loop driver skill is missing from the embedded set")
	}

	for _, required := range []string{
		"parlay-decision", // the block phases emit
		"AskUserQuestion", // what the driver does with it
	} {
		if !strings.Contains(string(driver), required) {
			t.Errorf("loop.skill.md does not mention %q — phase modules emit "+
				"decision requests with nothing on the receiving end", required)
		}
	}

	// Every phase module that says it returns a decision request must show
	// the shape, or the driver gets prose it cannot parse.
	for _, name := range phaseModules {
		for _, s := range skills {
			if s.Name != name {
				continue
			}
			body := string(s.Content)
			if strings.Contains(body, "decision request") && !strings.Contains(body, "parlay-decision") {
				t.Errorf("%s.skill.md refers to decision requests but never shows the "+
					"`parlay-decision` block — the driver has no format to match on", name)
			}
		}
	}
}

// moduleRef matches a `.parlay/modules/<name>.md` path in prose.
var moduleRef = regexp.MustCompile(`\.parlay/modules/([a-z0-9-]+)\.md`)

// TestConformance_ModuleReferencesResolve checks that every
// .parlay/modules/<name>.md a skill or agent points at is actually shipped
// as a module.
//
// The failure this prevents is quiet and late: a phase subagent told to read
// a module that does not exist has no menu entry to fall back on and no
// error to report — it proceeds without the instructions and produces
// plausible output built on nothing. Retiring or renaming a module is
// exactly when this happens, which is exactly when nobody re-reads the
// agent briefs.
func TestConformance_ModuleReferencesResolve(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}
	agents, err := ReadAllAgents()
	if err != nil {
		t.Fatalf("ReadAllAgents: %v", err)
	}

	shipped := map[string]bool{}
	for _, m := range ModuleSkills(skills) {
		shipped[m.Name] = true
	}
	if len(shipped) == 0 {
		t.Fatal("no module-surface skills — the surface split is not wired")
	}

	check := func(kind, name, body string) {
		for _, m := range moduleRef.FindAllStringSubmatch(body, -1) {
			if !shipped[m[1]] {
				t.Errorf("%s %q references .parlay/modules/%s.md, which is not "+
					"shipped as a module — either the skill is command-surface "+
					"(and should be invoked by name) or the reference is stale",
					kind, name, m[1])
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

// TestConformance_CommandSurfaceStaysSmall pins the result of the 0.2.0
// consolidation. The regression run needed 5 of 24 deployed skills; the
// other 19 were menu entries a designer had to choose between, several of
// them one-line wrappers around a CLI call.
//
// This is a deliberate ratchet, not a description. Adding a command-surface
// skill should require deciding that a person will invoke it by name — which
// is the question the count kept nobody from skipping.
func TestConformance_CommandSurfaceStaysSmall(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatalf("ReadAllSkills: %v", err)
	}

	want := map[string]bool{
		"loop":    true, // the driver
		"doctor":  true, // diagnosis-first repair and migration
		"onboard": true, // brownfield entry
	}

	got := map[string]bool{}
	for _, s := range CommandSkills(skills) {
		got[s.Name] = true
	}

	for name := range got {
		if !want[name] {
			t.Errorf("%q is command-surface but not in the expected set. "+
				"If a person really invokes it by name, add it here with a "+
				"note saying why; otherwise mark it `surface: module`.", name)
		}
	}
	for name := range want {
		if !got[name] {
			t.Errorf("%q is expected to be command-surface but is not — "+
				"the user-facing entry point disappeared from the menu", name)
		}
	}
}

// TestSchemaDigestCoversEveryDocumentedCode ties the digest to the schemas it
// summarizes. The digest exists so an agent stops discovering validator rules
// by triggering them; a code missing from it is a rule still learned the
// expensive way.
func TestSchemaDigestCoversEveryDocumentedCode(t *testing.T) {
	d, err := BuildSchemaDigest()
	if err != nil {
		t.Fatalf("BuildSchemaDigest: %v", err)
	}
	inDigest := map[string]bool{}
	for _, s := range d.Schemas {
		for c := range s.Codes {
			inDigest[c] = true
		}
	}
	if len(inDigest) == 0 {
		t.Fatal("digest extracted no codes at all")
	}

	for _, codes := range repoSchemaCodes(t) {
		for _, c := range codes {
			if !inDigest[c] {
				t.Errorf("%s is documented in a schema but absent from the digest", c)
			}
		}
	}

	// Every entry must say when the code fires. A bare list of codes is a
	// glossary with no definitions and sends the reader back to the schema,
	// which is what the digest exists to avoid.
	for _, s := range d.Schemas {
		for c, when := range s.Codes {
			if strings.TrimSpace(when) == "" {
				t.Errorf("%s (%s) has no 'fires when' text", c, s.File)
			}
		}
	}
}

// The rendered digest has to stay small enough to be worth reading first.
func TestSchemaDigestStaysCompact(t *testing.T) {
	d, err := BuildSchemaDigest()
	if err != nil {
		t.Fatalf("BuildSchemaDigest: %v", err)
	}
	rendered := len(RenderSchemaDigestMarkdown(d))
	// A tenth of the corpus is the loosest bound that still means "read this
	// first" rather than "read this too".
	if limit := d.TotalBytes / 10; rendered > limit {
		t.Errorf("digest is %d bytes against a %d-byte corpus (limit %d) — "+
			"it has grown into another thing to load rather than a way to avoid loading",
			rendered, d.TotalBytes, limit)
	}
}
