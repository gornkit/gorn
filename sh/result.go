package sh

import (
	"fmt"
	"os"
)

// Result is the status of a finished command; output belongs to IO, not Result.
type Result struct {
	code int
	err  error
}

// OK reports whether the command exited successfully.
func (r Result) OK() bool {
	return r.err == nil && r.code == 0
}

// Code returns the process exit code, or 1 for non-exit failures.
func (r Result) Code() int {
	return r.code
}

// Error returns the execution error, if any.
func (r Result) Error() error {
	return r.err
}

// OrExit exits the program if the result is not OK.
func (r Result) OrExit() Result {
	if r.OK() {
		return r
	}

	if r.err != nil {
		fmt.Fprintln(os.Stderr, r.err)
	}

	code := r.code
	if code == 0 {
		code = 1
	}
	os.Exit(code)
	return r
}

// OrExitf prints a message and exits the program if the result is not OK.
func (r Result) OrExitf(format string, args ...any) Result {
	if r.OK() {
		return r
	}

	fmt.Fprintf(os.Stderr, format, args...)
	fmt.Fprintln(os.Stderr)
	return r.OrExit()
}
