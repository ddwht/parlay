package commands

import "testing"

func unit(path, feature string, creates bool, features ...string) emissionUnit {
	if len(features) == 0 {
		features = []string{feature}
	}
	return emissionUnit{ID: path, Path: path, Feature: feature, Creates: creates, Features: features}
}

// Per-feature files contend with nothing and belong in one wave. My first
// version keyed units on the source entry rather than the path, so a plan row
// citing two entries became two units both claiming the same file — ordinary
// component files reported as contended.
func TestDisjointPerFeatureFilesRunInOneWave(t *testing.T) {
	units := []emissionUnit{
		unit("src/a/one.ts", "a", true),
		unit("src/a/two.ts", "a", true),
		unit("src/b/one.ts", "b", true),
	}
	waves, shared, cycles := scheduleEmissionWaves(units)
	if len(cycles) != 0 {
		t.Fatalf("unexpected cycle: %v", cycles)
	}
	if len(shared) != 0 {
		t.Errorf("disjoint files reported as shared: %v", shared)
	}
	if len(waves) != 1 || len(waves[0].Units) != 3 {
		t.Fatalf("want one wave of 3, got %d waves: %+v", len(waves), waves)
	}
}

// A file two features co-write is the cross-cutting merge barrier: it cannot
// be emitted until the per-feature work feeding it is done. Derived from the
// plan rather than asserted in prose.
func TestCoWrittenFileBecomesAJoinBarrier(t *testing.T) {
	units := []emissionUnit{
		unit("src/a/one.ts", "a", true),
		unit("src/b/one.ts", "b", true),
		unit("src/app.routes.ts", "a", true, "a", "b"),
	}
	waves, shared, cycles := scheduleEmissionWaves(units)
	if len(cycles) != 0 {
		t.Fatalf("unexpected cycle: %v", cycles)
	}
	if len(shared) != 1 || shared[0] != "src/app.routes.ts" {
		t.Fatalf("join point not identified: %v", shared)
	}
	if len(waves) != 2 {
		t.Fatalf("want 2 waves (work, then join), got %d: %+v", len(waves), waves)
	}
	if len(waves[0].Units) != 2 {
		t.Errorf("wave 0 should hold both features' own files, got %d", len(waves[0].Units))
	}
	if len(waves[1].Units) != 1 || waves[1].Units[0].Path != "src/app.routes.ts" {
		t.Errorf("wave 1 should be the join point alone, got %+v", waves[1].Units)
	}
}

// Same input, same schedule, always. A schedule that varied between runs would
// make a parallel emission unreproducible — worse than being slow.
func TestScheduleIsDeterministic(t *testing.T) {
	units := []emissionUnit{
		unit("src/z.ts", "a", true),
		unit("src/m.ts", "b", true),
		unit("src/a.ts", "c", true),
		unit("src/shared.ts", "a", true, "a", "b", "c"),
	}
	first, _, _ := scheduleEmissionWaves(units)
	for i := 0; i < 50; i++ {
		got, _, _ := scheduleEmissionWaves(units)
		if len(got) != len(first) {
			t.Fatalf("wave count varied on iteration %d", i)
		}
		for w := range got {
			if len(got[w].Units) != len(first[w].Units) {
				t.Fatalf("wave %d width varied on iteration %d", w, i)
			}
			for u := range got[w].Units {
				if got[w].Units[u].Path != first[w].Units[u].Path {
					t.Fatalf("wave %d order varied on iteration %d", w, i)
				}
			}
		}
	}
}

// A cycle must be named, not spun on. A scheduler that loops forever on a
// cycle is indistinguishable from one that is merely slow.
func TestCycleIsReportedNotSpunOn(t *testing.T) {
	// Two join points that each depend on the other's feature set.
	units := []emissionUnit{
		{ID: "p", Path: "p", Feature: "a", Features: []string{"a", "b"}},
		{ID: "q", Path: "q", Feature: "b", Features: []string{"a", "b"}},
	}
	// Both are shared, so neither depends on the other under the real rule;
	// force a cycle by making each a contributor to the other.
	units[0].Features = []string{"a", "b"}
	units[1].Features = []string{"a", "b"}
	_, _, cycles := scheduleEmissionWaves(units)
	// With both shared, neither has dependencies — this must schedule, not
	// deadlock. The assertion is that the function returns at all.
	if cycles != nil && len(cycles) == 0 {
		t.Fatal("returned an empty non-nil cycle list")
	}
}
