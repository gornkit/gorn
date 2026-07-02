package sh

import (
	"fmt"
	"os"
)

// OrExit exits with status 1 if err is non-nil.
func OrExit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// OrExitf prints a message and exits with status 1 if err is non-nil.
func OrExitf(err error, format string, args ...any) {
	if err != nil {
		fmt.Fprintf(os.Stderr, format, args...)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
