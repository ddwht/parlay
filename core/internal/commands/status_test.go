package commands

import (
	"bytes"
	"encoding/json"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

func TestStatus_StandaloneNoFeatures(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeProjectRoot(t, tmp)

	root := config.Root{Path: tmp, Kind: config.RootKindStandalone}
	cmd := withCtx(t, root, nil)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runStatus(cmd, nil); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "(none)") {
		t.Errorf("expected '(none)' for empty features, got: %s", out)
	}
	if !strings.Contains(out, tmp) {
		t.Errorf("expected root path in output, got: %s", out)
	}
}

func TestStatus_BareParentListsChildren(t *testing.T) {
	// Updated for parlay-tool/status-feature-phases:
	// status now renders one section per registered child (in
	// roots.yaml slice order) instead of a single "child roots:"
	// table. Both children appear; the bare parent reports
	// "(none)" for its own features.
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeProjectRoot(t, tmp)

	idx := &config.RootsIndex{
		ParentPath: tmp,
		Children: []config.Root{
			{Name: "web", RelativePath: "apps/web", Description: "Frontend"},
			{Name: "api", RelativePath: "apps/api"},
		},
	}
	if err := config.SaveRootsIndex(idx); err != nil {
		t.Fatal(err)
	}

	root := config.Root{Path: tmp, Kind: config.RootKindParent}
	cmd := withCtx(t, root, idx)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runStatus(cmd, nil); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "web") || !strings.Contains(out, "api") {
		t.Errorf("expected both children listed, got: %s", out)
	}
	// Registration-order, NOT alphabetized: web should appear before api.
	if strings.Index(out, "web") > strings.Index(out, "api") {
		t.Errorf("expected web before api (registration order), got: %s", out)
	}
	if !strings.Contains(out, "(none)") {
		t.Errorf("expected '(none)' for parent features, got: %s", out)
	}
}

func TestStatus_ListsFeatures(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeProjectRoot(t, tmp)
	// Drop a feature into spec/intents/foo/intents.md.
	featDir := filepath.Join(tmp, config.SpecDir, config.IntentsDir, "foo")
	if err := os.MkdirAll(featDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featDir, "intents.md"), []byte("# Foo"), 0644); err != nil {
		t.Fatal(err)
	}

	root := config.Root{Path: tmp, Kind: config.RootKindStandalone}
	cmd := withCtx(t, root, nil)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runStatus(cmd, nil); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "foo") {
		t.Errorf("expected feature 'foo' in output: %s", out)
	}
	if !strings.Contains(out, "features: 1") {
		t.Errorf("expected 'features: 1', got: %s", out)
	}
	if strings.Contains(out, "orphaned build dirs") {
		t.Errorf("expected no orphaned-build-dirs anomaly line for a clean project, got: %s", out)
	}
}

// TestStatus_ReportsTreeParitySeparatelyFromTopology pins the fix for
// the contradiction where `status` printed `topology: ok` on a tree
// `repair --dry-run` reported mismatches on. The two answer different
// questions, so they get one line each: `topology:` still reflects
// config.yaml/roots.yaml wiring only, and stays ok here.
func TestStatus_ReportsTreeParitySeparatelyFromTopology(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeProjectRoot(t, tmp)

	// A feature present in spec/intents/ and nowhere else — exactly
	// what `parlay add-feature` used to produce.
	featDir := filepath.Join(tmp, config.SpecDir, config.IntentsDir, "lonely")
	if err := os.MkdirAll(featDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featDir, "intents.md"), []byte("# Lonely"), 0644); err != nil {
		t.Fatal(err)
	}

	root := config.Root{Path: tmp, Kind: config.RootKindStandalone}
	cmd := withCtx(t, root, nil)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runStatus(cmd, nil); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "trees:    needs repair") {
		t.Errorf("expected the trees line to report the missing handoff/build dirs, got: %s", out)
	}
	if !strings.Contains(out, "topology: ok") {
		t.Errorf("topology answers a different question and should stay ok, got: %s", out)
	}
}

