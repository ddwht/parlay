package config

// parlay-feature: initiatives
// parlay-component: directory-classification-validation
// parlay-artifact: test

import (
	"os"
	"path/filepath"
	"testing"
)

func setupConfigTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestClassifyDir_Feature(t *testing.T) {
	setupConfigTestDir(t)
	dir := filepath.Join(SpecDir, IntentsDir, "my-feature")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "intents.md"), []byte("# Feature\n"), 0644)

	cls, err := ClassifyDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cls != DirClassFeature {
		t.Errorf("expected DirClassFeature, got %d", cls)
	}
}

func TestClassifyDir_Initiative(t *testing.T) {
	setupConfigTestDir(t)
	initDir := filepath.Join(SpecDir, IntentsDir, "auth-overhaul")
	childDir := filepath.Join(initDir, "login")
	os.MkdirAll(childDir, 0755)
	os.WriteFile(filepath.Join(childDir, "intents.md"), []byte("# Login\n"), 0644)

	cls, err := ClassifyDir(initDir)
	if err != nil {
		t.Fatal(err)
	}
	if cls != DirClassInitiative {
		t.Errorf("expected DirClassInitiative, got %d", cls)
	}
}

func TestClassifyDir_Deferred(t *testing.T) {
	setupConfigTestDir(t)
	dir := filepath.Join(SpecDir, IntentsDir, "empty-dir")
	os.MkdirAll(dir, 0755)

	cls, err := ClassifyDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cls != DirClassDeferred {
		t.Errorf("expected DirClassDeferred, got %d", cls)
	}
}

func TestClassifyDir_DeferredWithReadme(t *testing.T) {
	setupConfigTestDir(t)
	dir := filepath.Join(SpecDir, IntentsDir, "with-readme")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Why\n"), 0644)

	cls, err := ClassifyDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cls != DirClassDeferred {
		t.Errorf("expected DirClassDeferred (README is not a classification signal), got %d", cls)
	}
}

func TestClassifyDir_Authored(t *testing.T) {
	setupConfigTestDir(t)
	dir := filepath.Join(SpecDir, IntentsDir, "geometry-engine")
	os.MkdirAll(dir, 0755)
	// A unit carries intents.md exactly like a feature does — the
	// declaration is the only thing that tells them apart, so this case
	// is the one that fails if the authored probe is ordered after the
	// isFeature branch rather than before it.
	os.WriteFile(filepath.Join(dir, "intents.md"), []byte("# Engine\n"), 0644)
	os.WriteFile(filepath.Join(dir, AuthoredFile), []byte("unit: geometry-engine\n"), 0644)

	cls, err := ClassifyDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cls != DirClassAuthored {
		t.Errorf("expected DirClassAuthored, got %d", cls)
	}
}

func TestClassifyDir_AuthoredWithChildFeaturesIsHybrid(t *testing.T) {
	setupConfigTestDir(t)
	dir := filepath.Join(SpecDir, IntentsDir, "confused")
	childDir := filepath.Join(dir, "child-feature")
	os.MkdirAll(childDir, 0755)
	os.WriteFile(filepath.Join(dir, AuthoredFile), []byte("unit: confused\n"), 0644)
	os.WriteFile(filepath.Join(childDir, "intents.md"), []byte("# Child\n"), 0644)

	if _, err := ClassifyDir(dir); err == nil {
		t.Error("expected hybrid error for a unit declaration above child features, got nil")
	}
}

func TestScanTree_SeparatesUnitsFromFeatures(t *testing.T) {
	dir := setupConfigTestDir(t)
	root := filepath.Join(dir, SpecDir, IntentsDir)

	writeFeature := func(path string) {
		os.MkdirAll(path, 0755)
		os.WriteFile(filepath.Join(path, "intents.md"), []byte("# F\n"), 0644)
	}
	writeUnit := func(path string) {
		writeFeature(path)
		os.WriteFile(filepath.Join(path, AuthoredFile), []byte("unit: u\n"), 0644)
	}

	writeFeature(filepath.Join(root, "checkout"))
	writeUnit(filepath.Join(root, "geometry-engine"))
	writeFeature(filepath.Join(root, "auth-overhaul", "login"))
	writeUnit(filepath.Join(root, "auth-overhaul", "crypto-core"))

	features, err := ScanFeatureTree(root)
	if err != nil {
		t.Fatal(err)
	}
	units, err := ScanUnitTree(root)
	if err != nil {
		t.Fatal(err)
	}

	assertSet := func(label string, got, want []string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: got %v, want %v", label, got, want)
		}
		seen := map[string]bool{}
		for _, g := range got {
			seen[g] = true
		}
		for _, w := range want {
			if !seen[w] {
				t.Errorf("%s: missing %q (got %v)", label, w, got)
			}
		}
	}

	// A unit must not appear as a feature at either depth: every
	// AllFeatures caller assumes a buildfile and a handoff twin exist.
	assertSet("features", features, []string{"checkout", "auth-overhaul/login"})
	assertSet("units", units, []string{"geometry-engine", "auth-overhaul/crypto-core"})
}

