package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The code this package emits must equal the one the harness defines. The
// string is duplicated because the dependency is deliberately one-way and this
// package cannot import the harness; this test buys the cheap half of what an
// import would have.
func TestBundleNotBuiltCodeMatchesHarness(t *testing.T) {
	// Read the harness source rather than importing it.
	data, err := os.ReadFile(filepath.Join("..", "server", "server.go"))
	if err != nil {
		t.Skipf("harness source unavailable: %v", err)
	}
	re := regexp.MustCompile(`ErrUIBundleNotBuilt\s*=\s*errors\.New\("([^"]+)"\)`)
	m := re.FindSubmatch(data)
	if m == nil {
		t.Fatal("could not find ErrUIBundleNotBuilt in the harness source")
	}
	if got, want := UIBundleNotBuiltCode, string(m[1]); got != want {
		t.Fatalf("code drifted from the harness: ui has %q, server has %q", got, want)
	}
}

// An unbuilt bundle must be the documented 503 envelope, not a bare 500. The
// regression run got a third, undocumented state — neither the shell nor the
// documented error — with nothing saying a build step had been skipped.
func TestUnbuiltBundleEmitsDocumentedEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	writeBundleNotBuilt(rec)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 — a missing build artifact is not a server fault", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, rec.Body.String())
	}
	if body["code"] != UIBundleNotBuiltCode {
		t.Errorf("code = %v, want %q", body["code"], UIBundleNotBuiltCode)
	}
	// A remediation hint is the difference between a reportable state and a
	// dead end: the operator has to learn a build step was skipped.
	fix, _ := body["fix"].(string)
	if strings.TrimSpace(fix) == "" {
		t.Error("envelope carries no fix hint")
	}
	if body["severity"] == nil {
		t.Error("envelope carries no severity")
	}
}
