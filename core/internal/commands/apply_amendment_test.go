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

// The save still creates no authority for an evolution record — that has not
// changed and must not. What changed in 1b is that apply-amendment now owns the
// ceremony, so the refusal routes there instead of saying nothing can apply it.
func TestStage1_SaveStillCreatesNoAuthorityForEvolutionRecords(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir, sourceRoot, sourceFile := ledgerFeatureWithSource(t, dir)
	ledgerAt1(t, cfg, featureDir)
	writeAmendment(t, featureDir, "002-channel-choice.md", evolveAmendment)
	writeRefineJournal(t, cfg, "my-feature", 2)
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

	applyGovernanceConfirmed = true
	t.Cleanup(func() { applyGovernanceConfirmed = false })
	if _, err := runApplyGovernance_(t, cfg, "@my-feature"); err == nil {
		t.Error("apply-governance must not apply an evolution record")
	}

	if bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature")); bl.LastAppliedAmendment != 1 {
		t.Errorf("a record only apply-amendment may apply was applied elsewhere; marker = %d",
			bl.LastAppliedAmendment)
	}
}

// ---------------------------------------------------------------------
// Stage 1b — the evolve ceremony.
// ---------------------------------------------------------------------

const evolveAmendment = `---
amendment: channel-choice
date: 2026-09-01
amends_intents:
  - intent: check-readiness
    mode: revise
    version:
      title: Check Readiness Of Cluster Or Node
      goal: See if the cluster or any of its nodes is ready.
      persona: Admin
      verify:
        - Readiness is reported for the cluster and each node.
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions: []
affects: ["@my-feature/operation:x"]
---

## Change

Readiness now covers nodes as well as the cluster.

## Why

The promise was too narrow.

## Acceptance

- Node readiness is reported.
`

// evolvingFeature: a built feature whose founding promise owns two live
// operations, one of which this record declares it changes.
func evolvingFeature(t *testing.T, dir string) string {
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
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"+
			"  - id: y\n    kind: query\n    summary: does y\n    source: \"@my-feature/check-readiness\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeAmendment(t, featureDir, "001-channel-choice.md", evolveAmendment)
	saveLedgerBaseline(t, featureDir, func(b *Baseline) {
		b.LastAppliedAmendment = 0
		b.Sources.Amendments = map[string]string{}
	})
	writeRefineJournal(t, cfg, "my-feature", 1)
	return featureDir
}

func evolvePreflight(t *testing.T, cfg *config.Context) applyAmendmentPreflight {
	t.Helper()
	armApplyAmendment(t, "", true)
	out, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err != nil {
		t.Fatalf("evolve preflight: %v\n%s", err, out)
	}
	var pf applyAmendmentPreflight
	if err := json.Unmarshal([]byte(out), &pf); err != nil {
		t.Fatalf("preflight is not JSON: %v\n%s", err, out)
	}
	return pf
}

// The ceremony answers three questions, not one: what changed, what claim the
// human is making, and which downstream entries that claim covers.
func TestStage1b_CeremonyShowsDeltaAttestationAndScope(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	evolvingFeature(t, dir)

	armApplyAmendment(t, "", false)
	prose, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	// What changed — field-aware, so a cleared field is visible.
	for _, want := range []string{"goal", "before:", "after:", "See if the cluster is ready"} {
		if !strings.Contains(prose, want) {
			t.Errorf("the delta must show %q; got:\n%s", want, prose)
		}
	}
	// What claim.
	if !strings.Contains(prose, "You are asserting") || !strings.Contains(prose, "before/after replacement") {
		t.Errorf("the ceremony must state the claim the human is making; got:\n%s", prose)
	}
	if !strings.Contains(prose, "Nothing checks those two claims") {
		t.Error("the ceremony must not imply the mode was verified")
	}
	// Which subjects — both attributed entries, partitioned.
	if !strings.Contains(prose, "@my-feature/operation:x   [this record declares it changed]") {
		t.Errorf("the declared entry must be shown as declared; got:\n%s", prose)
	}
	if !strings.Contains(prose, "@my-feature/operation:y   [not declared changed]") {
		t.Errorf("the undeclared attributed entry must be shown, since it is what the closure "+
			"assertion is about; got:\n%s", prose)
	}
}

// A confirmed run applies, and the receipt records the whole subject.
func TestStage1b_ConfirmedEvolveAppliesAndReceiptsTheSubject(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	evolvingFeature(t, dir)
	pf := evolvePreflight(t, cfg)

	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("a correctly confirmed evolution must apply: %v", err)
	}

	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if bl.LastAppliedAmendment != 1 {
		t.Errorf("marker = %d, want 1", bl.LastAppliedAmendment)
	}
	r := bl.TransitionReceipts["001-channel-choice.md"]
	if r.Payload.Mode != transitionModeEvolve {
		t.Errorf("receipt mode = %q", r.Payload.Mode)
	}
	if len(r.Payload.Evolution) != 1 {
		t.Fatalf("the receipt must record the subject; got %+v", r.Payload.Evolution)
	}
	ev := r.Payload.Evolution[0]
	if ev.Attestation == "" {
		t.Error("the receipt must record the claim that was made, not merely that one was")
	}
	if !ev.PreservesUnlisted {
		t.Error("the closure declaration must be recorded")
	}
	if len(ev.Scope.Unlisted) != 1 || ev.Scope.Unlisted[0].Ref != "@my-feature/operation:y" {
		t.Errorf("the receipt must record the exact population the claim covered; got %+v", ev.Scope)
	}
	if ev.Scope.Unlisted[0].Fingerprint == "" {
		t.Error("the receipt must record what each entry MEANT, not only its address — the human " +
			"asserted continued support of a specific promise, not of a ref")
	}

	// And the promise now reads differently.
	res, err := resolveActiveIntents(cfg, "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range res.Active {
		if in.Slug == "check-readiness" && !strings.Contains(in.Goal, "any of its nodes") {
			t.Errorf("the revised promise is not in force; goal = %q", in.Goal)
		}
	}
}

// The stage boundary, as Stage 2 leaves it. Narrowing and declared exceptions
// are applicable now that their consequences are collected; a RETIREMENT is
// not, because ending a promise and rewording one are different approvals and
// one form cannot stand in for the other.
func TestStage2_UnsupportedTransitionsRefuseByReason(t *testing.T) {
	cases := []struct{ name, frontmatter, want string }{
		{
			name: "retire belongs to the withdrawal ceremony",
			frontmatter: `amends_intents:
  - intent: check-readiness
    mode: retire
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions: []
`,
			want: "not a change to a promise but the end of one",
		},
		{
			name: "no scope declaration at all",
			frontmatter: `amends_intents:
  - intent: check-readiness
    mode: revise
    version:
      title: T
      goal: A reworded promise.
      persona: Admin
`,
			want: "declares no scope_impact",
		},
		{
			name: "disposition contradicts the contract",
			frontmatter: `amends_intents:
  - intent: check-readiness
    mode: narrow
    version:
      title: T
      goal: A narrower promise.
      persona: Admin
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions:
    - intent: check-readiness
      ref: "@my-feature/operation:y"
      disposition: removed
`,
			want: "still in the contract",
		},
		{
			name: "narrowing that takes nothing away",
			frontmatter: `amends_intents:
  - intent: check-readiness
    mode: narrow
    version:
      title: T
      goal: A narrower promise.
      persona: Admin
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions: []
`,
			want: "takes nothing away is a revision",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupTestDir(t)
			cfg := testContext(t)
			featureDir := evolvingFeature(t, dir)
			if err := os.Remove(filepath.Join(featureDir, "amendments", "001-channel-choice.md")); err != nil {
				t.Fatal(err)
			}
			writeAmendment(t, featureDir, "001-unsupported.md",
				"---\namendment: unsupported\ndate: 2026-09-01\n"+tc.frontmatter+
					"---\n\n## Change\nSomething.\n\n## Why\nBecause.\n\n## Acceptance\n- Done.\n")
			// Re-capture for the record actually under test: an inventory is
			// evidence about the amendment it was taken for.
			writeRefineJournal(t, cfg, "my-feature", 1)

			armApplyAmendment(t, "", false)
			out, err := runApplyAmendment_(t, cfg, "@my-feature")
			if err == nil {
				t.Fatal("this transition must refuse")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal must say why; got %v", err)
			}
			if strings.Contains(out, "You are asserting") {
				t.Error("no attestation may be requested for a transition that cannot be applied")
			}
			if bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature")); bl.LastAppliedAmendment != 0 {
				t.Error("a refused transition advanced the marker")
			}
		})
	}
}

