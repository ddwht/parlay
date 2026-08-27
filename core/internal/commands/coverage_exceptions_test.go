// parlay-feature: parlay-tool/criterion-authority
// parlay-component: coverage-exception-ledger
// parlay-artifact: test

package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
)

func exceptionsFor(cs []AuthorizedCriterion, exs ...CoverageException) *CoverageExceptions {
	return &CoverageExceptions{
		Feature: "f", GrantedAt: "2026-08-27T00:00:00Z",
		CriteriaHash: CriteriaHash(cs), Exceptions: exs,
	}
}

func TestCoverageExceptions_FreshLedgerExcusesTheBulletItNames(t *testing.T) {
	cs := twoCriteria()
	rec := exceptionsFor(cs, CoverageException{
		Ref: cs[0].Ref, Text: cs[0].Text, Kind: ExceptionWaived, By: "interactive decision", At: "2026-08-27T00:00:00Z",
		Reason: "enforced by a database constraint, not by this operation",
	})

	v := EvaluateCoverageExceptions(t.TempDir(), rec, cs)
	if len(v.Blockers) != 0 {
		t.Fatalf("a fresh ledger should apply: %+v", v.Blockers)
	}
	if !v.Exempt.Excuses(agent.CriterionRef{Ref: cs[0].Ref, Text: cs[0].Text}) {
		t.Error("the named bullet should be excused")
	}
	if v.Exempt.Excuses(agent.CriterionRef{Ref: cs[1].Ref, Text: cs[1].Text}) {
		t.Error("a bullet-specific exception must not excuse a different one")
	}
}

// The hazard the whole record exists for: removing the blanket gate without a
// binding turns every exemption into a permanent unconditional waiver.
//
// The binding is per-exception, not whole-feature. A bullet-specific exception
// is valid exactly while the bullet it names is still declared — so REWORDING
// that bullet invalidates it, and rewording a different one does not.
func TestCoverageExceptions_ExceptionDiesWithTheBulletItNames(t *testing.T) {
	cs := twoCriteria()
	rec := exceptionsFor(cs, CoverageException{
		Ref: cs[0].Ref, Text: cs[0].Text, Kind: ExceptionWaived, By: "interactive decision", At: "2026-08-27T00:00:00Z", Reason: "r",
	})

	reworded := []AuthorizedCriterion{
		crit(cs[0].Ref, "archiving a customer with unpaid invoices returns a warning"),
		cs[1],
	}
	v := EvaluateCoverageExceptions(t.TempDir(), rec, reworded)
	if len(v.Blockers) == 0 {
		t.Fatal("the bullet this judgment was about no longer exists; the judgment has not been made about the new one")
	}
	if v.Exempt.Excuses(agent.CriterionRef{Ref: cs[0].Ref, Text: cs[0].Text}) {
		t.Error("a stale exception excuses nothing")
	}
}

// The improvement over binding to the whole standard: an unrelated change
// should not force re-review of a judgment it did not touch. Criteria authority
// approves the entire standard; an exception is a localized claim.
func TestCoverageExceptions_AnUnrelatedCriterionChangeDoesNotInvalidateIt(t *testing.T) {
	cs := twoCriteria()
	rec := exceptionsFor(cs, CoverageException{
		Ref: cs[0].Ref, Text: cs[0].Text, Kind: ExceptionWaived, By: "interactive decision", At: "2026-08-27T00:00:00Z", Reason: "r",
	})

	elsewhere := []AuthorizedCriterion{
		cs[0],
		crit(cs[1].Ref, "the archive button is HIDDEN while invoices are unpaid"),
	}
	v := EvaluateCoverageExceptions(t.TempDir(), rec, elsewhere)
	if len(v.Blockers) != 0 {
		t.Fatalf("rewording a presentation bullet says nothing about a waived operation bullet: %+v", v.Blockers)
	}
	if !v.Exempt.Excuses(agent.CriterionRef{Ref: cs[0].Ref, Text: cs[0].Text}) {
		t.Error("the untouched judgment should still hold")
	}
}

