package agent

import (
	"strings"
	"testing"
)

func codes(errs []ValidationError) []string {
	var out []string
	for _, e := range errs {
		out = append(out, e.Code)
	}
	return out
}

func hasCode(errs []ValidationError, code string) bool {
	for _, c := range codes(errs) {
		if c == code {
			return true
		}
	}
	return false
}

func boolPtr(b bool) *bool { return &b }

// The codegen boundary is the load-bearing rule. A tool that reads the spec
// makes the buildfile's completeness unfalsifiable, and does it invisibly —
// the emitted output looks no different. Rejecting at registration is the only
// point where it is detectable.
func TestReadSetCrossingTheSpecBoundaryIsRejected(t *testing.T) {
	for _, glob := range []string{
		"spec/intents/**",
		"spec/intents",
		"./spec/intents/submit-expense/intents.md",
		"spec/handoff/**",
		"**",
		"**/*.md",
	} {
		tc := &Toolchain{Skills: []ToolchainEntry{{
			ID: "t", Authority: "advisory", Required: boolPtr(false),
			Fallback: "skip", ReadSet: []string{glob},
		}}}
		if !hasCode(ValidateToolchain(tc, "src/"), "toolchain-read-set-crosses-spec-boundary") {
			t.Errorf("read-set %q was accepted; it can reach the design spec", glob)
		}
	}
}

// And must not refuse legitimate source globs, or the extension point is
// unusable and everyone forks instead.
func TestOrdinarySourceReadSetsAreAccepted(t *testing.T) {
	for _, glob := range []string{"src/**", "src/app/**/*.ts", "angular.json", "package.json", "specs/unit/**"} {
		tc := &Toolchain{Skills: []ToolchainEntry{{
			ID: "t", Authority: "advisory", Required: boolPtr(false),
			Fallback: "skip", ReadSet: []string{glob},
		}}}
		if hasCode(ValidateToolchain(tc, "src/"), "toolchain-read-set-crosses-spec-boundary") {
			t.Errorf("read-set %q refused, but it does not touch the spec tree", glob)
		}
	}
}

// A mutating tool with no preserves list is the case the gate exists for: a
// dropped data-testid and a wholesale reformat are indistinguishable by diff
// size, so something has to state which one is acceptable.
func TestMutatingToolNeedsPreserves(t *testing.T) {
	tc := &Toolchain{MCP: []ToolchainEntry{{
		Server: "fmt", Authority: "mutating", Required: boolPtr(false),
		Fallback: "skip", WriteSet: []string{"src/app/**"},
	}}}
	errs := ValidateToolchain(tc, "src/")
	if !hasCode(errs, "toolchain-mutating-without-preserves") {
		t.Fatalf("mutating tool accepted with no preserves: %v", codes(errs))
	}

	tc.MCP[0].Preserves = []string{"testcases", "markers"}
	if hasCode(ValidateToolchain(tc, "src/"), "toolchain-mutating-without-preserves") {
		t.Error("valid preserves list still rejected")
	}

	// The vocabulary is closed — an invented guarantee is not a guarantee.
	tc.MCP[0].Preserves = []string{"vibes"}
	if !hasCode(ValidateToolchain(tc, "src/"), "toolchain-mutating-without-preserves") {
		t.Error("preserves accepted a value outside the closed set")
	}
}

// Advisory means the output is a suggestion. A write path contradicts the
// declaration a reviewer relies on.
func TestAdvisoryToolCannotWrite(t *testing.T) {
	tc := &Toolchain{Skills: []ToolchainEntry{{
		ID: "review", Authority: "advisory", Required: boolPtr(false),
		Fallback: "skip", WriteSet: []string{"src/app/**"},
	}}}
	if !hasCode(ValidateToolchain(tc, "src/"), "toolchain-advisory-with-write-set") {
		t.Fatal("advisory tool accepted with a write-set")
	}
}

