package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A reply the tool writes is always well-formed and always where §3.4 says.
// That is the point of writing it through the CLI rather than editing the
// comment text: in YAML a reply one column over would silently re-anchor
// itself to different text, and a reply the tool wrote cannot land there.
func TestInsertReplyMatchesTheRequestsColumnAndHost(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
		line    int
		want    string
	}{
		{
			name: "yaml reply takes the request's column",
			file: "surface.yaml",
			content: `fragments:
  - name: add-task
    verify:
      - A task longer than the limit is refused
      # @dwht: also assert the message
`,
			line: 5,
			want: "      # @claude done: raised to 500",
		},
		{
			name: "markdown reply takes the request's column",
			file: "intents.md",
			content: `## Add Task

- Task text must be 200 characters or fewer
<!-- @dwht: product asked for 500 -->
`,
			line: 4,
			want: "<!-- @claude done: raised to 500 -->",
		},
		{
			name: "an indented markdown reply keeps the indent",
			file: "intents.md",
			content: `## Add Task

- Outer
  - Inner
  <!-- @dwht: wrong -->
`,
			line: 5,
			want: "  <!-- @claude done: raised to 500 -->",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempFile(t, tt.file, tt.content)
			out, err := insertAnnotationReply(path, []byte(tt.content), tt.line,
				parser.AnnotationKindDone, "claude", "raised to 500")
			if err != nil {
				t.Fatalf("insert: %v", err)
			}
			if !strings.Contains(string(out), tt.want) {
				t.Errorf("reply not written as %q:\n%s", tt.want, out)
			}
			// And the result must scan back as exactly one NEW entry saying
			// what was asked for — not merely as one answered thread, which a
			// duplicated or mangled entry could also produce.
			scan := parser.ScanAnnotations(path, out)
			if len(scan.Findings) != 0 {
				t.Fatalf("the written reply is not well-formed: %+v", scan.Findings)
			}
			if len(scan.Threads) != 1 || scan.Threads[0].State != parser.AnnotationAnswered {
				t.Fatalf("threads = %+v, want one answered", scan.Threads)
			}
			entries := scan.Threads[0].Entries
			if len(entries) != 2 {
				t.Fatalf("entries = %+v, want the request and one reply", entries)
			}
			reply := entries[1]
			if reply.By != "claude" || reply.Kind != parser.AnnotationKindDone || reply.Text != "raised to 500" {
				t.Errorf("the reply scanned back as @%s %s: %q", reply.By, reply.Kind, reply.Text)
			}
		})
	}
}

func TestInsertReplyRefusesWhatItCannotAnswer(t *testing.T) {
	const content = `- A bullet
<!-- @dwht: wrong -->
<!-- @dwht close -->
`
	path := writeTempFile(t, "intents.md", content)

	if _, err := insertAnnotationReply(path, []byte(content), 1,
		parser.AnnotationKindDone, "claude", "x"); err == nil ||
		!strings.Contains(err.Error(), parser.AnnotationReplyOrphaned) {
		t.Errorf("replying to a line that is not an entry: err = %v", err)
	}

	if _, err := insertAnnotationReply(path, []byte(content), 2,
		parser.AnnotationKindDone, "claude", "x"); err == nil ||
		!strings.Contains(err.Error(), parser.AnnotationAfterClose) {
		t.Errorf("replying into a closed thread: err = %v", err)
	}
}

// clear removes what a reviewer closed and nothing else. It is safe to run
// unattended precisely because closure is a fact in the file rather than an
// inference from the command being run.
func TestClearRemovesOnlyClosedThreads(t *testing.T) {
	const content = `## Add Task

- First bullet
<!-- @dwht: open, nobody has answered -->

- Second bullet
<!-- @dwht ask: why? -->
<!-- @claude answer: because -->

- Third bullet
<!-- @dwht: wrong -->
<!-- @claude done: fixed -->
<!-- @dwht close -->
`
	path := writeTempFile(t, "intents.md", content)
	removed, err := clearClosedThreads(path)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed %d threads, want 1", removed)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(after)
	if strings.Contains(body, "@dwht close") || strings.Contains(body, "done: fixed") {
		t.Errorf("the closed thread survived:\n%s", body)
	}
	for _, kept := range []string{"open, nobody has answered", "ask: why?", "answer: because"} {
		if !strings.Contains(body, kept) {
			t.Errorf("clear removed %q, which the reviewer had not closed:\n%s", kept, body)
		}
	}
	// The anchored text is untouched — clear removes comment lines only.
	for _, bullet := range []string{"- First bullet", "- Second bullet", "- Third bullet"} {
		if !strings.Contains(body, bullet) {
			t.Errorf("clear removed content: %q", bullet)
		}
	}
}

