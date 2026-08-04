package feedback

// parlay-feature: parlay-tool/feedback-mode
// parlay-component: recorder
// parlay-artifact: test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// Off unless asked for. The mode records what a person does with their own
// project, so defaulting it on would collect before anyone consented.
func TestDisabledByDefault(t *testing.T) {
	if Enabled(false, envFrom(nil)) {
		t.Error("feedback must be off when neither config nor env asks for it")
	}
}

// The env var overrides config in BOTH directions, unlike the no_studio
// precedent where flag and config OR together. A diagnostic mode needs
// "off for this one run" as much as "on for this one run", and an OR merge
// can only express the first.
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
		{"env off word over config on", true, "off", false},
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

func readEvents(t *testing.T, root string) []Event {
	t.Helper()
	dir := filepath.Join(root, ".parlay", Dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Event
	for _, e := range entries {
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

// Nothing is written when the mode is off — not an empty file, not a
// directory. A project that never opted in should be unable to tell the
// feature exists.
func TestNothingIsWrittenWhenDisabled(t *testing.T) {
	root := t.TempDir()
	Start(root, false, "")
	defer Stop()

	Record(KindNote, "test", map[string]any{"a": 1})
	Diagnostic("test", "some-code", "some message")

	if _, err := os.Stat(filepath.Join(root, ".parlay", Dir)); !os.IsNotExist(err) {
		t.Error("a disabled recorder must not create its directory")
	}
	if IsEnabled() {
		t.Error("IsEnabled must be false when Start was told not to")
	}
}

func TestRecordsEventsWhenEnabled(t *testing.T) {
	root := t.TempDir()
	Start(root, true, "")
	Diagnostic("authoring", "authored-field-missing", "summary: is required")
	Record(KindRetry, "build-feature", map[string]any{"after": "authored-field-missing"})
	Stop()

	events := readEvents(t, root)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(events), events)
	}
	if events[0].Kind != KindDiagnostic || events[0].Data["code"] != "authored-field-missing" {
		t.Errorf("first event = %+v", events[0])
	}
	if events[1].Kind != KindRetry {
		t.Errorf("second event = %+v", events[1])
	}
	if events[0].At == "" || events[0].Run == "" {
		t.Error("every event needs a timestamp and a run id")
	}
}

// The correlation id has to survive across processes, because a pipeline
// run is a dozen of them. The first version of this package minted one per
// process and offered a command to read it — but that command was itself a
// process, so it handed back the id of the asking and every event
// correlated against it joined a run of one.
func TestSuppliedRunIDIsAdoptedSoRunsSpanProcesses(t *testing.T) {
	root := t.TempDir()

	Start(root, true, "loop-run-7")
	Diagnostic("authoring", "some-code", "msg")
	Stop()

	// A second, separate "process" against the same root and the same id.
	Start(root, true, "loop-run-7")
	Record(KindRetry, "build-feature", map[string]any{"after": "some-code"})
	Stop()

	events := readEvents(t, root)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	for _, ev := range events {
		if ev.Run != "loop-run-7" {
			t.Errorf("event %s has run %q, want the supplied id — a retry that cannot be tied to its diagnostic answers nothing", ev.Kind, ev.Run)
		}
	}
}

// Without a supplied id an invocation is standalone and still gets its own,
// so a hand-run command is recorded rather than dropped.
func TestStandaloneInvocationStillGetsAnID(t *testing.T) {
	root := t.TempDir()
	Start(root, true, "")
	defer Stop()
	if RunID() == "" {
		t.Error("a standalone invocation must still carry a run id")
	}
}

// Telemetry must never break a command. An unwritable log is a reason to
// stop recording, not a reason to fail.
func TestUnwritableLogDoesNotPanicOrEnable(t *testing.T) {
	root := t.TempDir()
	// A file where the feedback directory needs to be.
	if err := os.MkdirAll(filepath.Join(root, ".parlay"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".parlay", Dir), []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	Start(root, true, "")
	defer Stop()
	Record(KindNote, "test", nil) // must not panic
	if IsEnabled() {
		t.Error("a recorder that could not open its log must report itself disabled")
	}
}

// One file per day, not per run: the patterns worth seeing — the same code
// four times, a phase that always retries — span runs.
func TestLogIsNamedForTheDay(t *testing.T) {
	root := t.TempDir()
	Start(root, true, "")
	Record(KindNote, "test", nil)
	Stop()

	want := time.Now().UTC().Format("2006-01-02") + ".jsonl"
	if _, err := os.Stat(filepath.Join(root, ".parlay", Dir, want)); err != nil {
		t.Errorf("expected a log named %s: %v", want, err)
	}
}
