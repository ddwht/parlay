package commands

import (
	"bytes"
	"sync"
	"time"

	"encoding/json"
	"errors"
	"github.com/gofrs/flock"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"gopkg.in/yaml.v3"
)

// Stage 0 — the combined transition gets an applier.
//
// These pin the real user journey, not only the predicates: a built feature
// whose founding promise still owns a live operation, a valid combined
// amendment accounting for it, the splice done and journalled, and then the
// two-proof ceremony end to end.

func runApplyAmendment_(t *testing.T, cfg *config.Context, ref string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := testCommandWithContext(t, cfg)
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := runApplyAmendment(cmd, []string{ref})
	return buf.String(), err
}

func armApplyAmendment(t *testing.T, confirm string, asJSON bool) {
	t.Helper()
	applyAmendmentConfirm = confirm
	applyAmendmentJSON = asJSON
	t.Cleanup(func() { applyAmendmentConfirm = ""; applyAmendmentJSON = false })
}

// strandedFeature is the state the remediation created: a built feature whose
// founding promise owns a live operation, and a combined amendment that
// accounts for it — the exact record no command could apply.
func strandedFeature(t *testing.T, dir string) string {
	t.Helper()
	cfg := testContext(t)
	featureDir := setupLedgerFeature(t, dir)

	intents, err := os.ReadFile(filepath.Join(featureDir, "intents.md"))
	if err != nil {
		t.Fatal(err)
	}
	intents = append(intents, []byte("\n## Survives\n\n**Goal**: Something that stays.\n**Persona**: Admin\n")...)
	if err := os.WriteFile(filepath.Join(featureDir, "intents.md"), intents, 0o644); err != nil {
		t.Fatal(err)
	}
	// A live operation justified by the promise about to be withdrawn.
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeAmendment(t, featureDir, "001-readiness-withdrawn-and-reworded.md", combinedAmendment)
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 0
		b.Sources.Amendments = map[string]string{}
	})
	writeRefineJournal(t, cfg, "my-feature", 1)
	return featureDir
}

func digestFrom(t *testing.T, cfg *config.Context) applyAmendmentPreflight {
	t.Helper()
	armApplyAmendment(t, "", true)
	out, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err != nil {
		t.Fatalf("preflight: %v\n%s", err, out)
	}
	var pf applyAmendmentPreflight
	if err := json.Unmarshal([]byte(out), &pf); err != nil {
		t.Fatalf("preflight is not JSON: %v\n%s", err, out)
	}
	return pf
}

// The journey. Preflight shows the dying promises and writes nothing; the
// confirmed rerun applies both halves in one advance.
func TestS0_CombinedRecordAppliesUnderBothProofs(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := strandedFeature(t, dir)
	blPath := baselinePath(cfg, "my-feature")

	before, err := os.ReadFile(blPath)
	if err != nil {
		t.Fatal(err)
	}

	// Preflight prints the promise, not the slug, and writes nothing.
	armApplyAmendment(t, "", false)
	prose, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err != nil {
		t.Fatalf("preflight must succeed: %v", err)
	}
	if !strings.Contains(prose, "Check Readiness") {
		t.Errorf("the preflight must show the promise itself; got %q", prose)
	}
	if !strings.Contains(prose, "See if the cluster is ready") {
		t.Error("the preflight must show the promise's goal — a slug is not a promise")
	}
	if !strings.Contains(prose, "@my-feature/operation:x") {
		t.Error("the preflight must show the contract entries the record changes")
	}
	if after, err := os.ReadFile(blPath); err != nil {
		t.Fatal(err)
	} else if string(before) != string(after) {
		t.Error("an unconfirmed preflight wrote to the baseline")
	}

	pf := digestFrom(t, cfg)
	if pf.Digest == "" {
		t.Fatal("preflight produced no digest")
	}

	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("a correctly confirmed transition must apply: %v", err)
	}

	bl := readFeatureBaseline(t, blPath)
	if bl.LastAppliedAmendment != 1 {
		t.Errorf("marker = %d, want 1", bl.LastAppliedAmendment)
	}
	if bl.Sources.Amendments["001-readiness-withdrawn-and-reworded.md"] == "" {
		t.Error("the applied record's hash must be recorded")
	}
	// The receipt, not a boolean: what was approved, under which scheme, from
	// which authority.
	receipt, ok := bl.TransitionReceipts["001-readiness-withdrawn-and-reworded.md"]
	if !ok {
		t.Fatal("the withdrawal ceremony must be recorded durably against the exact amendment")
	}
	if receipt.Payload.Scheme != transitionApprovalScheme {
		t.Errorf("receipt scheme = %q, want the canonicalisation it was computed under", receipt.Payload.Scheme)
	}
	if receipt.Digest != pf.Digest {
		t.Errorf("receipt digest = %q, want the approved digest %q", receipt.Digest, pf.Digest)
	}
	if receipt.Payload.Mode != transitionModeWithdrawAndSplice {
		t.Errorf("receipt mode = %q", receipt.Payload.Mode)
	}
	if len(receipt.Payload.Withdraws) != 1 || receipt.Payload.Withdraws[0].Slug != "check-readiness" {
		t.Errorf("the receipt must name what was withdrawn, not merely that something was; got %+v",
			receipt.Payload.Withdraws)
	}
	if receipt.Payload.Withdraws[0].Goal == "" {
		t.Error("the receipt must preserve the promise text that was approved — a slug is not " +
			"what the human agreed to")
	}
	if len(receipt.Payload.Affects) != 1 {
		t.Errorf("the receipt must record the affected entries; got %v", receipt.Payload.Affects)
	}
	// And the promise is actually out of force.
	res, err := resolveActiveIntents(cfg, "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range res.Active {
		if in.Slug == "check-readiness" {
			t.Error("the withdrawn promise is still active after the transition applied")
		}
	}
	_ = featureDir
}

