// parlay-feature: parlay-tool/criterion-authority
// parlay-component: criteria-authority-record
//
// Who approved the standard this feature is graded against?
//
// The founding coverage-review intent asked for a real thing: stop an agent
// authoring the operation, authoring the tests, and grading its own homework in
// one pass. The mechanism it chose could not deliver it — the prompt shows a
// suite NAME and defaults empty input to yes, and the reviewer it records comes
// from $USER, which is why five of nine real review files in this repo name a
// background process as the reviewer.
//
// The judgment it should have asked about happens earlier and is never shown.
// create-artifacts takes each intent Verify bullet, splits it into independently
// testable claims, and REWRITES any sentence carrying both a presentation and a
// contract claim into separate criteria on separate destinations. That is lossy
// semantic transformation, it happens after the designer's only choice (which
// artifact SET to write), and the phase reports artifact names rather than the
// criteria it produced. So the standard is rewritten after the last human look
// and never presented.
//
// This record fixes the subject rather than removing the stop: a person approves
// the criterion set itself, bound to its hash. Testcases may then regenerate
// freely — the old gate's worst friction, one full re-approval per regeneration
// — because what was approved is the standard, not the suites derived from it.

package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
)

const criteriaAuthoritySchemaVersion = 1

// AuthorizedCriterion is one approved (ref, text) pair, stored so a later run
// can show what CHANGED rather than only that something did.
type AuthorizedCriterion struct {
	Ref  string `yaml:"ref"`
	Text string `yaml:"text"`
}

// MachineRun is an audit event: a run that proceeded without human judgment.
//
// Kept separate from Approved, and never written into it, because they are not
// two flavours of the same thing. Human approval means a person read the
// mapping and accepted it. A machine run means the project chose to execute
// without that judgment — the separation the founding intent asked for is
// knowingly NOT provided for that run, and a record that blurred the two would
// be the same forgery the old artifact committed with $USER.
type MachineRun struct {
	At           string `yaml:"at"`
	CriteriaHash string `yaml:"criteria_hash"`
	Reason       string `yaml:"reason"`
}

// CriteriaAuthority is the per-feature record of who approved the standard.
type CriteriaAuthority struct {
	SchemaVersion int    `yaml:"schema_version"`
	Feature       string `yaml:"feature"`

	// Approved is durable HUMAN authority, or nil when nobody has approved
	// this feature's criteria. Only a person writes it.
	Approved *HumanApproval `yaml:"approved,omitempty"`

	// MachineRuns is an append-only audit trail. It never satisfies a later
	// run on its own: one CI escape must not permanently suppress the question
	// for everyone afterwards.
	MachineRuns []MachineRun `yaml:"machine_runs,omitempty"`
}

// HumanApproval records a person accepting a specific criterion set.
type HumanApproval struct {
	At           string                `yaml:"at"`
	CriteriaHash string                `yaml:"criteria_hash"`
	Criteria     []AuthorizedCriterion `yaml:"criteria"`
}

func criteriaAuthorityPath(cfg *config.Context, slug string) string {
	return filepath.Join(cfg.BuildPath(slug), "criteria-authority.yaml")
}