// A record touching several lineages is all-or-nothing: one unsupported
// transition refuses the whole thing, because a partly applied record leaves a
// state no reader can classify.
func TestStage1b_MultipleLineagesAreAllOrNothing(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)
	if err := os.Remove(filepath.Join(featureDir, "amendments", "001-channel-choice.md")); err != nil {
		t.Fatal(err)
	}
	writeAmendment(t, featureDir, "001-mixed.md", `---
amendment: mixed
date: 2026-09-01
amends_intents:
  - intent: check-readiness
    mode: revise
    version:
      title: T
      goal: A reworded promise.
      persona: Admin
  - intent: survives
    mode: narrow
    version:
      title: S
      goal: A narrower promise.
      persona: Admin
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions: []
---

## Change
Two lineages, one of them unsupported.

## Why
To prove it is all or nothing.

## Acceptance
- Refused.
`)
	writeRefineJournal(t, cfg, "my-feature", 1)
	armApplyAmendment(t, "", false)
	_, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err == nil {
		t.Fatal("one unsupported transition must refuse the whole record")
	}
	if !strings.Contains(err.Error(), "survives") {
		t.Errorf("the refusal must name the lineage it cannot apply; got %v", err)
	}
	if bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature")); bl.LastAppliedAmendment != 0 {
		t.Error("a partly applicable record was partly applied")
	}
}

// The approval is bound to the exact attributed population. Attribution comes
// from mutable contract artifacts, so the capsule comparison says nothing about
// it: without this binding, a contract edit between preflight and confirmation
// changes the subject the claim was about while leaving the token valid.
func TestStage1b_TokenIsBoundToTheAttributedPopulation(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)
	pf := evolvePreflight(t, cfg)

	// A third entry becomes attributed to the same promise after approval.
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"+
			"  - id: y\n    kind: query\n    summary: does y\n    source: \"@my-feature/check-readiness\"\n"+
			"  - id: z\n    kind: query\n    summary: does z\n    source: \"@my-feature/check-readiness\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	armApplyAmendment(t, pf.Digest, false)
	_, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err == nil {
		t.Fatal("an approval must not survive a change to the population it covered")
	}
	if bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature")); bl.LastAppliedAmendment != 0 {
		t.Error("the marker advanced despite the subject having changed")
	}
}

// A sequential revision uses the PREVIOUSLY APPLIED version as its before, not
// the founding text — approving a delta from a version nobody is running would
// describe a change that is not the one being made.
func TestStage1b_SequentialRevisionDiffsFromTheAppliedVersion(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)

	// Apply the first revision.
	pf := evolvePreflight(t, cfg)
	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("first revision: %v", err)
	}

	// A second one on top.
	writeAmendment(t, featureDir, "002-again.md", `---
amendment: again
date: 2026-09-02
amends_intents:
  - intent: check-readiness
    mode: revise
    version:
      title: Check Readiness Everywhere
      goal: See if anything in the fleet is ready.
      persona: Admin
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions: []
---

## Change
Broader still.

## Why
Fleet-wide now.

## Acceptance
- Fleet readiness is reported.
`)
	writeRefineJournal(t, cfg, "my-feature", 2)

	armApplyAmendment(t, "", false)
	prose, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err != nil {
		t.Fatalf("second preflight: %v", err)
	}
	if !strings.Contains(prose, "any of its nodes") {
		t.Errorf("the before must be the APPLIED version, not the founding text; got:\n%s", prose)
	}
	if strings.Contains(prose, "before: See if the cluster is ready.") {
		t.Error("the delta was taken from the founding text, describing a change nobody is making")
	}
}

// A receipt must survive its own storage round-trip. This is the regression for
// a real defect: the digest was minted over an in-memory payload while
// validation recomputed it from the decoded one, and YAML decodes an omitted
// list as empty rather than nil — so a freshly written receipt failed its own
// validation and made the feature's authority unreadable.
func TestStage1b_ReceiptSurvivesItsOwnStorageRoundTrip(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	evolvingFeature(t, dir)
	pf := evolvePreflight(t, cfg)
	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// The authority must remain readable — this is what broke.
	if _, err := observeAppliedAuthority(cfg, "my-feature"); err != nil {
		t.Fatalf("a freshly written receipt made the capsule unreadable: %v", err)
	}
	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	const key = "001-channel-choice.md"
	if err := bl.TransitionReceipts[key].Validate(key); err != nil {
		t.Errorf("a freshly written receipt must validate against itself: %v", err)
	}
}

// The approval is about what each entry MEANS, not where it lives. An edit that
// rewrites an operation while keeping its id and source produces the same ref
// and a different promise — my own counterexample, now pinned.
//
// The clean fixture's subject is asserted FIRST, so the refusal cannot be
// coming from an earlier guard.
func TestStage1b_TokenIsBoundToEntrySemanticsNotJustRefs(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)

	// Precondition: the clean fixture derives the exact expected subject.
	scopes, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"},
		[]string{"@my-feature/operation:x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(scopes) != 1 || len(scopes[0].Unlisted) != 1 ||
		scopes[0].Unlisted[0].Ref != "@my-feature/operation:y" {
		t.Fatalf("fixture: expected @my-feature/operation:y as the unlisted subject; got %+v", scopes)
	}

	pf := evolvePreflight(t, cfg)

	// Exactly one mutation: y's acceptance criteria change — its id and source
	// do not. A field the PARSER understands, deliberately: the fingerprint is
	// taken over the parsed entry, so it cannot see a key the parser drops.
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"+
			"  - id: y\n    kind: query\n    summary: does y\n    source: \"@my-feature/check-readiness\"\n"+
			"    verify:\n      - It now promises something it did not promise before.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"},
		[]string{"@my-feature/operation:x"})
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Unlisted[0].Ref != "@my-feature/operation:y" {
		t.Fatal("fixture: the ref must be unchanged, or this tests ref binding rather than semantics")
	}

	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err == nil {
		t.Fatal("an approval must not survive a change to what an attributed entry promises, " +
			"even when its address is identical")
	}
	if bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature")); bl.LastAppliedAmendment != 0 {
		t.Error("the marker advanced despite the subject having changed")
	}
}

// Deriving the population must FAIL CLOSED. An artifact that exists and will
// not parse is an unknown, and an unknown cannot be approved — an earlier
// version skipped it, so a file holding only unlisted attributed entries could
// produce an "exact population" with a hole in it.
func TestStage1b_UnreadableArtifactRefusesTheScopeDerivation(t *testing.T) {
	cases := []struct{ name, file, corrupt string }{
		{"capabilities", "capabilities.yaml", "operations: [ this is not: valid yaml\n"},
		{"surface", "surface.yaml", "fragments: [ broken\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupTestDir(t)
			featureDir := evolvingFeature(t, dir)

			// Precondition: clean, the derivation names the concrete subject.
			clean, err := deriveLineageScope(featureDir, "my-feature",
				[]string{"check-readiness"}, nil)
			if err != nil {
				t.Fatalf("fixture: the clean derivation must succeed: %v", err)
			}
			if len(clean) != 1 || len(clean[0].Unlisted) < 1 {
				t.Fatalf("fixture: expected attributed entries; got %+v", clean)
			}

			// Exactly one mutation.
			if err := os.WriteFile(filepath.Join(featureDir, tc.file), []byte(tc.corrupt), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err = deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"}, nil)
			if err == nil {
				t.Fatal("an artifact that exists and will not parse must refuse, not be skipped")
			}
			if !strings.Contains(err.Error(), "attributed to this promise") {
				t.Errorf("the refusal must identify it as the scope read rather than any error; "+
					"got %v", err)
			}
		})
	}
}

// The amendment itself can change between approval and the lock. Without a
// strict rehash inside it, the capsule would be written with the CURRENT bytes
// while the receipt kept the approved hash — surfacing only later as an
// unreadable capsule.
func TestStage1b_AmendmentChangedBeforeTheLockRefuses(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)
	pf := evolvePreflight(t, cfg)

	path := filepath.Join(featureDir, "amendments", "001-channel-choice.md")
	blBefore, err := os.ReadFile(baselinePath(cfg, "my-feature"))
	if err != nil {
		t.Fatal(err)
	}

	// Mutate the record exactly at the lock boundary, deterministically.
	var once sync.Once
	authorityLockAttemptHook = func(slug string) {
		once.Do(func() {
			body, rerr := os.ReadFile(path)
			if rerr != nil {
				t.Error(rerr)
				return
			}
			_ = os.WriteFile(path, append(body, []byte("\nQuietly appended after approval.\n")...), 0o644)
		})
	}
	t.Cleanup(func() { authorityLockAttemptHook = nil })

	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err == nil {
		t.Fatal("a record edited between approval and the lock must refuse")
	}
	blAfter, err := os.ReadFile(baselinePath(cfg, "my-feature"))
	if err != nil {
		t.Fatal(err)
	}
	if string(blBefore) != string(blAfter) {
		t.Error("the baseline was written for a record that had changed")
	}
	if _, err := observeAppliedAuthority(cfg, "my-feature"); err != nil {
		t.Errorf("the capsule must remain readable after the refusal: %v", err)
	}
}

