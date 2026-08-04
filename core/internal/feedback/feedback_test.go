package feedback

// parlay-feature: parlay-tool/feedback-mode
// parlay-component: recorder
// parlay-artifact: test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func stop() { Stop("test", "ok", time.Millisecond) }

func readEvents(t *testing.T, root string) []Event {
	t.Helper()
	dir := LogDir(root)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Event
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line == "" {
				continue
			}
			var ev Event
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				t.Fatalf("log line is not JSON: %q (%v)", line, err)
			}
			out = append(out, ev)
		}
	}
	return out
}

func rawLog(t *testing.T, root string) string {
	t.Helper()
	dir := LogDir(root)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		b.Write(data)
	}
	return b.String()
}

// ---------------------------------------------------------------------
// Enablement
// ---------------------------------------------------------------------

// Off unless asked for. The mode records what a person does with their own
// project, so defaulting it on would collect before anyone consented.
func TestDisabledByDefault(t *testing.T) {
	if Enabled(false, envFrom(nil)) {
		t.Error("feedback must be off when neither config nor env asks for it")
	}
}

// The env var overrides config in BOTH directions, unlike the no_studio
// precedent where flag and config OR together. A diagnostic mode needs
// "off for this one run" as much as "on for this one run".
func TestEnvOverridesConfigBothWays(t *testing.T) {
	cases := []struct {
		name   string
		config bool
		env    string
		want   bool
	}{
		{"env on over config off", false, "1", true},
		{"env off over config on", true, "0", false},
		{"env false over config on", true, "false", false},
		{"unset falls through to config on", true, "", true},
		{"unset falls through to config off", false, "", false},
		{"unrecognised value reads as on", false, "yes-please", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{}
			if tc.env != "" {
				env[EnvVar] = tc.env
			}
			if got := Enabled(tc.config, envFrom(env)); got != tc.want {
				t.Errorf("Enabled(%v, %q) = %v, want %v", tc.config, tc.env, got, tc.want)
			}
		})
	}
}

func TestNothingIsWrittenWhenDisabled(t *testing.T) {
	root := t.TempDir()
	Start(root, false, "")
	defer stop()

	Record(FindingData{Code: "some-code"})
	if _, err := os.Stat(LogDir(root)); !os.IsNotExist(err) {
		t.Error("a disabled recorder must not create its directory")
	}
	if IsEnabled() {
		t.Error("IsEnabled must be false when Start was told not to")
	}
}

// ---------------------------------------------------------------------
// THE acceptance criterion
// ---------------------------------------------------------------------

// The whole point of the redesign: a user sends this file without reading
// it, so nothing they typed or authored may survive into it.
//
// Adversarial input is pushed through every producer and the RAW BYTES are
// searched. Asserting on parsed fields would only prove the fields we
// thought to check are clean; asserting on bytes catches a leak through a
// field nobody remembered, which is the failure mode that matters.
func TestNothingSensitiveReachesTheLog(t *testing.T) {
	secrets := []string{
		"/Users/alice/work/acme-secret-project/spec/intents/pricing/intents.md",
		"alice@example.com",
		"The approval step should notify the requester before escalating",
		"yaml: line 12: could not find expected ':' in ProprietaryEntity",
		"AcmeInternalBillingEngine",
		"C:\\Users\\bob\\Documents\\unreleased",
	}

	root := t.TempDir()
	Start(root, true, "run-1")

	for _, s := range secrets {
		// Every field of every payload type, including the ones a caller
		// would never deliberately put user text in.
		Record(FindingData{Code: s, Mode: s, Severity: s, Site: s, Phase: s, Artifact: s, Subject: s})
		Record(AgentData{Kind: KindRetry, Skill: s, Phase: s, Artifact: s, Code: s,
			Changed: s, Needed: s, Decision: s, Option: s, Subject: s})
		Record(SessionData{Version: s, OS: s, Arch: s, MultiRoot: s,
			Features: s, Adapters: s, Interactive: s})
		Record(TallyData{Command: s, Exit: s, MsBucket: s, Findings: s, Completed: s})
		Finding(s, s, s, s)
		SetRunID(s)
	}
	Stop(secrets[0], secrets[1], time.Second)

	log := rawLog(t, root)
	if log == "" {
		t.Fatal("nothing was written — the test is not exercising anything")
	}
	for _, s := range secrets {
		for _, frag := range fragments(s, 8) {
			if strings.Contains(log, frag) {
				t.Errorf("an 8-char fragment of adversarial input reached the log: %q\nfrom: %q", frag, s)
				break
			}
		}
	}
}

// fragments yields every substring of length n, so a partially-escaped or
// truncated leak is caught as well as a verbatim one.
func fragments(s string, n int) []string {
	if len(s) < n {
		return []string{s}
	}
	var out []string
	for i := 0; i+n <= len(s); i++ {
		out = append(out, s[i:i+n])
	}
	return out
}

