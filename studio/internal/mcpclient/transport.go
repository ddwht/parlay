// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/figma-mcp-phase-0-wiring

package mcpclient

import "net/http"

// bearerTokenRoundTripper is the SOLE mechanism by which the Figma MCP
// authentication token reaches the network. The token is held only in
// the RoundTripper and on the Client struct (privately, never exposed);
// it never appears in mcp.Implementation, mcp.ClientOptions, or any
// structured-log field.
//
// The RoundTrip method clones the request before mutating headers — this
// is the contract documented on http.RoundTripper (Go's std library
// requires the implementation to be safe for concurrent use and to not
// modify the supplied *http.Request).
type bearerTokenRoundTripper struct {
	token string
	base  http.RoundTripper
}

// newBearerTokenRoundTripper constructs the RoundTripper. base defaults
// to http.DefaultTransport when nil.
func newBearerTokenRoundTripper(token string, base http.RoundTripper) *bearerTokenRoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &bearerTokenRoundTripper{token: token, base: base}
}

// RoundTrip implements http.RoundTripper. It clones the request, sets the
// Authorization header to "Bearer <token>", and delegates to the base
// transport. The cloned-request invariant prevents concurrent callers
// from observing each other's mutated headers.
func (rt *bearerTokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header.Set("Authorization", "Bearer "+rt.token)
	return rt.base.RoundTrip(cloned)
}
