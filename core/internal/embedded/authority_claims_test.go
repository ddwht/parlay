package embedded

import (
	"regexp"
	"strings"
	"testing"
)

// Deployed guidance must not claim the tool proves something it cannot.
//
// The CLI checks identity, freshness and shape: that an occurrence exists, is
// the one that was shown, has not moved underneath the decision, and came with
// an outcome and a reason. It cannot check cognition or authorship — `--reason`
// takes any non-empty string, and `--by` is asserted attribution that nothing
// verifies.
//
// Overclaiming here is worse than saying nothing. Someone auditing a project
// later reads these ledgers as evidence a person decided; if our own guidance
// told them the tool guarantees that, the overclaim becomes the thing they
// trusted. The obligation belongs on the agent following the workflow, stated
// as an obligation.
var overclaims = []*regexp.Regexp{
	regexp.MustCompile(`(?i)guarantees?\s+(that\s+)?(a\s+)?(person|human|reviewer)`),
	regexp.MustCompile(`(?i)verified\s+(reviewer|identity|author)`),
	regexp.MustCompile(`(?i)prove[sd]?\s+(that\s+)?(a\s+)?(person|human)\s+(reviewed|decided|looked)`),
	regexp.MustCompile(`(?i)ensures?\s+(that\s+)?(a\s+)?(person|human)\s+(reviewed|decided)`),
	regexp.MustCompile(`(?i)cannot be (faked|forged|bypassed) by an agent`),
}

// negatedBefore reports whether the text running up to a match denies it.
func negatedBefore(prefix string) bool {
	lower := strings.ToLower(strings.TrimSpace(prefix))
	// Only the tail matters: negation binds to what follows it closely.
	if len(lower) > 60 {
		lower = lower[len(lower)-60:]
	}
	for _, deny := range []string{"not", "never", "cannot", "no", "without", "isn't", "does not"} {
		if strings.HasSuffix(lower, " "+deny) || lower == deny {
			return true
		}
	}
	return false
}

func TestGuidanceDoesNotOverclaimHumanAuthority(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) == 0 {
		t.Fatal("no skills read — this check would pass over nothing")
	}

	checked := 0
	for _, s := range skills {
		body := string(s.Content)
		for _, line := range strings.Split(body, "\n") {
			for _, re := range overclaims {
				loc := re.FindStringIndex(line)
				if loc == nil {
					continue
				}
				// A denial of the claim is the correction, not the claim.
				// "`--by` is asserted attribution, not verified identity" must
				// stay sayable — and the negation has to be checked against the
				// text immediately BEFORE the match, not anywhere on the line,
				// or a line that both denies one claim and makes another slips
				// through.
				if negatedBefore(line[:loc[0]]) {
					continue
				}
				{
					t.Errorf("skill %s claims the tool establishes human authority it cannot check:\n  %s\n"+
						"State it as an obligation on whoever follows the workflow. The CLI proves identity, "+
						"freshness and shape — not cognition or authorship.", s.Name, strings.TrimSpace(line))
				}
			}
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("nothing was checked")
	}
}

// And the workflow that acquires judgments must state the boundary, not leave a
// reader to infer it.
func TestMigrationWalkthroughStatesItsTrustBoundary(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatal(err)
	}
	var body string
	for _, s := range skills {
		if s.Name == "migrate-coverage" {
			body = string(s.Content)
		}
	}
	if body == "" {
		t.Fatal("the migrate-coverage walkthrough is not deployed")
	}
	for _, want := range []string{
		"asserted attribution",
		"not verified identity",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the walkthrough must say %q — a reader who thinks `by` is verified will trust the ledger further than it can carry", want)
		}
	}
	if !strings.Contains(body, "obligation on you, not a guarantee") {
		t.Error("the governing rule must be marked as an obligation rather than an enforced property")
	}
}
