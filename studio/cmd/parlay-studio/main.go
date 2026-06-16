// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/studio-binary-boot-and-shutdown
// parlay-extends: studio-foundation/figma-mcp-via-host-agent/cross-cutting/retract-studio-direct-mcp-source-tree
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/embedded-source-and-deployer-subcommands

// Command parlay-studio is the Parlay Studio binary. main() is intentionally
// thin — it dispatches on os.Args[1] across three modes:
//
//   - parlay-studio init    → run the deployer's first-time bootstrap subcommand
//   - parlay-studio upgrade → run the deployer's idempotent re-deploy subcommand
//   - parlay-studio         → (no subcommand) boot the server harness, the
//     default invocation that preserves the pre-deployer behavior.
//
// Cobra is intentionally not in Studio's go.mod; the entry point dispatches
// via os.Args directly. The deployer subcommands parse their own flags off
// the residual os.Args[2:] slice.
//
// The boot orchestration helper lives in studio/internal/server/boot.go so
// that the sequence is unit-testable via injected fakes (config.Loader,
// http.Server.ListenAndServe, etc.).
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/parlay-tool/parlay/studio/internal/deployer"
	"github.com/parlay-tool/parlay/studio/internal/server"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "init":
			if err := deployer.Init(context.Background(), args[1:]); err != nil {
				fmt.Fprintf(os.Stderr, "parlay-studio init: %v\n", err)
				os.Exit(1)
			}
			return
		case "upgrade":
			if err := deployer.Upgrade(context.Background(), args[1:]); err != nil {
				fmt.Fprintf(os.Stderr, "parlay-studio upgrade: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}
	// Default: run the server harness.
	if err := server.Boot(context.Background(), server.BootDeps{}); err != nil {
		fmt.Fprintf(os.Stderr, "parlay-studio: %v\n", err)
		os.Exit(1)
	}
}
