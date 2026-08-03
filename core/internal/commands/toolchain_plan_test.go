// parlay-feature: parlay-tool/multi-adapter
// parlay-artifact: test
//
// toolchain-plan surfaces an adapter's Section-10 toolchain to the code phase.
// Exercised against the committed multitarget fixture (whose react-antd adapter
// declares a pre-emit mutating MCP scaffolder + a post-emit advisory review
// skill) and small single-target temp projects.

package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"gopkg.in/yaml.v3"
)

func runToolchainPlanJSON(t *testing.T, cfg *config.Context, feature, phase, stage string) toolchainPlanOutput {
	t.Helper()
	prevP, prevS := toolchainPlanPhase, toolchainPlanStage
	toolchainPlanPhase, toolchainPlanStage = phase, stage
	defer func() { toolchainPlanPhase, toolchainPlanStage = prevP, prevS }()

	cmd := testCommandWithContext(t, cfg)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runToolchainPlan(cmd, []string{feature}); err != nil {
		t.Fatalf("toolchain-plan: %v", err)
	}
	var out toolchainPlanOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("parse output: %v\n%s", err, buf.String())
	}
	return out
}

func multitargetCfg(t *testing.T) *config.Context {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "multitarget"))
	if err != nil {
		t.Fatal(err)
	}
	return config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Name: "multitarget", Path: abs, Kind: config.RootKindStandalone},
		Source:     config.SourceCwdWalkUp,
	}, nil)
}

func TestToolchainPlan_MultiTargetFixture(t *testing.T) {
	out := runToolchainPlanJSON(t, multitargetCfg(t), "notes", "code", "")
	if len(out.Entries) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(out.Entries), out.Entries)
	}
	// Deterministic order: pre-emit MCP before post-emit skill.
	if out.Entries[0].Kind != "mcp" || out.Entries[0].Stage != "pre-emit" {
		t.Errorf("entry[0] should be the pre-emit MCP scaffolder, got %+v", out.Entries[0])
	}
	if out.Entries[1].Kind != "skill" || out.Entries[1].Stage != "post-emit" {
		t.Errorf("entry[1] should be the post-emit review skill, got %+v", out.Entries[1])
	}
	for _, e := range out.Entries {
		if e.Target != "presentation" {
			t.Errorf("entry %q should be labeled target=presentation, got %q", e.Name, e.Target)
		}
		if e.Adapter != "react-antd.adapter.yaml" {
			t.Errorf("entry %q adapter = %q", e.Name, e.Adapter)
		}
		if e.Required {
			t.Errorf("entry %q required should resolve false", e.Name)
		}
	}
	mcp := out.Entries[0]
	if mcp.Server != "react-scaffold-mcp" || len(mcp.Tools) != 1 || mcp.Tools[0] != "scaffold_component" {
		t.Errorf("mcp entry projection wrong: %+v", mcp)
	}
	if mcp.Authority != "mutating" || mcp.OwnsMarkers != "parlay" {
		t.Errorf("mcp entry contract fields wrong: %+v", mcp)
	}
}

func TestToolchainPlan_StageFilter(t *testing.T) {
	cfg := multitargetCfg(t)
	pre := runToolchainPlanJSON(t, cfg, "notes", "code", "pre-emit")
	if len(pre.Entries) != 1 || pre.Entries[0].Kind != "mcp" {
		t.Fatalf("--stage pre-emit should return only the MCP scaffolder, got %+v", pre.Entries)
	}
	if pre.Stage != "pre-emit" {
		t.Errorf("stage should be echoed, got %q", pre.Stage)
	}
	post := runToolchainPlanJSON(t, cfg, "notes", "code", "post-emit")
	if len(post.Entries) != 1 || post.Entries[0].Kind != "skill" {
		t.Fatalf("--stage post-emit should return only the review skill, got %+v", post.Entries)
	}
}

func TestToolchainPlan_PhaseWithNoEntriesIsEmpty(t *testing.T) {
	out := runToolchainPlanJSON(t, multitargetCfg(t), "notes", "artifacts", "")
	if out.Entries == nil {
		t.Fatal("entries must be [] not null")
	}
	if len(out.Entries) != 0 {
		t.Errorf("no entry declares phase artifacts; want empty, got %+v", out.Entries)
	}
}

