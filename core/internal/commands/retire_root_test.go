// parlay-feature: parlay-tool/root-retirement
// parlay-component: cross-cutting/retirement-target-and-destination-preconditions
// parlay-artifact: test
//
// Suites: retirement-target-and-destination-preconditions and
// retirement-authorization-preview-and-unattended-runs. Shared fixture
// helpers for every root-retirement suite live here too.

package commands

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ddwht/parlay/core/internal/config"
)

// resetRetirementState pins every package-level seam the retirement
// commands read, restoring it when the test ends.
func resetRetirementState(t *testing.T) {
	t.Helper()
	prevDisp, prevPrev, prevNI := retireRootDispositions, retireRootPreview, retireRootNonInteractive
	prevTTY, prevHook, prevCheck := retireRootTTYOverride, retirementHook, retiredRootsCheck
	retireRootDispositions, retireRootPreview, retireRootNonInteractive = "", false, false
	retireRootTTYOverride, retirementHook, retiredRootsCheck = nil, nil, false
	t.Cleanup(func() {
		retireRootDispositions, retireRootPreview, retireRootNonInteractive = prevDisp, prevPrev, prevNI
		retireRootTTYOverride, retirementHook, retiredRootsCheck = prevTTY, prevHook, prevCheck
	})
}

// interactiveTTY pins the interactive probe to "a person is present".
func interactiveTTY(t *testing.T) {
	t.Helper()
	yes := true
	retireRootTTYOverride = &yes
}

// makeRetirementParent creates a parent root in a temp dir: config.yaml
// with an agent, an empty spec/intents tree, and no children yet.
func makeRetirementParent(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	makeProjectRoot(t, dir)
	return dir
}

// addRetirementChild registers a child root at relPath with the given
// features (each gets founding documents so the directory classifies as
// a feature).
func addRetirementChild(t *testing.T, parent, name, relPath string, features ...string) config.Root {
	t.Helper()
	childPath := filepath.Join(parent, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Join(childPath, config.ParlayDir), 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("ai-agent: Generic\nparent: " + strings.Repeat("../", strings.Count(relPath, "/")+1) + "\n")
	if err := os.WriteFile(filepath.Join(childPath, config.ParlayDir, config.ConfigFile), body, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, f := range features {
		writeChildFeature(t, childPath, f)
	}
	idx, err := config.LoadRootsIndex(parent)
	if err != nil {
		t.Fatal(err)
	}
	child := config.Root{
		Name:         name,
		RelativePath: filepath.FromSlash(relPath),
		Path:         childPath,
		ParentPath:   parent,
		Kind:         config.RootKindChild,
	}
	if _, err := config.AppendRootToIndex(idx, child); err != nil {
		t.Fatal(err)
	}
	return child
}

// writeChildFeature writes founding documents for one feature under the
// given root.
func writeChildFeature(t *testing.T, rootPath, slug string) string {
	t.Helper()
	featDir := filepath.Join(rootPath, config.SpecDir, config.IntentsDir, filepath.FromSlash(slug))
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	intents := "# Feature\n\n## Do The Thing\n\n**Goal**: g.\n**Persona**: p.\n"
	if err := os.WriteFile(filepath.Join(featDir, "intents.md"), []byte(intents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featDir, "dialogs.md"), []byte("# Dialogs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return featDir
}

// retireCmd builds a cobra command whose context resolves the parent as
// the active root, with the given stdin and a captured stdout.
func retireCmd(t *testing.T, parent, stdin string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	idx, err := config.LoadRootsIndex(parent)
	if err != nil {
		t.Fatal(err)
	}
	root := config.Root{Name: filepath.Base(parent), Path: parent, Kind: config.RootKindParent}
	cmd := withCtx(t, root, idx)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(strings.NewReader(stdin))
	return cmd, &buf
}

// writeDispositionsFile writes an operator-authored record and returns
// its path (outside the parent tree, so it never shows up in snapshots
// or sweeps).
func writeDispositionsFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "dispositions.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// deliveredDispositions builds a record declaring every named feature
// delivered-and-deleted with a rationale.
func deliveredDispositions(features ...string) string {
	var b strings.Builder
	b.WriteString("dispositions:\n")
	for _, f := range features {
		fmt.Fprintf(&b, "  - feature: %s\n    term: delivered-and-deleted\n    rationale: shipped in v1 and later removed\n", f)
	}
	return b.String()
}

// treeSnapshot hashes every file under root (path -> content hash +
// symlink targets), for byte-identical before/after comparisons.
func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			snap[rel+"/"] = "dir"
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			target, linkErr := os.Readlink(path)
			if linkErr != nil {
				return linkErr
			}
			snap[rel] = "link:" + target
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			// An unreadable fixture member (a chmod-000 file planted by
			// the test) still participates in the comparison by identity.
			snap[rel] = "unreadable"
			return nil
		}
		snap[rel] = fmt.Sprintf("%x", sha256.Sum256(data))
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return snap
}

func assertTreeUnchanged(t *testing.T, before, after map[string]string) {
	t.Helper()
	var diffs []string
	for k, v := range before {
		if after[k] != v {
			diffs = append(diffs, "changed or removed: "+k)
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			diffs = append(diffs, "added: "+k)
		}
	}
	sort.Strings(diffs)
	if len(diffs) > 0 {
		t.Fatalf("project tree is not byte-identical:\n  %s", strings.Join(diffs, "\n  "))
	}
}

// recordEvents replaces the retirement hook with an order recorder.
func recordEvents(t *testing.T) *[]string {
	t.Helper()
	var events []string
	retirementHook = func(event string) error {
		events = append(events, event)
		return nil
	}
	return &events
}

// --- Suite 1: retirement target and destination preconditions ----------

func TestRetireRoot_SingleChildResolves_UnknownRefusedEnumerating(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	addRetirementChild(t, parent, "old", "old", "alpha")
	addRetirementChild(t, parent, "lib", "lib", "beta")

	// Exactly one registered child resolves.
	retireRootPreview = true
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	cmd, out := retireCmd(t, parent, "")
	if err := runRetireRoot(cmd, []string{"old"}); err != nil {
		t.Fatalf("a target naming exactly one registered child must resolve: %v", err)
	}
	if !strings.Contains(out.String(), "Retirement preview: old") {
		t.Errorf("preview should report the resolved root; got:\n%s", out.String())
	}

	// A target naming none is refused, enumerating the registered roots.
	cmd, _ = retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{"ghost"})
	if err == nil {
		t.Fatal("a target naming no registered child must refuse")
	}
	for _, name := range []string{"old", "lib"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("the refusal must enumerate registered child %q; got: %v", name, err)
		}
	}
}