// The acceptance test for the raw-entry subject: `summary` is real semantic
// contract text that today's artifacts carry, and the parser drops it. An
// earlier fingerprint over the PARSED entry left an edit confined to it
// invisible — a live approval-token bypass, not a future limitation.
func TestStage1b_TokenIsBoundToUnmodelledEntryText(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)

	clean, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"},
		[]string{"@my-feature/operation:x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(clean) != 1 || len(clean[0].Unlisted) != 1 ||
		clean[0].Unlisted[0].Ref != "@my-feature/operation:y" {
		t.Fatalf("fixture: expected @my-feature/operation:y as the unlisted subject; got %+v", clean)
	}
	pf := evolvePreflight(t, cfg)

	// EXACTLY one mutation, to a key the parser does not model at all.
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"+
			"  - id: y\n    kind: query\n    summary: now promises something entirely different\n    source: \"@my-feature/check-readiness\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"},
		[]string{"@my-feature/operation:x"})
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Unlisted[0].Ref != clean[0].Unlisted[0].Ref {
		t.Fatal("fixture: the ref must be unchanged, or this tests ref binding")
	}
	if after[0].Unlisted[0].Fingerprint == clean[0].Unlisted[0].Fingerprint {
		t.Fatal("the fingerprint did not move for a change to real contract text — the subject " +
			"is still bound to what the parser happens to model")
	}

	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err == nil {
		t.Fatal("an approval must not survive a change to an entry's meaning, including text " +
			"the parser does not model")
	}
}

// Infrastructure has its own parsing and fingerprint path, so it needs its own
// witnesses rather than riding on the capabilities table.
func TestStage1b_InfrastructureScopeIsBoundAndFailsClosed(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := evolvingFeature(t, dir)
	infra := filepath.Join(featureDir, "infrastructure.md")
	body := "# Infra\n\n## Readiness probe boundary\n\n**Affects**: the probe\n" +
		"**Behavior**: it probes.\n**Source**: @my-feature/check-readiness\n"
	if err := os.WriteFile(infra, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// Precondition: the clean fixture yields a concrete infrastructure subject.
	clean, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ref, fp string
	for _, e := range clean[0].Unlisted {
		if strings.Contains(e.Ref, "/infrastructure:") {
			ref, fp = e.Ref, e.Fingerprint
		}
	}
	if ref == "" {
		t.Fatalf("fixture: expected an attributed infrastructure entry; got %+v", clean[0])
	}

	// A heading change that slugifies to the SAME ref must still move the
	// fingerprint, or the entry's human-facing identity would be bound only by
	// its address. This holds today because the fragment body is the verbatim
	// block including its heading line; the test guards against that changing,
	// and does NOT witness a hole that ever existed.
	renamed := strings.Replace(body, "## Readiness probe boundary", "## Readiness  probe  boundary", 1)
	if err := os.WriteFile(infra, []byte(renamed), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range after[0].Unlisted {
		if e.Ref == ref && e.Fingerprint == fp {
			t.Error("a changed heading that slugifies to the same ref left the fingerprint " +
				"unmoved — the entry's identity is bound only by its address")
		}
	}

	// And an unreadable infrastructure artifact refuses rather than being
	// skipped. err == nil is FATAL here: an earlier version only checked the
	// message when an error happened to occur, so a fixture the derivation
	// accepted would have passed silently.
	if err := os.Remove(infra); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(infra, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"}, nil)
	if err == nil {
		t.Fatal("an infrastructure artifact that exists and cannot be read must refuse, not be " +
			"skipped — a population with a hole in it is not the population")
	}
	if !strings.Contains(err.Error(), "attributed to this promise") {
		t.Errorf("the refusal must identify the scope read; got %v", err)
	}
}

// Absence and unreadability are different. A dangling symlink reads as ENOENT
// through Stat while being plainly present in the directory.
func TestStage1b_DanglingArtifactIsNotAbsence(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := evolvingFeature(t, dir)
	caps := filepath.Join(featureDir, "capabilities.yaml")
	if err := os.Remove(caps); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "nothing-here.yaml"), caps); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}

	_, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"}, nil)
	if err == nil {
		t.Fatal("a dangling artifact is an unknown subject, not an absent one")
	}
	if !strings.Contains(err.Error(), "attributed to this promise") {
		t.Errorf("the refusal must identify the scope read; got %v", err)
	}
}

// A YAML item can inherit meaning from an anchor defined OUTSIDE it. Hashing
// the item node verbatim binds `<<: *defaults` — the alias name — while leaving
// the anchor target unbound, so changing the target changes what the entry
// promises and leaves the fingerprint identical.
func TestStage1b_AliasedEntryMeaningIsBound(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := evolvingFeature(t, dir)
	caps := filepath.Join(featureDir, "capabilities.yaml")

	withDefaults := func(shared string) string {
		return "defaults: &d\n  kind: query\n  summary: " + shared + "\n" +
			"operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n" +
			"  - id: y\n    <<: *d\n    source: \"@my-feature/check-readiness\"\n"
	}
	if err := os.WriteFile(caps, []byte(withDefaults("shared original text")), 0o644); err != nil {
		t.Fatal(err)
	}

	// Precondition: the parsed entry really does inherit through the merge, or
	// the test never reaches the dependency it claims to pin.
	parsed, err := parser.ParseCapabilities(caps)
	if err != nil {
		t.Fatal(err)
	}
	var inherited bool
	for _, op := range parsed.Operations {
		if op.ID == "y" && op.Kind == "query" {
			inherited = true
		}
	}
	if !inherited {
		t.Fatal("fixture: operation y must inherit kind through the merge key")
	}

	before, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	fpOf := func(scopes []lineageScope, ref string) string {
		for _, e := range scopes[0].Unlisted {
			if e.Ref == ref {
				return e.Fingerprint
			}
		}
		return ""
	}
	was := fpOf(before, "@my-feature/operation:y")
	if was == "" {
		t.Fatalf("fixture: expected y in the population; got %+v", before[0])
	}

	// EXACTLY one mutation, to the anchor target rather than to the item.
	if err := os.WriteFile(caps, []byte(withDefaults("shared text, quite different now")), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fpOf(after, "@my-feature/operation:y") == was {
		t.Error("changing an anchor the entry merges from left its fingerprint unmoved — the " +
			"subject binds the alias name rather than the meaning it resolves to")
	}
}

// An ambiguous population is not a population. The derivation refuses rather
// than letting a last-writer-wins map decide which entry a ref names.
func TestStage1b_AmbiguousEntriesRefuseDerivation(t *testing.T) {
	cases := []struct{ name, file, body, want string }{
		{
			name: "duplicate operation id",
			file: "capabilities.yaml",
			body: "operations:\n  - id: y\n    kind: query\n    source: \"@my-feature/check-readiness\"\n" +
				"  - id: y\n    kind: command\n    source: \"@my-feature/check-readiness\"\n",
			want: "appears more than once",
		},
		{
			name: "duplicate surface fragment name",
			file: "surface.yaml",
			body: "fragments:\n  - name: Thing List\n    source: \"@my-feature/check-readiness\"\n" +
				"  - name: Thing List\n    source: \"@my-feature/check-readiness\"\n",
			want: "appears more than once",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupTestDir(t)
			featureDir := evolvingFeature(t, dir)

			// Precondition: clean, the derivation succeeds and names a subject.
			clean, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"}, nil)
			if err != nil || len(clean[0].Unlisted) == 0 {
				t.Fatalf("fixture: the clean derivation must yield a subject; %v %+v", err, clean)
			}

			if err := os.WriteFile(filepath.Join(featureDir, tc.file), []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err = deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"}, nil)
			if err == nil {
				t.Fatal("an ambiguous population must refuse rather than resolve by last writer")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal must say what is ambiguous; got %v", err)
			}
			if !strings.Contains(err.Error(), "attributed to this promise") &&
				!strings.Contains(err.Error(), "cannot be read") {
				t.Errorf("the refusal must identify the scope read; got %v", err)
			}
		})
	}
}

// The shape guards inside resolvedEntryFingerprints, tested where they are reachable.
//
// End to end the parser refuses a non-sequence `operations` before this runs,
// so asserting it through deriveLineageScope would witness the parser rather
// than this guard. It stays as defence in depth for a document the parser might
// one day accept, and is pinned here rather than pretended to be pinned there.
func TestStage1b_RawEntryNodesRefusesAmbiguousShapes(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"not a sequence", "operations:\n  id: y\n", "is not a sequence"},
		{"entry with no id", "operations:\n  - kind: query\n", "has no id"},
		{"duplicate id", "operations:\n  - id: y\n  - id: y\n", "appears more than once"},
		{"root is not a mapping", "- id: y\n", "not a mapping at its root"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "capabilities.yaml")
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := resolvedEntryFingerprints(path, "operations", "id")
			if err == nil {
				t.Fatal("an ambiguous or unenumerable document must refuse")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the error must say what is wrong; got %v", err)
			}
		})
	}

	// And a well-formed document enumerates cleanly.
	path := filepath.Join(t.TempDir(), "capabilities.yaml")
	if err := os.WriteFile(path, []byte("operations:\n  - id: a\n  - id: b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolvedEntryFingerprints(path, "operations", "id")
	if err != nil || len(got) != 2 || got["a"] == got["b"] {
		t.Errorf("a well-formed document must enumerate distinctly; got %v %v", got, err)
	}
}

// Stage 2's headline: a narrowing whose consequences are collected can be
// applied, and the ceremony shows what becomes of each entry it takes support
// from.
func TestStage2_NarrowingWithCollectedConsequencesApplies(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)
	if err := os.Remove(filepath.Join(featureDir, "amendments", "001-channel-choice.md")); err != nil {
		t.Fatal(err)
	}
	writeAmendment(t, featureDir, "001-narrowed.md", `---
amendment: narrowed
date: 2026-09-01
amends_intents:
  - intent: check-readiness
    mode: narrow
    version:
      title: Check Cluster Readiness Only
      goal: See if the cluster is ready. Node readiness is no longer promised.
      persona: Admin
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions:
    - intent: check-readiness
      ref: "@my-feature/operation:y"
      disposition: removed
---

## Change
Node readiness is no longer promised, and its operation is gone.

## Why
It was never implemented.

## Acceptance
- Only cluster readiness is reported.
`)
	// The real order: the journal captures what the promise justified, and
	// THEN the splice removes y. Capturing afterwards would record a
	// population the change had already emptied.
	writeRefineJournal(t, cfg, "my-feature", 1)
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	armApplyAmendment(t, "", false)
	prose, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err != nil {
		t.Fatalf("a narrowing whose consequences are declared and consistent must be applicable: %v", err)
	}
	if !strings.Contains(prose, "What becomes of the entries this record does declare") {
		t.Errorf("the ceremony must show the consequences; got:\n%s", prose)
	}
	if !strings.Contains(prose, "nothing takes it over") {
		t.Errorf("the disposition's operational meaning must be shown, not its label; got:\n%s", prose)
	}

	pf := evolvePreflight(t, cfg)
	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("confirmed narrowing: %v", err)
	}
	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if bl.LastAppliedAmendment != 1 {
		t.Errorf("marker = %d, want 1", bl.LastAppliedAmendment)
	}
	// The consequences are in the durable record, not just on screen.
	ev := bl.TransitionReceipts["001-narrowed.md"].Payload.Evolution
	if len(ev) != 1 || len(ev[0].Consequences) != 1 {
		t.Fatalf("the receipt must record what was declared to become of each entry; got %+v", ev)
	}
	c := ev[0].Consequences[0]
	if c.Ref != "@my-feature/operation:y" || c.Disposition != "removed" || c.Lineage != "check-readiness" {
		t.Errorf("the receipt must record the structured result, not a sentence; got %+v", c)
	}
	if c.BeforeFingerprint == "" {
		t.Error("the receipt must bind the entry as it was, or a later reader cannot audit what " +
			"was actually given up")
	}
	if c.AfterRef != "" {
		t.Errorf("a removed entry has no after subject; got %q", c.AfterRef)
	}
}