func TestCoverageExceptions_EntryWideIsAcceptedAndWarned(t *testing.T) {
	cs := twoCriteria()
	rec := exceptionsFor(cs, CoverageException{
		Ref: cs[0].Ref, Kind: ExceptionWaived, By: "interactive decision", At: "2026-08-27T00:00:00Z", Reason: "legacy, predates bullet identity",
		EntryHash: entryBulletsHash([]AuthorizedCriterion{cs[0]}),
	})

	v := EvaluateCoverageExceptions(t.TempDir(), rec, cs)
	if len(v.Blockers) != 0 {
		t.Fatalf("every exemption written before bullet identity is this shape: %+v", v.Blockers)
	}
	if !v.Exempt.Excuses(agent.CriterionRef{Ref: cs[0].Ref, Text: cs[0].Text}) {
		t.Error("entry-wide should excuse the entry")
	}
	if len(v.Warnings) == 0 || !strings.Contains(v.Warnings[0], "every bullet") {
		t.Errorf("it excuses more than one bullet, and should say so: %+v", v.Warnings)
	}

	// entry_hash is what keeps it from also excusing bullets added later, which
	// nobody judged.
	grown := append(twoCriteria(), crit(cs[0].Ref, "a second claim on the same operation"))
	if v := EvaluateCoverageExceptions(t.TempDir(), rec, grown); len(v.Blockers) == 0 {
		t.Error("adding a bullet to an entry-wide exception's entry must invalidate it")
	}
}

// A hand-authored exception claims a test parlay cannot inspect covers the
// criterion. Unnamed, that is not a claim.
func TestCoverageExceptions_HandAuthoredMustNameItsTest(t *testing.T) {
	cs := twoCriteria()
	rec := exceptionsFor(cs, CoverageException{
		Ref: cs[0].Ref, Text: cs[0].Text, Kind: ExceptionHandAuthored, By: "interactive decision", At: "2026-08-27T00:00:00Z", Reason: "covered by an integration suite",
	})
	if v := EvaluateCoverageExceptions(t.TempDir(), rec, cs); len(v.Blockers) == 0 {
		t.Error("an uninspectable test that is also unnamed excuses nothing")
	}
}

func TestCoverageExceptions_ExcusingSomethingTheContractDroppedBlocks(t *testing.T) {
	cs := twoCriteria()
	rec := exceptionsFor(cs, CoverageException{
		Ref: "@f/operation:gone", Text: "x", Kind: ExceptionWaived, By: "interactive decision", At: "2026-08-27T00:00:00Z", Reason: "r",
	})
	// Hash matches; the ref does not exist.
	rec.CriteriaHash = CriteriaHash(cs)
	if v := EvaluateCoverageExceptions(t.TempDir(), rec, cs); len(v.Blockers) == 0 {
		t.Error("excusing an entry that declares no criteria is a stale judgment wearing a fresh hash")
	}
}

func TestCoverageExceptions_NoLedgerIsNotAProblem(t *testing.T) {
	v := EvaluateCoverageExceptions(t.TempDir(), nil, twoCriteria())
	if len(v.Blockers) != 0 || len(v.Warnings) != 0 {
		t.Errorf("most features excuse nothing; that is the ordinary case: %+v", v)
	}
}

// The reserved kinds are refused, and carry no machinery of their own.
//
// hand-authored previously SUCCEEDED for any contained regular file whose hash
// matched — a false coverage path, since existence and a stable fingerprint
// establish that something is there and unchanged, never that it tests this
// criterion. The path checks and body hashing that guarded it were then
// reachable only from their own tests once the kind was refused ahead of them,
// which is the orphaned-leaf shape this release exists to eliminate. Removed
// rather than kept warm for a feature nobody has built.
func TestCoverageExceptions_ReservedKindsAreRefused(t *testing.T) {
	cs := twoCriteria()
	for _, kind := range []ExceptionKind{ExceptionHandAuthored, ExceptionStateOnly} {
		t.Run(string(kind), func(t *testing.T) {
			rec := exceptionsFor(cs, CoverageException{
				Ref: cs[0].Ref, Text: cs[0].Text, Kind: kind, Reason: "r",
			})
			v := EvaluateCoverageExceptions(t.TempDir(), rec, cs)
			if len(v.Blockers) == 0 {
				t.Fatalf("%s is reserved, not supported", kind)
			}
			if v.Exempt.Excuses(agent.CriterionRef{Ref: cs[0].Ref, Text: cs[0].Text}) {
				t.Error("a reserved kind must excuse nothing")
			}
		})
	}
}

