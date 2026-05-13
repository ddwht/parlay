// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/studio-binary-boot-and-shutdown
// parlay-extends: studio-foundation/figma-mcp-via-host-agent/cross-cutting/retract-studio-direct-mcp-source-tree

// Command parlay-studio is the Parlay Studio binary. main() is intentionally
// thin — it constructs the BootDeps with production defaults and delegates
// to server.Boot, which executes the documented 10-step boot sequence and
// blocks on the shutdown channel.
//
// The boot orchestration helper lives in studio/internal/server/boot.go so
// that the sequence is unit-testable via injected fakes (config.Loader,
// http.Server.ListenAndServe, etc.).
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/parlay-tool/parlay/studio/internal/server"
)

func main() {
	if err := server.Boot(context.Background(), server.BootDeps{}); err != nil {
		fmt.Fprintf(os.Stderr, "parlay-studio: %v\n", err)
		os.Exit(1)
	}
}