// Every disposition has an operational meaning the tool checks against the
// artifacts. A label whose interpretation is left to the reader is what
// `retained` was refused for in Stage 1b.
func TestStage2_DispositionsAreCheckedAgainstTheContract(t *testing.T) {
	dir := setupTestDir(t)
	featureDir := evolvingFeature(t, dir)
	scopes, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"},
		[]string{"@my-feature/operation:x"})
	if err != nil {
		t.Fatal(err)
	}
	// Precondition: y is attributed and present.
	var found bool
	for _, e := range scopes[0].Unlisted {
		if e.Ref == "@my-feature/operation:y" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fixture: expected y attributed and present; got %+v", scopes[0])
	}

	cfg := testContext(t)
	rec := func(disp, replacedBy string) parser.Amendment {
		return parser.Amendment{
			Affects: []string{"@my-feature/operation:x"},
			AmendsIntents: []parser.IntentAmendment{{
				Intent: "check-readiness", Mode: parser.IntentNarrow,
				Version: &parser.IntentVersion{Title: "T", Goal: "g", Persona: "Admin"},
			}},
			ScopeImpact: &parser.ScopeImpact{
				Version: 1, PreservesUnlisted: true,
				Exceptions: []parser.ScopeException{
					{Intent: "check-readiness", Ref: "@my-feature/operation:y",
						Disposition: disp, ReplacedBy: replacedBy},
				},
			},
		}
	}
	// before == after here: nothing was spliced away, so a "survives"
	// disposition is coherent and a "gone" one contradicts the contract.
	check := func(a parser.Amendment) scopeConsequences {
		return checkScopeConsequences(cfg, "my-feature", a, scopes, scopes, nil)
	}
	cases := []struct{ name, disp, replacedBy, want string }{
		{"removed but still present", "removed", "", "still in the contract"},
		{"revised but not in affects", "revised", "", "affects: does not name it"},
		{"replaced-by with no replacement", "replaced-by", "", "names no replacement"},
		{"replaced-by pointing nowhere", "replaced-by", "@my-feature/operation:ghost", "does not resolve"},
		{"unknown disposition", "sort-of-kept", "", "no defined meaning"},
		{"replacement on a disposition that carries none", "retained", "@my-feature/operation:x", "only replaced-by carries one"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := check(rec(tc.disp, tc.replacedBy))
			if len(got.Problems) == 0 {
				t.Fatal("a disposition that contradicts the contract must be reported")
			}
			joined := strings.Join(got.Problems, "\n")
			if !strings.Contains(joined, tc.want) {
				t.Errorf("the finding must say what contradicts; got %q", joined)
			}
		})
	}

	// And a coherent one produces a claim rather than a problem.
	ok := check(rec("retained", ""))
	if len(ok.Problems) != 0 {
		t.Errorf("a coherent disposition must not be reported; got %v", ok.Problems)
	}
	claims := ok.ByLineage["check-readiness"]
	if len(claims) != 1 || !strings.Contains(claims[0].Claim, "still supports it") {
		t.Errorf("the claim must state the disposition's meaning; got %+v", claims)
	}
	if claims[0].BeforeFingerprint == "" || claims[0].AfterFingerprint == "" {
		t.Errorf("a retained consequence must bind the entry as it was AND as it stands; got %+v",
			claims[0])
	}
}

// The claim Stage 2 exists to make true: an entry the promise justified cannot
// simply vanish. Scope derived after the splice cannot see a removed entry, so
// without the pre-splice capture this disappearance would be invisible.
func TestStage2_UnaccountedDisappearanceRefuses(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)
	if err := os.Remove(filepath.Join(featureDir, "amendments", "001-channel-choice.md")); err != nil {
		t.Fatal(err)
	}
	writeAmendment(t, featureDir, "001-quiet.md", `---
amendment: quiet
date: 2026-09-01
amends_intents:
  - intent: check-readiness
    mode: revise
    version:
      title: Check Readiness
      goal: See if the cluster is ready.
      persona: Admin
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions: []
---

## Change
Reword, and quietly drop an operation without saying so.

## Why
To prove the accounting notices.

## Acceptance
- Nothing.
`)
	// Capture BEFORE, then remove y without declaring any consequence.
	writeRefineJournal(t, cfg, "my-feature", 1)
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	armApplyAmendment(t, "", false)
	_, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err == nil {
		t.Fatal("an entry the promise justified disappeared with no declared consequence, and " +
			"that is exactly what this accounting exists to catch")
	}
	if !strings.Contains(err.Error(), "@my-feature/operation:y") {
		t.Errorf("the refusal must name the entry that vanished; got %v", err)
	}
}

