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

// The audit event is written only when a run actually proceeds. A record on a
// refused run would say a waiver happened when nothing did.
func TestCriteriaAuthorityCLI_MachineRunIsRecordedOnlyWhenItProceeds(t *testing.T) {
	dir := setupTestDir(t)
	writeCriteriaFixture(t, dir)
	cfg := testContext(t)

	// Refused: policy has not opted in.
	authorizeCriteriaMode = machineAuthorizationMode
	defer func() { authorizeCriteriaMode = "" }()
	if out, err := runCriteriaAuthorityCmd_(t, "@graded"); err == nil || out.Authorized {
		t.Fatal("a flag without the project opt-in must refuse")
	}
	if rec, _ := loadCriteriaAuthority(cfg, "graded"); rec != nil && len(rec.MachineRuns) > 0 {
		t.Fatal("nothing proceeded, so nothing should have been recorded")
	}

	// Permitted: both switches present.
	allowMachineAuthority(t, dir)
	out, err := runCriteriaAuthorityCmd_(t, "@graded")
	if err != nil || !out.Authorized || !out.Machine {
		t.Fatalf("project permits it and the run asked: %+v (%v)", out, err)
	}
	rec, loadErr := loadCriteriaAuthority(cfg, "graded")
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if rec == nil || len(rec.MachineRuns) != 1 {
		t.Fatalf("the waiver should be recorded exactly once: %+v", rec)
	}
	ev := rec.MachineRuns[0]
	if ev.PolicySource == "" || ev.RunID == "" || len(ev.Criteria) != 2 {
		t.Errorf("an audit event needs the policy that permitted it, the run that did it, and what it graded against: %+v", ev)
	}
	if rec.Approved != nil {
		t.Error("a machine run must never be written as human approval")
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
