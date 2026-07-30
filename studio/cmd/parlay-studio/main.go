// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/studio-binary-boot-and-shutdown
// parlay-extends: studio-foundation/figma-mcp-via-host-agent/cross-cutting/retract-studio-direct-mcp-source-tree
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/embedded-source-and-deployer-subcommands

// Command parlay-studio is retired. Every mode it had — the bare server boot,
// `domain-edit`, and the `init` / `upgrade` deployer subcommands — now lives in
// the `parlay` binary, and the packages that implement them are ordinary
// internal packages of the one module rather than a second program's guts.
//
// What is left is a redirect. It exists for exactly one release, so that a
// person with the old binary still on their PATH gets told where the command
// went instead of watching it vanish. It takes no action of its own and exits
// non-zero: a script that still calls parlay-studio has stopped working, and
// exiting 0 after printing a notice would hide that from the script while
// telling only the human.
//
// Delete this file and the Makefile's parlay-studio target together after that
// release.
package main

import (
	"fmt"
	"io"
	"os"
)

// exitRetired is the status every invocation exits with. Distinct from 1 so a
// caller that wants to special-case "the binary is retired" can, without having
// to match on the message text.
const exitRetired = 2

func main() {
	notice(os.Stderr, os.Args[1:])
	os.Exit(exitRetired)
}

// notice writes the redirect. It names the replacement for the specific
// subcommand when it recognizes one, because "run parlay --help" is a worse
// answer than the command the person was reaching for.
func notice(w io.Writer, args []string) {
	fmt.Fprint(w, "parlay-studio has been retired — its commands are part of `parlay` now.\n\n")

	var sub string
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "", "domain-edit":
		fmt.Fprint(w, "  parlay domain-edit\n")
	case "init":
		fmt.Fprint(w, "  parlay init\n")
	case "upgrade":
		fmt.Fprint(w, "  parlay upgrade\n")
	default:
		// Includes `version`, `help`, and anything unrecognized. There is no
		// version of this binary worth reporting — it does nothing — and the
		// full surface is the honest answer to everything else.
		fmt.Fprint(w, "  parlay domain-edit    open the domain-model editor\n")
		fmt.Fprint(w, "  parlay init           deploy parlay's skills into a project\n")
		fmt.Fprint(w, "  parlay upgrade        re-deploy them\n")
	}

	fmt.Fprint(w, "\nRun `parlay --help` for the full surface.\n")
}
