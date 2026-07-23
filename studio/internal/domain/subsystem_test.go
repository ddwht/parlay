// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/domain-model-subsystem-registration
// parlay-artifact: test

package domain

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/parlay-tool/parlay/studio/internal/server"
)

// TestSubsystemName asserts the registered tool name is the stable
// "domain-model" the harness collision path keys on.
func TestSubsystemName(t *testing.T) {
	if got := New("/project").Name(); got != "domain-model" {
		t.Fatalf("Name() = %q, want %q", got, "domain-model")
	}
}

// TestSubsystemSatisfiesToolRegistration is a compile-time-plus-runtime check
// that Subsystem plugs into the harness registration surface.
func TestSubsystemSatisfiesToolRegistration(t *testing.T) {
	var _ server.ToolRegistration = New("/project")
}

// TestMountRegistersExactlyTwoEndpointsUnderPrefix asserts Mount registers only
// the two persistence endpoints, both under /api/domain-model, and nothing
// outside that prefix.
func TestMountRegistersExactlyTwoEndpointsUnderPrefix(t *testing.T) {
	r := chi.NewRouter()
	New("/project").Mount(r)

	type route struct{ method, path string }
	var routes []route
	err := chi.Walk(r, func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes = append(routes, route{method, path})
		return nil
	})
	if err != nil {
		t.Fatalf("chi.Walk: %v", err)
	}

	if len(routes) != 2 {
		t.Fatalf("registered %d routes, want exactly 2: %+v", len(routes), routes)
	}
	var haveGet, havePut bool
	for _, rt := range routes {
		if !strings.HasPrefix(rt.path, "/api/domain-model") {
			t.Fatalf("route %s %s is mounted outside the /api/domain-model prefix", rt.method, rt.path)
		}
		switch rt.method {
		case http.MethodGet:
			haveGet = true
		case http.MethodPut:
			havePut = true
		}
	}
	if !haveGet || !havePut {
		t.Fatalf("want a GET (load) and a PUT (save) endpoint; got %+v", routes)
	}
}

// TestDuplicateRegistrationRejected asserts a second registration under the
// domain-model tool name is rejected at harness-construction time via the
// existing tool-name-collision path.
func TestDuplicateRegistrationRejected(t *testing.T) {
	_, err := server.New(server.Deps{
		Tools: []server.ToolRegistration{New("/a"), New("/b")},
	})
	if !errors.Is(err, server.ErrToolNameCollision) {
		t.Fatalf("expected ErrToolNameCollision for a duplicate tool name, got %v", err)
	}
}

// TestSubsystemSourceFilesExist mirrors the testcases file-exists cases: the
// subsystem and its handlers live in this package.
func TestSubsystemSourceFilesExist(t *testing.T) {
	for _, f := range []string{"subsystem.go", "handlers.go", "resolve.go", "loader.go", "etag.go", "save.go", "serialize.go"} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("expected source file %s to exist: %v", f, err)
		}
	}
}