// Graceful absence was hit for real: the Figma MCP server was connected and
// the browser tool was not, and every step assuming both was unreachable.
func TestOptionalToolNeedsFallback(t *testing.T) {
	tc := &Toolchain{MCP: []ToolchainEntry{{
		Server: "ng", Authority: "advisory", Required: boolPtr(false),
	}}}
	if !hasCode(ValidateToolchain(tc, "src/"), "toolchain-optional-without-fallback") {
		t.Fatal("optional tool accepted with no fallback")
	}

	// An omitted `required` is treated as optional on purpose: it means the
	// author did not consider absence, which is the case worth flagging.
	tc.MCP[0].Required = nil
	if !hasCode(ValidateToolchain(tc, "src/"), "toolchain-optional-without-fallback") {
		t.Error("omitted required: did not require a fallback")
	}

	tc.MCP[0].Required = boolPtr(true)
	if hasCode(ValidateToolchain(tc, "src/"), "toolchain-optional-without-fallback") {
		t.Error("required tool asked for a fallback it does not need")
	}
}

func TestWriteSetMustStayInsideSourceRoot(t *testing.T) {
	tc := &Toolchain{MCP: []ToolchainEntry{{
		Server: "ng", Authority: "mutating", Preserves: []string{"markers"},
		Required: boolPtr(true), WriteSet: []string{"../elsewhere/**", "/etc/hosts"},
	}}}
	errs := ValidateToolchain(tc, "src/app/")
	n := 0
	for _, c := range codes(errs) {
		if c == "toolchain-write-set-outside-source-root" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("want 2 escape findings, got %d: %v", n, codes(errs))
	}
}

func TestUnknownPhaseIsRejected(t *testing.T) {
	tc := &Toolchain{Skills: []ToolchainEntry{{
		ID: "t", Authority: "advisory", Required: boolPtr(true),
		Phase: []string{"code", "deploy"},
	}}}
	errs := ValidateToolchain(tc, "src/")
	if !hasCode(errs, "toolchain-unknown-phase") {
		t.Fatalf("unknown phase accepted: %v", codes(errs))
	}
	for _, e := range errs {
		if e.Code == "toolchain-unknown-phase" && !strings.Contains(e.Message, "deploy") {
			t.Errorf("message does not name the offending phase: %s", e.Message)
		}
	}
}

// An adapter with no toolchain block behaves exactly as before.
func TestAbsentToolchainIsSilent(t *testing.T) {
	if errs := ValidateToolchain(nil, "src/"); len(errs) != 0 {
		t.Fatalf("nil toolchain produced findings: %v", codes(errs))
	}
	if errs := ValidateToolchain(&Toolchain{}, "src/"); len(errs) != 0 {
		t.Fatalf("empty toolchain produced findings: %v", codes(errs))
	}
}

// The example in adapter.schema.md Section 10 must itself validate — a
// documented example that fails its own validator is how P2-9 and P2-10
// happened.
func TestSchemaSection10ExampleValidates(t *testing.T) {
	tc := &Toolchain{
		Skills: []ToolchainEntry{{
			ID: "angular-best-practices", Invoke: "/angular-review", Source: "community",
			Phase: []string{"code"}, Stage: "post-emit", Authority: "advisory",
			Required: boolPtr(false), ReadSet: []string{"src/**"}, WriteSet: nil,
			Fallback: "skip the review",
		}},
		MCP: []ToolchainEntry{{
			Server: "angular-cli-mcp", Tools: []string{"ng_generate", "ng_lint"},
			Phase: []string{"code"}, Stage: "pre-emit", Authority: "mutating",
			Required: boolPtr(false), ReadSet: []string{"src/**", "angular.json"},
			WriteSet: []string{"src/app/**"}, OwnsMarkers: "parlay",
			Preserves: []string{"testcases", "declared-elements", "markers"},
			Fallback:  "emit from adapter templates",
		}},
	}
	if errs := ValidateToolchain(tc, "src/"); len(errs) != 0 {
		for _, e := range errs {
			t.Errorf("schema example fails validation: %s — %s", e.Code, e.Message)
		}
	}
}

// Suite: the two fields that were parsed and never read (D10)
//
// Both were declared in the struct, documented in the field reference with a
// Required column, and consulted by nothing. The schema's own Section 10 example
// sets both correctly — so the fields were being authored right and simply never
// checked, which is the quiet version of this bug: no adapter had to be wrong for
// the gap to exist, and nothing would have reported one that was.