// No confirmation, no write — and the run stays retryable and byte-identical.
func TestS0_UnconfirmedRunIsInertAndRetryable(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	strandedFeature(t, dir)
	blPath := baselinePath(cfg, "my-feature")
	before, err := os.ReadFile(blPath)
	if err != nil {
		t.Fatal(err)
	}

	first := digestFrom(t, cfg)
	second := digestFrom(t, cfg)
	if first.Digest != second.Digest {
		t.Error("the digest must be stable across repeated preflights of an unchanged state")
	}
	after, err := os.ReadFile(blPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("preflight is read-only and must leave the baseline byte-identical")
	}
}

// A confirmation is bound to what was shown. Editing the record after the
// promise list was displayed invalidates it.
func TestS0_ConfirmationDoesNotSurviveAnEditedRecord(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := strandedFeature(t, dir)
	pf := digestFrom(t, cfg)

	path := filepath.Join(featureDir, "amendments", "001-readiness-withdrawn-and-reworded.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, []byte("\nQuietly appended.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	armApplyAmendment(t, pf.Digest, false)
	out, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err == nil {
		t.Fatal("a confirmation must not survive an edit to the record it approved")
	}
	if !strings.Contains(err.Error()+out, "changed since it was shown") {
		t.Errorf("the refusal must say the approved state moved; got %v", err)
	}
	if bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature")); bl.LastAppliedAmendment != 0 {
		t.Error("a refused confirmation advanced the marker")
	}
}

// Likewise if the promise set itself changes.
func TestS0_ConfirmationDoesNotSurviveAChangedPromiseSet(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := strandedFeature(t, dir)
	pf := digestFrom(t, cfg)

	// The founding promise's text changes under the approval.
	intents, err := os.ReadFile(filepath.Join(featureDir, "intents.md"))
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(intents), "See if the cluster is ready.", "See if anything at all is ready.", 1)
	if edited == string(intents) {
		t.Fatal("fixture: the promise text did not change")
	}
	if err := os.WriteFile(filepath.Join(featureDir, "intents.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err == nil {
		t.Fatal("a confirmation must not survive a change to the promises it approved")
	}
	if bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature")); bl.LastAppliedAmendment != 0 {
		t.Error("a refused confirmation advanced the marker")
	}
}

// A wrong or invented digest is refused.
func TestS0_StaleOrForgedDigestIsRefused(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	strandedFeature(t, dir)

	armApplyAmendment(t, "0000000000000000deadbeefdeadbeef", false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err == nil {
		t.Fatal("an invented digest must be refused")
	}
	if bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature")); bl.LastAppliedAmendment != 0 {
		t.Error("a forged digest advanced the marker")
	}
}