func TestRetireRoot_MultipleCandidatesRefusedWithoutSelecting(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	// "web" matches child A by name and child B by relative path.
	addRetirementChild(t, parent, "web", "frontend/web", "alpha")
	addRetirementChild(t, parent, "webapp", "web", "beta")

	events := recordEvents(t)
	cmd, _ := retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{"web"})
	if err == nil {
		t.Fatal("a target matching more than one candidate must refuse")
	}
	if !strings.Contains(err.Error(), "web") || !strings.Contains(err.Error(), "webapp") {
		t.Errorf("the refusal should name both candidates; got: %v", err)
	}
	if len(*events) != 0 {
		t.Errorf("no candidate may be selected — nothing should have run; events: %v", *events)
	}
}

func TestRetireRoot_UnregisteredRootConfigurationRefused(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	addRetirementChild(t, parent, "old", "old", "alpha")

	orphan := filepath.Join(parent, "orphan", config.ParlayDir)
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphan, config.ConfigFile), []byte("ai-agent: Generic\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd, _ := retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{"orphan"})
	if err == nil {
		t.Fatal("an unregistered directory carrying root configuration must refuse")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("the refusal should state the directory is not registered; got: %v", err)
	}
}

func TestRetireRoot_ParentRootIsNeverAValidTarget(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	addRetirementChild(t, parent, "old", "old", "alpha")

	cmd, _ := retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{filepath.Base(parent)})
	if err == nil {
		t.Fatal("the parent root must never resolve as a retirement target")
	}
	if !strings.Contains(err.Error(), "parent root") {
		t.Errorf("the refusal should say why the parent cannot retire; got: %v", err)
	}
}

func TestRetireRoot_ExistingDestinationRefusesBeforeContentsAreRead(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	addRetirementChild(t, parent, "old", "old", "alpha")
	if err := os.MkdirAll(retirementDestination(parent, "old"), 0o755); err != nil {
		t.Fatal(err)
	}

	events := recordEvents(t)
	cmd, _ := retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil {
		t.Fatal("an existing destination must refuse the run")
	}
	msg := err.Error()
	if !strings.Contains(msg, "earlier retirement") || !strings.Contains(msg, "unrelated content") {
		t.Errorf("the refusal must name both possible explanations; got: %v", err)
	}
	if len(*events) != 0 {
		t.Errorf("zero reads of the retiring root's contents may occur; events: %v", *events)
	}
}

