package main

import (
	"fmt"
	"os"

	"github.com/ddwht/parlay/internal/commands"
)

// Set by goreleaser ldflags.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	commands.SetVersion(version, commit)
	if err := commands.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
