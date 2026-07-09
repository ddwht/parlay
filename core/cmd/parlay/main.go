package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/ddwht/parlay/core/internal/commands"
)

// Set by goreleaser ldflags.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	commands.SetVersion(version, commit)
	err := commands.Execute()
	if err == nil {
		return
	}

	// Commands that need a specific exit code (JSON validators reporting
	// ok:false, the ambiguity-as-signal contract, etc.) return an
	// *commands.ExitCodeError instead of calling os.Exit directly — their
	// user-facing output is already written, so main exits silently with
	// that code rather than also printing "Error: exit code N".
	var exitErr *commands.ExitCodeError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.Code)
	}

	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