func TestRetireRoot_NoContentAccessBeforePreconditionsSucceed(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	addRetirementChild(t, parent, "old", "old", "alpha")

	events := recordEvents(t)
	cmd, _ := retireCmd(t, parent, "")
	if err := runRetireRoot(cmd, []string{"ghost"}); err == nil {
		t.Fatal("resolution must fail for this fixture")
	}
	if len(*events) != 0 {
		t.Errorf("no enumeration, sweep, or content read may precede resolution and destination checks; events: %v", *events)
	}
}

// --- Suite 9: retirement authorization, preview, unattended runs -------

func TestRetireRoot_PreviewLeavesProjectByteIdentical(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	addRetirementChild(t, parent, "old", "old", "alpha", "beta")
	retireRootPreview = true
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha", "beta"))

	before := treeSnapshot(t, parent)
	cmd, out := retireCmd(t, parent, "")
	if err := runRetireRoot(cmd, []string{"old"}); err != nil {
		t.Fatalf("preview must succeed: %v", err)
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))
	if !strings.Contains(out.String(), "Would preserve") {
		t.Errorf("the preview should report the extent of what would be preserved; got:\n%s", out.String())
	}
}

func TestRetireRoot_ExecutionRequiresExplicitAuthorizationAfterPreview(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	addRetirementChild(t, parent, "old", "old", "alpha")
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)

	// Answering no changes nothing.
	before := treeSnapshot(t, parent)
	cmd, _ := retireCmd(t, parent, "n\n")
	if err := runRetireRoot(cmd, []string{"old"}); err != nil {
		t.Fatalf("declining the confirmation is not an error: %v", err)
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))

	// Answering yes proceeds — after the preflight report was shown.
	cmd, out := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err != nil {
		t.Fatalf("authorized execution must proceed: %v", err)
	}
	text := out.String()
	previewIdx := strings.Index(text, "Retirement preview")
	promptIdx := strings.Index(text, "Retire this root?")
	if previewIdx == -1 || promptIdx == -1 || previewIdx > promptIdx {
		t.Errorf("the preflight report must be shown before the confirmation prompt; got:\n%s", text)
	}
	if _, err := os.Stat(retirementDestination(parent, "old")); err != nil {
		t.Errorf("execution should have preserved the root: %v", err)
	}
	idx, _ := config.LoadRootsIndex(parent)
	if _, ok := idx.Lookup("old"); ok {
		t.Error("execution should have deregistered the root")
	}
}

func TestRetireRoot_UnattendedExecutionRefusesNamingTheMissingPerson(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	addRetirementChild(t, parent, "old", "old", "alpha")
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	retireRootNonInteractive = true

	before := treeSnapshot(t, parent)
	cmd, _ := retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil {
		t.Fatal("an unattended execution must refuse")
	}
	if !strings.Contains(err.Error(), "person") {
		t.Errorf("the refusal must say the absence of a person to authorize is the reason; got: %v", err)
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))
}

func TestRetireRoot_UnattendedPreviewSucceedsAndWritesNothing(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	addRetirementChild(t, parent, "old", "old", "alpha")
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	retireRootPreview = true
	retireRootNonInteractive = true

	before := treeSnapshot(t, parent)
	cmd, out := retireCmd(t, parent, "")
	if err := runRetireRoot(cmd, []string{"old"}); err != nil {
		t.Fatalf("an unattended preview must succeed: %v", err)
	}
	if !strings.Contains(out.String(), "Retirement preview") {
		t.Errorf("the preview must be reported; got:\n%s", out.String())
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))
}