func TestClassifyDir_HybridError(t *testing.T) {
	setupConfigTestDir(t)
	dir := filepath.Join(SpecDir, IntentsDir, "hybrid")
	childDir := filepath.Join(dir, "child-feature")
	os.MkdirAll(childDir, 0755)
	os.WriteFile(filepath.Join(dir, "intents.md"), []byte("# Hybrid\n"), 0644)
	os.WriteFile(filepath.Join(childDir, "intents.md"), []byte("# Child\n"), 0644)

	_, err := ClassifyDir(dir)
	if err == nil {
		t.Error("expected hybrid directory error, got nil")
	}
}

func TestCheckSlugUniqueness_NoDuplicates(t *testing.T) {
	setupConfigTestDir(t)
	parent := filepath.Join(SpecDir, IntentsDir)
	os.MkdirAll(filepath.Join(parent, "feature-a"), 0755)
	os.MkdirAll(filepath.Join(parent, "feature-b"), 0755)

	err := CheckSlugUniqueness(parent)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestCheckSlugUniqueness_DetectsDuplicates(t *testing.T) {
	setupConfigTestDir(t)
	parent := filepath.Join(SpecDir, IntentsDir)
	os.MkdirAll(filepath.Join(parent, "password-reset"), 0755)
	os.MkdirAll(filepath.Join(parent, "password_reset"), 0755)

	err := CheckSlugUniqueness(parent)
	if err == nil {
		t.Error("expected duplicate slug error, got nil")
	}
}

func TestScanFeatureTree_MixedOrphansAndInitiatives(t *testing.T) {
	setupConfigTestDir(t)
	intentsRoot := filepath.Join(SpecDir, IntentsDir)

	orphan := filepath.Join(intentsRoot, "standalone")
	os.MkdirAll(orphan, 0755)
	os.WriteFile(filepath.Join(orphan, "intents.md"), []byte("# Standalone\n"), 0644)

	nested := filepath.Join(intentsRoot, "auth-overhaul", "login")
	os.MkdirAll(nested, 0755)
	os.WriteFile(filepath.Join(nested, "intents.md"), []byte("# Login\n"), 0644)

	result, err := ScanFeatureTree(intentsRoot)
	if err != nil {
		t.Fatal(err)
	}

	found := map[string]bool{}
	for _, id := range result {
		found[id] = true
	}
	if !found["standalone"] {
		t.Error("orphan feature 'standalone' missing from results")
	}
	if !found["auth-overhaul/login"] {
		t.Error("nested feature 'auth-overhaul/login' missing from results")
	}
}

func TestScanFeatureTree_SubInitiativeError(t *testing.T) {
	setupConfigTestDir(t)
	intentsRoot := filepath.Join(SpecDir, IntentsDir)

	deep := filepath.Join(intentsRoot, "a", "b", "c")
	os.MkdirAll(deep, 0755)
	os.WriteFile(filepath.Join(filepath.Join(intentsRoot, "a", "b"), "intents.md"), []byte("# B\n"), 0644)
	os.WriteFile(filepath.Join(deep, "intents.md"), []byte("# C\n"), 0644)

	_, err := ScanFeatureTree(intentsRoot)
	if err == nil {
		t.Error("expected sub-initiative error, got nil")
	}
}

func TestScanFeatureTree_DeferredSkipped(t *testing.T) {
	setupConfigTestDir(t)
	intentsRoot := filepath.Join(SpecDir, IntentsDir)

	os.MkdirAll(filepath.Join(intentsRoot, "empty-initiative"), 0755)

	feature := filepath.Join(intentsRoot, "real-feature")
	os.MkdirAll(feature, 0755)
	os.WriteFile(filepath.Join(feature, "intents.md"), []byte("# Real\n"), 0644)

	result, err := ScanFeatureTree(intentsRoot)
	if err != nil {
		t.Fatal(err)
	}

	for _, id := range result {
		if id == "empty-initiative" {
			t.Error("deferred directory should be invisible in enumeration")
		}
	}
	if len(result) != 1 || result[0] != "real-feature" {
		t.Errorf("expected [real-feature], got %v", result)
	}
}