// A value that fails validation is replaced, not dropped. The log has to
// record THAT something was rejected — that entry is a bug report about
// this package, and dropping the field would hide the one event worth
// investigating.
func TestRejectedValuesBecomeASentinelRatherThanVanishing(t *testing.T) {
	root := t.TempDir()
	Start(root, true, "run-1")
	Record(FindingData{Code: "valid-code", Mode: "/absolute/path/is/not/safe"})
	stop()

	events := readEvents(t, root)
	var finding *Event
	for i := range events {
		if events[i].Kind == KindFinding {
			finding = &events[i]
		}
	}
	if finding == nil {
		t.Fatal("no finding recorded")
	}
	if finding.Data["code"] != "valid-code" {
		t.Errorf("a safe value must survive: %q", finding.Data["code"])
	}
	if finding.Data["mode"] != Redacted {
		t.Errorf("mode = %q, want the %q sentinel", finding.Data["mode"], Redacted)
	}
}

// ---------------------------------------------------------------------
// Recording behaviour
// ---------------------------------------------------------------------

func TestRecordsFindingsAndStampsTheVersion(t *testing.T) {
	root := t.TempDir()
	Start(root, true, "run-1")
	Finding("authored-field-missing", "authoring", "error", "agent.validateunit")
	stop()

	events := readEvents(t, root)
	var f *Event
	for i := range events {
		if events[i].Kind == KindFinding {
			f = &events[i]
		}
	}
	if f == nil {
		t.Fatal("no finding recorded")
	}
	if f.V != SchemaVersion {
		t.Errorf("v = %d, want %d — export refuses anything older", f.V, SchemaVersion)
	}
	if f.Data["code"] != "authored-field-missing" || f.Data["severity"] != "error" {
		t.Errorf("finding payload = %+v", f.Data)
	}
	if f.Run == "" || f.Proc == "" {
		t.Errorf("run/proc = %q/%q, both required", f.Run, f.Proc)
	}
	if f.Run == "run-1" {
		t.Error("the run id must be hashed, not stored as given — the loop driver builds it from the feature name")
	}
}

// Denominators come from the tally, and the tally has to count what this
// process actually produced.
func TestTallyCountsThisProcessesFindings(t *testing.T) {
	root := t.TempDir()
	Start(root, true, "run-1")
	Finding("code-a", "build", "error", "site")
	Finding("code-b", "build", "error", "site")
	Stop("validate", "exit-1", 2*time.Second)

	events := readEvents(t, root)
	var tally *Event
	for i := range events {
		if events[i].Kind == KindTally {
			tally = &events[i]
		}
	}
	if tally == nil {
		t.Fatal("no tally recorded — denominators are impossible without it")
	}
	if tally.Data["findings"] != "2" {
		t.Errorf("findings = %q, want 2", tally.Data["findings"])
	}
	if tally.Data["command"] != "validate" || tally.Data["exit"] != "exit-1" {
		t.Errorf("tally payload = %+v", tally.Data)
	}
	if tally.Data["ms_bucket"] != "1s-10s" {
		t.Errorf("ms_bucket = %q, want the 1s-10s bucket", tally.Data["ms_bucket"])
	}
	if tally.Data["completed"] != "true" {
		t.Error("a tally that was written means the process finished; say so")
	}
}

// A killed process writes findings but never its tally, and failing runs
// are the likeliest to be killed — so counting tallies alone biases
// findings-per-run upward. The proc id on every line is what makes the
// incomplete run detectable instead of silently skewing the rate.
func TestFindingsCarryProcSoIncompleteRunsAreCountable(t *testing.T) {
	root := t.TempDir()

	Start(root, true, "run-1")
	Finding("code-a", "build", "error", "site")
	// No Stop: this process "died".
	forceClose()

	Start(root, true, "run-1")
	Finding("code-b", "build", "error", "site")
	Stop("validate", "ok", time.Millisecond)

	events := readEvents(t, root)
	procs := map[string]bool{}
	withTally := map[string]bool{}
	for _, e := range events {
		procs[e.Proc] = true
		if e.Kind == KindTally {
			withTally[e.Proc] = true
		}
	}
	if len(procs) != 2 {
		t.Fatalf("distinct procs = %d, want 2", len(procs))
	}
	if len(withTally) != 1 {
		t.Errorf("procs with a tally = %d, want 1 — the killed one must still be visible", len(withTally))
	}
}

// forceClose simulates a process dying without reaching Stop.
func forceClose() {
	active.mu.Lock()
	defer active.mu.Unlock()
	if active.file != nil {
		active.file.Close()
		active.file = nil
	}
	active.enabled = false
}

// ---------------------------------------------------------------------
// Correlation, salt, concurrency, containment
// ---------------------------------------------------------------------