// An ask answered, closed and swept restores the bytes exactly, so there is no
// drift and nothing to rebuild for. This is the property axis C was chosen to
// get, tested at the level it is claimed at.
func TestSweepRestoresTheFileExactly(t *testing.T) {
	const original = `## Add Task

- Task text must be 200 characters or fewer
`
	path := writeTempFile(t, "intents.md", original)

	annotated := original + "<!-- @dwht ask: why 200? -->\n<!-- @claude answer: the store column is varchar(200) -->\n<!-- @dwht close -->\n"
	if err := os.WriteFile(path, []byte(annotated), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := clearClosedThreads(path); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != original {
		t.Errorf("the sweep did not restore the bytes:\n--- want ---\n%q\n--- got ---\n%q", original, after)
	}
}

func TestSplitFileLine(t *testing.T) {
	for arg, want := range map[string]int{
		"spec/intents/x/intents.md:14": 14,
		"/abs/path/surface.yaml:7":     7,
	} {
		path, line, err := splitFileLine(arg)
		if err != nil || line != want || !strings.HasSuffix(arg, path+":"+itoa(line)) {
			t.Errorf("splitFileLine(%q) = %q, %d, %v", arg, path, line, err)
		}
	}
	for _, bad := range []string{"intents.md", "intents.md:", "intents.md:0", "intents.md:x", ":4"} {
		if _, _, err := splitFileLine(bad); err == nil {
			t.Errorf("splitFileLine(%q) accepted a malformed target", bad)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// A spec file with CRLF endings must come back with CRLF endings. The obvious
// implementation — split, rejoin with "\n" — rewrites every line in the file,
// so the sweep's promise that an ask answered, closed and swept restores the
// bytes exactly would be false for every reviewer on Windows.
func TestReplyAndSweepPreserveLineEndings(t *testing.T) {
	for _, tt := range []struct {
		name string
		eol  string
	}{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(tt.name, func(t *testing.T) {
			lines := []string{"## Add Task", "", "- Task text must be 200 characters or fewer", ""}
			original := strings.Join(lines, tt.eol)
			path := writeTempFile(t, "intents.md", original)

			// ask
			annotated := original + "<!-- @dwht ask: why 200? -->" + tt.eol
			if err := os.WriteFile(path, []byte(annotated), 0o644); err != nil {
				t.Fatal(err)
			}
			// answer, through the writer
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			withReply, err := insertAnnotationReply(path, content, 4,
				parser.AnnotationKindAnswer, "claude", "the store column is varchar(200)")
			if err != nil {
				t.Fatal(err)
			}
			if tt.eol == "\r\n" && strings.Contains(strings.ReplaceAll(string(withReply), "\r\n", ""), "\n") {
				t.Errorf("the writer left a bare LF in a CRLF file:\n%q", withReply)
			}
			if err := os.WriteFile(path, withReply, 0o644); err != nil {
				t.Fatal(err)
			}
			// close, through the writer
			content, err = os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			withClose, err := insertAnnotationReply(path, content, 4,
				parser.AnnotationKindClose, "dwht", "")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, withClose, 0o644); err != nil {
				t.Fatal(err)
			}
			// sweep
			if _, err := clearClosedThreads(path); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != original {
				t.Errorf("the round trip did not restore the bytes:\n--- want ---\n%q\n--- got ---\n%q", original, after)
			}
		})
	}
}

// The handle and the text are interpolated into a comment, so they are an
// injection surface. Every one of these must be refused BEFORE any byte is
// written.
func TestReplyRefusesUnsafeHandlesAndText(t *testing.T) {
	const content = `- A bullet
<!-- @dwht: wrong -->
`
	path := writeTempFile(t, "intents.md", content)

	for _, by := range []string{"", "dw ht", "dwht\nclose", "task-list/intent", "@dwht", "dwht -->"} {
		if _, err := insertAnnotationReply(path, []byte(content), 2,
			parser.AnnotationKindDone, by, "fixed"); err == nil {
			t.Errorf("--by %q was accepted", by)
		}
	}

	// `-->` in the text would close the comment early and write a SECOND
	// entry the reviewer never authored — here, one closing their own thread.
	unsafe := "done --> <!-- @dwht close"
	if _, err := insertAnnotationReply(path, []byte(content), 2,
		parser.AnnotationKindDone, "claude", unsafe); err == nil {
		t.Error("--text containing `-->` was accepted into a markdown comment")
	}

	// The same text is harmless in YAML, where a comment ends at the line end.
	const yml = `verify:
  - criterion A
  # @dwht: wrong
`
	ypath := writeTempFile(t, "surface.yaml", yml)
	if _, err := insertAnnotationReply(ypath, []byte(yml), 3,
		parser.AnnotationKindDone, "claude", unsafe); err != nil {
		t.Errorf("`-->` is not special in a yaml comment: %v", err)
	}
}

// A refused reply must leave the file exactly as it was. The writer returns
// new bytes rather than writing, so this is really a statement about the
// command: nothing reaches atomicfile unless the validation passed.
func TestRefusedReplyLeavesTheFileByteIdentical(t *testing.T) {
	const content = "## Add Task\r\n\r\n- A bullet\r\n<!-- @dwht: wrong -->\r\n"
	path := writeTempFile(t, "intents.md", content)

	for _, attempt := range []struct {
		line     int
		by, text string
	}{
		{1, "claude", "fixed"},        // not an entry
		{4, "dw ht", "fixed"},         // bad handle
		{4, "claude", "x --> <!-- y"}, // comment injection
	} {
		if _, err := insertAnnotationReply(path, []byte(content), attempt.line,
			parser.AnnotationKindDone, attempt.by, attempt.text); err == nil {
			t.Errorf("attempt %+v was accepted", attempt)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != content {
			t.Fatalf("a refused reply changed the file:\n%q", after)
		}
	}
}

// Inserting a reply shifts every entry below it. An identity that includes a
// line number therefore reports the shifted entries as new — and a valid reply
// gets REJECTED whenever a later thread happens to reuse the same handle and
// kind, which is the common case: one resolver answering several threads.
func TestReplyIsVerifiedAcrossLaterThreadsWithTheSameAuthor(t *testing.T) {
	const content = `## Add Task

- First bullet
<!-- @dwht: wrong -->

- Second bullet
<!-- @dwht: also wrong -->
<!-- @claude done: fixed the second one -->
`
	path := writeTempFile(t, "intents.md", content)
	out, err := insertAnnotationReply(path, []byte(content), 4,
		parser.AnnotationKindDone, "claude", "fixed the first one")
	if err != nil {
		t.Fatalf("a later thread with the same resolver and kind must not make this look duplicated: %v", err)
	}

	scan := parser.ScanAnnotations(path, out)
	if len(scan.Findings) != 0 {
		t.Fatalf("findings = %+v", scan.Findings)
	}
	if len(scan.Threads) != 2 {
		t.Fatalf("threads = %d, want 2", len(scan.Threads))
	}
	first, second := scan.Threads[0].Entries, scan.Threads[1].Entries
	if len(first) != 2 || first[1].Text != "fixed the first one" {
		t.Errorf("first thread = %+v", first)
	}
	if len(second) != 2 || second[1].Text != "fixed the second one" {
		t.Errorf("the later thread was disturbed: %+v", second)
	}
}

// The unterminated-final-line branch used to return before verification — the
// one shape most likely to render oddly was the one shape nothing checked.
func TestReplyOnAnUnterminatedFinalLineIsVerified(t *testing.T) {
	const content = "## Add Task\n\n- A bullet\n<!-- @dwht: wrong -->"
	path := writeTempFile(t, "intents.md", content)

	out, err := insertAnnotationReply(path, []byte(content), 4,
		parser.AnnotationKindDone, "claude", "fixed")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if strings.HasSuffix(string(out), "\n") {
		t.Errorf("the file had no trailing newline and should not gain one:\n%q", out)
	}
	scan := parser.ScanAnnotations(path, out)
	if len(scan.Threads) != 1 || len(scan.Threads[0].Entries) != 2 {
		t.Fatalf("threads = %+v", scan.Threads)
	}
	if got := scan.Threads[0].Entries[1]; got.By != "claude" || got.Text != "fixed" {
		t.Errorf("reply = %+v", got)
	}
}

// The worst thing this feature could do. An applied amendment's bytes are
// hashed into HashedSources.Amendments and re-checked by check-drift,
// apply-amendment, compaction and the applied-history reader; deleting a line
// from one makes the next check report recorded history as edited, against a
// record nobody touched. The sweep would be forging a ledger-integrity
// violation, which is worse than any thread it was removing.
//
// `reply` and `clear --file` both take a PATH rather than a feature, so
// neither passes through collection, and neither ever saw the routing
// collection performs. Governance is resolved from the path.
func TestReplyAndClearRefuseAppliedAmendments(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)

	featureDir := filepath.Join(cfg.IntentsRoot(), "task-list")
	amendments := filepath.Join(featureDir, "amendments")
	if err := os.MkdirAll(amendments, 0o755); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(amendments, "001-task-text-length.md")
	const body = `---
amendment: task-text-length
date: 2026-09-02
---

## Change

Raise the limit to 500.
<!-- @dwht: say which field -->
<!-- @claude done: the surface fragment -->
<!-- @dwht close -->
`
	if err := os.WriteFile(record, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// Applied: the baseline records authority through sequence 1.
	buildDir := cfg.BuildPath("task-list")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, ".baseline.yaml"),
		[]byte("generated-at: 2026-09-02T00:00:00Z\nintents: {}\nlast-applied-amendment: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = dir

	applied, err := annotationPathIsAppliedRecord(cfg, record)
	if err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("the fixture's record must read as applied, or the refusals below prove nothing")
	}
	if err := refuseWriteToAppliedRecord(cfg, record); err == nil ||
		!strings.Contains(err.Error(), parser.AnnotationInAppliedRecord) {
		t.Errorf("writing to an applied record: err = %v", err)
	}

	// And the sweep leaves it byte-identical.
	before, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	scans, err := collectFeatureAnnotations(cfg, "task-list")
	if err != nil {
		t.Fatal(err)
	}
	for _, scan := range scans {
		if scan.Path == record && !scan.Applied {
			t.Error("collection did not mark the record applied")
		}
	}
	after, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Error("collection modified an applied record")
	}

	// The COMMANDS, not just the helper. The original bug was exactly that
	// the refusal existed and nothing called it from these two paths, so a
	// test of the helper alone would have passed against the broken wiring.
	if err := runAnnotationsCmd(t, cfg, annotationsReplyCmd,
		[]string{record + ":9", "--kind", "done", "--by", "claude", "--text", "x"}); err == nil ||
		!strings.Contains(err.Error(), parser.AnnotationInAppliedRecord) {
		t.Errorf("annotations reply into an applied record: err = %v", err)
	}
	if err := runAnnotationsCmd(t, cfg, annotationsClearCmd,
		[]string{"--file", record}); err == nil ||
		!strings.Contains(err.Error(), parser.AnnotationInAppliedRecord) {
		t.Errorf("annotations clear --file on an applied record: err = %v", err)
	}
	final, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if string(final) != string(before) {
		t.Errorf("a refused command changed an applied record:\n%q", final)
	}

	// An UNAPPLIED record is a proposal in its review window and stays
	// writable — decision F.
	unapplied := filepath.Join(amendments, "002-later.md")
	if err := os.WriteFile(unapplied, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refuseWriteToAppliedRecord(cfg, unapplied); err != nil {
		t.Errorf("an unapplied record must stay writable: %v", err)
	}
}

// runAnnotationsCmd drives a subcommand the way the backlog tests do:
// ParseFlags and RunE rather than Execute, because Execute on a subcommand
// re-roots to rootCmd and parses `go test`'s own argv.
func runAnnotationsCmd(t *testing.T, cfg *config.Context, sub *cobra.Command, argv []string) error {
	t.Helper()
	var out strings.Builder
	sub.SetOut(&out)
	sub.SetErr(&out)
	sub.SetContext(config.WithCtx(context.Background(), cfg))
	sub.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue) })
	if err := sub.ParseFlags(argv); err != nil {
		return err
	}
	positional := sub.Flags().Args()
	if sub.Args != nil {
		if err := sub.Args(sub, positional); err != nil {
			return err
		}
	}
	return sub.RunE(sub, positional)
}