// The splice half is proven by the same journal evidence an ordinary
// refinement must produce. This transition relaxes nothing about the work.
func TestS0_UnprovenSpliceIsRefusedBeforeAnyPromiseIsShown(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	strandedFeature(t, dir)
	// The refinement never reached the test step.
	j := refineJournal{
		Feature:   "my-feature",
		Amendment: 1,
		Completed: []string{"amendment-written", "splice-applied"},
	}
	data, err := yaml.Marshal(&j)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(refineJournalPath(cfg, "my-feature"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	armApplyAmendment(t, "", false)
	out, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err == nil {
		t.Fatal("an unproven splice must refuse")
	}
	if !strings.Contains(err.Error(), "splice half") {
		t.Errorf("the refusal must name which proof failed; got %v", err)
	}
	if strings.Contains(out, "stop being in force") {
		t.Error("the promise list must not be shown when the splice proof already failed — " +
			"a human should not be asked to approve a withdrawal for work that is incomplete")
	}
}

// Shape routing fails closed, and never classifies a record into a mode it did
// not declare. This is the guard against a future evolution algebra being
// inferred from today's frontmatter.
func TestS0_UnsupportedShapesAreRefusedByName(t *testing.T) {
	cases := []struct {
		name, file, body, want string
	}{
		{"pure governance", "001-studio-detection-withdrawn.md", govAmendment, "apply-governance"},
		{"affects only", "001-readiness-wording.md", spliceAmendment, "ordinary refinement"},
		{"declares nothing", "001-declares-nothing.md", invalidAmendment, "no transition to apply"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupTestDir(t)
			cfg := testContext(t)
			featureDir := setupLedgerFeature(t, dir)
			intents, _ := os.ReadFile(filepath.Join(featureDir, "intents.md"))
			intents = append(intents, []byte("\n## Survives\n\n**Goal**: Stays.\n**Persona**: Admin\n")...)
			os.WriteFile(filepath.Join(featureDir, "intents.md"), intents, 0o644)
			writeAmendment(t, featureDir, tc.file, tc.body)
			saveLedgerBaseline(t, featureDir, func(b *Baseline) {
				b.LastAppliedAmendment = 0
				b.Sources.Amendments = map[string]string{}
			})
			writeRefineJournal(t, cfg, "my-feature", 1)

			armApplyAmendment(t, "", false)
			_, err := runApplyAmendment_(t, cfg, "@my-feature")
			if err == nil {
				t.Fatal("only the combined transition is supported; every other shape must refuse")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal must route the caller; got %v", err)
			}
		})
	}
}

// Exact-tail: this operation advances the marker past every record below it,
// so it applies exactly one.
func TestS0_MultipleUnappliedRecordsRefuse(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := strandedFeature(t, dir)
	writeAmendment(t, featureDir, "002-later.md", spliceAmendment)

	armApplyAmendment(t, "", false)
	_, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err == nil {
		t.Fatal("more than one unapplied record must refuse")
	}
	if !strings.Contains(err.Error(), "002-later") {
		t.Errorf("the refusal must name the records it will not advance past; got %v", err)
	}
}

// The recovery. This is the state the remediation created and could not exit:
// a valid, required, append-only record that no command would apply. It must
// be recoverable WITHOUT deleting or editing the amendment, and afterwards the
// feature must work normally again.
func TestS0_RecoversTheStrandedRecordWithoutTouchingIt(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := strandedFeature(t, dir)
	amendPath := filepath.Join(featureDir, "amendments", "001-readiness-withdrawn-and-reworded.md")
	recordBefore, err := os.ReadFile(amendPath)
	if err != nil {
		t.Fatal(err)
	}

	// 1. The save refuses, and now routes to the command that owns it rather
	//    than recommending a split that cannot work.
	sourceRoot := filepath.Join(dir, "cmd")
	sourceFile := filepath.Join(sourceRoot, "mine", "mine.go")
	writeMarkedFile(t, sourceFile, "my-feature", "r", "package mine")
	writeEmittedManifest(t, cfg, sourceFile)
	_, stderr, saveErr := runProjectSave(t, cfg, sourceRoot)
	if saveErr == nil {
		t.Fatal("the save must still refuse a combined record")
	}
	msg := saveErr.Error() + stderr
	if strings.Contains(msg, "Split it into a splice amendment") {
		t.Error("the refusal still recommends splitting, which cannot work: the accounting rule " +
			"is per-amendment, so a governance-only half trips it immediately")
	}
	if !strings.Contains(msg, "apply-amendment") {
		t.Errorf("the refusal must route to the command that owns this transition; got %q", msg)
	}
	saveBuildStatePartial = false
	saveBuildStateEmitted = ""

	// 2. apply-amendment recovers it.
	pf := digestFrom(t, cfg)
	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("the stranded record must be recoverable: %v", err)
	}

	// 3. The append-only record was never touched.
	recordAfter, err := os.ReadFile(amendPath)
	if err != nil {
		t.Fatalf("the amendment must still exist: %v", err)
	}
	if string(recordBefore) != string(recordAfter) {
		t.Error("recovery edited an append-only record")
	}

	// 4. And the feature works normally again: no pending tail, ledger sound,
	//    and an ordinary save proceeds.
	ca := computeCheckAmendments(cfg, "my-feature")
	for _, iss := range ca.Issues {
		if iss.Severity == "error" {
			t.Errorf("the ledger must be sound after recovery; got [%s] %s", iss.Code, iss.Message)
		}
	}
	if _, _, err := runProjectSave(t, cfg, sourceRoot); err != nil {
		t.Errorf("ordinary saves must work again after the transition applied: %v", err)
	}
}

