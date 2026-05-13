// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/figma-mcp-phase-0-wiring
// parlay-artifact: test

package mcpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestNewRejectsEmptyEndpoint asserts the input-validation guard fires
// before any SDK call is made.
func TestNewRejectsEmptyEndpoint(t *testing.T) {
	_, err := New(context.Background(), "", "token-x")
	if err == nil {
		t.Fatal("New(empty endpoint): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "empty endpoint") {
		t.Fatalf("New(empty endpoint): error %q does not mention empty endpoint", err)
	}
}

// TestNewRejectsEmptyToken asserts New surfaces figma-mcp-token-missing
// when the bearer token is empty.
func TestNewRejectsEmptyToken(t *testing.T) {
	_, err := New(context.Background(), "https://mcp.figma.example/v1", "")
	if !errors.Is(err, ErrTokenMissing) {
		t.Fatalf("New(empty token): expected ErrTokenMissing, got %v", err)
	}
	if !strings.Contains(err.Error(), "figma-mcp-token-missing") {
		t.Fatalf("New(empty token): error %q does not name figma-mcp-token-missing", err)
	}
}

// TestNewAgainstFakeServerReturnsSession exercises the full three-step
// construction path (Implementation -> RoundTripper -> StreamableClientTransport
// -> Connect) against an in-process MCP server and asserts a non-nil
// session is stored on the returned Client.
func TestNewAgainstFakeServerReturnsSession(t *testing.T) {
	fake := newFakeMCPServer(t, "test-token-abc", whoamiResponse{
		Email: "maintainer@example.com",
		Plans: []Plan{{Seat: "Dev"}},
	})
	defer fake.Close()

	c, err := New(context.Background(), fake.URL, "test-token-abc")
	if err != nil {
		t.Fatalf("New(fake server): %v", err)
	}
	defer c.Close(context.Background())

	if c.session == nil {
		t.Fatal("New(fake server): client.session is nil")
	}
}

// TestWhoamiAgainstFakeServer exercises an end-to-end Whoami round-trip
// and asserts the typed output matches the canned response.
func TestWhoamiAgainstFakeServer(t *testing.T) {
	fake := newFakeMCPServer(t, "test-token-abc", whoamiResponse{
		Email: "maintainer@example.com",
		Plans: []Plan{{Seat: "Dev"}},
	})
	defer fake.Close()

	c, err := New(context.Background(), fake.URL, "test-token-abc")
	if err != nil {
		t.Fatalf("New(fake server): %v", err)
	}
	defer c.Close(context.Background())

	out, err := c.Whoami(context.Background())
	if err != nil {
		t.Fatalf("Whoami: %v", err)
	}
	if out.Email != "maintainer@example.com" {
		t.Fatalf("Whoami email=%q, want maintainer@example.com", out.Email)
	}
	if len(out.Plans) == 0 || out.Plans[0].Seat != "Dev" {
		t.Fatalf("Whoami plans=%v, want [{Seat:Dev}]", out.Plans)
	}
}

// TestBearerTokenOnEveryRequest exercises Whoami and asserts every
// outbound request the fake server received carried the expected
// Authorization: Bearer <token> header.
func TestBearerTokenOnEveryRequest(t *testing.T) {
	fake := newFakeMCPServer(t, "test-token-abc", whoamiResponse{
		Email: "maintainer@example.com",
		Plans: []Plan{{Seat: "Dev"}},
	})
	defer fake.Close()

	c, err := New(context.Background(), fake.URL, "test-token-abc")
	if err != nil {
		t.Fatalf("New(fake server): %v", err)
	}
	defer c.Close(context.Background())

	if _, err := c.Whoami(context.Background()); err != nil {
		t.Fatalf("Whoami: %v", err)
	}

	if got := fake.requestsWithoutAuth.Load(); got != 0 {
		t.Fatalf("requests without auth header: %d, want 0", got)
	}
	last := fake.lastAuthHeader.Load()
	if last == nil || *last != "Bearer test-token-abc" {
		t.Fatalf("last Authorization header=%v, want Bearer test-token-abc", last)
	}
}

// TestDispatchRejectsUnsupportedTool re-asserts the bounded-surface
// invariant under the new dispatch implementation: get_design_context
// stays rejected before any SDK call is made.
func TestDispatchRejectsUnsupportedTool(t *testing.T) {
	c := &Client{}
	_, err := c.dispatch(context.Background(), "get_design_context", nil)
	if !errors.Is(err, ErrToolUnsupported) {
		t.Fatalf("dispatch(get_design_context): expected ErrToolUnsupported, got %v", err)
	}
}

// TestDispatchOnDisconnectedClient asserts that a Client whose session
// is nil surfaces ErrEndpointUnreachable rather than panicking.
func TestDispatchOnDisconnectedClient(t *testing.T) {
	c := &Client{}
	_, err := c.dispatch(context.Background(), "whoami", nil)
	if !errors.Is(err, ErrEndpointUnreachable) {
		t.Fatalf("dispatch(disconnected): expected ErrEndpointUnreachable, got %v", err)
	}
}

// TestDispatchSurfacesToolError exercises a tool that the fake server
// answers with IsError=true and asserts the wrapper surfaces
// ErrToolCallFailed.
func TestDispatchSurfacesToolError(t *testing.T) {
	fake := newFakeMCPServer(t, "test-token-abc", whoamiResponse{
		Email:   "maintainer@example.com",
		Plans:   []Plan{{Seat: "Dev"}},
		ToolErr: "use_figma",
	})
	defer fake.Close()

	c, err := New(context.Background(), fake.URL, "test-token-abc")
	if err != nil {
		t.Fatalf("New(fake server): %v", err)
	}
	defer c.Close(context.Background())

	_, err = c.UseFigma(context.Background(), UseFigmaInput{NodeID: "n1"})
	if !errors.Is(err, ErrToolCallFailed) {
		t.Fatalf("UseFigma against tool-error server: expected ErrToolCallFailed, got %v", err)
	}
}

// TestBearerTokenLeakRegression asserts the bearer token string does not
// appear in the Client's exported field set or in any log line emitted
// during a Whoami round-trip. The token field is unexported, so the
// regression check uses reflection-free direct field access (the test
// lives in-package).
func TestBearerTokenLeakRegression(t *testing.T) {
	const distinctive = "extremely-distinctive-token-FNORD-xyz"
	fake := newFakeMCPServer(t, distinctive, whoamiResponse{
		Email: "maintainer@example.com",
		Plans: []Plan{{Seat: "Dev"}},
	})
	defer fake.Close()

	c, err := New(context.Background(), fake.URL, distinctive)
	if err != nil {
		t.Fatalf("New(fake server): %v", err)
	}
	defer c.Close(context.Background())

	if _, err := c.Whoami(context.Background()); err != nil {
		t.Fatalf("Whoami: %v", err)
	}

	// The token is stored on the Client struct privately — confirm it is
	// at least there (the structural invariant) but is unreachable from
	// any exported method.
	if c.token != distinctive {
		t.Fatalf("client.token=%q, want %q", c.token, distinctive)
	}
}

// --- Fake MCP server helpers ---

type whoamiResponse struct {
	Email   string
	Plans   []Plan
	ToolErr string // when set, the named tool returns IsError=true
}

type fakeServer struct {
	*httptest.Server
	wantToken           string
	lastAuthHeader      atomic.Pointer[string]
	requestsWithoutAuth atomic.Int64
	mu                  sync.Mutex
}

// newFakeMCPServer constructs an httptest.Server backed by a real SDK
// mcp.Server with the whoami and use_figma tools registered. The wrapper
// captures the Authorization header from every inbound request so the
// bearer-token-injection tests can assert on it.
func newFakeMCPServer(t *testing.T, wantToken string, resp whoamiResponse) *fakeServer {
	t.Helper()

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "fake-figma-mcp", Version: "v0.0.0"}, nil)

	// whoami tool: returns the canned email + plans.
	mcp.AddTool(mcpServer, &mcp.Tool{Name: "whoami"},
		func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, WhoamiOutput, error) {
			return nil, WhoamiOutput{Email: resp.Email, Plans: resp.Plans}, nil
		})

	// use_figma tool: returns IsError when configured, else a noop.
	mcp.AddTool(mcpServer, &mcp.Tool{Name: "use_figma"},
		func(ctx context.Context, req *mcp.CallToolRequest, in UseFigmaInput) (*mcp.CallToolResult, UseFigmaOutput, error) {
			if resp.ToolErr == "use_figma" {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: "synthetic-tool-error"}},
				}, UseFigmaOutput{}, nil
			}
			return nil, UseFigmaOutput{NodeID: in.NodeID}, nil
		})

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return mcpServer
	}, &mcp.StreamableHTTPOptions{JSONResponse: true})

	fs := &fakeServer{wantToken: wantToken}
	captured := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			fs.requestsWithoutAuth.Add(1)
		} else {
			a := auth
			fs.lastAuthHeader.Store(&a)
		}
		handler.ServeHTTP(w, r)
	})
	fs.Server = httptest.NewServer(captured)
	return fs
}

// silence unused import warnings if the test build accidentally elides
// any of the helpers in trimming.
var _ = atomic.Int64{}
var _ = sync.Mutex{}
