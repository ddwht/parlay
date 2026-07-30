package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/spf13/cobra"
)

// setupPageProject writes two surfaces targeting one page and returns the
// project root.
func setupPageProject(t *testing.T) string {
	t.Helper()
	dir := setupTestDir(t)
	// mustContext resolves the root by walking up for the .parlay marker.
	if err := os.MkdirAll(filepath.Join(dir, config.ParlayDir), 0755); err != nil {
		t.Fatal(err)
	}
	for feature, body := range map[string]string{
		"alpha": `feature: alpha
fragments:
  - name: Alpha One
    shows: the first thing
    page: home
    region: main
    order: 1
`,
		"beta": `feature: beta
fragments:
  - name: Beta One
    shows: the second thing
    page: home
    region: main
    order: 2
`,
	} {
		fdir := filepath.Join(dir, config.SpecDir, config.IntentsDir, feature)
		if err := os.MkdirAll(fdir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fdir, "surface.yaml"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runLockPageCmd invokes the command with the given flags and stdin.
func runLockPageCmd(t *testing.T, stdin string, tty bool, args ...string) (string, error) {
	t.Helper()
	lockPageOwner, lockPageYes, lockPageRelock = "", false, false
	lockPageTTYOverride = &tty
	t.Cleanup(func() { lockPageTTYOverride = nil })

	cmd := testCommandWithContext(t, testContext(t))
	cmd.Use = "lock-page"
	cmd.Args = cobra.ExactArgs(1)
	cmd.RunE = runLockPage
	cmd.Flags().StringVar(&lockPageOwner, "owner", "", "")
	cmd.Flags().BoolVar(&lockPageYes, "yes", false, "")
	cmd.Flags().BoolVar(&lockPageRelock, "relock", false, "")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	return out.String(), cmd.Execute()
}

func manifestAt(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, config.SpecDir, "pages", "home.page.md"))
	if err != nil {
		return ""
	}
	return string(data)
}

// The defect: with no TTY the prompt's read failed instantly, the owner came
// back empty, and lock-page wrote a manifest recording nobody as the owner
// and exited 0. A manifest whose whole purpose is to record a layout decision
// silently recorded that nobody made it.
func TestLockPageRefusesToWriteAnOwnerlessManifestUnderAPipe(t *testing.T) {
	dir := setupPageProject(t)

	_, err := runLockPageCmd(t, "", false, "home")
	if err == nil {
		t.Fatal("piped stdin with no --owner must refuse, not write an ownerless manifest")
	}
	if !strings.Contains(err.Error(), "--owner") {
		t.Errorf("the error must name the way out: %v", err)
	}
	if m := manifestAt(t, dir); m != "" {
		t.Errorf("nothing should have been written:\n%s", m)
	}
}

// --owner is the non-interactive path, and it must work without a terminal.
func TestLockPageWritesTheOwnerFromTheFlag(t *testing.T) {
	dir := setupPageProject(t)

	if _, err := runLockPageCmd(t, "", false, "home", "--owner", "design", "--yes"); err != nil {
		t.Fatal(err)
	}
	m := manifestAt(t, dir)
	if !strings.Contains(m, "**Owner**: design") {
		t.Errorf("manifest should record the owner:\n%s", m)
	}
	// And it should list what the surfaces produce.
	for _, want := range []string{"@alpha/alpha-one", "@beta/beta-one"} {
		if !strings.Contains(m, want) {
			t.Errorf("manifest missing %s:\n%s", want, m)
		}
	}
}

// --yes without --owner is a request to skip the only step that supplies one.
func TestLockPageYesWithoutOwnerRefuses(t *testing.T) {
	setupPageProject(t)

	_, err := runLockPageCmd(t, "", true, "home", "--yes")
	if err == nil || !strings.Contains(err.Error(), "--yes requires --owner") {
		t.Errorf("err = %v, want a refusal naming the missing flag", err)
	}
}

// The prompt must read the command's stdin, not the process-global os.Stdin.
// Reading the global is what made this untestable and what made an injected
// answer impossible to supply.
func TestLockPagePromptReadsTheCommandsStdin(t *testing.T) {
	dir := setupPageProject(t)

	if _, err := runLockPageCmd(t, "platform\n", true, "home"); err != nil {
		t.Fatal(err)
	}
	if m := manifestAt(t, dir); !strings.Contains(m, "**Owner**: platform") {
		t.Errorf("the prompted answer should have been used:\n%s", m)
	}
}

// An empty answer at the prompt is still no owner.
func TestLockPageEmptyPromptAnswerRefuses(t *testing.T) {
	dir := setupPageProject(t)

	_, err := runLockPageCmd(t, "\n", true, "home")
	if err == nil || !strings.Contains(err.Error(), "ownerless") {
		t.Errorf("err = %v, want a refusal", err)
	}
	if m := manifestAt(t, dir); m != "" {
		t.Errorf("nothing should have been written:\n%s", m)
	}
}

// A draft manifest is re-derivable; a locked one is a decision someone made
// and must not be replaced by whatever the surfaces currently say. Before
// --relock the only way to pick up a new fragment was to delete the file,
// which loses the Owner: with it.
func TestLockPageRelockRespectsStatus(t *testing.T) {
	dir := setupPageProject(t)
	if _, err := runLockPageCmd(t, "", false, "home", "--owner", "design", "--yes"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, config.SpecDir, "pages", "home.page.md")

	// Without --relock, an existing manifest is left alone.
	if _, err := runLockPageCmd(t, "", false, "home", "--owner", "other", "--yes"); err == nil {
		t.Error("an existing manifest must not be silently replaced")
	}

	// Draft: re-derivable.
	if _, err := runLockPageCmd(t, "", false, "home", "--owner", "platform", "--yes", "--relock"); err != nil {
		t.Fatalf("a draft manifest should be re-derivable: %v", err)
	}
	if m := manifestAt(t, dir); !strings.Contains(m, "**Owner**: platform") {
		t.Errorf("re-derive should have taken the new owner:\n%s", m)
	}

	// Locked: refused even with --relock.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	locked := strings.Replace(string(data), "**Status**: draft", "**Status**: locked", 1)
	if err := os.WriteFile(path, []byte(locked), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = runLockPageCmd(t, "", false, "home", "--owner", "someone-else", "--yes", "--relock")
	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Errorf("err = %v, want a refusal naming the locked status", err)
	}
	if m := manifestAt(t, dir); strings.Contains(m, "someone-else") {
		t.Error("a locked manifest was overwritten")
	}
}