// invoke: is Required for Skills. Without it an adapter declares a skill the
// agent has no way to call, and the toolchain validator waved it through.
func TestSkillWithoutInvokeIsRejected(t *testing.T) {
	tc := &Toolchain{Skills: []ToolchainEntry{{
		ID: "reviewer", Source: "community", Phase: []string{"code"},
		Stage: "post-emit", Authority: "advisory",
		Required: boolPtr(false), Fallback: "skip", ReadSet: []string{"src/**"},
	}}}
	if !hasCode(ValidateToolchain(tc, "src/"), "toolchain-skill-without-invoke") {
		t.Error("a skill entry with no invoke: was accepted; nothing can reach it")
	}
}

// MCP entries are addressed by server name plus a tools: list — a different
// calling convention — so the rule must not fire on them.
func TestMCPEntryDoesNotNeedInvoke(t *testing.T) {
	tc := &Toolchain{MCP: []ToolchainEntry{{
		Server: "angular-cli-mcp", Tools: []string{"ng_lint"},
		Phase: []string{"code"}, Stage: "pre-emit", Authority: "advisory",
		Required: boolPtr(false), Fallback: "skip", ReadSet: []string{"src/**"},
	}}}
	if hasCode(ValidateToolchain(tc, "src/"), "toolchain-skill-without-invoke") {
		t.Error("an MCP entry was required to declare invoke:; it is addressed by server + tools")
	}
}

// owns-markers: is Required for Mutating, and is the marker-ownership contract:
// it decides whether parlay's markers survive the tool's rewrite. A file that
// falls out of the marker chain falls out of the hash chain and so out of the
// hand-edit guard — the incident the schema cites is 17 marked templates going
// invisible to scan-generated with nothing reporting it.
func TestMutatingToolNeedsOwnsMarkers(t *testing.T) {
	tc := &Toolchain{MCP: []ToolchainEntry{{
		Server: "fmt", Authority: "mutating", Required: boolPtr(false),
		Fallback: "skip", WriteSet: []string{"src/app/**"},
		Preserves: []string{"markers"},
	}}}
	if !hasCode(ValidateToolchain(tc, "src/"), "toolchain-mutating-without-owns-markers") {
		t.Error("a mutating entry with no owns-markers: was accepted")
	}
}

// The set is closed. An unrecognised value is worse than an absent one: it reads
// as a considered answer.
func TestOwnsMarkersIsAClosedSet(t *testing.T) {
	for _, value := range []string{"both", "none", "parlay-and-tool", "Parlay", "TOOL"} {
		tc := &Toolchain{MCP: []ToolchainEntry{{
			Server: "fmt", Authority: "mutating", Required: boolPtr(false),
			Fallback: "skip", WriteSet: []string{"src/app/**"},
			Preserves: []string{"markers"}, OwnsMarkers: value,
		}}}
		if !hasCode(ValidateToolchain(tc, "src/"), "toolchain-mutating-without-owns-markers") {
			t.Errorf("owns-markers %q was accepted; the set is {parlay, tool}", value)
		}
	}
	for _, value := range []string{"parlay", "tool"} {
		tc := &Toolchain{MCP: []ToolchainEntry{{
			Server: "fmt", Authority: "mutating", Required: boolPtr(false),
			Fallback: "skip", WriteSet: []string{"src/app/**"},
			Preserves: []string{"markers"}, OwnsMarkers: value,
		}}}
		if hasCode(ValidateToolchain(tc, "src/"), "toolchain-mutating-without-owns-markers") {
			t.Errorf("owns-markers %q was refused; it is in the closed set", value)
		}
	}
}

// An advisory entry declares no write-set and rewrites nothing, so marker
// ownership does not arise. Requiring it there would make the field noise.
func TestAdvisoryToolDoesNotNeedOwnsMarkers(t *testing.T) {
	tc := &Toolchain{Skills: []ToolchainEntry{{
		ID: "reviewer", Invoke: "/review", Authority: "advisory",
		Required: boolPtr(false), Fallback: "skip", ReadSet: []string{"src/**"},
	}}}
	if hasCode(ValidateToolchain(tc, "src/"), "toolchain-mutating-without-owns-markers") {
		t.Error("an advisory entry was required to declare owns-markers:")
	}
}
