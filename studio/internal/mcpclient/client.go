// parlay-feature: studio-foundation/figma-mcp-client
// parlay-component: cross-cutting/official-mcp-sdk-adoption
// parlay-extends: studio-foundation/web-server-harness/cross-cutting/figma-mcp-phase-0-wiring
//
// Package mcpclient is the sole importer of github.com/modelcontextprotocol/go-sdk
// in Parlay Studio. All MCP traffic to Figma flows through this package; direct
// SDK imports from elsewhere fail the import-boundary check in boundary_test.go.
//
// See studio/spec/intents/studio-foundation/figma-mcp-client for the spec.
package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// buildVersion is the package-internal build version advertised in the
// MCP Implementation handshake. Production builds may override this via
// `go build -ldflags="-X github.com/parlay-tool/parlay/studio/internal/mcpclient.buildVersion=v0.1.0"`.
// The default keeps the field non-empty so the SDK's Implementation
// validation does not reject Studio at startup.
var buildVersion = "dev"

// ErrTokenMissing (figma-mcp-token-missing) — New was invoked with an
// empty bearer token. Studio refuses to construct a client without a
// token because the remote Figma MCP server requires authentication for
// every request.
var ErrTokenMissing = errors.New(
	"figma-mcp-token-missing: Figma MCP bearer token is empty. " +
		"See studio/docs/figma-mcp-setup.md",
)

// ErrToolCallFailed (figma-mcp-tool-call-failed) — the SDK returned a
// CallToolResult with IsError=true. The wrapped detail carries the tool
// name and the result's content text.
var ErrToolCallFailed = errors.New("figma-mcp-tool-call-failed")

// ErrResponseMalformed (figma-mcp-response-malformed) — the SDK returned
// a StructuredContent value whose shape cannot be JSON-unmarshalled into
// the typed Out. The wrapped detail names the tool and the marshal /
// unmarshal failure.
var ErrResponseMalformed = errors.New("figma-mcp-response-malformed")

// Client wraps the official MCP Go SDK client and exposes one named
// method per supported Figma tool. There is no generic CallTool(name, args)
// passthrough — the wrapper is the enforcement boundary for the bounded
// tool surface.
type Client struct {
	sdk     *mcp.Client
	session *mcp.ClientSession
	// token is held privately so Probe can re-issue connections during
	// graceful shutdown if needed; it is NEVER logged or exposed.
	token string
}

// New constructs an MCP client connected to the configured remote Figma
// endpoint and authenticated via the supplied bearer token. The token is
// the second positional argument so callers cannot accidentally pass it
// as the endpoint.
//
// New executes the documented three-step sequence:
//
//  1. construct the SDK client with a Studio-identifying Implementation
//  2. build a bearer-token-injecting RoundTripper, wrap it in a
//     StreamableClientTransport pointed at endpoint
//  3. call sdkClient.Connect(ctx, transport, nil); on success store the
//     returned *mcp.ClientSession on the Client
//
// Failures from the SDK's Connect call are wrapped with
// ErrEndpointUnreachable so the boot orchestration's structured log can
// surface the upstream stable code.
func New(ctx context.Context, endpoint, token string) (*Client, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("mcpclient: empty endpoint")
	}
	if token == "" {
		return nil, ErrTokenMissing
	}

	impl := &mcp.Implementation{
		Name:    "parlay-studio",
		Version: buildVersion,
	}
	sdkClient := mcp.NewClient(impl, nil)

	httpClient := &http.Client{
		Transport: newBearerTokenRoundTripper(token, http.DefaultTransport),
	}
	transport := &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: httpClient,
	}

	session, err := sdkClient.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEndpointUnreachable, err)
	}

	return &Client{sdk: sdkClient, session: session, token: token}, nil
}

// Close shuts down the persistent MCP session. It is called by the
// graceful-shutdown path in cross-cutting/studio-binary-boot-and-shutdown.
// Close is idempotent: calling it twice (or on a Client whose session
// was never established) returns nil.
func (c *Client) Close(ctx context.Context) error {
	if c == nil || c.session == nil {
		return nil
	}
	if err := c.session.Close(); err != nil {
		return fmt.Errorf("mcpclient: close session: %w", err)
	}
	c.session = nil
	return nil
}