// A consequence must be about something the promise actually justified. Without
// the pre-splice inventory, any plausible absent ref could be declared removed
// and the record would be evidence only that somebody claimed a consequence.
func TestStage2_ConsequenceOverAnUnjustifiedEntryRefuses(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)
	before, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range before[0].Unlisted {
		if e.Ref == "@my-feature/operation:never-existed" {
			t.Fatal("fixture: the invented ref must not be attributed")
		}
	}

	rec := parser.Amendment{
		AmendsIntents: []parser.IntentAmendment{{
			Intent: "check-readiness", Mode: parser.IntentRevise,
			Version: &parser.IntentVersion{Title: "T", Goal: "g", Persona: "Admin"},
		}},
		ScopeImpact: &parser.ScopeImpact{
			Version: 1, PreservesUnlisted: true,
			Exceptions: []parser.ScopeException{{
				Intent: "check-readiness", Ref: "@my-feature/operation:never-existed",
				Disposition: "removed",
			}},
		},
	}
	got := checkScopeConsequences(cfg, "my-feature", rec, before, before, nil)
	var found bool
	for _, p := range got.Problems {
		if strings.Contains(p, "did not justify it before this change") {
			found = true
		}
	}
	if !found {
		t.Errorf("a consequence over an entry the promise never justified must refuse; got %v",
			got.Problems)
	}
}

// Consequences belong to a lineage. One exception on lineage A must not satisfy
// lineage B's obligation, and B's receipt must not carry A's fate.
func TestStage2_ConsequencesAreOwnedPerLineage(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)
	// Both promises own an operation.
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"+
			"  - id: s\n    kind: query\n    summary: does s\n    source: \"@my-feature/survives\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := deriveLineageScope(featureDir, "my-feature",
		[]string{"check-readiness", "survives"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 2 {
		t.Fatalf("fixture: expected both lineages attributed; got %+v", before)
	}

	// Both narrowed, but only one carries a consequence.
	rec := parser.Amendment{
		AmendsIntents: []parser.IntentAmendment{
			{Intent: "check-readiness", Mode: parser.IntentNarrow,
				Version: &parser.IntentVersion{Title: "T", Goal: "g", Persona: "Admin"}},
			{Intent: "survives", Mode: parser.IntentNarrow,
				Version: &parser.IntentVersion{Title: "S", Goal: "g", Persona: "Admin"}},
		},
		ScopeImpact: &parser.ScopeImpact{
			Version: 1, PreservesUnlisted: true,
			Exceptions: []parser.ScopeException{{
				Intent: "check-readiness", Ref: "@my-feature/operation:x", Disposition: "retained",
			}},
		},
	}
	got := checkScopeConsequences(cfg, "my-feature", rec, before, before,
		map[string]bool{"check-readiness": true, "survives": true})

	var named bool
	for _, p := range got.Problems {
		if strings.Contains(p, "survives") && strings.Contains(p, "no consequence for it") {
			named = true
		}
	}
	if !named {
		t.Errorf("a narrowed lineage with no consequence of its own must be named — one "+
			"exception elsewhere in the record does not cover it; got %v", got.Problems)
	}
	if len(got.ByLineage["survives"]) != 0 {
		t.Error("a lineage's claims must not include another lineage's fate")
	}
}

// EXISTENCE and ATTRIBUTION are different questions, and conflating them makes
// `removed` a lie in a specific way: an entry the splice RE-SOURCED to another
// promise has left this lineage's population while remaining in the contract.
// Attribution-based existence calls that "gone"; it is not gone, it moved.
func TestStage2_ReSourcedEntryIsNotRemoved(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)

	// Before: y is justified by check-readiness.
	before, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var attributed bool
	for _, e := range before[0].Unlisted {
		if e.Ref == "@my-feature/operation:y" {
			attributed = true
		}
	}
	if !attributed {
		t.Fatalf("fixture: y must start attributed to check-readiness; got %+v", before[0])
	}

	// The splice re-sources y to the other promise. It is out of this
	// lineage's population and still very much in the contract.
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"+
			"  - id: y\n    kind: query\n    summary: does y\n    source: \"@my-feature/survives\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range after[0].Unlisted {
		if e.Ref == "@my-feature/operation:y" {
			t.Fatal("fixture: y must have left this lineage's population, or the test proves nothing")
		}
	}

	rec := parser.Amendment{
		AmendsIntents: []parser.IntentAmendment{{
			Intent: "check-readiness", Mode: parser.IntentNarrow,
			Version: &parser.IntentVersion{Title: "T", Goal: "g", Persona: "Admin"},
		}},
		ScopeImpact: &parser.ScopeImpact{
			Version: 1, PreservesUnlisted: true,
			Exceptions: []parser.ScopeException{{
				Intent: "check-readiness", Ref: "@my-feature/operation:y", Disposition: "removed",
			}},
		},
	}
	got := checkScopeConsequences(cfg, "my-feature", rec, before, after, nil)
	var caught bool
	for _, p := range got.Problems {
		if strings.Contains(p, "still in the contract") {
			caught = true
		}
	}
	if !caught {
		t.Errorf("declaring a re-sourced entry `removed` must refuse — it exists, and existence "+
			"is a question about the contract rather than about this promise's population; "+
			"got %v", got.Problems)
	}
}

// A capture is evidence about the exact record it was taken for. A sequence is
// not identity: amendments are append-only, but a violation or replacement of
// NNN-slug.md keeps the sequence while changing the lineages and the meaning.
func TestStage2_CaptureIsBoundToTheExactRecord(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)
	path := filepath.Join(featureDir, "amendments", "001-channel-choice.md")

	// Precondition: with the capture intact, the ceremony proceeds.
	armApplyAmendment(t, "", false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("fixture: the unmodified record must preflight cleanly: %v", err)
	}

	// The record is replaced after capture, keeping its sequence.
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, []byte("\nAppended after capture.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	armApplyAmendment(t, "", false)
	_, err = runApplyAmendment_(t, cfg, "@my-feature")
	if err == nil {
		t.Fatal("a capture must not be consumed as evidence for bytes it was not taken from")
	}
	if !strings.Contains(err.Error(), "changed after its scope was captured") {
		t.Errorf("the refusal must say the record moved under its capture; got %v", err)
	}
}

// One contract entry may name several source intents. A shared entry that
// disappears owes a consequence under EACH promise that justified it, so
// keying duplicates on the ref alone would make completeness unsatisfiable.
func TestStage2_SharedEntryOwesAConsequencePerLineage(t *testing.T) {
	a := parser.Amendment{
		AmendsIntents: []parser.IntentAmendment{
			{Intent: "a", Mode: parser.IntentNarrow, Version: &parser.IntentVersion{Title: "A", Goal: "g", Persona: "p"}},
			{Intent: "b", Mode: parser.IntentNarrow, Version: &parser.IntentVersion{Title: "B", Goal: "g", Persona: "p"}},
		},
		ScopeImpact: &parser.ScopeImpact{
			Version: 1, PreservesUnlisted: true,
			Exceptions: []parser.ScopeException{
				{Intent: "a", Ref: "@f/operation:shared", Disposition: "removed"},
				{Intent: "b", Ref: "@f/operation:shared", Disposition: "removed"},
			},
		},
	}
	if problems := a.ValidateScopeImpact(); len(problems) != 0 {
		t.Errorf("one entry may owe a consequence under each promise that justified it; got %v",
			problems)
	}

	// But twice under the SAME lineage is still one entry with two fates.
	a.ScopeImpact.Exceptions[1].Intent = "a"
	problems := a.ValidateScopeImpact()
	if len(problems) == 0 {
		t.Fatal("the same entry dispositioned twice under one lineage must refuse")
	}
	if !strings.Contains(strings.Join(problems, "\n"), "one fate per lineage") {
		t.Errorf("the finding must say why; got %v", problems)
	}
}

// `retained` claims THIS promise still supports the entry. Existing somewhere
// is not the same thing: an entry re-sourced elsewhere still exists while no
// longer being supported here.
func TestStage2_RetainedRequiresContinuedAttribution(t *testing.T) {
	dir := setupTestDir(t)
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)

	before, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var was bool
	for _, e := range before[0].Unlisted {
		if e.Ref == "@my-feature/operation:y" {
			was = true
		}
	}
	if !was {
		t.Fatalf("fixture: y must start attributed here; got %+v", before[0])
	}

	// The splice re-sources y away. It exists; this promise no longer supports it.
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"+
			"  - id: y\n    kind: query\n    summary: does y\n    source: \"@my-feature/survives\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := deriveLineageScope(featureDir, "my-feature", []string{"check-readiness"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	rec := parser.Amendment{
		AmendsIntents: []parser.IntentAmendment{{
			Intent: "check-readiness", Mode: parser.IntentNarrow,
			Version: &parser.IntentVersion{Title: "T", Goal: "g", Persona: "Admin"},
		}},
		ScopeImpact: &parser.ScopeImpact{
			Version: 1, PreservesUnlisted: true,
			Exceptions: []parser.ScopeException{{
				Intent: "check-readiness", Ref: "@my-feature/operation:y", Disposition: "retained",
			}},
		},
	}
	got := checkScopeConsequences(cfg, "my-feature", rec, before, after, nil)
	var caught bool
	for _, p := range got.Problems {
		if strings.Contains(p, "no longer justified by that promise") {
			caught = true
		}
	}
	if !caught {
		t.Errorf("`retained` claims this promise still supports the entry, and it no longer "+
			"does; got %v", got.Problems)
	}
}