// A single-target project: one adapter carrying a toolchain, no adapter-set.
func writeSingleTargetProject(t *testing.T, toolchainYAML string) *config.Context {
	t.Helper()
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	adapters := filepath.Join(dir, ".parlay", "adapters")
	if err := os.MkdirAll(adapters, 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := "name: go-cli\nkind: presentation\nfile-conventions:\n  source-root: \"cmd/\"\n" + toolchainYAML
	if err := os.WriteFile(filepath.Join(adapters, "go-cli.adapter.yaml"), []byte(adapter), 0o644); err != nil {
		t.Fatal(err)
	}
	build := filepath.Join(dir, ".parlay", "build", "feat")
	if err := os.MkdirAll(build, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(build, "buildfile.yaml"), []byte("feature: feat\nadapter: go-cli\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Name: filepath.Base(dir), Path: dir, Kind: config.RootKindStandalone},
		Source:     config.SourceCwdWalkUp,
	}, nil)
}

func TestToolchainPlan_SingleTargetUnlabeled(t *testing.T) {
	cfg := writeSingleTargetProject(t, `toolchain:
  mcp:
    - server: scaffolder
      tools: [gen]
      phase: [code]
      stage: pre-emit
      authority: mutating
      required: false
      read-set: ["cmd/**"]
      write-set: ["cmd/**"]
      owns-markers: parlay
      preserves: [testcases]
      fallback: "templates"
  skills:
    - id: reviewer
      invoke: "/review"
      phase: [code]
      stage: post-emit
      authority: advisory
      required: false
      fallback: "skip"
`)
	out := runToolchainPlanJSON(t, cfg, "feat", "code", "")
	if len(out.Entries) != 2 {
		t.Fatalf("want 2 entries, got %+v", out.Entries)
	}
	for _, e := range out.Entries {
		if e.Target != "" {
			t.Errorf("single-target entries carry no target, got %q for %q", e.Target, e.Name)
		}
		if e.Adapter != "go-cli.adapter.yaml" {
			t.Errorf("adapter basename = %q", e.Adapter)
		}
	}
}

// A required:true entry resolves required=true (distinct from the *bool nil case).
func TestToolchainPlan_RequiredResolvesFromPointer(t *testing.T) {
	cfg := writeSingleTargetProject(t, `toolchain:
  skills:
    - id: must-have
      invoke: "/x"
      phase: [code]
      stage: post-emit
      authority: advisory
      required: true
`)
	out := runToolchainPlanJSON(t, cfg, "feat", "code", "")
	if len(out.Entries) != 1 || !out.Entries[0].Required {
		t.Fatalf("required:true must resolve true, got %+v", out.Entries)
	}
}

// Two identical entries collapse to one (dedup on target/kind/name/stage).
func TestToolchainPlan_Dedup(t *testing.T) {
	cfg := writeSingleTargetProject(t, `toolchain:
  skills:
    - id: dup
      invoke: "/d"
      phase: [code]
      stage: post-emit
      authority: advisory
      required: false
      fallback: "skip"
    - id: dup
      invoke: "/d"
      phase: [code]
      stage: post-emit
      authority: advisory
      required: false
      fallback: "skip"
`)
	out := runToolchainPlanJSON(t, cfg, "feat", "code", "")
	if len(out.Entries) != 1 {
		t.Fatalf("duplicate entries must collapse to one, got %+v", out.Entries)
	}
}

// The committed fixture's toolchain block must stay valid against the adapter's
// own source-root (the axis ValidateToolchain bounds write-sets on).
func TestFixtureToolchainValidates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "multitarget", ".parlay", "adapters", "react-antd.adapter.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		FileConventions struct {
			SourceRoot string `yaml:"source-root"`
		} `yaml:"file-conventions"`
		Toolchain *agent.Toolchain `yaml:"toolchain"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Toolchain == nil {
		t.Fatal("fixture react-antd adapter should declare a toolchain")
	}
	if errs := agent.ValidateToolchain(doc.Toolchain, doc.FileConventions.SourceRoot); len(errs) != 0 {
		t.Fatalf("fixture toolchain must validate against source-root %q: %+v", doc.FileConventions.SourceRoot, errs)
	}
}
