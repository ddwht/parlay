// parlay-feature: parlay-tool/criterion-authority
// parlay-component: criteria-authority-record
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

func crit(ref, text string) AuthorizedCriterion { return AuthorizedCriterion{Ref: ref, Text: text} }

func twoCriteria() []AuthorizedCriterion {
	return []AuthorizedCriterion{
		crit("@f/operation:archive", "archiving a customer with unpaid invoices is rejected"),
		crit("@f/fragment:customer-detail", "the archive button is disabled while invoices are unpaid"),
	}
}

func approvedFor(cs []AuthorizedCriterion) *CriteriaAuthority {
	return &CriteriaAuthority{
		Feature:  "f",
		Approved: &HumanApproval{At: "2026-08-27T00:00:00Z", CriteriaHash: CriteriaHash(cs), Criteria: cs},
	}
}

func TestCriteriaAuthority_ApprovedCriteriaProceedEverywhere(t *testing.T) {
	v := EvaluateCriteriaAuthority(approvedFor(twoCriteria()), twoCriteria(), false, false)
	if !v.Proceed || v.Machine {
		t.Errorf("a person approved this exact standard; %+v", v)
	}
}

// The friction the old gate was most disliked for: one full re-approval per
// regeneration. What is approved here is the standard, so regenerating
// testcases against unchanged criteria asks nothing.
func TestCriteriaAuthority_ReorderingAndReformattingDoNotReapprove(t *testing.T) {
	cs := twoCriteria()
	rec := approvedFor(cs)

	shuffled := []AuthorizedCriterion{cs[1], cs[0]}
	if !EvaluateCriteriaAuthority(rec, shuffled, false, false).Proceed {
		t.Error("order is not identity; reordering must not invalidate an approval")
	}

	respaced := []AuthorizedCriterion{
		crit(cs[0].Ref, "  archiving a customer with unpaid invoices is rejected  "),
		cs[1],
	}
	if !EvaluateCriteriaAuthority(rec, respaced, false, false).Proceed {
		t.Error("whitespace is not identity either")
	}
}

func TestCriteriaAuthority_ChangedStandardRefusesAndShowsTheChange(t *testing.T) {
	rec := approvedFor(twoCriteria())
	changed := []AuthorizedCriterion{
		twoCriteria()[0],
		crit("@f/fragment:customer-detail", "the archive button is HIDDEN while invoices are unpaid"),
	}
	v := EvaluateCriteriaAuthority(rec, changed, false, false)
	if v.Proceed {
		t.Fatal("a rewritten standard is not the approved one")
	}
	if len(v.Added) != 1 || len(v.Removed) != 1 {
		t.Errorf("a stale record should show WHAT changed, not only that something did: %+v", v)
	}
}

// Both switches or neither counts, and a refusal names the missing half.
func TestCriteriaAuthority_MachineAuthorizationNeedsBothSwitches(t *testing.T) {
	cs := twoCriteria()

	if v := EvaluateCriteriaAuthority(nil, cs, true, false); v.Proceed {
		t.Error("a flag alone must not waive the separation; the project has not opted in")
	}
	if v := EvaluateCriteriaAuthority(nil, cs, false, true); v.Proceed {
		t.Error("an opt-in alone must not make every run self-authorizing")
	}
	v := EvaluateCriteriaAuthority(nil, cs, true, true)
	if !v.Proceed || !v.Machine {
		t.Fatalf("project permits it and the run asked; %+v", v)
	}
	// The record must not read as approval. It is a waiver.
	if !strings.Contains(v.Reason, "WAIVED") {
		t.Errorf("machine mode records that nobody looked, not that anybody did: %q", v.Reason)
	}
}

// One unattended escape must not permanently answer the question for everyone
// who comes after.
func TestCriteriaAuthority_APastMachineRunDoesNotAuthorizeALaterOne(t *testing.T) {
	cs := twoCriteria()
	rec := &CriteriaAuthority{
		Feature:     "f",
		MachineRuns: []MachineRun{{At: "2026-08-26T00:00:00Z", CriteriaHash: CriteriaHash(cs), Reason: "ci"}},
	}
	v := EvaluateCriteriaAuthority(rec, cs, false, true)
	if v.Proceed {
		t.Fatal("an audit event is not standing authority")
	}
	if !strings.Contains(v.Reason, "nobody looked") {
		t.Errorf("the refusal should say what the earlier run actually recorded: %q", v.Reason)
	}
}

// --- production path -----------------------------------------------------
//
// The rules above are a pure function, and a pure function nobody reaches is
// the failure this session has already shipped twice. These run through
// computeGate, which is what the loop and the CLI actually call.