// A record naming one real promise and one that does not exist must refuse
// outright. Presenting only the promises that resolved would bind an approval
// that never mentioned the others, then advance the whole amendment on it.
//
// The earlier "withdraws is empty" check missed this by construction: the list
// is non-empty, so nothing fired.
func TestS0_MixedValidAndUnknownPromisesRefuse(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := setupLedgerFeature(t, dir)
	intents, _ := os.ReadFile(filepath.Join(featureDir, "intents.md"))
	intents = append(intents, []byte("\n## Survives\n\n**Goal**: Stays.\n**Persona**: Admin\n")...)
	os.WriteFile(filepath.Join(featureDir, "intents.md"), intents, 0o644)
	os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"), 0o644)

	writeAmendment(t, featureDir, "001-mixed.md", `---
amendment: mixed
date: 2026-09-01
supersedes_intents:
  - check-readiness
  - no-such-promise
affects: ["@my-feature/operation:x"]
---

## Change
Withdraw one real promise and one that does not exist.

## Why
To prove the unknown one is not silently dropped.

## Acceptance
- Refused.
`)
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 0
		b.Sources.Amendments = map[string]string{}
	})
	writeRefineJournal(t, cfg, "my-feature", 1)

	armApplyAmendment(t, "", false)
	out, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err == nil {
		t.Fatal("a record naming a promise this feature does not declare must refuse")
	}
	if strings.Contains(out, "stop being in force") {
		t.Error("a promise list was shown for a record that cannot be soundly approved")
	}
	if bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature")); bl.LastAppliedAmendment != 0 {
		t.Error("the marker advanced on a refused record")
	}
}

// The ledger's structural rules are authoritative, and are checked before any
// promise is shown. A journal proves work happened; it proves nothing about
// whether the record itself is sound.
func TestS0_LedgerErrorsRefuseBeforeTheCeremony(t *testing.T) {
	cases := []struct {
		name string
		caps string
		want string
	}{
		{
			name: "affects does not resolve",
			caps: "operations: []\n",
			want: "amendment-affects-unresolved",
		},
		{
			name: "scope accounting incomplete",
			// A second entry sourced to the retiring promise that the
			// amendment does not account for.
			caps: "operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n  - id: y\n    kind: command\n    summary: does y\n    source: \"@my-feature/check-readiness\"\n",
			want: "intent-supersession-unaccounted-affect",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupTestDir(t)
			cfg := testContext(t)
			featureDir := setupLedgerFeature(t, dir)
			intents, _ := os.ReadFile(filepath.Join(featureDir, "intents.md"))
			intents = append(intents, []byte("\n## Survives\n\n**Goal**: Stays.\n**Persona**: Admin\n")...)
			os.WriteFile(filepath.Join(featureDir, "intents.md"), intents, 0o644)
			os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"), []byte(tc.caps), 0o644)
			writeAmendment(t, featureDir, "001-readiness-withdrawn-and-reworded.md", combinedAmendment)
			saveLedgerBaseline(t, featureDir, func(b *Baseline) {
				b.LastAppliedAmendment = 0
				b.Sources.Amendments = map[string]string{}
			})
			writeRefineJournal(t, cfg, "my-feature", 1)

			armApplyAmendment(t, "", false)
			out, err := runApplyAmendment_(t, cfg, "@my-feature")
			if err == nil {
				t.Fatal("an unsound ledger must refuse")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal must name the finding; got %v", err)
			}
			if strings.Contains(out, "stop being in force") {
				t.Error("no promise list may be shown over an unsound ledger")
			}
			if bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature")); bl.LastAppliedAmendment != 0 {
				t.Error("the marker advanced on a refused record")
			}
		})
	}
}

// A digest is a bearer token, and its security domain is every invocation that
// accepts it. Two features holding byte-identical records and capsules must not
// share one.
func TestS0_DigestIsBoundToItsFeature(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	strandedFeature(t, dir)
	mine := digestFrom(t, cfg)

	// An identical feature under a different slug.
	other := filepath.Join(dir, "spec", "intents", "other-feature")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "spec", "intents", "my-feature")
	for _, n := range []string{"intents.md", "capabilities.yaml"} {
		data, err := os.ReadFile(filepath.Join(src, n))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(other, n), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	amend, err := os.ReadFile(filepath.Join(src, "amendments", "001-readiness-withdrawn-and-reworded.md"))
	if err != nil {
		t.Fatal(err)
	}
	// The amendment's refs name my-feature; retarget them so the record is
	// valid for the copy, then assert the digests still differ.
	writeAmendment(t, other, "001-readiness-withdrawn-and-reworded.md",
		strings.ReplaceAll(string(amend), "@my-feature/", "@other-feature/"))

	payloadA, err := buildTransitionPayload("my-feature", parser.Amendment{
		Seq: 1, FileSlug: "x", Path: filepath.Join(src, "amendments", "001-readiness-withdrawn-and-reworded.md"),
	}, nil, nil, appliedAuthority{}, "p")
	if err != nil {
		t.Fatal(err)
	}
	payloadB := payloadA
	payloadB.Feature = "other-feature"
	dA, _ := transitionDigest(payloadA)
	dB, _ := transitionDigest(payloadB)
	if dA == dB {
		t.Error("two features sharing identical content produced the same approval token — a " +
			"token minted for one would authorise the other")
	}
	if mine.Digest == "" {
		t.Error("fixture produced no digest")
	}
}