// A replacement is a subject the human approved, so the ceremony must show it
// and the receipt must bind it.
// replacedByAmendment narrows check-readiness and declares that y's work is
// taken over by x. Written as a fixture because three tests need the same
// record: the applying run, the stored receipt, and the mutation at the lock.
const replacedByAmendment = `---
amendment: replaced
date: 2026-09-01
amends_intents:
  - intent: check-readiness
    mode: narrow
    version:
      title: Check Cluster Readiness Only
      goal: See if the cluster is ready. Node readiness folds into the cluster check.
      persona: Admin
scope_impact:
  version: 1
  preserves_unlisted: true
  exceptions:
    - intent: check-readiness
      ref: "@my-feature/operation:y"
      disposition: replaced-by
      replaced_by: "@my-feature/operation:z"
---

## Change
Node readiness is no longer its own operation; z reports it.

## Why
Two operations answered one question.

## Acceptance
- z reports node readiness.
`

// setupReplacement lays down the narrowing, captures the pre-splice inventory,
// and then performs the splice that removes y. The order matters and is the
// real one: capture first, mutate second.
//
// The replacement z is sourced from a DIFFERENT promise on purpose. That is the
// ordinary shape of a narrowing — work does not usually vanish, it moves to
// whatever already owns that ground — and it is the case the lineage scope
// cannot see: z is not attributed to check-readiness, so it is outside the
// scope digest entirely. Only the whole-contract index reaches it, and only the
// consequence digest notices when it changes.
func setupReplacement(t *testing.T, dir string) (*config.Context, string) {
	t.Helper()
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)
	if err := os.Remove(filepath.Join(featureDir, "amendments", "001-channel-choice.md")); err != nil {
		t.Fatal(err)
	}
	writeAmendment(t, featureDir, "001-replaced.md", replacedByAmendment)
	// z exists before the change; the narrowing moves work onto it, it does not
	// conjure it. Written before the capture so the inventory sees the real
	// starting contract.
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n"+
			"  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"+
			"  - id: y\n    kind: query\n    summary: does y\n    source: \"@my-feature/check-readiness\"\n"+
			"  - id: z\n    kind: query\n    summary: reports cluster readiness\n    source: \"@my-feature/survives\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRefineJournal(t, cfg, "my-feature", 1)
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte(replacedCapabilities("reports cluster and node readiness")), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfg, featureDir
}

// replacedCapabilities is the post-splice contract: y is gone, and z — which
// belongs to another promise — carries the work, with the given summary.
func replacedCapabilities(zSummary string) string {
	return "operations:\n" +
		"  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n" +
		"  - id: z\n    kind: query\n    summary: " + zSummary + "\n    source: \"@my-feature/survives\"\n"
}

// replacementRecord returns the parsed amendment the capture belongs to.
func replacementRecord(t *testing.T, featureDir string) parser.Amendment {
	t.Helper()
	all, err := parser.LoadFeatureAmendments(featureDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range all {
		if a.Seq == 1 {
			return a
		}
	}
	t.Fatal("amendment 1 not found")
	return parser.Amendment{}
}

// The end-to-end witness for replacement identity: the ceremony names the
// replacement, the confirmed run stores it, and the stored receipt binds it by
// fingerprint. A helper-level check of checkScopeConsequences cannot say any of
// this — it never runs the command, never confirms, and never reads back what
// was written, so the claim "shown and stored" would rest on the one layer that
// was tested rather than on the path a user actually takes.
func TestStage2_ReplacementIsShownStoredAndBoundEndToEnd(t *testing.T) {
	dir := setupTestDir(t)
	cfg, _ := setupReplacement(t, dir)

	armApplyAmendment(t, "", false)
	prose, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err != nil {
		t.Fatalf("a coherent replacement must be applicable: %v", err)
	}
	if !strings.Contains(prose, "@my-feature/operation:z") {
		t.Errorf("the ceremony must name the canonical replacement — the human is approving THAT "+
			"entry taking the work over, not the bare fact that something did; got:\n%s", prose)
	}

	pf := evolvePreflight(t, cfg)
	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("confirmed replacement: %v", err)
	}

	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	if bl.LastAppliedAmendment != 1 {
		t.Fatalf("marker = %d, want 1", bl.LastAppliedAmendment)
	}
	ev := bl.TransitionReceipts["001-replaced.md"].Payload.Evolution
	if len(ev) != 1 || len(ev[0].Consequences) != 1 {
		t.Fatalf("the receipt must record the consequence; got %+v", ev)
	}
	c := ev[0].Consequences[0]
	if c.Ref != "@my-feature/operation:y" || c.Disposition != "replaced-by" {
		t.Errorf("stored consequence subject = %+v", c)
	}
	if c.AfterRef != "@my-feature/operation:z" {
		t.Errorf("the stored receipt must name the replacement; got %q", c.AfterRef)
	}
	if len(c.AfterFingerprint) != 64 || c.AfterFingerprint == c.BeforeFingerprint {
		t.Errorf("the replacement must be bound by its own fingerprint, distinct from what it "+
			"replaced; before=%q after=%q", c.BeforeFingerprint, c.AfterFingerprint)
	}
	if _, err := observeAppliedAuthority(cfg, "my-feature"); err != nil {
		t.Errorf("the capsule must stay readable after a consequence-bearing write: %v", err)
	}
}

// The replacement is not attributed to the narrowed promise — it belongs to
// another one — so the lineage scope does not contain it and the scope digest
// is blind to it by construction. The population is unchanged and the ref is
// unchanged. What moved is what the replacement MEANS — and that is exactly the approval the human gave: they
// agreed that THIS entry takes the retired work over. The consequence digest is
// the only thing standing between that approval and a different entry wearing
// its address.
func TestStage2_ReplacementMutatedBeforeTheLockRefuses(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupReplacement(t, dir)
	pf := evolvePreflight(t, cfg)

	blBefore, err := os.ReadFile(baselinePath(cfg, "my-feature"))
	if err != nil {
		t.Fatal(err)
	}
	caps := filepath.Join(featureDir, "capabilities.yaml")
	// Precondition: the run reaches the lock with the approved state intact.
	// Without this the test could pass by refusing for some earlier reason.
	armApplyAmendment(t, pf.Digest, false)
	reached := false
	authorityLockAttemptHook = func(string) { reached = true }
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("the unmutated run must apply, or the mutation below proves nothing: %v", err)
	}
	authorityLockAttemptHook = nil
	if !reached {
		t.Fatal("the run never reached the authority lock")
	}

	// Now the real subject: same fixture, mutated at the lock boundary.
	dir2 := setupTestDir(t)
	cfg2, featureDir2 := setupReplacement(t, dir2)
	pf2 := evolvePreflight(t, cfg2)
	caps = filepath.Join(featureDir2, "capabilities.yaml")
	blBefore, err = os.ReadFile(baselinePath(cfg2, "my-feature"))
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	authorityLockAttemptHook = func(string) {
		once.Do(func() {
			// The ref survives; the promise the entry makes does not.
			_ = os.WriteFile(caps,
				[]byte(replacedCapabilities("reports cluster readiness only, not node readiness")), 0o644)
		})
	}
	t.Cleanup(func() { authorityLockAttemptHook = nil })

	armApplyAmendment(t, pf2.Digest, false)
	_, err = runApplyAmendment_(t, cfg2, "@my-feature")
	if err == nil {
		t.Fatal("a replacement whose content changed after approval must refuse: the human " +
			"approved an entry, not an address")
	}
	if !strings.Contains(err.Error(), "changed between approval and write") {
		t.Errorf("the refusal must identify the consequence check, not merely be non-nil; got: %v", err)
	}
	blAfter, err := os.ReadFile(baselinePath(cfg2, "my-feature"))
	if err != nil {
		t.Fatal(err)
	}
	if string(blBefore) != string(blAfter) {
		t.Error("the baseline was written for a replacement that had changed")
	}
	bl := readFeatureBaseline(t, baselinePath(cfg2, "my-feature"))
	if bl.LastAppliedAmendment != 0 {
		t.Errorf("marker advanced to %d on a refused write", bl.LastAppliedAmendment)
	}
}

