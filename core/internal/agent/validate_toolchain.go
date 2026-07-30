package agent

import (
	"fmt"
	"strings"
)

// Toolchain validation for adapter.schema.md Section 10.
//
// An extension point is a contract published to third parties, which makes it
// the least forgiving place to have documentation that the code does not
// enforce. Two of these rules are load-bearing rather than tidy:
//
// read-set is the codegen boundary. Codegen must never read spec/intents/**;
// that boundary currently holds, and it is the load-bearing test of whether
// the buildfile is doing its job — if a generator can produce a
// test-passing prototype from buildfile plus adapter alone, the buildfile is
// complete. An external tool with unrestricted filesystem access breaks that
// silently: the output looks no different. So the read-set is rejected at
// registration rather than trusted at run time.
//
// preserves is the admission gate for mutating tools. parlay's contract is
// functional determinism measured at the testcase boundary, so "did the bytes
// change" is the wrong question to ask a formatter. "Do the testcases still
// pass, is every declared element still present, are the markers intact" is
// the right one.

// toolchainPhases is the closed set of pipeline phases a tool may attach to.
var toolchainPhases = map[string]bool{
	"intents": true, "dialogs": true, "artifacts": true, "build": true, "code": true,
}

// preservesVocabulary is the closed set of behavioral guarantees a mutating
// tool can be held to.
var preservesVocabulary = map[string]bool{
	"testcases": true, "declared-elements": true, "markers": true,
}

// ToolchainEntry is one skill or MCP server declaration. Skills and MCP
// servers share every field that carries a rule; only their identity differs,
// so they validate through one path rather than two that drift.
type ToolchainEntry struct {
	ID          string   `yaml:"id"`
	Server      string   `yaml:"server"`
	Invoke      string   `yaml:"invoke"`
	Tools       []string `yaml:"tools"`
	Source      string   `yaml:"source"`
	Phase       []string `yaml:"phase"`
	Stage       string   `yaml:"stage"`
	Authority   string   `yaml:"authority"`
	Required    *bool    `yaml:"required"`
	ReadSet     []string `yaml:"read-set"`
	WriteSet    []string `yaml:"write-set"`
	OwnsMarkers string   `yaml:"owns-markers"`
	Preserves   []string `yaml:"preserves"`
	Fallback    string   `yaml:"fallback"`
}

// Name returns whichever identity the entry carries, for error messages.
func (e ToolchainEntry) Name() string {
	if e.ID != "" {
		return e.ID
	}
	if e.Server != "" {
		return e.Server
	}
	return "<unnamed>"
}

// Toolchain is the adapter's toolchain block.
type Toolchain struct {
	Skills []ToolchainEntry `yaml:"skills"`
	MCP    []ToolchainEntry `yaml:"mcp"`
}

// specBoundaryPrefixes are the paths codegen must never read. A read-set glob
// that could match any of these is refused.
var specBoundaryPrefixes = []string{"spec/intents", "spec/handoff"}

// ValidateToolchain checks every entry against the Section 10 rules.
// sourceRoot is file-conventions.source-root, used to bound write sets; an
// empty sourceRoot skips only that one check.
func ValidateToolchain(tc *Toolchain, sourceRoot string) []ValidationError {
	if tc == nil {
		return nil
	}
	var errs []ValidationError
	for _, e := range tc.Skills {
		errs = append(errs, validateToolchainEntry(e, sourceRoot, "skill")...)
	}
	for _, e := range tc.MCP {
		errs = append(errs, validateToolchainEntry(e, sourceRoot, "mcp")...)
	}
	return errs
}

