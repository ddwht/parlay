// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/studio-binary-boot-and-shutdown
package server

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openBrowser launches the operator's default browser at url.
//
// This is the production implementation of BootDeps.OpenBrowser. Until it
// existed the field was declared, called, and defaulted to a no-op that was
// never replaced by anything outside tests — so `open_browser` resolved to
// true, boot took the branch, and called a function that returned nil without
// doing anything. `--no-browser` and its absence were behaviourally
// identical, and BrowserPath ("/domain-model" for `parlay domain-edit`) was
// dead configuration: the only way to reach the editor was to read the port
// out of the log and type the route by hand.
//
// It lives here rather than in the command layer for the same reason
// bind127OnlyListener does: the package owns the production defaults for its
// own dependency struct, and a test can substitute a recorder without
// spawning a real browser.
//
// Errors are returned, not fatal. Boot logs them as a warning and carries on
// — a headless box, a container, or an SSH session has no browser to open and
// that must not stop the server from serving.
func openBrowser(url string) error {
	name, args := browserCommand(url)
	if name == "" {
		return fmt.Errorf("no browser launcher known for %s", runtime.GOOS)
	}
	// Start, not Run: the launcher hands off to a browser process that
	// outlives it, and waiting would block boot until the browser exits.
	return exec.Command(name, args...).Start()
}

// browserCommand maps the current platform to its URL-opening command.
// Split out from openBrowser so the mapping is testable without executing
// anything.
func browserCommand(url string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{url}
	case "windows":
		// rundll32 is the launcher that does not depend on a shell being
		// present and does not mangle a URL containing & the way `cmd /c
		// start` does.
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		// xdg-open is the freedesktop.org standard and is what every mainstream
		// Linux and BSD desktop provides.
		return "xdg-open", []string{url}
	}
}
