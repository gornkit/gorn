package sh

import (
	"fmt"
	"os"
	"strings"
)

type Result struct {
	Op  string
	Cmd struct {
		Name string
		Args []string
	}
	Code           int
	Err            error
	Stdout         string
	Stderr         string
	StdoutCaptured bool
	StderrCaptured bool
}

// OK returns true if the command executed successfully (exit code 0 and no error).
func (r Result) OK() bool { return r.Code == 0 && r.Err == nil }

// Error returns the error associated with the command execution, if any.
func (r Result) Error() error { return r.Err }

// OrExit checks if the command executed successfully.
// If not, it prints the error details to stderr and
// exits the program with the appropriate exit code.
func (r Result) OrExit() Result {
	if r.OK() {
		return r
	}

	fmt.Fprint(os.Stderr, formatFailure(r))
	if r.StderrCaptured {
		fmt.Fprintln(os.Stderr, r.Stderr)
	}
	code := r.Code
	if code == 0 {
		code = 1
	}

	os.Exit(code)
	return Result{}
}

// OrExitf checks if the command executed successfully.
// If not, it prints a formatted error message to stderr and
// exits the program with the appropriate exit code.
func (r Result) OrExitf(format string, args ...any) Result {
	if r.OK() {
		return r
	}

	fmt.Fprintf(os.Stderr, format, args...)
	fmt.Fprintln(os.Stderr)
	return r.OrExit()
}

func formatFailure(r Result) string {
	bld := new(strings.Builder)

	if r.Op != "" {
		fmt.Fprintf(bld, "operation: %s\n", r.Op)
		fmt.Fprintf(bld, "cause: %v\n", r.Err)
	} else {
		fmt.Fprintf(bld, "command: %s %s\n", r.Cmd.Name, strings.Join(r.Cmd.Args, " "))
		fmt.Fprintf(bld, "exit status: %d\n", r.Code)
	}

	return bld.String()
}