// The confirmed write must not clobber a concurrent authority change. Atomic
// rename prevents a torn file, not a lost update.
func TestS0_ConfirmedWriteRefusesAConcurrentAuthorityChange(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := strandedFeature(t, dir)
	pf := digestFrom(t, cfg)

	// Somebody else advances this feature's authority after the approval was
	// obtained and before the confirmed run writes.
	writeAmendment(t, featureDir, "000-earlier.md", appliedAmendment)
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 0
		b.Sources.Amendments = map[string]string{"000-earlier.md": "abc123abc123abc1"}
	})

	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err == nil {
		t.Fatal("the confirmed write must refuse when the authority it approved from has moved")
	}
	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if bl.Sources.Amendments["000-earlier.md"] != "abc123abc123abc1" {
		t.Error("the concurrent writer's authority evidence was erased by a stale prior capsule")
	}
	if bl.LastAppliedAmendment != 0 {
		t.Error("the marker advanced despite the refusal")
	}
}

// A cooperative lock protects only an invariant every cooperating writer
// shares. A lock held by one command and skipped by the others is not
// exclusion — it is the same lost-update race with a different opponent. These
// prove the compare half fires for each authority writer.

func TestS0_SaveRefusesAStaleAuthorityPlan(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)
	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)
	writeEmittedManifest(t, cfg, sourceFile)

	planned, err := observeAppliedAuthority(cfg, "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	// A concurrent writer advances the authority after the plan was made.
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 1
		b.Sources.Amendments = map[string]string{
			"001-first.md":              "aaaaaaaaaaaaaaaa",
			"999-someone-elses-work.md": "bbbbbbbbbbbbbbbb",
		}
	})

	err = withVerifiedAuthority(cfg, "my-feature", planned, func(appliedAuthority) error {
		t.Error("the transaction body must not run over a moved authority")
		return nil
	})
	if err == nil {
		t.Fatal("a stale plan must refuse")
	}
	if !strings.Contains(err.Error(), "changed while this operation was preparing") {
		t.Errorf("the refusal must say the authority moved; got %v", err)
	}
	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if bl.Sources.Amendments["999-someone-elses-work.md"] != "bbbbbbbbbbbbbbbb" {
		t.Error("the concurrent writer's evidence was erased")
	}
	_ = sourceRoot
}

// Every file that mutates an authority capsule or replaces a baseline must
// enter the shared transaction boundary.
//
// Honest about what this does: it enumerates production callers of
// applyAuthorityCapsule and marshalBaseline and requires each file to reference
// withVerifiedAuthority. That DOES notice a new writer added to a file with no
// boundary reference, which a fixed three-file list would not. It does not
// notice a writer that replaces the baseline by some other means entirely; the
// durable defence is centralising baseline replacement, and this is the cheap
// approximation until then.
func TestS0_EveryBaselineWriterEntersTheBoundary(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := string(body)
		writes := strings.Contains(src, "applyAuthorityCapsule(") || strings.Contains(src, "marshalBaseline(")
		if !writes {
			continue
		}
		// baseline.go defines the capsule writer itself; the boundary lives
		// in authority_preflight.go.
		if name == "baseline.go" || name == "authority_preflight.go" {
			continue
		}
		checked++
		if !strings.Contains(src, "withVerifiedAuthority") {
			t.Errorf("%s writes a feature baseline but never enters withVerifiedAuthority — a "+
				"lock one writer skips is not exclusion", name)
		}
	}
	if checked == 0 {
		t.Error("the scan found no baseline writers, so it is not testing anything")
	}
}

// A receipt must be checkable against itself: the key names the amendment it
// describes, and the digest is recomputable from the stored payload.
func TestS0_ReceiptValidatesAgainstItself(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	strandedFeature(t, dir)
	pf := digestFrom(t, cfg)
	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatal(err)
	}

	const key = "001-readiness-withdrawn-and-reworded.md"
	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	receipt := bl.TransitionReceipts[key]
	if err := receipt.Validate(key); err != nil {
		t.Errorf("a freshly written receipt must validate: %v", err)
	}
	// The stored payload must be complete enough to recompute the digest —
	// an earlier version stored a subset and could not be checked at all.
	if receipt.Payload.Feature == "" || receipt.Payload.AmendmentHash == "" ||
		receipt.Payload.SpliceProof == "" || receipt.Payload.AmendmentSeq == 0 {
		t.Errorf("the receipt payload is missing domain fields; got %+v", receipt.Payload)
	}

	// Filed under the wrong key.
	if err := receipt.Validate("some-other-amendment.md"); err == nil {
		t.Error("a receipt filed under a key it does not describe must not validate")
	}
	// Mutated content, digest left alone.
	tampered := receipt
	tampered.Payload.Withdraws = append([]withdrawnPromise(nil), receipt.Payload.Withdraws...)
	tampered.Payload.Withdraws[0].Goal = "something nobody approved"
	if err := tampered.Validate(key); err == nil {
		t.Error("a receipt whose payload no longer matches its digest must not validate")
	}
}

