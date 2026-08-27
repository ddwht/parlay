// parlay-feature: parlay-tool/criterion-authority
// parlay-component: retired-artifact-lint
// parlay-artifact: test

package embedded

import (
	"os"
	"strings"
	"testing"
)

// retiredArtifacts are artifacts the tool no longer reads or writes.
//
// Deleting an implementation does not delete the guidance describing it, and
// deployed skills and schemas are what an agent actually follows. This release
// removed the coverage-review gate and left four live documents still telling
// an agent to consult it — a codegen read-set permitting the file, a freshness
// rule claiming to ride its hashes, and two tables describing its exemptions as
// the mechanism. Each would have sent an agent to a file nothing produces.
var retiredArtifacts = []string{
	"coverage-review.yaml",
	// The schema document, not just the artifact: a versioning table still
	// used it as a live example of an artifact needing no schema_version, and
	// matching only the .yaml spelling walked straight past it.
	"coverage-review.schema.md",
	"parlay review-coverage",
	"parlay-review-coverage",
	"check-review-gate",
}

// retirementExplanations may name a retired artifact, because saying what
// something WAS is how a reader understands why the current shape exists.
//
// Narrow on purpose: a line explaining the retirement, or provenance naming the
// founding component, is history. A line telling an agent to read the file is
// not, and the difference is whether it appears in an instruction or in an
// explanation.
var retirementExplanations = []string{
	"parlay-extends:",               // provenance names the founding component
	"retired",                       // an explicit retirement note
	"no longer",                     // "codegen no longer consults..."
	"That gate is retired",          //
	"the retired suite-name gate",   //
	"came from the coverage-review", //
}

// Deployed guidance must not send an agent to an artifact nothing produces.
//
// Frozen spec history is deliberately NOT scanned: intents and dialogs record
// what a feature promised when it was founded, and rewriting them to match the
// present is exactly what the amendment ledger exists to avoid. This covers
// only what is deployed and followed.
// Repo-root documents are checked too. The first version of this lint scanned
// only deployed skills and schemas, and README.md was still advertising the
// removed command while CLAUDE.md — which the deployer WRITES into every
// project — still listed the retired artifact as a current build output. Those
// are the two documents a person is most likely to read first.
func TestRetiredArtifactsAreNotInRepoDocs(t *testing.T) {
	for _, path := range []string{"../../../README.md", "../../../CLAUDE.md"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("%s not readable from here: %v", path, err)
		}
		checkRetired(t, path, string(body))
	}
}

func TestRetiredArtifactsAreNotLiveGuidance(t *testing.T) {
	skills, err := ReadAllSkills()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range skills {
		checkRetired(t, "skill "+s.Name, string(s.Content))
	}

	names, err := SchemaNames()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		body, err := ReadSchema(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		checkRetired(t, "schema "+name, string(body))
	}
}

func checkRetired(t *testing.T, where, body string) {
	t.Helper()
	for i, line := range strings.Split(body, "\n") {
		for _, artifact := range retiredArtifacts {
			if !strings.Contains(line, artifact) {
				continue
			}
			if explainsRetirement(line) {
				continue
			}
			t.Errorf("%s line %d names the retired %q as live guidance:\n    %s\n"+
				"Deleting an implementation does not delete the instructions describing it, and this is what an agent follows.",
				where, i+1, artifact, strings.TrimSpace(line))
		}
	}
}

func explainsRetirement(line string) bool {
	for _, ok := range retirementExplanations {
		if strings.Contains(line, ok) {
			return true
		}
	}
	return false
}