// The correlation id must survive across processes, because a pipeline run
// is a dozen of them.
func TestSuppliedRunIDIsAdoptedSoRunsSpanProcesses(t *testing.T) {
	root := t.TempDir()
	Start(root, true, "loop-run-7")
	Finding("code-a", "build", "error", "site")
	stop()
	Start(root, true, "loop-run-7")
	Record(AgentData{Kind: KindRetry, Skill: "build-feature", Code: "code-a"})
	stop()

	// The id is hashed, so what matters is that every event across both
	// processes carries the SAME digest — correlation without the
	// plaintext, which is built from the feature name.
	seen := map[string]bool{}
	for _, ev := range readEvents(t, root) {
		seen[ev.Run] = true
		if ev.Run == "loop-run-7" {
			t.Error("the run id reached the log in plaintext")
		}
	}
	if len(seen) != 1 {
		t.Errorf("events span %d run ids, want 1 — a retry that cannot be tied to its diagnostic answers nothing", len(seen))
	}
}

// Stable within a project so "the same feature failed four times" survives;
// different across projects so the digest cannot be joined between users or
// attacked with a dictionary of common slugs.
func TestSaltIsStableWithinAProjectAndDiffersAcross(t *testing.T) {
	rootA, rootB := t.TempDir(), t.TempDir()

	Start(rootA, true, "")
	a1 := Hash("checkout")
	a2 := Hash("Checkout ")
	stop()

	Start(rootA, true, "")
	a3 := Hash("checkout")
	stop()

	Start(rootB, true, "")
	b1 := Hash("checkout")
	stop()

	if a1 == "" || a1 != a3 {
		t.Errorf("hash must be stable across runs of one project: %q vs %q", a1, a3)
	}
	if a1 != a2 {
		t.Errorf("normalization must make %q and %q hash alike", "checkout", "Checkout ")
	}
	if a1 == b1 {
		t.Error("two projects must not produce the same digest for the same name")
	}
	if strings.Contains(a1, "checkout") {
		t.Error("the hash leaks its input")
	}
}

// Concurrent parlay processes append to one day file. A torn line would
// make the log unparseable exactly when several things were happening,
// which is when it is most worth reading.
func TestConcurrentWritersDoNotTearLines(t *testing.T) {
	root := t.TempDir()
	Start(root, true, "run-1")
	defer stop()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				Finding(fmt.Sprintf("code-%d", n), "build", "error", "site")
			}
		}(i)
	}
	wg.Wait()

	// readEvents fails the test on any line that is not valid JSON.
	if got := len(readEvents(t, root)); got != 200 {
		t.Errorf("got %d events, want 200 — lines were lost or merged", got)
	}
}

// .parlay/ is version controlled by convention, so without this a user who
// enables the mode commits their log and pushes it.
func TestGitignoreIsWrittenOnFirstEnable(t *testing.T) {
	root := t.TempDir()
	Start(root, true, "")
	defer stop()

	data, err := os.ReadFile(filepath.Join(LogDir(root), ".gitignore"))
	if err != nil {
		t.Fatalf("no .gitignore written: %v", err)
	}
	if !strings.Contains(string(data), "*") {
		t.Errorf(".gitignore does not ignore anything: %q", data)
	}
}

func TestLogAndSaltAreOwnerOnly(t *testing.T) {
	root := t.TempDir()
	Start(root, true, "")
	Finding("code-a", "build", "error", "site")
	stop()

	for _, p := range []string{LogPath(root), filepath.Join(LogDir(root), SaltFile)} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("%s mode = %o, want 600", filepath.Base(p), perm)
		}
	}
}

// Telemetry must never break a command.
func TestUnwritableLogDoesNotPanicOrEnable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".parlay"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".parlay", Dir), []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	Start(root, true, "")
	defer stop()
	Record(FindingData{Code: "code-a"}) // must not panic
	if IsEnabled() {
		t.Error("a recorder that could not open its log must report itself disabled")
	}
}

// One file per day, because the patterns worth seeing span runs.
func TestLogIsNamedForTheDay(t *testing.T) {
	root := t.TempDir()
	Start(root, true, "")
	Finding("code-a", "build", "error", "site")
	stop()

	if _, err := os.Stat(LogPath(root)); err != nil {
		t.Errorf("expected today's log at %s: %v", LogPath(root), err)
	}
}

// Only the process that created the file owes the header, so a pipeline
// run's dozen invocations do not each pay a project scan.
func TestOnlyTheFirstWriterOwesTheSessionHeader(t *testing.T) {
	root := t.TempDir()

	Start(root, true, "")
	first := NeedsSession()
	stop()

	Start(root, true, "")
	// Nothing was written by the first process, so the file is still
	// empty and this process legitimately owes the header too.
	Record(SessionData{Version: "dev", OS: "darwin"})
	stop()

	Start(root, true, "")
	third := NeedsSession()
	stop()

	if !first {
		t.Error("the process that creates the file owes the header")
	}
	if third {
		t.Error("a process opening a non-empty file must not re-emit the header")
	}
}

func TestBucketsAreSafeTokens(t *testing.T) {
	for _, v := range []string{Bucket(0), Bucket(3), Bucket(12), Bucket(500),
		bucketDuration(time.Millisecond), bucketDuration(500 * time.Millisecond),
		bucketDuration(5 * time.Second), bucketDuration(time.Minute)} {
		if encodeValue(v) != v {
			t.Errorf("bucket %q does not survive its own encoder", v)
		}
	}
}