// Prior receipts are covered by digest, not by presence. An earlier version
// recorded presence only, so a changed prior receipt produced the same next
// token.
func TestS0_PriorReceiptsAreCoveredByDigestNotPresence(t *testing.T) {
	a := appliedAuthority{Receipts: map[string]TransitionReceipt{"001-x.md": {Digest: "aaaa"}}}
	b := appliedAuthority{Receipts: map[string]TransitionReceipt{"001-x.md": {Digest: "bbbb"}}}
	if sameAuthority(a, b) {
		t.Error("two capsules whose receipts differ in content must not compare equal")
	}
	if got := receiptDigests(a.Receipts)["001-x.md"]; got != "aaaa" {
		t.Errorf("receiptDigests must carry the digest, not presence; got %q", got)
	}

	rec := parser.Amendment{Seq: 2, Path: "/nonexistent"}
	_ = rec
	pa := transitionPayload{Prior: priorCapsuleSnapshot{Receipts: receiptDigests(a.Receipts)}}
	pb := transitionPayload{Prior: priorCapsuleSnapshot{Receipts: receiptDigests(b.Receipts)}}
	da, _ := transitionDigest(pa)
	db, _ := transitionDigest(pb)
	if da == db {
		t.Error("a changed prior receipt must change the next approval token")
	}
}

// A validator nothing calls protects nothing. These drive the real read path:
// a receipt that is unsound must make observation FAIL, not be copied forward
// into trusted authority.
func TestS0_UnsoundReceiptsFailAuthorityObservation(t *testing.T) {
	const key = "001-readiness-withdrawn-and-reworded.md"
	cases := []struct {
		name   string
		mutate func(*Baseline)
		want   string
		// writers says whether the writer assertions are meaningful for this
		// mutation. The marker mutation puts the combined record back into the
		// pending tail, so governance refuses on its shape guard before ever
		// reading the capsule — a correct refusal, but not the one under test.
		writers bool
	}{
		{
			name:    "payload mutated, digest left alone",
			writers: true,
			mutate: func(b *Baseline) {
				r := b.TransitionReceipts[key]
				r.Payload.Withdraws[0].Goal = "something nobody approved"
				b.TransitionReceipts[key] = r
			},
			want: "internally inconsistent",
		},
		{
			name:    "filed under the wrong key",
			writers: true,
			mutate: func(b *Baseline) {
				r := b.TransitionReceipts[key]
				delete(b.TransitionReceipts, key)
				b.TransitionReceipts["999-elsewhere.md"] = r
			},
			want: "describes",
		},
		{
			name:    "records another feature",
			writers: true,
			mutate: func(b *Baseline) {
				r := b.TransitionReceipts[key]
				r.Payload.Feature = "someone-elses-feature"
				d, _ := transitionDigest(r.Payload)
				r.Digest = d
				b.TransitionReceipts[key] = r
			},
			want: "but is stored under",
		},
		{
			name:    "amendment hash disagrees with the capsule",
			writers: true,
			mutate: func(b *Baseline) {
				b.Sources.Amendments[key] = "0000badc0ffee000"
			},
			want: "the capsule records",
		},
		{
			name: "sequence above the applied marker",
			mutate: func(b *Baseline) {
				b.LastAppliedAmendment = 0
			},
			want: "above the applied marker",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupTestDir(t)
			cfg := testContext(t)
			featureDir := strandedFeature(t, dir)
			pf := digestFrom(t, cfg)
			armApplyAmendment(t, pf.Digest, false)
			if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
				t.Fatalf("seed: %v", err)
			}

			blPath := baselinePath(cfg, "my-feature")
			bl := readFeatureBaseline(t, blPath)
			tc.mutate(&bl)
			data, err := marshalBaseline(&bl)
			if err != nil {
				t.Fatal(err)
			}
			corrupted := string(data)
			if err := os.WriteFile(blPath, data, 0o644); err != nil {
				t.Fatal(err)
			}

			if _, err := observeAppliedAuthority(cfg, "my-feature"); err == nil {
				t.Error("an unsound receipt must fail authority observation, not be copied " +
					"forward into trusted authority")
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the error must say what is unsound; got %v", err)
			}

			if !tc.writers {
				return
			}
			// And no writer proceeds over it. Each writer is given REAL work
			// first, so its refusal cannot be confused with having nothing to
			// do — governance gets a genuinely pending record, and the save
			// gets a valid emission.
			writeAmendment(t, featureDir, "002-studio-detection-withdrawn.md", govAmendment)
			applyGovernanceConfirmed = true
			t.Cleanup(func() { applyGovernanceConfirmed = false })
			govOut, govErr := runApplyGovernance_(t, cfg, "@my-feature")
			if govErr == nil {
				t.Error("apply-governance proceeded over an unsound receipt")
			} else if !strings.Contains(govErr.Error()+govOut, "receipt") {
				t.Errorf("governance must refuse BECAUSE the receipt is unsound, not for some "+
					"other reason; got %v", govErr)
			}

			sourceRoot := filepath.Join(dir, "cmd")
			sourceFile := filepath.Join(sourceRoot, "mine", "mine.go")
			writeMarkedFile(t, sourceFile, "my-feature", "r", "package mine")
			emittedPath := writeEmittedManifest(t, cfg, sourceFile)
			projBefore, projExisted := os.ReadFile(projectBaselinePath(cfg))
			_, saveStderr, saveErr := runProjectSave(t, cfg, sourceRoot)
			if saveErr == nil {
				t.Error("the save proceeded over an unsound receipt")
			} else if !strings.Contains(saveErr.Error()+saveStderr, "receipt") {
				t.Errorf("the save must refuse BECAUSE the receipt is unsound; got %v", saveErr)
			}

			after, err := os.ReadFile(blPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != corrupted {
				t.Error("a writer rewrote a baseline whose receipt could not be validated")
			}
			if _, err := os.Stat(emittedPath); err != nil {
				t.Errorf("the manifest was consumed by a refused save: %v", err)
			}
			projAfter, projExistsNow := os.ReadFile(projectBaselinePath(cfg))
			if (projExisted == nil) != (projExistsNow == nil) || string(projBefore) != string(projAfter) {
				t.Error("project-level state changed despite the refusal")
			}
		})
	}
}

