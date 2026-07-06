package app

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/gornkit/gorn/pkg/gornparser"
)

type RunOpts struct {
	Stdout    io.Writer
	Stderr    io.Writer
	Verbose   bool
	PrintGen  bool
	PrintMod  bool
	PrintMain bool
	Script    string
	Args      []string
}

func RunCmd(o RunOpts) error {
	script, err := parseScript(o.Script)
	if err != nil {
		return fmt.Errorf("parse %s: %w", o.Script, err)
	}

	if o.Verbose {
		_, _ = fmt.Fprintln(o.Stderr, "--- invocation ---")
		_, _ = fmt.Fprintf(o.Stderr, "Script: %s\n", o.Script)
		_, _ = fmt.Fprintf(o.Stderr, "Args:   %v\n", o.Args)
		flags := []string{}
		if o.PrintGen {
			flags = append(flags, "--print-gen")
		}
		if o.PrintMod {
			flags = append(flags, "--print-mod")
		}
		if o.PrintMain {
			flags = append(flags, "--print-main")
		}
		_, _ = fmt.Fprintf(o.Stderr, "Flags:  %v\n", flags)
		script.Dump(o.Stderr)
	}

	// Generation is the validation step: always generate so an invalid script
	// (e.g. a preamble import conflict) is surfaced, even for a plain run.
	gen, err := gornparser.Generate(script)
	if err != nil {
		// A format failure carries the raw, unformatted main file; dump it to
		// stderr for debugging before surfacing the error.
		var genErr *gornparser.GenerateError
		if errors.As(err, &genErr) && genErr.Raw != nil {
			_, _ = fmt.Fprintln(o.Stderr, "--- generated main.go (unformatted) ---")
			_, _ = fmt.Fprint(o.Stderr, string(genErr.Raw))
		}
		return fmt.Errorf("generate: %w", err)
	}

	// The print flags are inspect-only: print and do not run.
	if o.PrintGen || o.PrintMod || o.PrintMain {
		printArtifacts(o, gen)
		return nil
	}

	_, _ = fmt.Fprintln(o.Stderr, "gorn: run pipeline not implemented yet; use --print-gen to inspect generated output")
	return nil
}

// printArtifacts prints the requested generated artifacts to stdout. When more
// than one is requested it prefixes headers; a single artifact is emitted raw
// so it stays pipeable.
func printArtifacts(o RunOpts, gen *gornparser.Generated) {
	printMod := o.PrintGen || o.PrintMod
	printMain := o.PrintGen || o.PrintMain
	withHeaders := printMod && printMain

	if printMod {
		if withHeaders {
			_, _ = fmt.Fprintln(o.Stdout, "// --- go.mod ---")
		}
		_, _ = fmt.Fprint(o.Stdout, string(gen.ModGenerated))
	}
	if printMain {
		if withHeaders {
			_, _ = fmt.Fprintln(o.Stdout, "// --- main.go ---")
		}
		_, _ = fmt.Fprint(o.Stdout, string(gen.MainFileFormatted))
	}
}

// parseScript wraps gornparser, handling the "-" (stdin) convention that
// scriptPath already validated for us.
func parseScript(path string) (*gornparser.Script, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		return gornparser.ParseSource("-", data)
	}
	return gornparser.ParseFile(path)
}