// The capture is evidence, and evidence is checked before it is used. Matching
// filename, hash and sequence prove WHICH record the capture belongs to; they
// say nothing about whether the payload beside that metadata is intact. A
// partially written journal that drops a lineage would otherwise have
// completeness iterate only what survived — quietly comparing against a smaller
// population than the one that existed, which for `revise` (no per-lineage
// "must declare something" fallback) means the prior population is simply gone.
func TestStage2_MalformedCaptureRefusesBeforeConsequences(t *testing.T) {
	cases := []struct {
		name    string
		break_  func(j *refineJournal)
		wantErr string
	}{
		{"omitted lineage", func(j *refineJournal) {
			j.ScopeBefore = nil
		}, "no entry for"},
		{"duplicated lineage", func(j *refineJournal) {
			j.ScopeBefore = append(j.ScopeBefore, j.ScopeBefore[0])
		}, "twice"},
		{"foreign lineage", func(j *refineJournal) {
			j.ScopeBefore[0].Lineage = "not-a-lineage-this-record-changes"
		}, "does not change"},
		{"duplicated ref", func(j *refineJournal) {
			j.ScopeBefore[0].Unlisted = append(j.ScopeBefore[0].Unlisted, j.ScopeBefore[0].Unlisted[0])
		}, "twice under"},
		{"empty fingerprint", func(j *refineJournal) {
			j.ScopeBefore[0].Unlisted[0].Fingerprint = ""
		}, "no usable fingerprint"},
		{"non-canonical ref", func(j *refineJournal) {
			j.ScopeBefore[0].Unlisted[0].Ref = "operation:y"
		}, "canonical contract reference"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupTestDir(t)
			cfg, featureDir := setupReplacement(t, dir)
			rec := replacementRecord(t, featureDir)

			j, err := loadRefineJournal(cfg, "my-feature")
			if err != nil || j == nil {
				t.Fatalf("load journal: %v", err)
			}
			// Precondition: the capture is well-formed and usable BEFORE the
			// break, so the refusal below is attributable to the break alone.
			if err := scopeCaptureMatches(j, rec, []string{"check-readiness"}); err != nil {
				t.Fatalf("the fixture capture must be valid to begin with: %v", err)
			}
			if len(j.ScopeBefore) == 0 || len(j.ScopeBefore[0].Unlisted) == 0 {
				t.Fatalf("the fixture must actually carry a captured population; got %+v", j.ScopeBefore)
			}
			tc.break_(j)
			// The metadata is deliberately left untouched: filename, hash,
			// sequence and lineage list all still match. Only the payload is
			// damaged, which is precisely the case metadata cannot catch.
			err = scopeCaptureMatches(j, rec, []string{"check-readiness"})
			if err == nil {
				t.Fatal("a malformed capture must be refused before anything relies on it")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("the refusal must identify what is wrong with the evidence; want %q, got: %v",
					tc.wantErr, err)
			}
		})
	}
}

// The digest recorded with the capture is not signed authenticity and is not
// claimed as such. It detects the partial write and the after-the-fact edit —
// which is what a journal on disk is actually exposed to.
func TestStage2_CaptureDigestDetectsAnAlteredInventory(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupReplacement(t, dir)
	rec := replacementRecord(t, featureDir)
	j, err := loadRefineJournal(cfg, "my-feature")
	if err != nil || j == nil {
		t.Fatalf("load journal: %v", err)
	}
	if j.ScopeBeforeDigest == "" {
		t.Fatal("the capture must record a digest, or there is nothing to verify against")
	}
	// A fingerprint swapped for another valid-looking one: structurally fine,
	// so only the digest can see it.
	j.ScopeBefore[0].Unlisted[0].Fingerprint = strings.Repeat("a", 64)
	if err := scopeCaptureMatches(j, rec, []string{"check-readiness"}); err == nil {
		t.Fatal("an altered inventory must be refused")
	} else if !strings.Contains(err.Error(), "does not match the digest") {
		t.Errorf("the refusal must name the digest as what failed; got: %v", err)
	}

	j2, err := loadRefineJournal(cfg, "my-feature")
	if err != nil {
		t.Fatal(err)
	}
	j2.ScopeBeforeDigest = ""
	if err := scopeCaptureMatches(j2, rec, []string{"check-readiness"}); err == nil {
		t.Fatal("a capture with no digest must be refused rather than trusted — stripping the " +
			"check is the cheapest way to defeat it")
	}
}

// --- The replacement ref space ---

// crossFeatureReplacement lays down the narrowing whose replacement lives in
// ANOTHER feature. This is not an exotic case: `affects:` has always been
// cross-feature, `replaced_by` accepts the same grammar, and work that stops
// being promised here usually moves to whatever already owns that ground —
// which is frequently somewhere else. Before the resolver, this record was
// schema-valid and permanently unappliable, and the message told the designer
// their true statement did not resolve against the contract.
func crossFeatureReplacement(t *testing.T, dir string) (*config.Context, string) {
	t.Helper()
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)
	if err := os.Remove(filepath.Join(featureDir, "amendments", "001-channel-choice.md")); err != nil {
		t.Fatal(err)
	}
	writeAmendment(t, featureDir, "001-replaced.md",
		strings.ReplaceAll(replacedByAmendment, "@my-feature/operation:z", "@other-feature/operation:z"))

	otherDir := cfg.FeaturePath("other-feature")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: z\n    kind: query\n    summary: reports cluster readiness\n    source: \"@other-feature/owns-it\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRefineJournal(t, cfg, "my-feature", 1)
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfg, featureDir
}

func TestStage2_ReplacementMayLiveInAnotherFeature(t *testing.T) {
	dir := setupTestDir(t)
	cfg, _ := crossFeatureReplacement(t, dir)

	armApplyAmendment(t, "", false)
	prose, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err != nil {
		t.Fatalf("a replacement in another feature is a legal ref and must resolve: %v", err)
	}
	if !strings.Contains(prose, "@other-feature/operation:z") {
		t.Errorf("the ceremony must name the cross-feature replacement; got:\n%s", prose)
	}

	pf := evolvePreflight(t, cfg)
	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("confirmed cross-feature replacement: %v", err)
	}
	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	c := bl.TransitionReceipts["001-replaced.md"].Payload.Evolution[0].Consequences[0]
	if c.AfterRef != "@other-feature/operation:z" {
		t.Errorf("the receipt must name the cross-feature replacement; got %q", c.AfterRef)
	}
	if !validSHA256(c.AfterFingerprint) {
		t.Errorf("a cross-feature replacement must be bound by a real digest; got %q",
			c.AfterFingerprint)
	}
}

// The other half of the same contract: the replacement is bound by CONTENT, not
// by address, wherever it lives. A cross-feature replacement that is resolved
// but not fingerprinted would be a weaker promise than a local one.
func TestStage2_CrossFeatureReplacementIsBoundToItsContent(t *testing.T) {
	dir := setupTestDir(t)
	cfg, _ := crossFeatureReplacement(t, dir)
	pf := evolvePreflight(t, cfg)
	otherCaps := filepath.Join(cfg.FeaturePath("other-feature"), "capabilities.yaml")
	blBefore, err := os.ReadFile(baselinePath(cfg, "my-feature"))
	if err != nil {
		t.Fatal(err)
	}

	var once sync.Once
	authorityLockAttemptHook = func(string) {
		once.Do(func() {
			_ = os.WriteFile(otherCaps, []byte("operations:\n  - id: z\n    kind: query\n"+
				"    summary: reports something else entirely\n    source: \"@other-feature/owns-it\"\n"), 0o644)
		})
	}
	t.Cleanup(func() { authorityLockAttemptHook = nil })

	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err == nil {
		t.Fatal("a cross-feature replacement whose content changed after approval must refuse")
	} else if !strings.Contains(err.Error(), "changed between approval and write") {
		t.Errorf("the refusal must identify the consequence check; got: %v", err)
	}
	blAfter, err := os.ReadFile(baselinePath(cfg, "my-feature"))
	if err != nil {
		t.Fatal(err)
	}
	if string(blBefore) != string(blAfter) {
		t.Error("the baseline was written for a replacement that had changed")
	}
}

