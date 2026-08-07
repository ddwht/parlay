// parlay-section: cross-cutting

package commands

import "os"

// These two helpers used to live in studio_hook.go, whose other contents
// existed to offer the retired parlay-studio binary an open-editor prompt.
// They were never specific to that: lock-page and migrate-domain-operations
// both gate interactive prompts on ttyInteractive, and outlived the hook.

// ttyInteractive returns true exactly when both stdin and stdout are
// attached to a terminal. The override lets tests inject a fixed
// answer without standing up a fake PTY. The live probe uses
// os.File.Stat — a character-device file with no Mode().IsRegular()
// bit means stdin/stdout is a TTY rather than a pipe or redirected
// file. Avoids pulling in golang.org/x/term or mattn/go-isatty.
func ttyInteractive(override *bool) bool {
	if override != nil {
		return *override
	}
	return isCharDevice(os.Stdin) && isCharDevice(os.Stdout)
}

// isCharDevice returns true when f's stat reports the character-device
// bit set — the cross-platform proxy for "this is a TTY" without an
// extra dependency.
func isCharDevice(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