// holdAuthorityLock takes a feature's authority lock and returns a release fn,
// so a test can create real contention rather than simulating it.
func holdAuthorityLock(t *testing.T, cfg *config.Context, slug string) func() {
	t.Helper()
	dir := cfg.BuildPath(slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lock := flock.New(filepath.Join(dir, authorityLockName))
	ok, err := lock.TryLock()
	if err != nil || !ok {
		t.Fatalf("could not take the authority lock for the test: %v", err)
	}
	return func() { _ = lock.Unlock() }
}

func shortenAuthorityLockWait(t *testing.T) {
	t.Helper()
	prevWait, prevRetry := authorityLockWait, authorityLockRetry
	authorityLockWait, authorityLockRetry = 150*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { authorityLockWait, authorityLockRetry = prevWait, prevRetry })
}

// An authority conflict must fail the SAVE — the real command — not be filed as
// an ordinary skipped feature that lets the run continue into stage 2 and
// manifest consumption.
//
// An earlier version of this test called withVerifiedAuthority directly and
// then checked a manifest no command had touched, so deleting save's
// errAuthorityConflict branch would have left it green. It now drives
// runProjectSave into real lock contention.
func TestS0_AuthorityConflictFailsTheSaveRatherThanSkipping(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)

	// A successful save first, so there is project-level state to compare.
	if _, _, err := runProjectSave(t, cfg, sourceRoot); err != nil {
		t.Fatalf("seed save: %v", err)
	}
	projBefore, err := os.ReadFile(projectBaselinePath(cfg))
	if err != nil {
		t.Fatal(err)
	}
	blBefore, err := os.ReadFile(baselinePath(cfg, "my-feature"))
	if err != nil {
		t.Fatal(err)
	}

	writeAmendment(t, featureDir, "002-readiness-wording.md", spliceAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)
	emittedPath := writeEmittedManifest(t, cfg, sourceFile)

	shortenAuthorityLockWait(t)
	release := holdAuthorityLock(t, cfg, "my-feature")
	defer release()

	res, _, err := runProjectSave(t, cfg, sourceRoot)
	if err == nil {
		t.Fatalf("the save must fail on an authority conflict, not report success; result = %+v", res)
	}
	if !errors.Is(err, errAuthorityConflict) {
		t.Errorf("the failure must be identifiable as an authority conflict rather than an "+
			"ordinary skipped feature; got %v", err)
	}
	if res != nil {
		for _, s := range res.Skipped {
			if s.Slug == "my-feature" {
				t.Error("the conflict was filed as a skipped feature, which is how a failed " +
					"comparison becomes a successful partial save")
			}
		}
	}

	if projAfter, err := os.ReadFile(projectBaselinePath(cfg)); err != nil {
		t.Fatal(err)
	} else if string(projBefore) != string(projAfter) {
		t.Error("stage 2 ran after an authority conflict")
	}
	if blAfter, err := os.ReadFile(baselinePath(cfg, "my-feature")); err != nil {
		t.Fatal(err)
	} else if string(blBefore) != string(blAfter) {
		t.Error("the feature baseline was written despite the conflict")
	}
	if _, err := os.Stat(emittedPath); err != nil {
		t.Errorf("the manifest was consumed by a save that failed: %v", err)
	}
}