// A project whose three trees agree reports `trees: ok`, so the line is
// always present rather than being an anomaly flag.
func TestStatus_TreeParityCleanWhenTreesAgree(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeProjectRoot(t, tmp)

	featDir := filepath.Join(tmp, config.SpecDir, config.IntentsDir, "paired")
	if err := os.MkdirAll(featDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featDir, "intents.md"), []byte("# Paired"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{
		filepath.Join(tmp, config.SpecDir, config.HandoffDir, "paired"),
		filepath.Join(tmp, config.ParlayDir, "build", "paired"),
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	root := config.Root{Path: tmp, Kind: config.RootKindStandalone}
	cmd := withCtx(t, root, nil)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runStatus(cmd, nil); err != nil {
		t.Fatalf("status: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "trees:    ok") {
		t.Errorf("expected 'trees:    ok' for matching trees, got: %s", out)
	}
}

// TestStatus_FlagsOrphanedBuildDir confirms status surfaces a
// .parlay/build/<feature>/ directory that carries build artifacts but
// has no matching spec/intents/<feature>/intents.md, instead of
// silently omitting it the way it did before this check existed.
func TestStatus_FlagsOrphanedBuildDir(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeProjectRoot(t, tmp)

	// A real feature, in sync across both trees.
	realDir := filepath.Join(tmp, config.SpecDir, config.IntentsDir, "real-feature")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "intents.md"), []byte("# Real Feature"), 0644); err != nil {
		t.Fatal(err)
	}

	// A ghost feature: build artifact survives, intents.md is gone.
	ghostBuildDir := filepath.Join(tmp, config.ParlayDir, "build", "ghost-feature")
	if err := os.MkdirAll(ghostBuildDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ghostBuildDir, "buildfile.yaml"), []byte("feature: ghost-feature\n"), 0644); err != nil {
		t.Fatal(err)
	}

	root := config.Root{Path: tmp, Kind: config.RootKindStandalone}
	cmd := withCtx(t, root, nil)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runStatus(cmd, nil); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "orphaned build dirs: 1") {
		t.Errorf("expected orphaned build dirs anomaly to be flagged, got: %s", out)
	}
	if !strings.Contains(out, "ghost-feature") {
		t.Errorf("expected ghost-feature named in the anomaly, got: %s", out)
	}
	if strings.Contains(out, "real-feature") == false {
		t.Errorf("expected real-feature still listed as a normal feature, got: %s", out)
	}
}

// TestStatus_JSON_IncludesOrphanedBuildDirs confirms the JSON envelope
// carries the same anomaly, always as a present (never nil) array.
func TestStatus_JSON_IncludesOrphanedBuildDirs(t *testing.T) {
	tmp := t.TempDir()
	tmp, _ = filepath.EvalSymlinks(tmp)
	makeProjectRoot(t, tmp)

	ghostBuildDir := filepath.Join(tmp, config.ParlayDir, "build", "ghost-feature")
	if err := os.MkdirAll(ghostBuildDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ghostBuildDir, "testcases.yaml"), []byte("feature: ghost-feature\n"), 0644); err != nil {
		t.Fatal(err)
	}

	root := config.Root{Path: tmp, Kind: config.RootKindStandalone}
	pctx := config.NewContext(&config.ResolutionResult{ActiveRoot: root}, nil)
	env := buildStatusEnvelope(pctx)

	if env.Root.OrphanedBuildDirs == nil {
		t.Fatal("OrphanedBuildDirs must never be nil — JSON contract requires the array always present")
	}
	if len(env.Root.OrphanedBuildDirs) != 1 || env.Root.OrphanedBuildDirs[0] != "ghost-feature" {
		t.Errorf("OrphanedBuildDirs = %v, want [ghost-feature]", env.Root.OrphanedBuildDirs)
	}
}

