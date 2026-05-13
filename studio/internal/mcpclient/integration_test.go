//go:build integration

// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/figma-mcp-phase-0-wiring
// parlay-artifact: test

// Integration tests against Figma's real remote MCP server. Skipped from
// the default `go test ./...` invocation; run with `go test -tags
// integration ./internal/mcpclient/...` and the environment variables
// PARLAY_STUDIO_FIGMA_MCP_URL and PARLAY_STUDIO_FIGMA_MCP_TOKEN set to
// credentials with at least the Dev seat.
//
// The suite exercises the same code paths as the unit tests but against
// the real transport so SDK contract drifts surface in CI early.
package mcpclient

import (
	"context"
	"errors"
	"os"
	"testing"
)

const (
	envURL   = "PARLAY_STUDIO_FIGMA_MCP_URL"
	envToken = "PARLAY_STUDIO_FIGMA_MCP_TOKEN"
)

func requireFigmaCreds(t *testing.T) (string, string) {
	t.Helper()
	url := os.Getenv(envURL)
	token := os.Getenv(envToken)
	if url == "" || token == "" {
		t.Skipf("integration test skipped: %s and %s must be set", envURL, envToken)
	}
	return url, token
}

// TestIntegration_ProbeAgainstRealFigma runs the startup probe against
// the live Figma remote MCP server and asserts a Dev or Full seat is
// returned.
func TestIntegration_ProbeAgainstRealFigma(t *testing.T) {
	url, token := requireFigmaCreds(t)
	res, err := ProbeWithToken(context.Background(), url, token)
	if err != nil {
		t.Fatalf("ProbeWithToken: %v", err)
	}
	if res.Email == "" {
		t.Fatal("ProbeWithToken: empty email")
	}
	if res.Seat != "Dev" && res.Seat != "Full" {
		t.Fatalf("ProbeWithToken: seat=%q, want Dev or Full", res.Seat)
	}
}

// TestIntegration_UseFigmaRoundTrip exercises a single use_figma round
// trip against the live server. The fixture node ID must be set via
// PARLAY_STUDIO_FIGMA_FIXTURE_NODE_ID; missing the variable skips this
// case so the harness still runs on credentials-only environments.
func TestIntegration_UseFigmaRoundTrip(t *testing.T) {
	url, token := requireFigmaCreds(t)
	nodeID := os.Getenv("PARLAY_STUDIO_FIGMA_FIXTURE_NODE_ID")
	if nodeID == "" {
		t.Skip("PARLAY_STUDIO_FIGMA_FIXTURE_NODE_ID not set; skipping use_figma round-trip")
	}

	c, err := New(context.Background(), url, token)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close(context.Background())

	out, err := c.UseFigma(context.Background(), UseFigmaInput{NodeID: nodeID})
	if err != nil {
		t.Fatalf("UseFigma: %v", err)
	}
	if out == nil {
		t.Fatal("UseFigma: nil output")
	}
}

// TestIntegration_GetMetadataRoundTrip exercises get_metadata on the
// same fixture node as UseFigma.
func TestIntegration_GetMetadataRoundTrip(t *testing.T) {
	url, token := requireFigmaCreds(t)
	nodeID := os.Getenv("PARLAY_STUDIO_FIGMA_FIXTURE_NODE_ID")
	if nodeID == "" {
		t.Skip("PARLAY_STUDIO_FIGMA_FIXTURE_NODE_ID not set; skipping get_metadata round-trip")
	}

	c, err := New(context.Background(), url, token)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close(context.Background())

	out, err := c.GetMetadata(context.Background(), GetMetadataInput{NodeID: nodeID})
	if err != nil {
		t.Fatalf("GetMetadata: %v", err)
	}
	if out == nil {
		t.Fatal("GetMetadata: nil output")
	}
}

// TestIntegration_GetCodeConnectSuggestionsRejected asserts that even
// when the live server accepts the call, our wrapper still rejects
// `get_design_context` (the regression for the bounded surface).
func TestIntegration_GetDesignContextRejected(t *testing.T) {
	c := &Client{}
	_, err := c.dispatch(context.Background(), "get_design_context", nil)
	if !errors.Is(err, ErrToolUnsupported) {
		t.Fatalf("dispatch(get_design_context): expected ErrToolUnsupported, got %v", err)
	}
}