// A domain entity is root-scoped, and the ref grammar admits it. It resolves
// against the root model, exactly as `affects:` does.
func domainReplacement(t *testing.T, dir string) (*config.Context, string) {
	t.Helper()
	cfg := testContext(t)
	featureDir := evolvingFeature(t, dir)
	if err := os.Remove(filepath.Join(featureDir, "amendments", "001-channel-choice.md")); err != nil {
		t.Fatal(err)
	}
	writeAmendment(t, featureDir, "001-replaced.md",
		strings.ReplaceAll(replacedByAmendment, "@my-feature/operation:z", "@my-feature/domain:Cluster"))
	if err := os.WriteFile(cfg.DomainModelPath(),
		[]byte("schema_version: 1\nentities:\n  - name: Cluster\n    description: the cluster\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRefineJournal(t, cfg, "my-feature", 1)
	if err := os.WriteFile(filepath.Join(featureDir, "capabilities.yaml"),
		[]byte("operations:\n  - id: x\n    kind: command\n    summary: does x\n    source: \"@my-feature/check-readiness\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfg, featureDir
}

func TestStage2_ReplacementMayBeADomainEntity(t *testing.T) {
	dir := setupTestDir(t)
	cfg, _ := domainReplacement(t, dir)

	armApplyAmendment(t, "", false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("a domain entity is a legal replacement ref and must resolve: %v", err)
	}
	pf := evolvePreflight(t, cfg)
	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("confirmed domain replacement: %v", err)
	}
	bl := readFeatureBaseline(t, baselinePath(cfg, "my-feature"))
	c := bl.TransitionReceipts["001-replaced.md"].Payload.Evolution[0].Consequences[0]
	if c.AfterRef != "@my-feature/domain:Cluster" || !validSHA256(c.AfterFingerprint) {
		t.Errorf("the domain replacement must be named and bound; got %+v", c)
	}
}

// The domain half of the same content-binding rule. The fingerprint comes from
// the raw entity node, not the parsed struct, for the reason it does everywhere
// else here: the schema has not modelled every field an entity carries, and an
// edit confined to an unmodelled one is exactly the change an approval token
// must not miss.
func TestStage2_DomainReplacementIsBoundToItsContent(t *testing.T) {
	dir := setupTestDir(t)
	cfg, _ := domainReplacement(t, dir)
	pf := evolvePreflight(t, cfg)
	blBefore, err := os.ReadFile(baselinePath(cfg, "my-feature"))
	if err != nil {
		t.Fatal(err)
	}

	var once sync.Once
	authorityLockAttemptHook = func(string) {
		once.Do(func() {
			// `notes` is not in the parsed entity struct today. A fingerprint
			// over the parsed value would see nothing here.
			_ = os.WriteFile(cfg.DomainModelPath(),
				[]byte("schema_version: 1\nentities:\n  - name: Cluster\n    description: the cluster\n    notes: and it no longer covers node readiness\n"), 0o644)
		})
	}
	t.Cleanup(func() { authorityLockAttemptHook = nil })

	armApplyAmendment(t, pf.Digest, false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err == nil {
		t.Fatal("a domain replacement whose content changed after approval must refuse")
	} else if !strings.Contains(err.Error(), "changed between approval and write") {
		t.Errorf("the refusal must identify the consequence check; got: %v", err)
	}
	blAfter, err := os.ReadFile(baselinePath(cfg, "my-feature"))
	if err != nil {
		t.Fatal(err)
	}
	if string(blBefore) != string(blAfter) {
		t.Error("the baseline was written for a replacement that had changed")
	}
}

// Two ways the root model can stop being a usable subject, kept apart because
// they fail at different readers and a single assertion would hide one of them.
//
//   - A duplicated name is caught by the fingerprint reader itself: "Cluster"
//     then has two possible meanings and the approval cannot say which it bound.
//   - An entry the raw reader cannot key at all — no `name` — cannot be
//     identified, so no reference can name it.
//
// Both refusals come from the fingerprint reader. The parsed/raw count check
// downstream of it has no reachable failure mode given that strictness; it is
// kept as a consistency assertion, not claimed as a tested guard.
func TestStage2_UnusableDomainModelRefusesResolution(t *testing.T) {
	cases := []struct {
		name    string
		model   string
		wantErr string
	}{
		{"duplicated entity", "schema_version: 1\nentities:\n  - name: Cluster\n    description: one\n" +
			"  - name: Cluster\n    description: another\n", "appears more than once"},
		{"unkeyable entry", "schema_version: 1\nentities:\n  - name: Cluster\n    description: one\n" +
			"  - description: nameless\n", "has no name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupTestDir(t)
			cfg, _ := domainReplacement(t, dir)
			// Precondition: this fixture resolves before the model is broken.
			armApplyAmendment(t, "", false)
			if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
				t.Fatalf("the fixture must resolve to begin with: %v", err)
			}
			if err := os.WriteFile(cfg.DomainModelPath(), []byte(tc.model), 0o644); err != nil {
				t.Fatal(err)
			}
			armApplyAmendment(t, "", false)
			_, err := runApplyAmendment_(t, cfg, "@my-feature")
			if err == nil {
				t.Fatal("an unusable domain model must refuse rather than resolve against a guess")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("want %q in the refusal; got: %v", tc.wantErr, err)
			}
		})
	}
}

// Absence and failure are different answers, and only one of them is the
// designer's fault. An unreadable target artifact must say so; reporting it as
// "does not resolve against the contract" sends someone to fix a reference that
// was correct.
func TestStage2_UnreachableReplacementIsNotReportedAsAbsent(t *testing.T) {
	dir := setupTestDir(t)
	cfg, _ := crossFeatureReplacement(t, dir)
	otherCaps := filepath.Join(cfg.FeaturePath("other-feature"), "capabilities.yaml")
	if err := os.WriteFile(otherCaps, []byte("operations:\n  - id: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	armApplyAmendment(t, "", false)
	_, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err == nil {
		t.Fatal("an unreadable replacement target must fail closed")
	}
	if strings.Contains(err.Error(), "does not resolve against the contract") {
		t.Errorf("an unreadable artifact was reported as a missing reference; got: %v", err)
	}
	if !strings.Contains(err.Error(), "cannot be read") {
		t.Errorf("the refusal must say the artifact could not be read; got: %v", err)
	}
}

// A ref naming a feature that has no spec directory is a genuinely bad
// reference, and must say THAT rather than blaming the contract's contents.
func TestStage2_ReplacementInAnUnknownFeatureNamesTheFeature(t *testing.T) {
	dir := setupTestDir(t)
	cfg, _ := crossFeatureReplacement(t, dir)
	if err := os.RemoveAll(cfg.FeaturePath("other-feature")); err != nil {
		t.Fatal(err)
	}
	armApplyAmendment(t, "", false)
	_, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err == nil {
		t.Fatal("a replacement in a feature that does not exist must refuse")
	}
	if !strings.Contains(err.Error(), "other-feature") {
		t.Errorf("the refusal must name the unknown feature; got: %v", err)
	}
}

// The directory probe makes the same absence-versus-failure distinction the
// resolver makes above it. A feature dir that is a file, or one that cannot be
// read at all, is not a missing feature.
func TestStage2_UnreadableFeatureDirIsNotAMissingFeature(t *testing.T) {
	dir := setupTestDir(t)
	cfg, _ := crossFeatureReplacement(t, dir)
	other := cfg.FeaturePath("other-feature")
	// Precondition: it resolves while the directory is a directory.
	armApplyAmendment(t, "", false)
	if _, err := runApplyAmendment_(t, cfg, "@my-feature"); err != nil {
		t.Fatalf("the fixture must resolve to begin with: %v", err)
	}

	if err := os.RemoveAll(other); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	armApplyAmendment(t, "", false)
	_, err := runApplyAmendment_(t, cfg, "@my-feature")
	if err == nil {
		t.Fatal("a feature path that is not a directory must refuse")
	}
	if strings.Contains(err.Error(), "has no spec directory") {
		t.Errorf("a path that exists was reported as a missing feature; got: %v", err)
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("the refusal must say what it actually found; got: %v", err)
	}
}

// A digest is not a length. Sixty-four `z` characters are exactly as long as a
// SHA-256 and carry none of its meaning, and the comment at this boundary
// claimed to require a valid one.
func TestStage2_CaptureFingerprintMustBeRealHex(t *testing.T) {
	dir := setupTestDir(t)
	cfg, featureDir := setupReplacement(t, dir)
	rec := replacementRecord(t, featureDir)
	j, err := loadRefineJournal(cfg, "my-feature")
	if err != nil || j == nil {
		t.Fatalf("load journal: %v", err)
	}
	if err := scopeCaptureMatches(j, rec, []string{"check-readiness"}); err != nil {
		t.Fatalf("the fixture capture must be valid to begin with: %v", err)
	}

	j.ScopeBefore[0].Unlisted[0].Fingerprint = strings.Repeat("z", 64)
	// Recompute the outer digest so it AGREES with the tampered inventory.
	// Otherwise the digest check would refuse first and this test would prove
	// nothing about the fingerprint rule it names.
	j.ScopeBeforeDigest = scopeInventoryDigest(j.ScopeBefore)

	err = scopeCaptureMatches(j, rec, []string{"check-readiness"})
	if err == nil {
		t.Fatal("a 64-character non-hex fingerprint must be refused — length is not validity")
	}
	if !strings.Contains(err.Error(), "no usable fingerprint") {
		t.Errorf("the refusal must name the fingerprint; got: %v", err)
	}
}