func validateToolchainEntry(e ToolchainEntry, sourceRoot, kind string) []ValidationError {
	var errs []ValidationError
	at := fmt.Sprintf("toolchain.%s[%s]", kind, e.Name())

	add := func(code, msg string) {
		errs = append(errs, ValidationError{Code: code, Message: fmt.Sprintf("%s: %s", at, msg)})
	}

	for _, glob := range e.ReadSet {
		if globCrossesSpecBoundary(glob) {
			add("toolchain-read-set-crosses-spec-boundary",
				fmt.Sprintf("read-set %q reaches into the design spec. Codegen reads the buildfile, not intents — "+
					"a tool that reads the spec makes the buildfile's completeness unfalsifiable, and does it invisibly", glob))
		}
	}

	if sourceRoot != "" {
		for _, glob := range e.WriteSet {
			if !globWithinRoot(glob, sourceRoot) {
				add("toolchain-write-set-outside-source-root",
					fmt.Sprintf("write-set %q escapes source-root %q", glob, sourceRoot))
			}
		}
	}

	// A skill entry has to say how the agent calls it. The field reference
	// marks invoke: required for Skills, and nothing enforced it — an adapter
	// could declare a skill the agent has no way to reach, and the toolchain
	// validator would pass it. MCP entries are exempt: they are addressed by
	// server name plus a tools: list, which is a different calling convention.
	if e.ID != "" && e.Server == "" && strings.TrimSpace(e.Invoke) == "" {
		add("toolchain-skill-without-invoke",
			"a skill entry needs invoke: — it names how the agent calls the skill, "+
				"and without it the entry declares a tool nothing can reach")
	}

	switch e.Authority {
	case "mutating":
		// owns-markers is the marker-ownership contract, and it was parsed and
		// then read by nothing. It decides whether parlay's markers survive a
		// tool's rewrite; a file that falls out of the marker chain falls out of
		// the hash chain and therefore out of the hand-edit guard, which is how
		// 17 marked templates went invisible to scan-generated with nothing
		// reporting it. Required for mutating entries, closed set {parlay, tool}.
		switch strings.TrimSpace(e.OwnsMarkers) {
		case "parlay", "tool":
			// declared
		case "":
			add("toolchain-mutating-without-owns-markers",
				"authority: mutating requires owns-markers: — it decides whether parlay's markers "+
					"survive the tool's rewrite, and a file outside the marker chain is outside the "+
					"hand-edit guard with nothing reporting it")
		default:
			add("toolchain-mutating-without-owns-markers",
				fmt.Sprintf("owns-markers: %q is not in the closed set {parlay, tool}", e.OwnsMarkers))
		}

		if len(e.Preserves) == 0 {
			add("toolchain-mutating-without-preserves",
				"authority: mutating requires a preserves: list — without it nothing says what the tool must not break, "+
					"and a dropped data-testid looks the same as a reformat")
		}
		for _, p := range e.Preserves {
			if !preservesVocabulary[p] {
				add("toolchain-mutating-without-preserves",
					fmt.Sprintf("preserves: %q is not in the closed set {testcases, declared-elements, markers}", p))
			}
		}
	case "advisory":
		if len(e.WriteSet) > 0 {
			add("toolchain-advisory-with-write-set",
				"authority: advisory cannot declare a write-set — advisory output is a suggestion, "+
					"so a write path contradicts the declaration a reviewer relies on")
		}
	}

	// required: false is the graceful-absence case and needs a fallback.
	// required is a pointer so an omitted field is distinguishable from an
	// explicit false: omitted means the author did not consider absence,
	// which is exactly the case worth flagging.
	if e.Required == nil || !*e.Required {
		if strings.TrimSpace(e.Fallback) == "" {
			add("toolchain-optional-without-fallback",
				"an optional tool needs a fallback: — the build has to succeed on a machine where the tool is not installed")
		}
	}

	for _, ph := range e.Phase {
		if !toolchainPhases[ph] {
			add("toolchain-unknown-phase",
				fmt.Sprintf("phase %q is not one of {intents, dialogs, artifacts, build, code}", ph))
		}
	}

	return errs
}

// globCrossesSpecBoundary reports whether a glob could match a path under a
// spec-boundary prefix.
//
// Conservative by construction: a bare "**" or "." matches everything and so
// crosses the boundary, and any glob whose literal prefix leads into the spec
// tree crosses it. The failure this prevents is undetectable after the fact,
// so a false positive here (a refused registration, with a message saying
// exactly which glob and why) is much cheaper than a false negative.
func globCrossesSpecBoundary(glob string) bool {
	g := strings.TrimPrefix(strings.TrimSpace(glob), "./")
	if g == "" {
		return false
	}
	if g == "**" || g == "*" || g == "." || g == "/" || strings.HasPrefix(g, "**") {
		return true
	}
	for _, prefix := range specBoundaryPrefixes {
		if g == prefix || strings.HasPrefix(g, prefix+"/") || strings.HasPrefix(g, prefix) {
			return true
		}
		// A wildcard early in the glob can expand into the spec tree:
		// "*/intents/**" reaches spec/intents.
		if i := strings.Index(g, "*"); i >= 0 {
			head := g[:i]
			if strings.HasPrefix(prefix, strings.TrimSuffix(head, "/")) && head != "" {
				rest := strings.TrimPrefix(g[i:], "*")
				rest = strings.TrimPrefix(rest, "*")
				rest = strings.TrimPrefix(rest, "/")
				if rest != "" && strings.Contains(prefix+"/", strings.SplitN(rest, "/", 2)[0]) {
					return true
				}
			}
		}
	}
	return false
}

// globWithinRoot reports whether a write glob stays inside sourceRoot.
func globWithinRoot(glob, sourceRoot string) bool {
	g := strings.TrimPrefix(strings.TrimSpace(glob), "./")
	root := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(sourceRoot), "./"), "/")
	if root == "" {
		return true
	}
	if strings.HasPrefix(g, "..") {
		return false
	}
	return g == root || strings.HasPrefix(g, root+"/")
}
