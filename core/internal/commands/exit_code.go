package commands

import "fmt"

// ExitCodeError signals that the process should terminate with a specific
// non-zero exit code, without main printing an additional "Error: ..."
// line — the command has already written its own user-facing output via
// cmd.OutOrStdout() / cmd.ErrOrStderr() before returning it.
//
// This is the core of the testability substrate: os.Exit terminates the
// whole process (including the test binary), so any RunE code path that
// called it directly could only be exercised end-to-end by spawning a
// real subprocess. Returning a typed error instead lets tests call the
// runXxx handler in-process and assert on the returned error's Code
// field, and lets Execute()'s caller (main) decide how to translate it
// into a process exit code.
type ExitCodeError struct {
	Code int
}

func (e *ExitCodeError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

// NewExitCodeError constructs an ExitCodeError for the given code. Command
// handlers that need a specific non-zero exit status return this instead
// of calling os.Exit directly.
func NewExitCodeError(code int) error {
	return &ExitCodeError{Code: code}
}