// Governance must read the baseline it modifies UNDER the lock.
//
// The mutation has to land AFTER governance has planned and BEFORE it acquires
// the lock, or the test proves nothing: the pre-lock-read implementation would
// have read a mutation made earlier and preserved it happily. The lock-attempt
// hook makes that interleaving deterministic rather than timing-dependent.
func TestS0_GovernanceDoesNotClobberNonAuthorityContent(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := setupLedgerFeature(t, dir)
	intents, _ := os.ReadFile(filepath.Join(featureDir, "intents.md"))
	intents = append(intents, []byte("\n## Survives\n\n**Goal**: Stays.\n**Persona**: Admin\n")...)
	os.WriteFile(filepath.Join(featureDir, "intents.md"), intents, 0o644)
	writeAmendment(t, featureDir, "001-studio-detection-withdrawn.md", govAmendment)
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 0
		b.Sources.Amendments = map[string]string{}
	})

	blPath := baselinePath(cfg, "my-feature")
	reached := make(chan struct{})
	proceed := make(chan struct{})
	var once sync.Once
	authorityLockAttemptHook = func(slug string) {
		if slug == "my-feature" {
			once.Do(func() { close(reached) })
			<-proceed
		}
	}
	t.Cleanup(func() { authorityLockAttemptHook = nil })

	applyGovernanceConfirmed = true
	t.Cleanup(func() { applyGovernanceConfirmed = false })

	done := make(chan error, 1)
	go func() {
		_, err := runApplyGovernance_(t, cfg, "@my-feature")
		done <- err
	}()

	// Governance has planned and read nothing yet; it is about to take the
	// lock. A concurrent writer changes NON-authority content now.
	<-reached
	bl := readFeatureBaseline(t, blPath)
	bl.BuildfileSections = map[string]string{"routes": "written-by-a-concurrent-save"}
	data, err := marshalBaseline(&bl)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	close(proceed)

	if err := <-done; err != nil {
		t.Fatalf("apply-governance: %v", err)
	}

	after := readFeatureBaseline(t, blPath)
	if after.LastAppliedAmendment != 1 {
		t.Errorf("the governance advance did not happen; marker = %d", after.LastAppliedAmendment)
	}
	if after.BuildfileSections["routes"] != "written-by-a-concurrent-save" {
		t.Errorf("governance wrote a baseline it read BEFORE the lock, restoring stale content "+
			"over a concurrent writer; buildfile-sections = %v", after.BuildfileSections)
	}
}

// A vocabulary that lands with no applier must be INERT, not opportunistically
// applied by whichever path happens to accept its other fields. This is the
// WP4 mistake in reverse: there, a path existed that no guidance named; here, a
// record shape exists that no ceremony owns.
func TestStage1_EvolutionRecordsAreInertUntilTheirApplierExists(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)

	writeAmendment(t, featureDir, "002-channel-choice.md", `---
amendment: channel-choice
date: 2026-09-01
amends_intents:
  - intent: check-readiness
    mode: revise
    goal: See if the cluster or any of its nodes is ready.
    verify:
      - Readiness is reported for the cluster and each node.
affects: ["@my-feature/operation:x"]
---

## Change
The readiness promise now covers nodes as well as the cluster.

## Why
It was too narrow.

## Acceptance
- Node readiness is reported.
`)
	writeRefineJournal(t, cfg, "my-feature", 2)

	// The save refuses, naming the reason.
	writeEmittedManifest(t, cfg, sourceFile)
	_, stderr, err := runProjectSave(t, cfg, sourceRoot)
	if err == nil {
		t.Fatal("a save must not apply an evolution record")
	}
	if !strings.Contains(err.Error()+stderr, "amends_intents") {
		t.Errorf("the refusal must name the vocabulary rather than misclassifying the record by "+
			"its other fields; got %v", err)
	}
	saveBuildStatePartial = false
	saveBuildStateEmitted = ""

	// apply-amendment refuses too, and says the ceremony does not exist yet
	// rather than routing it somewhere.
	armApplyAmendment(t, "", false)
	_, err = runApplyAmendment_(t, cfg, "@my-feature")
	if err == nil {
		t.Fatal("apply-amendment must not apply an evolution record")
	}
	if !strings.Contains(err.Error(), "no applier exists") {
		t.Errorf("the refusal must say the ceremony does not exist yet; got %v", err)
	}

	// apply-governance refuses as well.
	applyGovernanceConfirmed = true
	t.Cleanup(func() { applyGovernanceConfirmed = false })
	if _, err := runApplyGovernance_(t, cfg, "@my-feature"); err == nil {
		t.Error("apply-governance must not apply an evolution record")
	}

	// Nothing moved.
	if bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature")); bl.LastAppliedAmendment != 1 {
		t.Errorf("an inert record was applied; marker = %d", bl.LastAppliedAmendment)
	}
}
