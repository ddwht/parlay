package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anthropics/parlay/internal/config"
	"github.com/anthropics/parlay/internal/parser"
	"gopkg.in/yaml.v3"
)

// writeFeatureFiles creates a feature directory with intents.md, dialogs.md,
// and surface.md populated with provided content. Empty strings skip the file.
func writeFeatureFiles(t *testing.T, slug, intents, dialogs, surface string) string {
	t.Helper()
	featureDir := config.FeaturePath(slug)
	if err := os.MkdirAll(featureDir, 0755); err != nil {
		t.Fatal(err)
	}
	if intents != "" {
		os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(intents), 0644)
	}
	if dialogs != "" {
		os.WriteFile(filepath.Join(featureDir, "dialogs.md"), []byte(dialogs), 0644)
	}
	if surface != "" {
		os.WriteFile(filepath.Join(featureDir, "surface.md"), []byte(surface), 0644)
	}
	return featureDir
}

// writeBaseline saves a Baseline yaml file at the canonical location for slug.
func writeBaseline(t *testing.T, slug string, b Baseline) {
	t.Helper()
	if err := os.MkdirAll(config.BuildPath(slug), 0755); err != nil {
		t.Fatal(err)
	}
	data, _ := yaml.Marshal(b)
	if err := os.WriteFile(baselinePath(slug), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// writeBuildfile writes a minimal buildfile.yaml at the canonical location.
func writeBuildfile(t *testing.T, slug, content string) {
	t.Helper()
	if err := os.MkdirAll(config.BuildPath(slug), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(config.BuildPath(slug), "buildfile.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// runDiffForTest invokes the diff logic directly (bypassing cobra) and
// returns the structured output. We can't easily call runDiff because it
// prints JSON to stdout — instead we replicate the orchestration here so
// tests can assert on the struct.
func runDiffForTest(t *testing.T, slug string) diffOutput {
	t.Helper()
	featurePath := config.FeaturePath(slug)
	output := diffOutput{Feature: slug}

	var storedBaseline Baseline
	if blData, err := os.ReadFile(baselinePath(slug)); err == nil {
		if err := yaml.Unmarshal(blData, &storedBaseline); err != nil {
			t.Fatal(err)
		}
		if storedBaseline.Sources == nil {
			output.FirstBuild = true
		}
	} else {
		output.FirstBuild = true
	}
	stored := storedBaseline.Sources
	if stored == nil {
		stored = &HashedSources{}
	}

	currentIntents, _ := parser.ParseIntentsFile(filepath.Join(featurePath, "intents.md"))
	currentDialogs, _ := parser.ParseDialogsFile(filepath.Join(featurePath, "dialogs.md"))
	currentFragments, _ := parser.ParseSurfaceFile(filepath.Join(featurePath, "surface.md"))

	currentIntentHashes := make(map[string]string)
	for _, intent := range currentIntents {
		currentIntentHashes[intent.Slug] = hashIntentContent(intent)
	}
	currentDialogHashes := make(map[string]string)
	for _, dialog := range currentDialogs {
		currentDialogHashes[dialog.Slug] = hashDialogContent(dialog)
	}
	currentFragmentHashes := make(map[string]string)
	fragmentBySlug := make(map[string]*parser.Fragment)
	for i := range currentFragments {
		fragSlug := parser.Slugify(currentFragments[i].Name)
		currentFragmentHashes[fragSlug] = hashFragmentContent(currentFragments[i])
		fragmentBySlug[fragSlug] = &currentFragments[i]
	}

	output.Intents = diffStringMap(stored.Intents, currentIntentHashes)
	output.Dialogs = diffStringMap(stored.Dialogs, currentDialogHashes)
	output.Fragments = diffStringMap(stored.SurfaceFragments, currentFragmentHashes)

	buildfilePath := filepath.Join(config.BuildPath(slug), "buildfile.yaml")
	if fileExists(buildfilePath) {
		output.HasBuildfile = true
		if !output.FirstBuild {
			output.Components = computeComponentImpact(
				buildfilePath, slug,
				currentIntents, currentDialogs, fragmentBySlug,
				output.Intents, output.Dialogs, output.Fragments,
			)
			output.Sections = computeSectionDiff(buildfilePath, storedBaseline.BuildfileSections)
		}
	}
	return output
}

func TestDiff_FirstBuild(t *testing.T) {
	setupTestDir(t)

	intents := `## Do Something

**Goal**: Do the thing
**Persona**: User
`
	writeFeatureFiles(t, "my-feature", intents, "", "")

	out := runDiffForTest(t, "my-feature")
	if !out.FirstBuild {
		t.Error("expected first_build=true when no baseline exists")
	}
	if len(out.Intents.New) != 1 || out.Intents.New[0] != "do-something" {
		t.Errorf("expected one new intent 'do-something', got %v", out.Intents.New)
	}
	if out.HasBuildfile {
		t.Error("expected has_buildfile=false")
	}
}

func TestDiff_NoChanges(t *testing.T) {
	setupTestDir(t)

	intents := `## Do Something

**Goal**: Do the thing
**Persona**: User
`
	writeFeatureFiles(t, "my-feature", intents, "", "")

	// Save a baseline that matches current state.
	parsed, _ := parser.ParseIntentsFile(filepath.Join(config.FeaturePath("my-feature"), "intents.md"))
	b := Baseline{
		GeneratedAt: "2026-04-07T00:00:00Z",
		Intents:     map[string]IntentHash{},
		Sources:     &HashedSources{Intents: map[string]string{}},
	}
	for _, intent := range parsed {
		b.Intents[intent.Slug] = hashIntent(intent)
		b.Sources.Intents[intent.Slug] = hashIntentContent(intent)
	}
	writeBaseline(t, "my-feature", b)

	out := runDiffForTest(t, "my-feature")
	if out.FirstBuild {
		t.Error("expected first_build=false")
	}
	if len(out.Intents.Changed) != 0 || len(out.Intents.New) != 0 || len(out.Intents.Removed) != 0 {
		t.Errorf("expected no intent changes, got %+v", out.Intents)
	}
}

func TestDiff_ChangedIntent(t *testing.T) {
	setupTestDir(t)

	original := `## Do Something

**Goal**: Original goal
**Persona**: User
`
	writeFeatureFiles(t, "my-feature", original, "", "")

	// Save baseline from original
	parsed, _ := parser.ParseIntentsFile(filepath.Join(config.FeaturePath("my-feature"), "intents.md"))
	b := Baseline{
		GeneratedAt: "2026-04-07T00:00:00Z",
		Intents:     map[string]IntentHash{},
		Sources:     &HashedSources{Intents: map[string]string{}},
	}
	for _, intent := range parsed {
		b.Intents[intent.Slug] = hashIntent(intent)
		b.Sources.Intents[intent.Slug] = hashIntentContent(intent)
	}
	writeBaseline(t, "my-feature", b)

	// Modify the intent
	modified := `## Do Something

**Goal**: Updated goal
**Persona**: User
`
	os.WriteFile(filepath.Join(config.FeaturePath("my-feature"), "intents.md"), []byte(modified), 0644)

	out := runDiffForTest(t, "my-feature")
	if len(out.Intents.Changed) != 1 || out.Intents.Changed[0] != "do-something" {
		t.Errorf("expected do-something in Changed, got %v", out.Intents.Changed)
	}
}

func TestDiff_NewAndRemovedIntents(t *testing.T) {
	setupTestDir(t)

	// Baseline with intent-a and intent-b
	intentA := parser.Intent{Title: "Intent A", Slug: "intent-a", Goal: "Do A", Persona: "User"}
	intentB := parser.Intent{Title: "Intent B", Slug: "intent-b", Goal: "Do B", Persona: "User"}
	b := Baseline{
		GeneratedAt: "2026-04-07T00:00:00Z",
		Intents: map[string]IntentHash{
			"intent-a": hashIntent(intentA),
			"intent-b": hashIntent(intentB),
		},
		Sources: &HashedSources{
			Intents: map[string]string{
				"intent-a": hashIntentContent(intentA),
				"intent-b": hashIntentContent(intentB),
			},
		},
	}
	writeBaseline(t, "my-feature", b)

	// Current: intent-a unchanged, intent-b removed, intent-c new
	current := `## Intent A

**Goal**: Do A
**Persona**: User

---

## Intent C

**Goal**: Do C
**Persona**: User
`
	writeFeatureFiles(t, "my-feature", current, "", "")

	out := runDiffForTest(t, "my-feature")
	if len(out.Intents.New) != 1 || out.Intents.New[0] != "intent-c" {
		t.Errorf("expected intent-c in New, got %v", out.Intents.New)
	}
	if len(out.Intents.Removed) != 1 || out.Intents.Removed[0] != "intent-b" {
		t.Errorf("expected intent-b in Removed, got %v", out.Intents.Removed)
	}
	if len(out.Intents.Changed) != 0 {
		t.Errorf("expected no Changed intents, got %v", out.Intents.Changed)
	}
}

func TestDiff_ComponentImpact_DirtyViaIntent(t *testing.T) {
	setupTestDir(t)

	// Source state: one intent, one dialog, one fragment, one component
	intents := `## Do Something

**Goal**: Original goal
**Persona**: User
`
	dialogs := `### Do Something Flow

**Trigger**: User triggers it

User: Do it
System: Done
`
	surface := `## Action Button

**Shows**: A button
**Source**: @my-feature/do-something
`
	writeFeatureFiles(t, "my-feature", intents, dialogs, surface)

	// Save baseline
	parsed, _ := parser.ParseIntentsFile(filepath.Join(config.FeaturePath("my-feature"), "intents.md"))
	parsedDialogs, _ := parser.ParseDialogsFile(filepath.Join(config.FeaturePath("my-feature"), "dialogs.md"))
	parsedFragments, _ := parser.ParseSurfaceFile(filepath.Join(config.FeaturePath("my-feature"), "surface.md"))
	b := Baseline{
		GeneratedAt: "2026-04-07T00:00:00Z",
		Intents:     map[string]IntentHash{},
		Sources: &HashedSources{
			Intents:          map[string]string{},
			Dialogs:          map[string]string{},
			SurfaceFragments: map[string]string{},
		},
	}
	for _, intent := range parsed {
		b.Intents[intent.Slug] = hashIntent(intent)
		b.Sources.Intents[intent.Slug] = hashIntentContent(intent)
	}
	for _, dialog := range parsedDialogs {
		b.Sources.Dialogs[dialog.Slug] = hashDialogContent(dialog)
	}
	for _, frag := range parsedFragments {
		b.Sources.SurfaceFragments[parser.Slugify(frag.Name)] = hashFragmentContent(frag)
	}
	writeBaseline(t, "my-feature", b)

	// Buildfile with one component referencing the action-button fragment
	buildfile := `feature: my-feature
adapter: go-cli
components:
  do-something-action:
    source: "@my-feature/action-button"
`
	writeBuildfile(t, "my-feature", buildfile)

	// Modify the intent
	modified := `## Do Something

**Goal**: Updated goal
**Persona**: User
`
	os.WriteFile(filepath.Join(config.FeaturePath("my-feature"), "intents.md"), []byte(modified), 0644)

	out := runDiffForTest(t, "my-feature")
	if !out.HasBuildfile {
		t.Fatal("expected has_buildfile=true")
	}
	if len(out.Components.Dirty) != 1 {
		t.Fatalf("expected 1 dirty component, got %d: %+v", len(out.Components.Dirty), out.Components)
	}
	if out.Components.Dirty[0].Name != "do-something-action" {
		t.Errorf("expected dirty=do-something-action, got %s", out.Components.Dirty[0].Name)
	}
	// Should report the changed intent as the upstream cause
	found := false
	for _, src := range out.Components.Dirty[0].ChangedSources {
		if src == "intent:do-something" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected changed_sources to include intent:do-something, got %v",
			out.Components.Dirty[0].ChangedSources)
	}
}

func TestDiff_ComponentImpact_StableWhenUnrelatedChange(t *testing.T) {
	setupTestDir(t)

	// Two intents, two fragments, two components — change only intent A
	intents := `## Intent A

**Goal**: Goal A
**Persona**: User

---

## Intent B

**Goal**: Goal B
**Persona**: User
`
	surface := `## Fragment A

**Shows**: Stuff for A
**Source**: @my-feature/intent-a

---

## Fragment B

**Shows**: Stuff for B
**Source**: @my-feature/intent-b
`
	writeFeatureFiles(t, "my-feature", intents, "", surface)

	parsed, _ := parser.ParseIntentsFile(filepath.Join(config.FeaturePath("my-feature"), "intents.md"))
	parsedFragments, _ := parser.ParseSurfaceFile(filepath.Join(config.FeaturePath("my-feature"), "surface.md"))
	b := Baseline{
		GeneratedAt: "2026-04-07T00:00:00Z",
		Intents:     map[string]IntentHash{},
		Sources: &HashedSources{
			Intents:          map[string]string{},
			SurfaceFragments: map[string]string{},
		},
	}
	for _, intent := range parsed {
		b.Intents[intent.Slug] = hashIntent(intent)
		b.Sources.Intents[intent.Slug] = hashIntentContent(intent)
	}
	for _, frag := range parsedFragments {
		b.Sources.SurfaceFragments[parser.Slugify(frag.Name)] = hashFragmentContent(frag)
	}
	writeBaseline(t, "my-feature", b)

	buildfile := `feature: my-feature
adapter: go-cli
components:
  comp-a:
    source: "@my-feature/fragment-a"
  comp-b:
    source: "@my-feature/fragment-b"
`
	writeBuildfile(t, "my-feature", buildfile)

	// Modify only intent A
	modified := `## Intent A

**Goal**: Updated goal A
**Persona**: User

---

## Intent B

**Goal**: Goal B
**Persona**: User
`
	os.WriteFile(filepath.Join(config.FeaturePath("my-feature"), "intents.md"), []byte(modified), 0644)

	out := runDiffForTest(t, "my-feature")
	if len(out.Components.Dirty) != 1 || out.Components.Dirty[0].Name != "comp-a" {
		t.Errorf("expected only comp-a dirty, got %+v", out.Components.Dirty)
	}
	if len(out.Components.Stable) != 1 || out.Components.Stable[0] != "comp-b" {
		t.Errorf("expected only comp-b stable, got %+v", out.Components.Stable)
	}
}

func TestDiff_SectionChanged(t *testing.T) {
	dir := setupTestDir(t)

	intents := `## Do Something

**Goal**: Do the thing
**Persona**: User
`
	writeFeatureFiles(t, "my-feature", intents, "", "")

	// Original buildfile with one model
	originalBuildfile := `feature: my-feature
adapter: go-cli
models:
  Task:
    properties:
      text:
        type: string
routes:
  - path: "do"
    page: cli
components:
  do-comp:
    source: "@my-feature/do-something"
`
	writeBuildfile(t, "my-feature", originalBuildfile)

	// Save baseline with source hashes AND section hashes via saveBuildState.
	// We need a source root with at least one marker file for saveBuildState.
	sourceRoot := filepath.Join(dir, "cmd", "my-feature")
	writeMarkedFile(t, filepath.Join(sourceRoot, "do.go"),
		"my-feature", "do-comp", "func Do() {}")

	err := saveBuildStateForFeature("my-feature", sourceRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Verify sections are reported as stable when nothing changed
	out := runDiffForTest(t, "my-feature")
	if out.Sections == nil {
		t.Fatal("expected sections in diff output")
	}
	if out.Sections["models"] != "stable" {
		t.Errorf("sections.models = %q, want stable", out.Sections["models"])
	}
	if out.Sections["routes"] != "stable" {
		t.Errorf("sections.routes = %q, want stable", out.Sections["routes"])
	}

	// Now modify the models section in the buildfile (add a property)
	modifiedBuildfile := `feature: my-feature
adapter: go-cli
models:
  Task:
    properties:
      text:
        type: string
      priority:
        type: string
routes:
  - path: "do"
    page: cli
components:
  do-comp:
    source: "@my-feature/do-something"
`
	writeBuildfile(t, "my-feature", modifiedBuildfile)

	out = runDiffForTest(t, "my-feature")
	if out.Sections["models"] != "changed" {
		t.Errorf("sections.models = %q, want changed after adding property", out.Sections["models"])
	}
	if out.Sections["routes"] != "stable" {
		t.Errorf("sections.routes = %q, want stable (routes didn't change)", out.Sections["routes"])
	}
}

func TestDiff_ComponentImpact_RemovedFragment(t *testing.T) {
	setupTestDir(t)

	intents := `## Intent A

**Goal**: Goal A
**Persona**: User
`
	originalSurface := `## Fragment A

**Shows**: Stuff
**Source**: @my-feature/intent-a
`
	writeFeatureFiles(t, "my-feature", intents, "", originalSurface)

	parsed, _ := parser.ParseIntentsFile(filepath.Join(config.FeaturePath("my-feature"), "intents.md"))
	parsedFragments, _ := parser.ParseSurfaceFile(filepath.Join(config.FeaturePath("my-feature"), "surface.md"))
	b := Baseline{
		GeneratedAt: "2026-04-07T00:00:00Z",
		Intents:     map[string]IntentHash{},
		Sources: &HashedSources{
			Intents:          map[string]string{},
			SurfaceFragments: map[string]string{},
		},
	}
	for _, intent := range parsed {
		b.Intents[intent.Slug] = hashIntent(intent)
		b.Sources.Intents[intent.Slug] = hashIntentContent(intent)
	}
	for _, frag := range parsedFragments {
		b.Sources.SurfaceFragments[parser.Slugify(frag.Name)] = hashFragmentContent(frag)
	}
	writeBaseline(t, "my-feature", b)

	buildfile := `feature: my-feature
adapter: go-cli
components:
  comp-a:
    source: "@my-feature/fragment-a"
`
	writeBuildfile(t, "my-feature", buildfile)

	// Replace surface with empty content (fragment removed)
	emptySurface := `# My Feature — Surface
`
	os.WriteFile(filepath.Join(config.FeaturePath("my-feature"), "surface.md"), []byte(emptySurface), 0644)

	out := runDiffForTest(t, "my-feature")
	if len(out.Components.Removed) != 1 || out.Components.Removed[0] != "comp-a" {
		t.Errorf("expected comp-a in Removed, got %+v", out.Components)
	}
}
