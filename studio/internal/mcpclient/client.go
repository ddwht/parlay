// parlay-feature: studio-foundation/figma-mcp-client
// parlay-component: cross-cutting/official-mcp-sdk-adoption
//
// Package mcpclient is the sole importer of github.com/modelcontextprotocol/go-sdk
// in Parlay Studio. All MCP traffic to Figma flows through this package; direct
// SDK imports from elsewhere fail the import-boundary check in boundary_test.go.
//
// See studio/spec/intents/studio-foundation/figma-mcp-client for the spec.
package mcpclient

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Client wraps the official MCP Go SDK client and exposes one named method per
// supported Figma tool. There is no generic CallTool(name, args) passthrough —
// the wrapper is the enforcement boundary for the bounded tool surface.
type Client struct {
	sdk *mcp.Client
}

// New returns a Client wrapping an MCP session against the configured remote
// endpoint. Authentication mechanics are delegated to Figma's remote-server
// documentation; this package does not invent its own auth flow.
//
// The prototype's connection construction is intentionally minimal; Phase 0
// wiring fills in the concrete transport against the live Studio harness.
func New(ctx context.Context, endpoint string) (*Client, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("mcpclient: empty endpoint")
	}
	return &Client{}, nil
}