func TestRetireRoot_RootWhoseFeaturesHaveArtifactsIsPermitted(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	child := addRetirementChild(t, parent, "old", "old", "alpha")
	// The features carry buildfiles, testcases, and generated code —
	// built features do not block ROOT retirement.
	buildDir := filepath.Join(child.Path, config.ParlayDir, "build", "alpha")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"buildfile.yaml": "feature: alpha\nschema_version: 1\n",
		"testcases.yaml": "schema_version: 3\nfeature: alpha\nsuites: []\n",
	} {
		if err := os.WriteFile(filepath.Join(buildDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srcDir := filepath.Join(child.Path, "internal")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "alpha.go"),
		[]byte("// generated for alpha\npackage alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)
	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err != nil {
		t.Fatalf("built features must not block root retirement: %v", err)
	}
	idx, _ := config.LoadRootsIndex(parent)
	if _, ok := idx.Lookup("old"); ok {
		t.Error("the retirement should have completed")
	}
}

func TestRetireRoot_FeatureLevelRetirementRefusalKeepsItsMeaning(t *testing.T) {
	// Retiring a built FEATURE on its own stays refused by the existing
	// feature-retirement-has-output check, message unchanged — nothing
	// in root retirement touches it.
	dir := setupTestDir(t)
	featDir := writeBareFeature(t, dir, "bare")
	writeAmendment(t, featDir, "001-close-the-feature.md", retirementAmendment("obsolete", "", bareIntents))
	writeBaselineApplied(t, "bare", 1)
	buildDir := testContext(t).BuildPath("bare")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "buildfile.yaml"), []byte("feature: bare\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ := runCheckAmendments_(t, "@bare")
	found := false
	for _, iss := range out.Issues {
		if iss.Code == "feature-retirement-has-output" {
			found = true
			if !strings.Contains(iss.Message, "retirement records a decision and removes nothing") {
				t.Errorf("the existing refusal message must stay unchanged; got: %q", iss.Message)
			}
		}
	}
	if !found {
		t.Fatalf("feature-level retirement of a built feature must still refuse with feature-retirement-has-output; issues=%+v", out.Issues)
	}
}

func TestRetireRoot_RetirementRecordIsNotAcceptedAsAnOwner(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	addRetirementChild(t, parent, "old", "old", "alpha")
	retireRootDispositions = writeDispositionsFile(t, `dispositions:
  - feature: alpha
    term: authority-re-homed-to
    target: "@old-retirement-record"
    rationale: the record should own it
`)
	retireRootNonInteractive = true

	cmd, _ := retireCmd(t, parent, "")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil {
		t.Fatal("re-homing surviving work to the retirement record must refuse")
	}
	if !strings.Contains(err.Error(), "only a live feature can own surviving work") {
		t.Errorf("the refusal should say only a live feature can own surviving work; got: %v", err)
	}
}

// --- Suite 1 (continued): registration is not path authorization ------
//
// The roots index is an ordinary YAML file, and every destructive step
// of a retirement is derived from the path it records: the archive walk
// reads that directory, and the final step deletes it. So a registered
// path that does not resolve strictly inside the project is refused
// before anything is enumerated, archived, or removed.

// registerRawRoot rewrites the parent's roots index by hand, so a test
// can present the registration a bad merge or a hand edit would leave
// rather than the one AppendRootToIndex would allow.
func registerRawRoot(t *testing.T, parent, name, relativePath string) {
	t.Helper()
	body := "children:\n" +
		"  - name: " + name + "\n" +
		"    relative-path: " + relativePath + "\n"
	path := filepath.Join(parent, config.ParlayDir, config.RootsIndexFile)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// plantOutsideRoot builds a directory that looks exactly like a child
// root — config, a feature with founding documents, and a file whose
// survival the test checks — at a location the project does not contain.
func plantOutsideRoot(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, config.ParlayDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.ParlayDir, config.ConfigFile),
		[]byte("ai-agent: Generic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeChildFeature(t, dir, "ghost")
	if err := os.WriteFile(filepath.Join(dir, "precious.txt"),
		[]byte("content that is not the project's to destroy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRetireRoot_RegisteredPathEscapingTheProjectRefusesBeforeAnyMutation(t *testing.T) {
	// Each case registers a path a retirement must not act on, with a
	// real, populated directory behind it wherever the path would land.
	cases := []struct {
		name string
		// rel builds the registered relative-path, given the parent and
		// a scratch directory outside the project; it also plants the
		// content that must survive, and returns the directory whose
		// survival is asserted.
		setup func(t *testing.T, parent, scratch string) (rel string, mustSurvive string)
		// want is a phrase the refusal has to contain, so the message
		// names the reason rather than failing incidentally.
		want string
	}{
		{
			name: "traversal-out-of-the-project",
			setup: func(t *testing.T, parent, scratch string) (string, string) {
				outside := filepath.Join(parent, "..", "escape")
				plantOutsideRoot(t, outside)
				return "../escape", outside
			},
			want: "escapes the project root",
		},
		{
			name: "absolute-registration",
			setup: func(t *testing.T, parent, scratch string) (string, string) {
				plantOutsideRoot(t, scratch)
				return scratch, scratch
			},
			want: "is absolute",
		},
		{
			name: "symlink-resolving-out-of-the-project",
			setup: func(t *testing.T, parent, scratch string) (string, string) {
				plantOutsideRoot(t, scratch)
				if err := os.Symlink(scratch, filepath.Join(parent, "linked")); err != nil {
					t.Fatal(err)
				}
				return "linked", scratch
			},
			want: "outside the project root",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetRetirementState(t)
			parent := makeRetirementParent(t)
			scratch, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			rel, mustSurvive := tc.setup(t, parent, scratch)
			registerRawRoot(t, parent, "old", rel)

			// Everything else about the run is in order: a complete
			// disposition record, a person present, and a yes. Only the
			// registered path is wrong, so nothing but the containment
			// check can be what refuses.
			retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("ghost"))
			interactiveTTY(t)

			outsideBefore := treeSnapshot(t, mustSurvive)
			parentBefore := treeSnapshot(t, parent)

			cmd, _ := retireCmd(t, parent, "y\n")
			err = runRetireRoot(cmd, []string{"old"})
			if err == nil {
				t.Fatal("a registered path that does not resolve inside the project must refuse the retirement")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal must say why the path was refused (want %q); got: %v", tc.want, err)
			}

			// Fail closed means fail before anything happens: neither the
			// project nor the directory the registration pointed at has
			// been read into an archive or removed.
			assertTreeUnchanged(t, parentBefore, treeSnapshot(t, parent))
			assertTreeUnchanged(t, outsideBefore, treeSnapshot(t, mustSurvive))
			if _, statErr := os.Stat(filepath.Join(mustSurvive, "precious.txt")); statErr != nil {
				t.Errorf("content outside the project must survive a refused retirement: %v", statErr)
			}
		})
	}
}

func TestRetireRoot_JournalNamingAnEscapingPathRefusesTheResume(t *testing.T) {
	// A journal is a file on disk like any other, and the deletion
	// target of a resumed run is derived from it, so the containment
	// check is applied again on every resume rather than once at
	// registration time.
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	outside := filepath.Join(parent, "..", "escape-journal")
	plantOutsideRoot(t, outside)
	addRetirementChild(t, parent, "old", "old", "alpha")
	interactiveTTY(t)

	if err := os.MkdirAll(retiredRootsDir(parent), 0o755); err != nil {
		t.Fatal(err)
	}
	journal := &RetirementJournal{
		Root:         "old",
		RelativePath: "../escape-journal",
		Outstanding:  []string{journalStepDeregisterRoot},
	}
	if err := WriteRetirementJournal(parent, journal); err != nil {
		t.Fatal(err)
	}

	before := treeSnapshot(t, outside)
	cmd, _ := retireCmd(t, parent, "y\n")
	err := runRetireRoot(cmd, []string{"old"})
	if err == nil || !strings.Contains(err.Error(), "not inside the project") {
		t.Fatalf("a resume whose journal names a path outside the project must refuse; got: %v", err)
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, outside))
}

// --- Suite 1 (continued): the NAME is a path component too -------------
//
// The registered name is concatenated into the staging directory, the
// archive destination and the journal filename, and all three are then
// created, renamed and removed. So a name is as much a deletion target
// as a path is, and roots.yaml is as editable for one as for the other.

func TestRetireRoot_RootNameMustBeAPlainSlug(t *testing.T) {
	for _, name := range []string{
		"../../evil",  // traversal, the case that reaches RemoveAll
		"..",          // the parent of the retired-roots directory
		"nested/old",  // a separator makes the destination a subtree
		"old/../..",   // traversal behind an innocent prefix
		"old.journal", // a dot collides with the journal's own naming
		".hidden",     // leading dot
		"Old",         // case, so a name maps to exactly one path
		"old root",    // whitespace
		"old\\evil",   // the other separator
		"-old",        // leading dash
		"old-",        // trailing dash
		"",            // names no location at all
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateRootName(name); err == nil {
				t.Errorf("%q must be refused as a registered root name — it is a path component of the archive destination, the staging directory and the journal file", name)
			}
		})
	}
	for _, name := range []string{"old", "core", "studio", "parlay-tool", "root2", "a"} {
		if err := validateRootName(name); err != nil {
			t.Errorf("%q is an ordinary root name and must be accepted: %v", name, err)
		}
	}
}

