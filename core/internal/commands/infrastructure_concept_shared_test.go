// parlay-feature: parlay-tool/collision-detection-tier
// parlay-component: infrastructure-concept-shared
// parlay-artifact: test
//
// Tests the infrastructure-concept-shared warning: a concept named in two
// different features' Affects: is a named finding (normalized so casing and
// spacing do not hide the match); a concept only one feature constrains — even
// across two of its own fragments — is not; and validate --project surfaces the
// warning as a line without failing.

package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/config"
)

func writeFeatureInfra(t *testing.T, root, feature, body string) {
	t.Helper()
	dir := filepath.Join(root, config.SpecDir, "intents", feature)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "infrastructure.md"), []byte(body), 0644); err != nil {
		t.Fatalf("write infrastructure.md: %v", err)
	}
}

func TestInfraConceptShared_TwoFeaturesSameConceptWarns(t *testing.T) {
	root := t.TempDir()
	writeFeatureInfra(t, root, "caching", "## Cache\n\n**Affects**: The Response Cache\n**Behavior**: caches for process lifetime\n**Source**: @caching/x\n")
	// Different casing + spacing on the same concept — normalization must
	// still see it as the same concept.
	writeFeatureInfra(t, root, "invalidation", "## Bust\n\n**Affects**: the  response cache\n**Behavior**: never invalidates within a session\n**Source**: @invalidation/y\n")

	errs := sharedInfrastructureConcepts(filepath.Join(root, config.SpecDir))
	if len(errs) != 1 {
		t.Fatalf("expected exactly one infrastructure-concept-shared warning, got %d: %+v", len(errs), errs)
	}
	if errs[0].Code != "infrastructure-concept-shared" {
		t.Fatalf("wrong code: %s", errs[0].Code)
	}
	if !strings.Contains(errs[0].Message, "caching") || !strings.Contains(errs[0].Message, "invalidation") {
		t.Errorf("warning should name both features; got %q", errs[0].Message)
	}
}

func TestInfraConceptShared_SingleFeatureMultipleFragmentsDoesNotWarn(t *testing.T) {
	root := t.TempDir()
	writeFeatureInfra(t, root, "caching",
		"## Cache read\n\n**Affects**: the response cache\n**Behavior**: reads through\n**Source**: @caching/x\n---\n## Cache write\n\n**Affects**: the response cache\n**Behavior**: writes back\n**Source**: @caching/x\n")

	errs := sharedInfrastructureConcepts(filepath.Join(root, config.SpecDir))
	if len(errs) != 0 {
		t.Errorf("one feature constraining a concept across its own fragments must not warn; got %+v", errs)
	}
}

func TestInfraConceptShared_ValidateProjectPrintsLine(t *testing.T) {
	root := t.TempDir()
	writeFeatureInfra(t, root, "caching", "## Cache\n\n**Affects**: the response cache\n**Behavior**: caches\n**Source**: @caching/x\n")
	writeFeatureInfra(t, root, "invalidation", "## Bust\n\n**Affects**: the response cache\n**Behavior**: never invalidates\n**Source**: @invalidation/y\n")

	cfg := config.NewContext(&config.ResolutionResult{
		ActiveRoot: config.Root{Path: root, Kind: config.RootKindStandalone},
	}, nil)
	cmd := testCommandWithContext(t, cfg)
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := runValidateProject(cmd); err != nil {
		t.Fatalf("shared concepts are warnings and must not fail the command: %v", err)
	}
	if !strings.Contains(buf.String(), "infrastructure-concept-shared") {
		t.Errorf("expected the concept-shared line in validate --project output; got %q", buf.String())
	}
}