// --- production path -----------------------------------------------------
//
// The evaluation above existed and was tested at the leaf while validate.go
// copied only the excused set and discarded every blocker and every error. So
// a stale ledger excused nothing and SAID nothing — the drop-and-proceed
// behaviour the freshness rule exists to prevent, under a comment claiming the
// opposite. These run through the real gate.

func writeExceptions(t *testing.T, cfg *config.Context, slug string, rec *CoverageExceptions) {
	t.Helper()
	if err := saveCoverageExceptions(cfg, slug, rec); err != nil {
		t.Fatal(err)
	}
}

func TestCoverageExceptions_GateReportsAStaleLedger(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)

	current, err := CurrentCriteria(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	writeExceptions(t, cfg, "graded", &CoverageExceptions{
		Feature: "graded", GrantedAt: "2026-08-27T00:00:00Z",
		Exceptions: []CoverageException{{
			Ref: current[0].Ref, Text: "a claim this contract never made",
			Kind: ExceptionWaived, By: "interactive decision", At: "2026-08-27T00:00:00Z", Reason: "r",
		}},
	})

	out, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !gateHasCode(out.Blockers, "coverage-exception-invalid") {
		t.Fatalf("a ledger excusing a criterion nobody declared must reach the boundary, not vanish: %+v", out.Blockers)
	}
}

func TestCoverageExceptions_GateReportsAnUnreadableLedger(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)

	path := coverageExceptionsPath(cfg, "graded")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("exceptions: [\n  broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ := computeGate(cfg, "graded", gateStageCode)
	if !gateHasCode(out.Blockers, "coverage-exception-invalid") {
		t.Errorf("an unreadable ledger is not a feature with nothing excused: %+v", out.Blockers)
	}
}

func TestCoverageExceptions_GateStaysQuietWithNoLedger(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)

	out, _ := computeGate(testContext(t), "graded", gateStageCode)
	if gateHasCode(out.Blockers, "coverage-exception-invalid") {
		t.Errorf("most features excuse nothing; that is the ordinary case: %+v", out.Blockers)
	}
}

// A downgrade decision must name the case it accepts. One that names none
// would accept every weakening of that criterion, including ones nobody saw —
// which is the review it exists to be, granted in advance and in the dark.
func TestCoverageExceptions_GateRefusesStateOnlyAsAnExemption(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	current, _ := CurrentCriteria(cfg, "graded")

	writeExceptions(t, cfg, "graded", &CoverageExceptions{
		Feature: "graded", GrantedAt: "2026-08-27T00:00:00Z",
		Exceptions: []CoverageException{{
			Ref: current[0].Ref, Text: current[0].Text,
			Kind: ExceptionStateOnly, By: "interactive decision", At: "2026-08-27T00:00:00Z", Reason: "observed by state",
		}},
	})

	out, _ := computeGate(cfg, "graded", gateStageCode)
	var msg string
	for _, b := range out.Blockers {
		if b.Code == "coverage-exception-invalid" {
			msg = b.Message
		}
	}
	if msg == "" || !strings.Contains(msg, "names no suite:/case:") {
		t.Errorf("a downgrade decision naming no case would accept every weakening of that criterion, including ones nobody saw: %+v", out.Blockers)
	}
}

