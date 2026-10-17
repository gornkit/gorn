package cli

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

// StatusError carries a CLI exit status. Silent errors have already been
// reported by the child process; the entry point must not duplicate them.
type StatusError struct {
	Code   int
	Err    error
	Silent bool
}

func (e *StatusError) Error() string { return e.Err.Error() }
func (e *StatusError) Unwrap() error { return e.Err }

// ExitStatus returns the process status for an execution/usage error, or 1 for
// source, build, startup, and other runner failures. A nil error is success.
func ExitStatus(err error) int {
	if err == nil {
		return 0
	}
	var status *StatusError
	if errors.As(err, &status) {
		return status.Code
	}
	return 1
}

func scriptError(err error) error {
	var exited *exec.ExitError
	if !errors.As(err, &exited) {
		return fmt.Errorf("start script: %w", err)
	}
	code := exited.ExitCode()
	if code < 0 {
		code = 1
		if status, ok := exited.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			code = 128 + int(status.Signal())
		}
	}
	return &StatusError{Code: code, Err: err, Silent: true}
}