func TestRetireRoot_MaliciousRootNameRefusesBeforeAnyMutation(t *testing.T) {
	// The registered PATH here is perfectly ordinary and contained. Only
	// the name is hostile, so nothing but the name check can refuse.
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	childPath := filepath.Join(parent, "old")
	if err := os.MkdirAll(filepath.Join(childPath, config.ParlayDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childPath, config.ParlayDir, config.ConfigFile),
		[]byte("ai-agent: Generic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeChildFeature(t, childPath, "alpha")

	// A directory outside the retired-roots location that a traversing
	// name would reach through the destination rollback.
	outside := filepath.Join(parent, "..", "name-escape")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "precious.txt"), []byte("not the project's\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	registerRawRoot(t, parent, "../../name-escape", "old")
	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)

	before := treeSnapshot(t, parent)
	outsideBefore := treeSnapshot(t, outside)

	cmd, _ := retireCmd(t, parent, "y\n")
	err := runRetireRoot(cmd, []string{"../../name-escape"})
	if err == nil {
		t.Fatal("a registered root name that is not a plain slug must refuse the retirement")
	}
	if !strings.Contains(err.Error(), "plain slug") {
		t.Errorf("the refusal must say the name is not a usable path component; got: %v", err)
	}
	assertTreeUnchanged(t, before, treeSnapshot(t, parent))
	assertTreeUnchanged(t, outsideBefore, treeSnapshot(t, outside))
}

// TestRetireRoot_SymlinkSwappedAfterValidationCannotCarryTheDeleteOut
// pins the guarantee that replaced the lexical check. Containment is
// resolved, and then — in the interval before the deletion — an
// intermediate directory is replaced with a symlink pointing out of the
// project. An ordinary os.RemoveAll on the resolved path would follow
// it. The deletion goes through a handle rooted at the project instead,
// which resolves and removes in one sequence, so the swap is refused
// rather than raced.
func TestRetireRoot_SymlinkSwappedAfterValidationCannotCarryTheDeleteOut(t *testing.T) {
	resetRetirementState(t)
	parent := makeRetirementParent(t)

	// The child lives one directory down, so there is an intermediate
	// component to swap.
	child := addRetirementChild(t, parent, "old", "nested/old", "alpha")
	if err := os.WriteFile(filepath.Join(child.Path, "keep.txt"), []byte("child content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// What the swapped-in symlink points at: a directory outside the
	// project holding the same leaf name the deletion will ask for.
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(outside, "old")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(victim, "precious.txt"),
		[]byte("content outside the project\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	retireRootDispositions = writeDispositionsFile(t, deliveredDispositions("alpha"))
	interactiveTTY(t)

	// The seam fires after containment has been resolved and the child
	// directory stat'd, and before the removal — exactly the window a
	// check-then-act sequence leaves open.
	nested := filepath.Join(parent, "nested")
	retirementHook = func(event string) error {
		if event != "remove-contents" {
			return nil
		}
		if err := os.RemoveAll(nested); err != nil {
			return err
		}
		return os.Symlink(outside, nested)
	}

	cmd, _ := retireCmd(t, parent, "y\n")
	if err := runRetireRoot(cmd, []string{"old"}); err == nil {
		t.Fatal("a deletion whose path was swapped for a route out of the project must fail, not follow it")
	}

	// The guarantee: content outside the project is untouched.
	if _, err := os.Stat(filepath.Join(victim, "precious.txt")); err != nil {
		t.Errorf("content outside the project must survive the swap: %v", err)
	}
	entries, err := os.ReadDir(victim)
	if err != nil || len(entries) != 1 {
		t.Errorf("the directory outside the project must be intact: %v %v", entries, err)
	}
}

func TestRetireRoot_RootedDeletionRefusesAPathLeavingTheProject(t *testing.T) {
	// The same guarantee at its own level, so the property is pinned
	// independently of the command that relies on it.
	resetRetirementState(t)
	parent := makeRetirementParent(t)
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "inner", "precious.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(parent, "hop")); err != nil {
		t.Fatal(err)
	}

	if err := removeUnderParent(parent, filepath.Join("hop", "inner")); err == nil {
		t.Error("a deletion routed through a symlink out of the project must be refused")
	}
	if _, err := os.Stat(filepath.Join(outside, "inner", "precious.txt")); err != nil {
		t.Errorf("the content outside the project must survive: %v", err)
	}
}