// A person's recorded judgment must not evaporate because the file that held
// it stopped being read. validate.go used to fold legacy exemptions in and no
// longer does, so silence here would drop them into uncovered warnings a build
// may still pass.
func TestCoverageExceptions_GateRefusesStrandedLegacyExemptions(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)

	buildDir := cfg.BuildPath("graded")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `feature: graded
reviewed_at: "2026-05-01T00:00:00Z"
reviewed_by: node
review_method: cli
buildfile_hash: sha256:x
testcases_hash: sha256:y
approved_suites:
    - customer-detail
exemptions:
    - suite: customer-detail
      item: "@graded/operation:customer.archive"
      reason: enforced by a database constraint
`
	if err := os.WriteFile(filepath.Join(buildDir, "coverage-review.yaml"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ := computeGate(cfg, "graded", gateStageCode)
	var msg string
	for _, b := range out.Blockers {
		if b.Code == "coverage-exception-invalid" {
			msg = b.Message
		}
	}
	if msg == "" {
		t.Fatalf("stranded exemptions must be reported, not dropped: %+v", out.Blockers)
	}
	if !strings.Contains(msg, "migrate-coverage-exceptions") {
		t.Errorf("the refusal should name the remedy: %q", msg)
	}
}

// The approvals half proved nothing and is gone on purpose, so a legacy file
// carrying only those strands nothing.
func TestCoverageExceptions_LegacyApprovalsAloneStrandNothing(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)

	buildDir := cfg.BuildPath("graded")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "coverage-review.yaml"), []byte(
		"feature: graded\nreviewed_at: \"2026-05-01T00:00:00Z\"\nreviewed_by: node\napproved_suites:\n    - customer-detail\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ := computeGate(cfg, "graded", gateStageCode)
	if gateHasCode(out.Blockers, "coverage-exception-invalid") {
		t.Errorf("suite approvals were the half that proved nothing; losing them is the point: %+v", out.Blockers)
	}
}

// A weakened observation stops the run unless somebody accepted it.
//
// The case cites its criterion correctly and every mechanical walk passes, so
// nothing else can see this: the suite review that caught it is gone, and
// criterion approval happens before testcases exist. A warning would not do —
// warnings advance in CI, so the unattended path this release enables would
// permit agent-authored weakening with nobody in the loop.
func TestTestcasesReadiness_UnapprovedDowngradeBlocks(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	approveClean(t, cfg)
	weaken(t, cfg)

	r := CheckTestcasesReadiness(cfg, "graded")
	var found string
	for _, b := range r.Blockers {
		if strings.Contains(b, "criterion-observed-weakly") {
			found = b
		}
	}
	if found == "" {
		t.Fatalf("an unaccepted weakening must stop the run: blockers=%+v warnings=%+v", r.Blockers, r.Warnings)
	}
	if !strings.Contains(found, "nobody accepted that") {
		t.Errorf("say what is missing rather than that the downgrade is wrong: %q", found)
	}
}

// And a recorded decision naming that exact case lets it through.
func TestTestcasesReadiness_AcceptedDowngradePasses(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	approveClean(t, cfg)
	suite, caseName, ref, text := weaken(t, cfg)

	if err := saveCoverageExceptions(cfg, "graded", &CoverageExceptions{
		Feature: "graded", GrantedAt: "2026-08-27T00:00:00Z",
		Exceptions: []CoverageException{{
			Ref: ref, Text: text, Kind: ExceptionStateOnly, Suite: suite, Case: caseName, By: "interactive decision", At: "2026-08-27T00:00:00Z",
			Reason: "the control is rendered by a third-party component this suite cannot reach",
		}},
	}); err != nil {
		t.Fatal(err)
	}

	r := CheckTestcasesReadiness(cfg, "graded")
	for _, b := range r.Blockers {
		if strings.Contains(b, "criterion-observed-weakly") {
			t.Errorf("this exact case was accepted: %s", b)
		}
	}
}