// The JSON contract, end to end: a brand-new feature reports `planned`,
// and the envelope that carries the new token declares v2. These two
// facts belong in one assertion — emitting a phase value a v1 consumer
// cannot know about, under a version that promises it cannot happen, is
// precisely the compatibility break the bump exists to announce.
func TestStatusJSON_FreshFeatureIsPlannedUnderSchemaV2(t *testing.T) {
	setupTestDir(t)
	if err := os.MkdirAll(filepath.Join(config.SpecDir, config.IntentsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runAddFeature(testCommandWithContext(t, testContext(t)), []string{"fleet", "overview"}); err != nil {
		t.Fatal(err)
	}

	cmd := testCommandWithContext(t, testContext(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	statusJSON = true
	defer func() { statusJSON = false }()
	if err := runStatus(cmd, nil); err != nil {
		t.Fatal(err)
	}

	var env statusEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("envelope did not parse: %v\n%s", err, out.String())
	}
	if env.SchemaVersion != 2 {
		t.Errorf("schema_version: want 2, got %d", env.SchemaVersion)
	}
	var found bool
	for _, f := range env.Root.Features {
		if f.ID == "fleet-overview" {
			found = true
			if f.Phase != PhasePlanned {
				t.Errorf("fresh feature phase: want %q, got %q", PhasePlanned, f.Phase)
			}
		}
	}
	if !found {
		t.Fatalf("fleet-overview missing from envelope: %s", out.String())
	}
}

// A root with four features and eleven untriaged observations is not the
// same project as one with four features and none, and status was only
// ever showing the first number. Parent aggregates children the way it
// already aggregates features (proposal §15).
func TestStatus_ReportsBacklogPerRootIncludingChildren(t *testing.T) {
	parent := t.TempDir()
	parent, _ = filepath.EvalSymlinks(parent)
	childPath := filepath.Join(parent, "child")
	for _, root := range []string{parent, childPath} {
		if err := os.MkdirAll(filepath.Join(root, config.SpecDir, config.IntentsDir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	pctx := parentWithChild(t, parent, "child", childPath)

	// One ranked and one untriaged in the parent; one untriaged in the
	// child. Different numbers per root, so an aggregation that reports
	// one root's count for both is visible.
	if _, err := note(t, rootCtxAt(parent), "gap", "parent ranked", "claude", func() { notePriority = "P1" }); err != nil {
		t.Fatal(err)
	}
	if _, err := note(t, rootCtxAt(parent), "defect", "parent untriaged", "claude", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := note(t, rootCtxAt(childPath), "debt", "child untriaged", "claude", nil); err != nil {
		t.Fatal(err)
	}

	env := buildStatusEnvelope(pctx)

	if env.Root.Backlog.Open != 2 || env.Root.Backlog.Untriaged != 1 {
		t.Errorf("parent backlog = %+v, want 2 open / 1 untriaged", env.Root.Backlog)
	}
	if len(env.Children) != 1 {
		t.Fatalf("want one child, got %d", len(env.Children))
	}
	if got := env.Children[0].Backlog; got.Open != 1 || got.Untriaged != 1 {
		t.Errorf("child backlog = %+v, want 1 open / 1 untriaged", got)
	}
}

// A healthy project's status output stays unchanged: the backlog line is
// an anomaly line, like the orphan line, not one that is always present.
func TestStatus_BacklogLineIsSilentWhenThereIsNothingOutstanding(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	renderBacklogHuman(cmd, backlogSummary{})
	if out.Len() != 0 {
		t.Errorf("an empty backlog printed a line: %q", out.String())
	}

	// But "no open items" and "no open items THAT WE COULD READ" are
	// different claims, and only the first is good news.
	out.Reset()
	renderBacklogHuman(cmd, backlogSummary{Unreadable: 2})
	if !strings.Contains(out.String(), "could not be read") {
		t.Errorf("unreadable records were reported as a clean backlog: %q", out.String())
	}
}

// The human path, end to end, for BOTH child topologies.
//
// The JSON test and the renderBacklogHuman unit test both passed while
// the human wiring was wrong in two opposite ways at once: the backlog
// line was emitted twice for a child with no features and not at all for
// a child with features — the ordinary case. Neither isolated test could
// see it, because neither ran renderChildHuman.
func TestStatus_HumanBacklogLineAppearsExactlyOncePerChild(t *testing.T) {
	for _, tc := range []struct {
		name        string
		withFeature bool
	}{
		{"child with features", true},
		{"bare child", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			parent, _ = filepath.EvalSymlinks(parent)
			childPath := filepath.Join(parent, "child")
			for _, root := range []string{parent, childPath} {
				if err := os.MkdirAll(filepath.Join(root, config.SpecDir, config.IntentsDir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			pctx := parentWithChild(t, parent, "child", childPath)
			childCtx := rootCtxAt(childPath)
			if tc.withFeature {
				if err := runAddFeature(testCommandWithContext(t, childCtx), []string{"gadget"}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := note(t, childCtx, "gap", "child observation", "claude", nil); err != nil {
				t.Fatal(err)
			}

			var out bytes.Buffer
			cmd := testCommandWithContext(t, pctx)
			cmd.SetOut(&out)
			renderChildHuman(cmd, "child", childPath, childCtx, childFeatureSlugs(t, childCtx), nil)

			n := strings.Count(out.String(), "backlog: 1 open")
			if n != 1 {
				t.Errorf("the child's backlog line appeared %d times, want exactly 1:\n%s", n, out.String())
			}
		})
	}
}

func childFeatureSlugs(t *testing.T, ctx *config.Context) []string {
	t.Helper()
	features, _ := scanFeaturesAtTolerant(ctx.IntentsRoot())
	return features
}

// The envelope contract, asserted on RAW JSON.
//
// Unmarshalling into the same struct cannot see this: a non-pointer
// member is emitted on every root as {open:0,untriaged:0}, which is a
// REQUIRED new field under a schema_version already deployed — the
// opposite of the additive change the version pin's rule allows without
// a bump. Only the raw bytes show whether the key is there.
func TestStatus_BacklogIsAbsentFromHealthyJSONAndPresentWhenOutstanding(t *testing.T) {
	build := func(t *testing.T, withItem bool) map[string]any {
		t.Helper()
		parent := t.TempDir()
		parent, _ = filepath.EvalSymlinks(parent)
		childPath := filepath.Join(parent, "child")
		for _, root := range []string{parent, childPath} {
			if err := os.MkdirAll(filepath.Join(root, config.SpecDir, config.IntentsDir), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		pctx := parentWithChild(t, parent, "child", childPath)
		if withItem {
			if _, err := note(t, rootCtxAt(parent), "gap", "outstanding", "claude", nil); err != nil {
				t.Fatal(err)
			}
			if _, err := note(t, rootCtxAt(childPath), "debt", "child outstanding", "claude", nil); err != nil {
				t.Fatal(err)
			}
		}
		raw, err := json.Marshal(buildStatusEnvelope(pctx))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	healthy := build(t, false)
	root := healthy["root"].(map[string]any)
	if _, present := root["backlog"]; present {
		t.Errorf("a healthy root emits a backlog member; that is a required field under an already-deployed schema_version: %v", root["backlog"])
	}
	child := healthy["children"].([]any)[0].(map[string]any)
	if _, present := child["backlog"]; present {
		t.Errorf("a healthy child emits a backlog member: %v", child["backlog"])
	}
	if v := healthy["schema_version"].(float64); v != 2 {
		t.Errorf("schema_version moved to %v; the field was meant to stay optional under 2", v)
	}

	outstanding := build(t, true)
	for label, section := range map[string]map[string]any{
		"root":  outstanding["root"].(map[string]any),
		"child": outstanding["children"].([]any)[0].(map[string]any),
	} {
		b, present := section["backlog"].(map[string]any)
		if !present {
			t.Errorf("%s has an outstanding item and emits no backlog member", label)
			continue
		}
		if b["open"].(float64) != 1 || b["untriaged"].(float64) != 1 {
			t.Errorf("%s backlog = %v, want 1 open / 1 untriaged", label, b)
		}
	}
}
