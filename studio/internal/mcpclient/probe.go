// parlay-feature: studio-foundation/figma-mcp-client
// parlay-component: cross-cutting/figma-mcp-startup-probe

package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ProbeResult is what the startup probe returns on success. Callers use it
// to log the resolved endpoint, the authenticated user's email, and the
// canvas-write seat tier. Auth secrets never appear in ProbeResult.
type ProbeResult struct {
	Endpoint string
	Email    string
	Seat     string // "Dev" or "Full" — the canvas-write-capable seats
}

// canvasWriteSeats names the seat tiers Figma grants canvas-write capability
// in its MCP plan model. Callers compare whoami's plans[].seat against this
// set; anything outside it fails with ErrSeatInsufficient.
var canvasWriteSeats = map[string]struct{}{
	"Dev":  {},
	"Full": {},
}

// Probe runs once at Studio startup against the configured MCP endpoint and
// returns the resolved (endpoint, email, seat) tuple on success. The three
// stable failure modes are surfaced via ErrEndpointUnsupported,
// ErrEndpointUnreachable, and ErrSeatInsufficient — callers use errors.Is
// to dispatch.
//
// The probe invokes whoami exactly once per Studio startup, regardless of
// how many subsequent tool calls Studio makes through the wrapper.
//
// Probe's signature is endpoint-only for backwards-compatibility with the
// figma-mcp-client tests; the underlying connection is established via
// probeConnect, which the boot orchestration overrides to supply the
// real token. Callers that need authenticated probing should use
// ProbeWithToken directly.
func Probe(ctx context.Context, endpoint string) (ProbeResult, error) {
	return ProbeWithToken(ctx, endpoint, "")
}

// ProbeWithToken is the authenticated variant of Probe. Boot uses it so
// the startup whoami runs over the same bearer-token transport the
// persistent session will use.
func ProbeWithToken(ctx context.Context, endpoint, token string) (ProbeResult, error) {
	if isDesktopShape(endpoint) {
		return ProbeResult{}, ErrEndpointUnsupported
	}

	c, err := probeConnect(ctx, endpoint, token)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("%w: %v", ErrEndpointUnreachable, err)
	}
	defer func() {
		_ = c.Close(ctx)
	}()

	resp, err := c.Whoami(ctx)
	if err != nil {
		// "The endpoint reached us but advertised the desktop variant"
		// surfaces as ErrEndpointUnsupported; anything else is treated as
		// a transport-layer unreachable.
		if errors.Is(err, ErrEndpointUnsupported) {
			return ProbeResult{}, err
		}
		return ProbeResult{}, fmt.Errorf("%w: %v", ErrEndpointUnreachable, err)
	}

	seat, ok := pickCanvasWriteSeat(resp.Plans)
	if !ok {
		return ProbeResult{}, ErrSeatInsufficient
	}

	return ProbeResult{
		Endpoint: endpoint,
		Email:    resp.Email,
		Seat:     seat,
	}, nil
}

// probeConnect is the test-overridable bridge between Probe and New.
// Production probes call New with the supplied token (or a placeholder
// when the legacy Probe API was used); tests swap probeConnect for a
// constructor returning a *Client backed by a fake session.
var probeConnect = func(ctx context.Context, endpoint, token string) (*Client, error) {
	if token == "" {
		// Legacy callers (Probe with endpoint-only signature) get a
		// placeholder token so New does not reject the call. The real
		// remote will still 401 if the token is wrong; this preserves the
		// "unreachable" surface area for the original test suite.
		token = "probe-without-token"
	}
	return New(ctx, endpoint, token)
}

// isDesktopShape detects the Figma desktop MCP server endpoint by URL shape.
// Figma's desktop variant binds to localhost; the remote variant uses HTTPS
// against a Figma-hosted host. The check is conservative — non-HTTPS URLs
// and any host resolving to localhost are treated as desktop.
func isDesktopShape(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	if u.Scheme != "https" {
		return true
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// pickCanvasWriteSeat walks the whoami response's plans[] array and returns
// the first canvas-write-capable seat ("Dev" or "Full"). The second return
// is false when no plan entry carries a canvas-write seat — that maps to
// ErrSeatInsufficient at the caller.
func pickCanvasWriteSeat(plans []Plan) (string, bool) {
	for _, p := range plans {
		if _, ok := canvasWriteSeats[p.Seat]; ok {
			return p.Seat, true
		}
	}
	return "", false
}
