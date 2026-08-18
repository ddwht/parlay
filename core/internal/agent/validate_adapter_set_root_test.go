// parlay-feature: parlay-tool/multi-adapter
// parlay-component: adapter-kind-discriminator
// parlay-artifact: test
//
// Encodes the reported defect: three of the four bundled presets derive
// presentation paths that drop the framework's source directory, and nothing
// noticed. This walks the presets parlay actually ships — not a fixture — so
// the finding cannot come back by way of a preset nobody re-checked.

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ddwht/parlay/core/internal/parser"
)

// presetExpectation records whether a preset's presentation slot loses the
// framework's source directory to the root override.
//
// All four are false now. Three of them were true — react-nest-prisma,
// angular-nest-prisma and angular-clarity-only were the reported defect, and
// react-antd-only passed only because its `root: src` happened to equal
// react-antd's `source-root: "src/"`. Every bundled adapter now declares
// project-root, so root: substitutes for the project location and leaves the
// framework directory alone; there is nothing left to lose.
//
// The map stays rather than collapsing to a loop, because the interesting
// assertion is per-preset and the next adapter added here should have to
// state which column it belongs in.
var presetExpectations = map[string]bool{
	"react-nest-prisma":    false, // project-root apps/web + source-root src/
	"angular-nest-prisma":  false, // project-root apps/web + source-root src/app/
	"angular-clarity-only": false, // project-root "."      + source-root src/app/
	"react-antd-only":      false, // project-root "."      + source-root src/
}

// stagePreset copies a bundled preset and the adapters it pins into a temp
// project, so ValidateAdapterSet can resolve them the way a real project does.
func stagePreset(t *testing.T, preset string) string {
	t.Helper()
	root := t.TempDir()
	parlayDir := filepath.Join(root, ".parlay")
	adaptersDir := filepath.Join(parlayDir, "adapters")
	if err := os.MkdirAll(adaptersDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	src := filepath.Join("../embedded/presets", preset+".adapter-set.yaml")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read preset %s: %v", preset, err)
	}
	dst := filepath.Join(parlayDir, "adapter-set.yaml")
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("write adapter-set: %v", err)
	}

	bundled, err := filepath.Glob("../embedded/adapters/*.adapter.yaml")
	if err != nil {
		t.Fatalf("glob adapters: %v", err)
	}
	for _, a := range bundled {
		b, err := os.ReadFile(a)
		if err != nil {
			t.Fatalf("read %s: %v", a, err)
		}
		if err := os.WriteFile(filepath.Join(adaptersDir, filepath.Base(a)), b, 0o644); err != nil {
			t.Fatalf("write adapter: %v", err)
		}
	}
	return dst
}

func TestBundledPresets_RootOverrideLossiness(t *testing.T) {
	for preset, wantLossy := range presetExpectations {
		t.Run(preset, func(t *testing.T) {
			setPath := stagePreset(t, preset)
			content, err := os.ReadFile(setPath)
			if err != nil {
				t.Fatalf("read staged adapter-set: %v", err)
			}

			outcomes := ValidateAdapterSet(ModeAuthoring, setPath, content)
			gotLossy := findCode(outcomes, "adapter-root-override-lossy")

			// Nothing else should be wrong with a preset parlay ships.
			for _, o := range outcomes {
				if o.Code != "adapter-root-override-lossy" && o.Severity == SeverityError {
					t.Errorf("unrelated failure in bundled preset: [%s] %s", o.Code, o.Message)
				}
			}

			switch {
			case wantLossy && !gotLossy:
				t.Errorf("preset %s drops the framework source directory from every derived "+
					"presentation path, and the validator did not say so. This is the reported "+
					"defect; the diagnostic that catches it has regressed.\noutcomes: %+v", preset, outcomes)
			case !wantLossy && gotLossy:
				t.Errorf("preset %s composes losslessly but was flagged. A false positive here "+
					"trains people to ignore the code.\noutcomes: %+v", preset, outcomes)
			}
		})
	}
}

// TestDogfoodedAdapterSetsValidateClean is the companion to
// TestBundledAdaptersValidateClean, which checks adapter files but never the
// topology that pins them.
//
// It found a live one on its first run. studio/.parlay/adapter-set.yaml pinned
// `root: internal/editor/ui` against an adapter still declaring
// `source-root: "studio/internal/ui/src/"` — a directory that had not existed
// since the studio module was absorbed. The commit immediately before this one
// was titled "Point the editor's adapter at where its code actually lives, and
// fix stale prose"; it corrected the adapter-set and the Go adapter and missed
// this file. A deliberate hunt for exactly this bug, by someone who knew it was
// there, found one of two. That is the argument for checking it mechanically.
func TestDogfoodedAdapterSetsValidateClean(t *testing.T) {
	sets, err := filepath.Glob("../../../*/.parlay/adapter-set.yaml")
	if err != nil {
		t.Fatalf("glob adapter-sets: %v", err)
	}
	if len(sets) == 0 {
		t.Skip("no dogfooded adapter-sets in this checkout")
	}
	for _, s := range sets {
		content, err := os.ReadFile(s)
		if err != nil {
			t.Fatalf("read %s: %v", s, err)
		}
		for _, o := range ValidateAdapterSet(ModeBuild, s, content) {
			if o.Severity == SeverityError {
				t.Errorf("%s\n    [%s] %s", s, o.Code, o.Message)
			}
		}
	}
}

// TestRootOverrideLossy_QuietWhenTheSubstitutionLosesNothing pins the shape of
// the rule rather than the presets that currently exercise it. The backend
// case — source-root and root naming the same project location — must stay
// silent, or the diagnostic fires on every correct four-slot project.
func TestRootOverrideLossy_QuietWhenTheSubstitutionLosesNothing(t *testing.T) {
	cases := []struct {
		name       string
		sourceRoot string
		root       string
		wantLossy  bool
	}{
		{"identical", "apps/api", "apps/api", false},
		{"trailing slash only", "src/", "src", false},
		{"leading dot-slash only", "./src", "src", false},
		{"adapter declares nothing", "", "apps/web", false},
		{"target declares nothing", "src/", "", false},
		{"framework dir replaced by project location", "src/", "apps/web", true},
		{"nested framework dir truncated", "src/app/", "src", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var shape adapterFileShape
			shape.FileConventions.SourceRoot = c.sourceRoot
			target := parser.AdapterSetTarget{Adapter: "test-adapter", Root: c.root}
			got := len(checkRootOverrideIsLossless(ModeAuthoring, "presentation", target, shape)) > 0
			if got != c.wantLossy {
				t.Errorf("source-root %q + root %q: lossy=%v, want %v", c.sourceRoot, c.root, got, c.wantLossy)
			}
		})
	}
}
