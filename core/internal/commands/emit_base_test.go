// parlay-feature: parlay-tool/multi-adapter
// parlay-component: adapter-emit-base-resolution
// parlay-artifact: test

package commands

import "testing"

// TestEmitBase_SplitModel walks the compositions the bundled adapters actually
// produce. The presentation rows are the reported defect: under the old
// one-field model each of them dropped the framework's source directory,
// because the target root replaced the whole of source-root instead of only
// the project location in front of it.
func TestEmitBase_SplitModel(t *testing.T) {
	cases := []struct {
		name        string
		projectRoot string
		sourceRoot  string
		targetRoot  string
		want        string
	}{
		// Presentation under a multi-target topology — the broken cases.
		{"react-antd under apps/web", ".", "src/", "apps/web", "apps/web/src"},
		{"angular under apps/web", ".", "src/app/", "apps/web", "apps/web/src/app"},

		// The same adapters with no adapter-set at all. These must be
		// unchanged from the old behaviour: a single-package React app has
		// always emitted to src/, and an upgrade that moved it would be a
		// regression dressed as a fix.
		{"react-antd single-target", ".", "src/", "", "src"},
		{"angular single-target", ".", "src/app/", "", "src/app"},

		// The -only presets now pin root to the project location.
		{"react-antd-only", ".", "src/", ".", "src"},
		{"angular-clarity-only", ".", "src/app/", ".", "src/app"},

		// Backend slots: project location in project-root, framework dir in
		// source-root, and the topology substitutes only the former.
		{"nestjs under apps/api", "apps/api", "src", "apps/api", "apps/api/src"},
		{"nestjs relocated", "apps/api", "src", "services/api", "services/api/src"},
		{"prisma has no source dir", "apps/api", ".", "apps/api", "apps/api"},
		{"go-cli single-target", ".", "cmd/", "", "cmd"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := emitBase(c.projectRoot, c.sourceRoot, c.targetRoot); got != c.want {
				t.Errorf("emitBase(%q, %q, %q) = %q, want %q",
					c.projectRoot, c.sourceRoot, c.targetRoot, got, c.want)
			}
		})
	}
}

// TestEmitBase_LegacyIsBugCompatible pins the old behaviour for adapters that
// have not opted in.
//
// Third-party and onboarded adapters declare no project-root, and an upgrade
// must not silently relocate their output — including where the old behaviour
// was wrong. The wrongness is reported by adapter-root-override-lossy instead,
// which is a diagnostic the author can act on rather than a surprise diff in
// someone's repository.
func TestEmitBase_LegacyIsBugCompatible(t *testing.T) {
	// The reported defect, preserved exactly: root replaces the whole
	// source-root, so src/ is gone.
	if got := emitBase("", "src/", "apps/web"); got != "apps/web" {
		t.Errorf("legacy react-antd under apps/web = %q, want %q (unchanged old behaviour)", got, "apps/web")
	}
	// Legacy backend, where replacement was always lossless.
	if got := emitBase("", "apps/api", "apps/api"); got != "apps/api" {
		t.Errorf("legacy nestjs = %q, want %q", got, "apps/api")
	}
	// Legacy single-target: source-root used as-is.
	if got := emitBase("", "src/", ""); got != "src" {
		t.Errorf("legacy single-target = %q, want %q", got, "src")
	}
	// No roots at all — must stay empty, not become "." and prefix every
	// derived path with "./".
	if got := emitBase("", "", ""); got != "" {
		t.Errorf("no roots = %q, want empty", got)
	}
}
