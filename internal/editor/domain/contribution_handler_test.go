// parlay-feature: domain-model-editor/feature-contributions
// parlay-component: cross-cutting/contribution-diff-and-apply
// parlay-artifact: test

package domain

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
)

func mountWithContribution(root string, src ContributionSource) http.Handler {
	r := chi.NewRouter()
	s := New(root, nil, src)
	s.validate = cleanValidator
	s.Mount(r)
	return r
}

func getContribution(t *testing.T, router http.Handler) (int, contributionResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/domain-model/contribution", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp contributionResponse
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v (body: %s)", err, w.Body.String())
		}
	}
	return w.Code, resp
}

// An ordinary editing session was not opened to review anything. That is not
// an error and not an empty review — it is no review at all, and the UI needs
// to be able to tell the difference.
func TestContributionEndpointReportsAbsenceForAnOrdinarySession(t *testing.T) {
	code, resp := getContribution(t, mountWithContribution(t.TempDir(), ContributionSource{}))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if resp.Present {
		t.Errorf("an ordinary session has no contribution to review: %#v", resp)
	}
}

// The flag named a feature that has not written a contribution. Also not an
// error — contributions are optional.
func TestContributionEndpointReportsAbsenceWhenTheFileIsMissing(t *testing.T) {
	root := t.TempDir()
	code, resp := getContribution(t, mountWithContribution(root, ContributionSource{
		Feature: "submit-expense",
		Path:    filepath.Join(root, "spec", "intents", "submit-expense", "domain-model.yaml"),
	}))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if resp.Present {
		t.Errorf("a feature with no contribution file has nothing to review: %#v", resp)
	}
}

func TestContributionEndpointReturnsTheDeltaAgainstTheRootModel(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "domain-model.yaml"), []byte(`schema_version: 1
entities:
  - name: ExpenseReport
    fields:
      - name: id
        type: uuid
`), 0644); err != nil {
		t.Fatal(err)
	}
	featDir := filepath.Join(root, "spec", "intents", "submit-expense")
	if err := os.MkdirAll(featDir, 0755); err != nil {
		t.Fatal(err)
	}
	contributionPath := filepath.Join(featDir, "domain-model.yaml")
	if err := os.WriteFile(contributionPath, []byte(`schema_version: 1
entities:
  - name: ExpenseReport
    fields:
      - name: id
        type: uuid
      - name: settledAt
        type: datetime
`), 0644); err != nil {
		t.Fatal(err)
	}

	code, resp := getContribution(t, mountWithContribution(root, ContributionSource{
		Feature: "submit-expense", Path: contributionPath,
	}))
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if !resp.Present || resp.Feature != "submit-expense" {
		t.Fatalf("response = %#v", resp)
	}
	if resp.Delta == nil || len(resp.Delta.Additions) != 1 {
		t.Fatalf("delta = %#v", resp.Delta)
	}
	if resp.Delta.Additions[0].Path != "entities.ExpenseReport.fields.settledAt" {
		t.Errorf("addition = %#v", resp.Delta.Additions[0])
	}
	if len(resp.Delta.Redundant) != 1 {
		t.Errorf("the field the root already declares identically is redundant: %#v", resp.Delta.Redundant)
	}
	// The proposed model itself travels too — the overlay renders it, not
	// just the summary of what changed.
	if resp.Model == nil || len(resp.Model.Entities) != 1 {
		t.Errorf("the contribution model must be returned: %#v", resp.Model)
	}
}

// A broken contribution file gets the same actionable envelope a broken root
// model gets — the reader who has to fix it is told which file and why.
func TestABrokenContributionIsAnActionableFailureNotAServerError(t *testing.T) {
	root := t.TempDir()
	featDir := filepath.Join(root, "spec", "intents", "submit-expense")
	if err := os.MkdirAll(featDir, 0755); err != nil {
		t.Fatal(err)
	}
	contributionPath := filepath.Join(featDir, "domain-model.yaml")
	if err := os.WriteFile(contributionPath, []byte("entities: [\n"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/domain-model/contribution", nil)
	w := httptest.NewRecorder()
	mountWithContribution(root, ContributionSource{
		Feature: "submit-expense", Path: contributionPath,
	}).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (validation-failed), got body: %s", w.Code, w.Body.String())
	}
}