// CriteriaHash fingerprints a criterion set by identity, not by file bytes.
//
// Over the sorted (ref, text) pairs with text canonicalized the same way the
// criterion walker canonicalizes it, so the hash tracks what the criteria SAY
// and where they live. Reformatting an artifact, reordering fragments, or
// editing anything that is not a criterion does not invalidate an approval —
// which is the difference between asking a person once and asking them on every
// regeneration.
func CriteriaHash(criteria []AuthorizedCriterion) string {
	lines := make([]string, 0, len(criteria))
	for _, c := range criteria {
		lines = append(lines, c.Ref+"\x00"+agent.CanonicalCriterionText(c.Text))
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func loadCriteriaAuthority(cfg *config.Context, slug string) (*CriteriaAuthority, error) {
	data, err := os.ReadFile(criteriaAuthorityPath(cfg, slug))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rec CriteriaAuthority
	if err := yaml.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("invalid criteria-authority file: %w", err)
	}
	return &rec, nil
}

func saveCriteriaAuthority(cfg *config.Context, slug string, rec *CriteriaAuthority) error {
	rec.SchemaVersion = criteriaAuthoritySchemaVersion
	rec.Feature = slug
	data, err := yaml.Marshal(rec)
	if err != nil {
		return err
	}
	path := criteriaAuthorityPath(cfg, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// AuthorityVerdict answers whether a run may proceed, and why not when it may
// not.
type AuthorityVerdict struct {
	// Proceed is true when this run is authorized.
	Proceed bool
	// Machine is true when it is authorized only because this invocation
	// carried the machine flag — the separation guarantee is waived, not met.
	Machine bool
	// Reason explains a refusal, or names the waiver when Machine is set.
	Reason string
	// Added and Removed describe how the current criteria differ from what was
	// approved, so a stale record can show the change rather than announce it.
	Added, Removed []AuthorizedCriterion
}

// EvaluateCriteriaAuthority applies the gate rules.
//
//	matching human approval             -> proceed everywhere
//	project opt-in AND invocation flag  -> proceed, and record the waiver
//	one without the other               -> refuse, naming the missing half
//	past machine run, no flag now       -> refuse; one CI escape must not
//	                                       permanently answer for everyone
//	criteria changed                    -> refuse, showing what changed
func EvaluateCriteriaAuthority(rec *CriteriaAuthority, current []AuthorizedCriterion, machineFlag, policyAllows bool) AuthorityVerdict {
	hash := CriteriaHash(current)

	if rec != nil && rec.Approved != nil && rec.Approved.CriteriaHash == hash {
		return AuthorityVerdict{Proceed: true, Reason: "criteria approved by a person"}
	}

	// Both switches, or neither counts. The project must have opted in — a
	// committed, reviewable decision that this project may ever waive the
	// separation — AND this invocation must exercise it. Either alone is
	// refused, with a message naming which half is missing so the remedy is
	// obvious rather than guessed at.
	if machineFlag && !policyAllows {
		return AuthorityVerdict{
			Reason: "this run asked to proceed without human approval, but the project has not opted in — " +
				"set parlay.criterion-authority.allow-machine: true in .parlay/config.yaml if this project may waive " +
				"the separation between authoring a standard and grading against it. It is a committed decision on purpose",
		}
	}
	if policyAllows && !machineFlag && (rec == nil || rec.Approved == nil) {
		return AuthorityVerdict{
			Reason: unapprovedReason(rec) + "; this project permits machine authorization but this run did not ask " +
				"for it — pass --authorize-criteria=machine to exercise that permission",
		}
	}
	if machineFlag && policyAllows {
		return AuthorityVerdict{
			Proceed: true, Machine: true,
			Reason: "proceeding without human approval: the project permits it and this run asked for it. " +
				"The separation between authoring the standard and grading against it is WAIVED for this run, not satisfied",
		}
	}

	if rec == nil || rec.Approved == nil {
		return AuthorityVerdict{Reason: unapprovedReason(rec)}
	}

	added, removed := diffCriteria(rec.Approved.Criteria, current)
	return AuthorityVerdict{
		Reason:  "the criteria changed since they were approved",
		Added:   added,
		Removed: removed,
	}
}

func diffCriteria(approved, current []AuthorizedCriterion) (added, removed []AuthorizedCriterion) {
	key := func(c AuthorizedCriterion) string {
		return c.Ref + "\x00" + agent.CanonicalCriterionText(c.Text)
	}
	was := map[string]AuthorizedCriterion{}
	for _, c := range approved {
		was[key(c)] = c
	}
	now := map[string]AuthorizedCriterion{}
	for _, c := range current {
		now[key(c)] = c
		if _, ok := was[key(c)]; !ok {
			added = append(added, c)
		}
	}
	for k, c := range was {
		if _, ok := now[k]; !ok {
			removed = append(removed, c)
		}
	}
	sortCriteria(added)
	sortCriteria(removed)
	return added, removed
}

func sortCriteria(cs []AuthorizedCriterion) {
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].Ref != cs[j].Ref {
			return cs[i].Ref < cs[j].Ref
		}
		return cs[i].Text < cs[j].Text
	})
}

// unapprovedReason states that nobody approved, and says so in the same words
// whether or not earlier runs proceeded anyway.
//
// A prior machine run is named rather than omitted, because the difference
// between "nobody has looked at this yet" and "runs have been proceeding
// without anyone looking" is exactly what a reader needs and exactly what the
// old artifact could not express. It is deliberately NOT authority: letting an
// audit event answer here is how one unattended escape silently removes the
// question for everyone after it.
func unapprovedReason(rec *CriteriaAuthority) string {
	reason := "nobody has approved the criteria this feature is graded against"
	if rec != nil && len(rec.MachineRuns) > 0 {
		reason += fmt.Sprintf(" — %d earlier run(s) proceeded under machine authorization, which records that nobody looked rather than that anybody did", len(rec.MachineRuns))
	}
	return reason
}