func writeCriteriaFixture(t *testing.T, dir string) {
	t.Helper()
	featDir := filepath.Join(dir, "spec", "intents", "graded")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"intents.md": "# Graded\n\n## Archive A Customer\n\n**Goal**: g.\n**Persona**: p.\n",
		"capabilities.yaml": `schema_version: 1
feature: graded
operations:
  - id: customer.archive
    source: '@graded/archive-a-customer'
    kind: command
    subject:
      entity: Customer
    verify:
      - archiving a customer with unpaid invoices is rejected
    steps:
      - { type: validate-input }
`,
		"surface.yaml": `feature: graded
fragments:
    - name: Customer Detail
      page: customers
      region: main
      shows: detail
      order: 1
      actions: invoke
      source: '@graded/archive-a-customer'
      verify:
        - the archive button is disabled while invoices are unpaid
`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(featDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func gateCodeFor(t *testing.T, slug string) gateOutput {
	t.Helper()
	out, err := computeGate(testContext(t), slug, gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCriteriaAuthority_GateRefusesAnUnapprovedStandard(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)

	out := gateCodeFor(t, "graded")
	if !gateHasCode(out.Blockers, "criteria-authority-missing") {
		t.Fatalf("codegen must not run against a standard nobody approved; blockers=%+v", out.Blockers)
	}
	if out.Passed {
		t.Error("gate must not pass")
	}
}

func TestCriteriaAuthority_GateAcceptsAnApprovedStandard(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)

	current, err := CurrentCriteria(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != 2 {
		t.Fatalf("the fixture declares two criteria, one per destination; got %+v", current)
	}
	if err := RecordHumanApproval(cfg, "graded", current, "2026-08-27T00:00:00Z", "interactive decision", "dec-1"); err != nil {
		t.Fatal(err)
	}

	if gateHasCode(gateCodeFor(t, "graded").Blockers, "criteria-authority-missing") {
		t.Error("an approved standard should pass this gate")
	}
}

// The friction the old gate was disliked for, proven end to end: the standard
// is what was approved, so regenerating derived work asks nothing.
func TestCriteriaAuthority_GateStillPassesAfterUnrelatedArtifactEdits(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	current, _ := CurrentCriteria(cfg, "graded")
	if err := RecordHumanApproval(cfg, "graded", current, "2026-08-27T00:00:00Z", "interactive decision", "dec-1"); err != nil {
		t.Fatal(err)
	}

	// Change something real that is not a criterion.
	surface := filepath.Join(dir, "spec", "intents", "graded", "surface.yaml")
	body, _ := os.ReadFile(surface)
	edited := strings.Replace(string(body), "region: main", "region: sidebar", 1)
	if err := os.WriteFile(surface, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	if gateHasCode(gateCodeFor(t, "graded").Blockers, "criteria-authority-missing") {
		t.Error("moving a fragment between regions does not change what it must do; reapproval is friction with no question in it")
	}
}

func TestCriteriaAuthority_GateRefusesAfterTheStandardChanges(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	current, _ := CurrentCriteria(cfg, "graded")
	if err := RecordHumanApproval(cfg, "graded", current, "2026-08-27T00:00:00Z", "interactive decision", "dec-1"); err != nil {
		t.Fatal(err)
	}

	surface := filepath.Join(dir, "spec", "intents", "graded", "surface.yaml")
	body, _ := os.ReadFile(surface)
	edited := strings.Replace(string(body), "the archive button is disabled while invoices are unpaid",
		"the archive button is hidden while invoices are unpaid", 1)
	if err := os.WriteFile(surface, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	out := gateCodeFor(t, "graded")
	var msg string
	for _, b := range out.Blockers {
		if b.Code == "criteria-authority-missing" {
			msg = b.Message
		}
	}
	if msg == "" {
		t.Fatalf("a rewritten standard is not the approved one; blockers=%+v", out.Blockers)
	}
	if !strings.Contains(msg, "no longer") || !strings.Contains(msg, "now:") {
		t.Errorf("the refusal must show what changed rather than only that something did: %q", msg)
	}
}

// A hand-edited hash must not authorize stored evidence of a different
// standard.
func TestCriteriaAuthority_GateRefusesATamperedRecord(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	current, _ := CurrentCriteria(cfg, "graded")
	if err := RecordHumanApproval(cfg, "graded", current, "2026-08-27T00:00:00Z", "interactive decision", "dec-1"); err != nil {
		t.Fatal(err)
	}

	path := criteriaAuthorityPath(cfg, "graded")
	body, _ := os.ReadFile(path)
	tampered := strings.Replace(string(body), "archiving a customer with unpaid invoices is rejected",
		"anything at all is fine", 1)
	if err := os.WriteFile(path, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	if !gateHasCode(gateCodeFor(t, "graded").Blockers, "criteria-authority-missing") {
		t.Error("a record whose hash disagrees with the criteria it stores authorizes nothing")
	}
}

// The gate reports what is true of the feature. Exercising a waiver is
// something an invocation does, so the gate must not answer that governance
// question for a caller that never asked.
func TestCriteriaAuthority_GateDoesNotSelfAuthorizeEvenWhenPolicyAllows(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfgDir := filepath.Join(dir, ".parlay")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"),
		[]byte("ai-agent: Claude Code\ncriterion-authority.allow-machine: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !gateHasCode(gateCodeFor(t, "graded").Blockers, "criteria-authority-missing") {
		t.Error("policy permits a waiver; it does not exercise one")
	}
}

// --- CLI ------------------------------------------------------------------

func runCriteriaAuthorityCmd_(t *testing.T, args ...string) (criteriaAuthorityOutput, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	err := runCriteriaAuthority(cmd, args)
	var out criteriaAuthorityOutput
	if buf.Len() > 0 {
		if jsonErr := json.Unmarshal(buf.Bytes(), &out); jsonErr != nil {
			t.Fatalf("not JSON: %v\n%s", jsonErr, buf.String())
		}
	}
	return out, err
}

func allowMachineAuthority(t *testing.T, dir string) {
	t.Helper()
	cfgDir := filepath.Join(dir, ".parlay")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"),
		[]byte("ai-agent: Claude Code\ncriterion-authority.allow-machine: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCriteriaAuthorityCLI_RefusesUnapprovedWithNonZeroExit(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	authorizeCriteriaMode = ""

	out, err := runCriteriaAuthorityCmd_(t, "@graded")
	if err == nil {
		t.Error("a refusal must exit non-zero so CI notices it")
	}
	if out.Authorized {
		t.Error("nobody approved this standard")
	}
	if out.Hash == "" || len(out.Criteria) != 2 {
		t.Errorf("the report should name the standard it refused over: %+v", out)
	}
}

// The reporting command records NOTHING. It logged a waiver for a run that
// never proceeded, while the boundary that actually advanced refused — it was
// handed machineFlag=false by a caller with no way to pass anything else. So
// the flag there previews, and the gate exercises.
func TestCriteriaAuthorityCLI_ReportingDoesNotRecordAWaiver(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	allowMachineAuthority(t, dir)
	cfg := testContext(t)

	authorizeCriteriaMode = machineAuthorizationMode
	defer func() { authorizeCriteriaMode = "" }()

	out, err := runCriteriaAuthorityCmd_(t, "@graded")
	if err != nil || !out.Authorized || !out.Machine {
		t.Fatalf("the preview should say an advancing run would be permitted: %+v (%v)", out, err)
	}
	rec, _ := loadCriteriaAuthority(cfg, "graded")
	if rec != nil && len(rec.MachineRuns) > 0 {
		t.Error("nothing advanced, so nothing should have been recorded")
	}
}

// The CI path, through the advancing COMMAND rather than the evaluation.
func runGateCmd_(t *testing.T, slug, stage string) error {
	t.Helper()
	prev := gateStage
	gateStage = stage
	defer func() { gateStage = prev }()
	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	return runInternalGate(cmd, []string{slug})
}

// The waiver is exercised by the advancing command, only after the whole
// boundary passed, and only at the code crossing.
//
// Driven through commitPendingWaiver with a passing verdict rather than by
// constructing a feature that clears every unrelated check, so the property
// under test is the transaction and not the fixture.
func TestCriteriaAuthority_WaiverIsCommittedOnlyByAPassedCodeBoundary(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	current, err := CurrentCriteria(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	pending := &pendingMachineRun{criteria: current, reason: "authorized"}

	// Passed, at the code boundary: recorded.
	if err := commitPendingWaiver(cfg, "graded", gateStageCode,
		gateOutput{Passed: true, PendingWaiver: pending}); err != nil {
		t.Fatal(err)
	}
	rec, _ := loadCriteriaAuthority(cfg, "graded")
	if rec == nil || len(rec.MachineRuns) != 1 {
		t.Fatalf("expected one waiver, got %+v", rec)
	}
	if rec.Approved != nil {
		t.Error("a waiver must never become approval")
	}

	// Done, same pipeline: NOT recorded again. A normal loop crosses code and
	// then done, and logging both writes one pipeline as two runs that
	// proceeded without human approval.
	if err := commitPendingWaiver(cfg, "graded", gateStageDone,
		gateOutput{Passed: true, PendingWaiver: pending}); err != nil {
		t.Fatal(err)
	}
	rec, _ = loadCriteriaAuthority(cfg, "graded")
	if len(rec.MachineRuns) != 1 {
		t.Errorf("the done crossing inherits the code run's authorization rather than logging its own: %+v", rec.MachineRuns)
	}
}

// The negative control that matters most: one subcheck permitting the waiver
// says nothing about the aggregate. Recording before the boundary decided
// produced an audit trail asserting that a REFUSED run had advanced.
func TestCriteriaAuthority_NoWaiverIsRecordedWhenSomethingElseBlocks(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	allowMachineAuthority(t, dir)
	cfg := testContext(t)
	// No testcases at all: the criteria subcheck would permit the waiver, and
	// the boundary still refuses.

	gateAuthorizeCriteria = machineAuthorizationMode
	defer func() { gateAuthorizeCriteria = "" }()

	out, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if out.Passed {
		t.Fatal("a feature with criteria and no testcases must not advance")
	}
	if out.PendingWaiver != nil {
		t.Error("a boundary that did not pass has no waiver to exercise")
	}
	// And the command writes nothing for a verdict that did not pass.
	if err := commitPendingWaiver(cfg, "graded", gateStageCode, out); err != nil {
		t.Fatal(err)
	}
	if rec, _ := loadCriteriaAuthority(cfg, "graded"); rec != nil && len(rec.MachineRuns) > 0 {
		t.Errorf("nothing advanced, so the audit trail must not say one did: %+v", rec.MachineRuns)
	}
}

// A typo silently meaning "no waiver" stops the run for a reason its author
// believes they addressed, with nothing saying the flag was ignored.
func TestCriteriaAuthority_GateRejectsAnUnknownFlagValue(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	gateAuthorizeCriteria = "yes"
	defer func() { gateAuthorizeCriteria = "" }()

	if err := runGateCmd_(t, "@graded", gateStageCode); err == nil {
		t.Error("an unknown mode must be refused, not treated as absent")
	}
}

// writePassingTestcases writes a current-shape file discharging every criterion
// the fixture declares, in the real step vocabulary — action/target for steps,
// verify/target for expectations — so a test about authorization is not also a
// test about coverage.
func writePassingTestcases(t *testing.T, cfg *config.Context, slug string) {
	t.Helper()
	buildDir := cfg.BuildPath(slug)
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `schema_version: 3
feature: graded
suites:
  - name: Customer Detail
    kind: presentation
    component: customer-detail
    source_refs:
      - "@graded/fragment:Customer Detail"
    file: src/CustomerDetail.test.tsx
    cases:
      - name: disabled while unpaid
        criterion:
          ref: "@graded/fragment:Customer Detail"
          text: the archive button is disabled while invoices are unpaid
        exercises: ["@graded/fragment:Customer Detail"]
        observes: ["@graded/fragment:Customer Detail"]
        coverage: full
        steps:
          - action: render
            target: "@graded/fragment:Customer Detail"
          - verify: element
            target: "@graded/fragment:Customer Detail"
            expected: disabled
  - name: Archive
    kind: operation
    operation: "@graded/operation:customer.archive"
    source_refs:
      - "@graded/operation:customer.archive"
    file: test/archive.spec.ts
    cases:
      - name: rejects unpaid
        criterion:
          ref: "@graded/operation:customer.archive"
          text: archiving a customer with unpaid invoices is rejected
        exercises: ["@graded/operation:customer.archive"]
        observes: ["@graded/operation:customer.archive"]
        coverage: full
        steps:
          - action: click
            target: "@graded/operation:customer.archive"
          - verify: state
            target: "@graded/operation:customer.archive"
            expected: rejected
`
	if err := os.WriteFile(filepath.Join(buildDir, "testcases.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Either switch alone refuses, at the boundary this time.
func TestCriteriaAuthority_GateNeedsBothSwitches(t *testing.T) {
	t.Run("flag without the project opt-in", func(t *testing.T) {
		dir := setupTestDir(t)
		writeCriteriaFixture(t, dir)
		gateAuthorizeCriteria = machineAuthorizationMode
		defer func() { gateAuthorizeCriteria = "" }()

		out, _ := computeGate(testContext(t), "graded", gateStageCode)
		if !gateHasCode(out.Blockers, "criteria-authority-missing") {
			t.Error("a flag alone must not waive the separation")
		}
	})

	t.Run("opt-in without the flag", func(t *testing.T) {
		dir := setupTestDir(t)
		writeCriteriaFixture(t, dir)
		allowMachineAuthority(t, dir)

		out, _ := computeGate(testContext(t), "graded", gateStageCode)
		if !gateHasCode(out.Blockers, "criteria-authority-missing") {
			t.Error("an opt-in alone must not make every run self-authorizing")
		}
	})
}

// A waiver authorizes ITS run and no later one. Otherwise one CI escape answers
// the question permanently for everyone after it.
func TestCriteriaAuthority_ALaterUnflaggedRunStillStops(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	allowMachineAuthority(t, dir)
	cfg := testContext(t)

	gateAuthorizeCriteria = machineAuthorizationMode
	if out, _ := computeGate(cfg, "graded", gateStageCode); gateHasCode(out.Blockers, "criteria-authority-missing") {
		t.Fatalf("the authorized run should advance: %+v", out.Blockers)
	}
	gateAuthorizeCriteria = ""

	out, _ := computeGate(cfg, "graded", gateStageCode)
	if !gateHasCode(out.Blockers, "criteria-authority-missing") {
		t.Error("the next run did not ask, so it must stop — an audit event is not standing authority")
	}
}

func TestCriteriaAuthorityCLI_ApprovalRequiresAnIdentity(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)

	approveCriteriaBy = ""
	var buf bytes.Buffer
	cmd := testCommandWithContext(t, testContext(t))
	cmd.SetOut(&buf)
	if err := runApproveCriteria(cmd, []string{"@graded"}); err == nil {
		t.Error("an approval that cannot say what accepted the standard is the forgery this record exists to avoid")
	}

	approveCriteriaBy = "interactive decision"
	defer func() { approveCriteriaBy = "" }()
	if err := runApproveCriteria(cmd, []string{"@graded"}); err != nil {
		t.Fatalf("with an identity it should record: %v", err)
	}
	rec, _ := loadCriteriaAuthority(testContext(t), "graded")
	if rec == nil || rec.Approved == nil || rec.Approved.Authority != "interactive decision" {
		t.Errorf("the identity must be stored as given, not derived: %+v", rec)
	}
}

func TestCriteriaAuthorityCLI_RejectsAnUnknownMode(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	authorizeCriteriaMode = "yes"
	defer func() { authorizeCriteriaMode = "" }()

	if _, err := runCriteriaAuthorityCmd_(t, "@graded"); err == nil {
		t.Error("the mode is a closed value; a typo must not silently mean no waiver")
	}
}

// The mechanical half must actually block. It was graduated to error and
// nothing ran it in build mode: validate --type testcases hardcodes authoring,
// and no boundary called the walkers at all, so the middle was advisory on
// every path that mattered.
func TestTestcasesReadiness_UncoveredCriterionBlocksTheGate(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)

	// A current-shape file whose one suite discharges nothing.
	buildDir := cfg.BuildPath("graded")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "testcases.yaml"), []byte(`schema_version: 3
feature: graded
suites:
  - name: Customer Detail
    kind: presentation
    source_refs:
      - "@graded/fragment:Customer Detail"
    file: src/CustomerDetail.test.tsx
    cases:
      - name: renders
        exercises: ["@graded/fragment:Customer Detail"]
        observes: ["@graded/fragment:Customer Detail"]
        steps:
          - { type: render, target: "@graded/fragment:Customer Detail" }
        expectations:
          - { type: shows, target: "@graded/fragment:Customer Detail" }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	var graduated bool
	for _, b := range out.Blockers {
		if b.Code == "testcases-not-ready" && strings.Contains(b.Message, "[verify-criterion-uncovered]") {
			graduated = true
		}
	}
	if !graduated {
		t.Fatalf("a current-shape file leaving a criterion undischarged must block the boundary: %+v", out.Blockers)
	}
}

// The same file below the graduation version keeps its warning: that is why
// these were warnings at all, and the rule must not fail projects over a fact
// they could not have recorded.
func TestTestcasesReadiness_LegacyShapeDoesNotBlock(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)

	buildDir := cfg.BuildPath("graded")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "testcases.yaml"), []byte(`schema_version: 2
feature: graded
suites:
  - name: Customer Detail
    kind: presentation
    source_refs:
      - "@graded/fragment:Customer Detail"
    file: src/CustomerDetail.test.tsx
    cases:
      - name: renders
        exercises: ["@graded/fragment:Customer Detail"]
        observes: ["@graded/fragment:Customer Detail"]
        steps:
          - { type: render, target: "@graded/fragment:Customer Detail" }
        expectations:
          - { type: shows, target: "@graded/fragment:Customer Detail" }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ := computeGate(cfg, "graded", gateStageCode)
	// Other codes are errors at any version and are not what this is about;
	// the claim is that the GRADUATED ones do not fire on a legacy shape.
	for _, b := range out.Blockers {
		if b.Code != "testcases-not-ready" {
			continue
		}
		for code := range agent.GraduatingCodes() {
			if strings.Contains(b.Message, "["+code+"]") {
				t.Errorf("%s must not block a file that predates the field it checks: %s", code, b.Message)
			}
		}
	}
}

// Absence is judged against the SUBJECT. An earlier version returned success
// for a missing testcases.yaml on the belief that the buildfile checks report
// it — they do not, and the code that emits testcases-not-found is the gate
// being removed. So a feature with criteria, a valid buildfile and no tests at
// all would have passed.
func TestTestcasesReadiness_MissingTestcasesBlocksAFeatureWithCriteria(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)

	out, _ := computeGate(testContext(t), "graded", gateStageCode)
	var msg string
	for _, b := range out.Blockers {
		if b.Code == "testcases-not-ready" {
			msg = b.Message
		}
	}
	if msg == "" {
		t.Fatalf("two criteria and nothing discharging them: %+v", out.Blockers)
	}
	if !strings.Contains(msg, "no testcases.yaml") {
		t.Errorf("say what is missing: %q", msg)
	}
}

// A genuinely criterion-free feature may legitimately have none.
func TestTestcasesReadiness_MissingTestcasesIsFineWithNoCriteria(t *testing.T) {
	dir := setupTestDir(t)
	featDir := filepath.Join(dir, "spec", "intents", "bare-contract")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"intents.md": "# Bare\n\n## Do It\n\n**Goal**: g.\n**Persona**: p.\n",
		"surface.yaml": `feature: bare-contract
fragments:
    - name: Thing
      page: things
      region: main
      shows: detail
      order: 1
      source: '@bare-contract/do-it'
`,
	} {
		if err := os.WriteFile(filepath.Join(featDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	r := CheckTestcasesReadiness(testContext(t), "bare-contract")
	if len(r.Blockers) != 0 {
		t.Errorf("a feature declaring no criteria needs nothing to discharge them: %+v", r.Blockers)
	}
}

// writeCleanCodeBoundary builds a feature that clears the ENTIRE code boundary.
//
// It exists because a mutation test without a clean control proves nothing: if
// the unmutated fixture already blocks, a blocker in the mutated one is not
// evidence the mutation caused it. Being unable to construct this was itself
// diagnostic — the transactional property had been tested against a verdict
// assembled by hand rather than one production produces.
func writeCleanCodeBoundary(t *testing.T, dir string) *config.Context {
	t.Helper()
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	writePassingTestcases(t, cfg, "graded")

	adapters := cfg.AdaptersPath()
	if err := os.MkdirAll(adapters, 0o755); err != nil {
		t.Fatal(err)
	}
	adapterPath := filepath.Join(adapters, "react-antd.adapter.yaml")
	if err := os.WriteFile(adapterPath, []byte("name: react-antd\nkind: presentation\nfile-conventions:\n  project-root: \".\"\n  source-root: src/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sigs, err := computeSourceSignatures(cfg.FeaturePath("graded"), cfg.Root.Path, adapterPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	sigYaml := ""
	for k, v := range sigs {
		sigYaml += "  " + k + ": " + v + "\n"
	}
	bd := cfg.BuildPath("graded")
	if err := os.MkdirAll(bd, 0o755); err != nil {
		t.Fatal(err)
	}
	// A feature that has reached the code boundary has been built, so it has a
	// baseline. Without one the drift walk has nothing to compare against and
	// the ledger claim silently contributes nothing — which made the fixture
	// unrealistic in exactly the direction that hides a fail-open.
	if err := saveBuildStateForFeature(cfg, "graded", cfg.Root.Path); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(bd, "buildfile.yaml"), []byte(`schema_version: 1
feature: graded
adapter: react-antd
components:
  customer-detail:
    kind: component
    source: "@graded/fragment:Customer Detail"
plan:
  creates:
    - path: src/features/graded/CustomerDetail.tsx
      sources: ["component/customer-detail"]
source-signatures:
`+sigYaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// The control itself: without it every mutation below would be arguing from a
// fixture that was already blocked.
func TestCleanCodeBoundary_Passes(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	current, _ := CurrentCriteria(cfg, "graded")
	if err := RecordHumanApproval(cfg, "graded", current, "2026-08-27T00:00:00Z", "interactive decision", "dec-1"); err != nil {
		t.Fatal(err)
	}

	out, err := computeGate(cfg, "graded", gateStageCode)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Passed {
		t.Fatalf("the clean control must pass or nothing below proves anything: %+v", out.Blockers)
	}
}

// The machine path, end to end through the advancing command against a
// boundary that genuinely passes.
func TestMachineAuthorization_CleanRunRecordsExactlyOneWaiver(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	allowMachineAuthority(t, dir)

	gateAuthorizeCriteria = machineAuthorizationMode
	defer func() { gateAuthorizeCriteria = "" }()

	if err := runGateCmd_(t, "@graded", gateStageCode); err != nil {
		t.Fatalf("both switches set and the boundary otherwise clean: %v", err)
	}
	rec, _ := loadCriteriaAuthority(cfg, "graded")
	if rec == nil || len(rec.MachineRuns) != 1 {
		t.Fatalf("exactly one waiver for one advancing run: %+v", rec)
	}
	if rec.Approved != nil {
		t.Error("a waiver must never become approval")
	}

	// A later run that does not ask still stops: an audit event is not
	// standing authority.
	gateAuthorizeCriteria = ""
	out, _ := computeGate(cfg, "graded", gateStageCode)
	if out.Passed {
		t.Error("the next run did not ask for the waiver, so it must stop")
	}
}

// The mutation that matters, now provable: an unrelated blocker on an otherwise
// authorized run must leave no audit trail claiming it advanced.
func TestMachineAuthorization_UnrelatedBlockerLeavesNoAudit(t *testing.T) {
	dir := setupTestDir(t)
	cfg := writeCleanCodeBoundary(t, dir)
	allowMachineAuthority(t, dir)

	// Mutate exactly one thing, and verify the mutation actually blocks before
	// concluding anything from the absence of an audit record — otherwise this
	// test passes when the mutation silently did nothing.
	if err := os.Remove(filepath.Join(cfg.BuildPath("graded"), "testcases.yaml")); err != nil {
		t.Fatal(err)
	}
	if out, _ := computeGate(cfg, "graded", gateStageCode); out.Passed {
		t.Fatal("the mutation did not block, so this test would prove nothing")
	}

	gateAuthorizeCriteria = machineAuthorizationMode
	defer func() { gateAuthorizeCriteria = "" }()

	_ = runGateCmd_(t, "@graded", gateStageCode)
	rec, _ := loadCriteriaAuthority(cfg, "graded")
	if rec != nil && len(rec.MachineRuns) > 0 {
		t.Errorf("the boundary refused, so the audit trail must not say a run advanced: %+v", rec.MachineRuns)
	}
}

// The defect: `gate done --authorize-criteria=machine` invoked directly, with
// no code boundary ever crossed, consumed the waiver and passed while writing
// nothing. "Done inherits the code run's authorization" is only true when code
// actually ran; when it did not, done inherited from nothing and the run that
// most needed an audit trail left none.
func TestCriteriaAuthority_DirectDoneRecordsItsOwnWaiver(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	current, err := CurrentCriteria(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}

	// Done is the first and only boundary crossed. Nothing precedes it.
	if err := commitPendingWaiver(cfg, "graded", gateStageDone,
		gateOutput{Passed: true, PendingWaiver: &pendingMachineRun{criteria: current, reason: "authorized"}}); err != nil {
		t.Fatal(err)
	}
	rec, _ := loadCriteriaAuthority(cfg, "graded")
	if rec == nil || len(rec.MachineRuns) != 1 {
		t.Fatalf("a directly invoked done boundary that consumed a machine waiver must record it; got %+v", rec)
	}
	if !strings.Contains(rec.MachineRuns[0].Reason, gateStageDone) {
		t.Errorf("the record must say which boundary consumed the waiver, so a reader can tell a direct done from an ordinary loop; got %q", rec.MachineRuns[0].Reason)
	}
	if rec.Approved != nil {
		t.Error("a waiver must never become approval")
	}

	// Crossing done again inside the SAME execution must not log twice.
	if err := commitPendingWaiver(cfg, "graded", gateStageDone,
		gateOutput{Passed: true, PendingWaiver: &pendingMachineRun{criteria: current, reason: "authorized"}}); err != nil {
		t.Fatal(err)
	}
	rec, _ = loadCriteriaAuthority(cfg, "graded")
	if len(rec.MachineRuns) != 1 {
		t.Errorf("one execution authorizing one standard must read as one run, not two: %+v", rec.MachineRuns)
	}
}

// The correction to the above. An earlier version inherited on criteria hash
// alone, so an unchanged standard machine-run once made every later direct done
// crossing inherit from it and write nothing. A hash proves what was waived,
// not which execution waived it — the same distinction
// APastMachineRunDoesNotAuthorizeALaterOne holds for authority, applied to the
// audit trail.
func TestCriteriaAuthority_ALaterExecutionDoesNotInheritAnEarlierAudit(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	current, err := CurrentCriteria(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}

	// Monday's pipeline, machine-authorized and recorded.
	t.Setenv("PARLAY_RUN_ID", "mondays-pipeline")
	if err := commitPendingWaiver(cfg, "graded", gateStageCode,
		gateOutput{Passed: true, PendingWaiver: &pendingMachineRun{criteria: current, reason: "authorized"}}); err != nil {
		t.Fatal(err)
	}

	// Friday: a direct done crossing, same unchanged standard, different
	// execution. It consumed its own waiver and owes its own record.
	t.Setenv("PARLAY_RUN_ID", "fridays-direct-done")
	if err := commitPendingWaiver(cfg, "graded", gateStageDone,
		gateOutput{Passed: true, PendingWaiver: &pendingMachineRun{criteria: current, reason: "authorized"}}); err != nil {
		t.Fatal(err)
	}

	rec, _ := loadCriteriaAuthority(cfg, "graded")
	if len(rec.MachineRuns) != 2 {
		t.Fatalf("a later execution must not inherit an earlier execution's audit event; got %+v", rec.MachineRuns)
	}
	if rec.MachineRuns[1].RunID == rec.MachineRuns[0].RunID {
		t.Errorf("the two events must be distinguishable by execution: %+v", rec.MachineRuns)
	}
}

// Inside one pipeline the carrier is shared, so code and done log once between
// them. This is the shape a CI job or a loop that sets PARLAY_RUN_ID gets.
func TestCriteriaAuthority_OnePipelineLogsOneEventAcrossBothBoundaries(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	current, err := CurrentCriteria(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PARLAY_RUN_ID", "one-pipeline")
	for _, stage := range []string{gateStageCode, gateStageDone} {
		if err := commitPendingWaiver(cfg, "graded", stage,
			gateOutput{Passed: true, PendingWaiver: &pendingMachineRun{criteria: current, reason: "authorized"}}); err != nil {
			t.Fatal(err)
		}
	}
	rec, _ := loadCriteriaAuthority(cfg, "graded")
	if len(rec.MachineRuns) != 1 {
		t.Fatalf("one pipeline crossing both boundaries must log one event, not two: %+v", rec.MachineRuns)
	}
}

// A standard that changed between code and done is not the standard code
// authorized, so done must record rather than inherit.
func TestCriteriaAuthority_DoneRecordsWhenTheStandardChanged(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	current, err := CurrentCriteria(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}
	if err := commitPendingWaiver(cfg, "graded", gateStageCode,
		gateOutput{Passed: true, PendingWaiver: &pendingMachineRun{criteria: current, reason: "authorized"}}); err != nil {
		t.Fatal(err)
	}

	changed := append([]AuthorizedCriterion{}, current...)
	changed = append(changed, AuthorizedCriterion{Ref: current[0].Ref, Text: "a criterion nobody authorized"})
	if err := commitPendingWaiver(cfg, "graded", gateStageDone,
		gateOutput{Passed: true, PendingWaiver: &pendingMachineRun{criteria: changed, reason: "authorized"}}); err != nil {
		t.Fatal(err)
	}
	rec, _ := loadCriteriaAuthority(cfg, "graded")
	if len(rec.MachineRuns) != 2 {
		t.Fatalf("done ran against a different standard than code authorized, so it must record its own: %+v", rec.MachineRuns)
	}
}

// A capabilities file that exists but cannot be read must not read as one that
// is absent. Skipping it returns a standard SHORT of the criteria it should
// carry, and that understated standard is what then gets approved, hashed and
// graded against — so a machine that cannot read the file passes a boundary the
// same file would have failed.
func TestCurrentCriteria_UnreadableCapabilitiesIsNotAnAbsentOne(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)

	before, err := CurrentCriteria(cfg, "graded")
	if err != nil || len(before) == 0 {
		t.Fatalf("fixture must declare criteria for this to witness anything: %d, %v", len(before), err)
	}

	// A real stat failure that is not NOT-EXIST: the parent directory is made
	// unsearchable, so stat on the child returns EACCES. A directory in the
	// file's place would not witness this — stat succeeds on a directory and
	// the failure lands in the parse branch, which already failed closed.
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits do not produce a stat failure")
	}
	featDir := cfg.FeaturePath("graded")
	if err := os.Chmod(featDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(featDir, 0o755) })

	got, err := CurrentCriteria(cfg, "graded")
	if err == nil {
		t.Fatalf("an unreadable capabilities artifact must fail closed; got %d criteria and no error", len(got))
	}
	if len(got) != 0 {
		t.Errorf("nothing may be returned alongside the failure; got %d criteria", len(got))
	}
}

// The gate reads a machine run's stored hash to decide whether THIS execution
// already recorded its waiver. Nothing checked that the hash described the
// criteria stored beside it, so a forged or corrupt entry could satisfy that
// check — and the boundary would skip writing the audit record for a run that
// proceeded without human approval. The one entry whose absence nobody notices
// is the one that most needed to exist.
func TestCriteriaAuthority_AMachineRunMustDescribeItsOwnCriteria(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)
	current, err := CurrentCriteria(cfg, "graded")
	if err != nil {
		t.Fatal(err)
	}

	// A genuine run first, so the fixture is one a forged entry could hide in.
	if err := commitPendingWaiver(cfg, "graded", gateStageCode,
		gateOutput{Passed: true, PendingWaiver: &pendingMachineRun{criteria: current, reason: "authorized"}}); err != nil {
		t.Fatal(err)
	}
	rec, err := loadCriteriaAuthority(cfg, "graded")
	if err != nil || len(rec.MachineRuns) != 1 {
		t.Fatalf("fixture must hold one genuine run: %v %+v", err, rec)
	}

	for _, tc := range []struct {
		name, want string
		damage     func(*MachineRun)
	}{
		{"hash does not match its criteria", "edited by hand or is corrupt",
			func(r *MachineRun) {
				r.CriteriaHash = CriteriaHash([]AuthorizedCriterion{{Ref: "@x/operation:y", Text: "something else"}})
			}},
		{"criteria replaced under a kept hash", "edited by hand or is corrupt",
			func(r *MachineRun) {
				r.Criteria = []AuthorizedCriterion{{Ref: "@x/operation:y", Text: "something else"}}
			}},
		{"no policy source", "names no policy source",
			func(r *MachineRun) { r.PolicySource = "" }},
		{"no time", "records no time",
			func(r *MachineRun) { r.At = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			damaged := *rec
			run := rec.MachineRuns[0]
			tc.damage(&run)
			damaged.MachineRuns = []MachineRun{run}
			if err := saveCriteriaAuthority(cfg, "graded", &damaged); err != nil {
				t.Fatal(err)
			}
			_, err := loadCriteriaAuthority(cfg, "graded")
			if err == nil {
				t.Fatal("a machine run that does not describe itself must be refused, not read")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q; got: %v", tc.want, err)
			}
		})
	}
}