// A decision naming a DIFFERENT case does not cover this one: accepting that
// checking the store is honest here says nothing about another case doing the
// same thing for another reason.
func TestTestcasesReadiness_DowngradeDecisionIsPerCase(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	approveClean(t, cfg)
	_, _, ref, text := weaken(t, cfg)

	if err := saveCoverageExceptions(cfg, "graded", &CoverageExceptions{
		Feature: "graded", GrantedAt: "2026-08-27T00:00:00Z",
		Exceptions: []CoverageException{{
			Ref: ref, Text: text, Kind: ExceptionStateOnly,
			Suite: "Some Other Suite", Case: "some other case", By: "interactive decision", At: "2026-08-27T00:00:00Z", Reason: "r",
		}},
	}); err != nil {
		t.Fatal(err)
	}

	r := CheckTestcasesReadiness(cfg, "graded")
	blocked := false
	for _, b := range r.Blockers {
		if strings.Contains(b, "criterion-observed-weakly") {
			blocked = true
		}
	}
	if !blocked {
		t.Error("a decision about a different case must not cover this one")
	}
}

// weaken turns the fixture's presentation case into a state-only observation
// and returns its identity.
func weaken(t *testing.T, cfg *config.Context) (suite, caseName, ref, text string) {
	t.Helper()
	path := filepath.Join(cfg.BuildPath("graded"), "testcases.yaml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := strings.Replace(string(body), "coverage: full", "coverage: state-only", 1)
	if out == string(body) {
		t.Fatal("fixture has no coverage: full to weaken; the test would prove nothing")
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	return "Customer Detail", "disabled while unpaid",
		"@graded/fragment:Customer Detail", "the archive button is disabled while invoices are unpaid"
}

// --- authoring -------------------------------------------------------------
//
// R1 shipped a record the tool could read, validate, bind and block on, that a
// person could only produce by hand-editing YAML against a shape documented
// nowhere. A decision artifact with no authoring path is one nobody makes
// deliberately.

func recordExc(t *testing.T, cfg *config.Context, slug string) error {
	t.Helper()
	cmd := testCommandWithContext(t, cfg)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	return runRecordException(cmd, []string{"@" + slug})
}

func resetExcFlags() {
	recordExceptionRef, recordExceptionText, recordExceptionKind = "", "", string(ExceptionWaived)
	recordExceptionReason, recordExceptionBy = "", ""
	recordExceptionSuite, recordExceptionCase = "", ""
}

func TestRecordException_WritesAWaiverTheGateHonours(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	approveClean(t, cfg)
	defer resetExcFlags()

	resetExcFlags()
	recordExceptionRef = "@graded/operation:customer.archive"
	recordExceptionText = "archiving a customer with unpaid invoices is rejected"
	recordExceptionReason = "enforced by a database constraint, not by this operation"
	recordExceptionBy = "interactive decision"

	if err := recordExc(t, cfg, "graded"); err != nil {
		t.Fatal(err)
	}

	v := CheckCoverageExceptions(cfg, "graded")
	if len(v.Blockers) != 0 {
		t.Fatalf("a record this command just wrote should validate: %+v", v.Blockers)
	}
	if !v.Exempt.Excuses(agent.CriterionRef{Ref: recordExceptionRef, Text: recordExceptionText}) {
		t.Error("the waiver should excuse the bullet it names")
	}
}

// A text matching no bullet would produce a ledger that reads as working and
// excuses nothing, so it is refused at the point of writing rather than
// discovered at a boundary later.
func TestRecordException_RefusesACriterionThatDoesNotExist(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	defer resetExcFlags()

	resetExcFlags()
	recordExceptionRef = "@graded/operation:customer.archive"
	recordExceptionText = "a claim this contract never made"
	recordExceptionReason = "r"
	recordExceptionBy = "interactive decision"

	if err := recordExc(t, cfg, "graded"); err == nil {
		t.Error("an exception about a criterion nobody declared excuses nothing and looks like it does")
	}
}

func TestRecordException_StateOnlyNeedsTheCaseItAccepts(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	defer resetExcFlags()

	resetExcFlags()
	recordExceptionKind = string(ExceptionStateOnly)
	recordExceptionRef = "@graded/fragment:Customer Detail"
	recordExceptionText = "the archive button is disabled while invoices are unpaid"
	recordExceptionReason = "r"
	recordExceptionBy = "interactive decision"

	if err := recordExc(t, cfg, "graded"); err == nil {
		t.Error("a downgrade decision naming no case accepts every weakening of that criterion, including ones nobody saw")
	}
}

// The migration reports and writes nothing: whether an old judgment still
// applies is the one thing nobody but its author can say.
func TestMigrateExceptions_ReportsWithoutWriting(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)

	legacy := `feature: graded
reviewed_at: "2026-05-01T00:00:00Z"
reviewed_by: node
exemptions:
    - suite: s
      item: "@graded/operation:customer.archive"
      reason: enforced by a database constraint
    - suite: s
      item: "@graded/operation:gone"
      reason: this operation was removed
`
	if err := os.WriteFile(filepath.Join(cfg.BuildPath("graded"), "coverage-review.yaml"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	cmd := testCommandWithContext(t, cfg)
	cmd.SetOut(&buf)
	if err := runMigrateExceptions(cmd, []string{"@graded"}); err != nil {
		t.Fatal(err)
	}
	var out migrateExceptionsOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, buf.String())
	}
	if len(out.Pending) != 2 {
		t.Fatalf("both stranded judgments should be listed: %+v", out.Pending)
	}
	// One excuses a criterion that still exists; one does not, and that is the
	// difference between re-deciding and deleting.
	var live, dead int
	for _, p := range out.Pending {
		if p.StillDeclared {
			live++
		} else {
			dead++
		}
	}
	if live != 1 || dead != 1 {
		t.Errorf("the report should say which are still about something: %+v", out.Pending)
	}
	if _, err := os.Stat(coverageExceptionsPath(cfg, "graded")); !os.IsNotExist(err) {
		t.Error("the migration must not write a decision on the author's behalf")
	}
}

// The seam a whole-branch read found: the stranded check used to fire only
// while coverage-exceptions.yaml did not exist, so the FIRST fresh decision
// created the file and every other stranded judgment vanished from the
// blocker. Migrating one silently abandoned the rest.
func TestCoverageExceptions_MigratingOneDoesNotStrandTheOthers(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	approveClean(t, cfg)
	defer resetExcFlags()

	legacy := `feature: graded
reviewed_at: "2026-05-01T00:00:00Z"
exemptions:
    - suite: s
      item: "@graded/operation:customer.archive"
      criterion_text: archiving a customer with unpaid invoices is rejected
      reason: enforced by a database constraint
    - suite: s
      item: "@graded/fragment:Customer Detail"
      criterion_text: the archive button is disabled while invoices are unpaid
      reason: the control is third-party
`
	if err := os.WriteFile(filepath.Join(cfg.BuildPath("graded"), "coverage-review.yaml"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	// Identity now comes from the listing, not from re-deriving (ref, text).
	entries, _, lErr := loadLegacyEntries(cfg, "graded")
	if lErr != nil {
		t.Fatal(lErr)
	}
	fpFor := func(ref string) legacyEntry {
		t.Helper()
		for _, e := range entries {
			if e.Ref == ref {
				return e
			}
		}
		t.Fatalf("fixture has no entry for %s", ref)
		return legacyEntry{}
	}

	// Answer the first.
	resetExcFlags()
	recordExceptionLegacyFP = fpFor("@graded/operation:customer.archive").Fingerprint
	recordExceptionLegacyDup = 0
	recordExceptionRef = "@graded/operation:customer.archive"
	recordExceptionText = "archiving a customer with unpaid invoices is rejected"
	recordExceptionReason = "enforced by a database constraint"
	recordExceptionBy = "interactive decision"
	recordExceptionFromLegacy = true
	if err := recordExc(t, cfg, "graded"); err != nil {
		t.Fatal(err)
	}

	v := CheckCoverageExceptions(cfg, "graded")
	var stranded string
	for _, b := range v.Blockers {
		if strings.Contains(b, "nothing has answered") {
			stranded = b
		}
	}
	if stranded == "" {
		t.Fatal("the second judgment is still unanswered and must keep being reported")
	}
	if strings.Contains(stranded, "customer.archive") {
		t.Error("the one just answered should no longer be listed")
	}
	if !strings.Contains(stranded, "Customer Detail") {
		t.Errorf("the unanswered one should be named: %q", stranded)
	}

	// Drop the second deliberately, and the boundary goes quiet.
	dropLegacyFP = fpFor("@graded/fragment:Customer Detail").Fingerprint
	dropLegacyDup = 0
	dropLegacyReason = "the control was removed in the redesign"
	dropLegacyBy = "interactive decision"
	defer func() {
		dropLegacyFP, dropLegacyDup, dropLegacyReason, dropLegacyBy = "", 0, "", ""
	}()

	cmd := testCommandWithContext(t, cfg)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runDropLegacyExemption(cmd, []string{"@graded"}); err != nil {
		t.Fatal(err)
	}

	v2 := CheckCoverageExceptions(cfg, "graded")
	for _, b := range v2.Blockers {
		if strings.Contains(b, "nothing has answered") {
			t.Errorf("every stranded judgment is answered now: %s", b)
		}
	}

	// And the drop is recorded, because a judgment abandoned without a trace
	// is indistinguishable from one nobody noticed.
	rec, _ := loadCoverageExceptions(cfg, "graded")
	if rec == nil || len(rec.ReconciledLegacy) != 2 {
		t.Fatalf("both dispositions should be recorded: %+v", rec)
	}
	var dropped int
	for _, d := range rec.ReconciledLegacy {
		if d.Disposition == "dropped" {
			dropped++
			if d.Reason == "" || d.By == "" {
				t.Error("a deliberate deletion records why and what decided it")
			}
		}
	}
	if dropped != 1 {
		t.Errorf("expected one recorded deletion, got %d", dropped)
	}
}

// One criterion may be observed weakly by several cases, each for its own
// reason, each needing its own judgment. Keying every decision on (ref, text)
// alone made the second such decision look like a duplicate of the first, so
// the ledger refused a legitimate shape and the only way forward was to leave
// one case unreviewed.
func TestCoverageExceptions_TwoCasesMayEachDowngradeOneCriterion(t *testing.T) {
	cs := twoCriteria()
	rec := exceptionsFor(cs,
		CoverageException{
			Ref: cs[0].Ref, Text: cs[0].Text, Kind: ExceptionStateOnly,
			Suite: "Store honesty", Case: "writes land", CaseHash: "aaa",
			By: "interactive decision", At: "2026-08-27T00:00:00Z",
			Reason: "state is the only honest observation for a write",
		},
		CoverageException{
			Ref: cs[0].Ref, Text: cs[0].Text, Kind: ExceptionStateOnly,
			Suite: "Store honesty", Case: "deletes land", CaseHash: "bbb",
			By: "interactive decision", At: "2026-08-27T00:00:00Z",
			Reason: "a delete has no return value to assert on",
		},
	)

	v := EvaluateCoverageExceptions(t.TempDir(), rec, cs)
	for _, b := range v.Blockers {
		if strings.Contains(b, "excused twice") {
			t.Fatalf("two cases downgrading one criterion is legitimate, not a duplicate claim: %s", b)
		}
	}
	if len(v.AcceptedDowngrades) != 2 {
		t.Fatalf("both decisions must stand on their own; got %d", len(v.AcceptedDowngrades))
	}
}

// A waiver, unlike a downgrade, is a claim about the criterion itself. Two of
// them cannot both be reviewed — one silently shadows the other.
func TestCoverageExceptions_OneCriterionCannotBeWaivedTwice(t *testing.T) {
	cs := twoCriteria()
	dup := CoverageException{
		Ref: cs[0].Ref, Text: cs[0].Text, Kind: ExceptionWaived,
		By: "interactive decision", At: "2026-08-27T00:00:00Z",
		Reason: "enforced by a database constraint",
	}
	v := EvaluateCoverageExceptions(t.TempDir(), exceptionsFor(cs, dup, dup), cs)
	found := false
	for _, b := range v.Blockers {
		if strings.Contains(b, "excused twice") {
			found = true
		}
	}
	if !found {
		t.Fatalf("two waivers on one criterion must still be refused; got %+v", v.Blockers)
	}
}

// legacyFixture writes a retired review whose two exemptions share a ref and
// criterion text but record DIFFERENT judgments. Keyed on (ref, text) alone,
// answering one marked both answered — so a judgment nobody reviewed passed as
// reconciled.
func legacyFixture(t *testing.T, cfg *config.Context, body string) {
	t.Helper()
	if err := os.MkdirAll(cfg.BuildPath("graded"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.BuildPath("graded"), "coverage-review.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const twoJudgmentsOnOneBullet = `schema_version: 1
feature: graded
exemptions:
    - suite: s
      item: "@graded/operation:customer.archive"
      criterion_text: archiving a customer with unpaid invoices is rejected
      reason: enforced by a database constraint
    - suite: s
      item: "@graded/operation:customer.archive"
      criterion_text: archiving a customer with unpaid invoices is rejected
      reason: the archive path is unreachable from the UI in this release
`

func TestCoverageExceptions_AnsweringOneJudgmentDoesNotAnswerAnother(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	legacyFixture(t, cfg, twoJudgmentsOnOneBullet)

	entries, _, err := loadLegacyEntries(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("fixture must hold two entries, got %d", len(entries))
	}
	if entries[0].Fingerprint == entries[1].Fingerprint {
		t.Fatal("two judgments differing only in reason must not share a fingerprint — reason is what distinguishes them")
	}

	resetExcFlags()
	recordExceptionLegacyFP = entries[0].Fingerprint
	recordExceptionRef = "@graded/operation:customer.archive"
	recordExceptionText = "archiving a customer with unpaid invoices is rejected"
	recordExceptionReason = "still enforced by the constraint"
	recordExceptionBy = "interactive decision"
	recordExceptionFromLegacy = true
	if err := recordExc(t, cfg, "graded"); err != nil {
		t.Fatal(err)
	}

	stranded, unreadable := strandedLegacyExemptions(cfg, "graded", mustLoadExc(t, cfg))
	if unreadable != "" {
		t.Fatal(unreadable)
	}
	if len(stranded) != 1 {
		t.Fatalf("the second judgment was never reviewed and must still be reported; got %d stranded: %v", len(stranded), stranded)
	}
}

// Dispositions answer entries in ONE version of the legacy file. If the file
// changed after they were written, they no longer demonstrably answer what is
// there now — an entry may have been added, or one somebody judged reworded
// into a different judgment.
func TestCoverageExceptions_EditingTheLegacyFileInvalidatesItsDispositions(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	legacyFixture(t, cfg, twoJudgmentsOnOneBullet)

	entries, _, err := loadLegacyEntries(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		resetExcFlags()
		recordExceptionLegacyFP = e.Fingerprint
		recordExceptionRef, recordExceptionText = e.Ref, e.Text
		recordExceptionReason = "still holds"
		recordExceptionBy = "interactive decision"
		recordExceptionFromLegacy = true
		if err := recordExc(t, cfg, "graded"); err != nil {
			t.Fatal(err)
		}
	}
	if stranded, _ := strandedLegacyExemptions(cfg, "graded", mustLoadExc(t, cfg)); len(stranded) != 0 {
		t.Fatalf("both are answered, so nothing should be stranded: %v", stranded)
	}

	// The file gains a third judgment nobody has seen.
	legacyFixture(t, cfg, twoJudgmentsOnOneBullet+
		"    - suite: s\n      item: \"@graded/fragment:Customer Detail\"\n      reason: added later\n")

	stranded, _ := strandedLegacyExemptions(cfg, "graded", mustLoadExc(t, cfg))
	if len(stranded) == 0 {
		t.Fatal("the legacy file changed after reconciliation and must not read as still answered")
	}
	if !strings.Contains(strings.Join(stranded, " "), "changed after") {
		t.Errorf("the report should say the file moved under the dispositions; got %v", stranded)
	}
}

func mustLoadExc(t *testing.T, cfg *config.Context) *CoverageExceptions {
	t.Helper()
	rec, err := loadCoverageExceptions(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	return rec
}
