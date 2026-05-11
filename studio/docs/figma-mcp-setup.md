<!--
parlay-feature: studio-foundation/figma-mcp-client
parlay-component: cross-cutting/figma-mcp-startup-probe
-->

# Figma MCP setup for Parlay Studio

Parlay Studio talks to Figma exclusively through Figma's official **remote** MCP server. This doc covers what you need on the Figma side, how to configure Studio, and what each startup error code means.

## Prereqs (Figma side)

- A Figma account with a **Dev** or **Full** seat on a Professional, Organization, or Enterprise plan. View and Collab seats cannot use the MCP write tools Studio depends on.
- The remote MCP server enabled for your Figma workspace. See Figma's official guide: [Get started with the Figma MCP server](https://help.figma.com/hc/en-us/articles/39216419318551-Get-started-with-the-Figma-MCP-server).
- Authentication credentials per Figma's docs (OAuth or token; Figma's docs specify the exact flow).

## Studio configuration

Set Studio's Figma MCP endpoint in the environment:

```
STUDIO_FIGMA_MCP_URL=https://mcp.figma.com/...
```

Studio runs a one-shot startup probe against this endpoint via Figma's `whoami` tool. The probe succeeds when the endpoint is the remote variant, responds at the transport layer, and your Figma seat tier grants canvas-write capability.

On a successful startup Studio logs the resolved endpoint, the authenticated user's email (from the `whoami` response), and the canvas-write seat tier — `Dev` or `Full`, sourced from `plans[].seat`. Auth tokens and other secrets are never logged.

## Startup error codes

If Studio fails to start because of an MCP misconfiguration, you'll see one of these three stable error codes.

### `figma-mcp-endpoint-unsupported`

The configured `STUDIO_FIGMA_MCP_URL` resolves to Figma's desktop-app MCP server (e.g. `localhost`-bound, or matches the desktop signature in the `whoami` response). Studio cannot use the desktop variant because canvas-write capability — required by Studio's Design Loop — is only available on the remote server.

**Fix**: Point `STUDIO_FIGMA_MCP_URL` at Figma's remote MCP server (an HTTPS URL hosted by Figma, not `localhost`). See Figma's guide linked above.

### `figma-mcp-endpoint-unreachable`

The configured endpoint did not respond at the transport layer — DNS failed to resolve, the connection dropped, or the server returned a non-MCP error.

**Fix**: Check that the endpoint URL is correct, that your network can reach Figma's servers, and that your auth credentials are valid. The Studio log line includes the underlying transport error for diagnosis.

### `figma-mcp-seat-insufficient`

The `whoami` response reached Studio, but none of the plans your account belongs to (`plans[]`) advertise a `Dev` or `Full` seat. Studio cannot invoke canvas-write tools without one of those two seats.

**Fix**: Upgrade your Figma seat to Dev or Full, or switch to an account that has one. See Figma's [Plans, access, and permissions](https://developers.figma.com/docs/figma-mcp-server/plans-access-and-permissions/) page for what each seat allows.

## Why these constraints exist

The choice of remote-only MCP, the bounded tool surface, and the strict SDK pin are pinned in [`studio/spec/intents/studio-foundation/figma-mcp-client/`](../spec/intents/studio-foundation/figma-mcp-client/). The intents file is the canonical source of truth for what Studio requires from Figma; this doc translates those requirements into an operator-facing setup checklist.
