// parlay-feature: studio-foundation/figma-mcp-client
// parlay-component: cross-cutting/figma-mcp-startup-probe

package mcpclient

import "errors"

// Stable error codes surfaced to Studio operators when the startup probe or
// the wrapper rejects a configured MCP endpoint or tool call. Each error
// message includes a tail "See studio/docs/figma-mcp-setup.md" pointing at
// the canonical setup doc.
//
// See studio/spec/intents/studio-foundation/figma-mcp-client.

// ErrEndpointUnsupported (figma-mcp-endpoint-unsupported) — configured
// endpoint matches the Figma desktop-server variant. Studio requires the
// remote MCP server because canvas-write capability is only available there.
var ErrEndpointUnsupported = errors.New(
	"figma-mcp-endpoint-unsupported: configured MCP endpoint is the Figma " +
		"desktop variant; Studio requires the remote server. " +
		"See studio/docs/figma-mcp-setup.md",
)

// ErrEndpointUnreachable (figma-mcp-endpoint-unreachable) — configured remote
// endpoint did not respond at the transport layer (DNS failure, network drop,
// or non-MCP response).
var ErrEndpointUnreachable = errors.New(
	"figma-mcp-endpoint-unreachable: configured MCP endpoint did not " +
		"respond. See studio/docs/figma-mcp-setup.md",
)

// ErrSeatInsufficient (figma-mcp-seat-insufficient) — whoami response's
// plans[] array carried no entry with seat in {Dev, Full}. Studio cannot
// invoke canvas-write tools on lower seat tiers (View, Collab).
var ErrSeatInsufficient = errors.New(
	"figma-mcp-seat-insufficient: Figma seat lacks canvas-write capability " +
		"(Dev or Full required). See studio/docs/figma-mcp-setup.md",
)

// ErrToolUnsupported (figma-mcp-tool-unsupported) — caller attempted to
// invoke a Figma MCP tool that is not on the bounded supported list. The
// supported list lives in tools.go; adding to it is a spec change against
// figma-mcp-client, not a code-only change.
var ErrToolUnsupported = errors.New("figma-mcp-tool-unsupported")
