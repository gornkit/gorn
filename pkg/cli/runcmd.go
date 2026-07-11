package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/gornkit/gorn/pkg/app"
	"github.com/gornkit/gorn/pkg/cache"
	"github.com/gornkit/gorn/pkg/script"
)

// RunOpts configures a single Run invocation.
type RunOpts struct {
	Stdout         io.Writer
	Stderr         io.Writer
	Verbose        bool
	PrintMod       bool
	PrintMain      bool
	NoCache        bool
	ScriptPath     string
	Args           []string
	invocationText string
}

// invocation returns a human-readable summary of this invocation (script,
// args, flags) for verbose output. The result is memoized.
func (r *RunOpts) invocation() string {
	if r.invocationText == "" {
		var builder strings.Builder
		builder.WriteString("--- invocation ---\n")
		fmt.Fprintf(&builder, "Script: %s\n", r.ScriptPath)
		fmt.Fprintf(&builder, "Args:   %v\n", r.Args)
		flags := []string{}
		if r.PrintMod {
			flags = append(flags, "--print-mod")
		}
		if r.PrintMain {
			flags = append(flags, "--print-main")
		}
		if r.NoCache {
			flags = append(flags, "--no-cache")
		}
		fmt.Fprintf(&builder, "Flags:  %v\n", flags)

		r.invocationText = builder.String()
	}

	return r.invocationText
}

// Run reads, parses, generates, builds (on cache miss), and runs the script
// named by o.ScriptPath. Print flags short-circuit to emit artifacts instead of
// running.
func Run(o RunOpts) error {
	source, err := readSource(o.ScriptPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", o.ScriptPath, err)
	}

	cacheRoot, err := cache.Open()
	if err != nil {
		return err
	}

	if !o.NoCache {
		if appBin, ok := cacheRoot.Lookup(source); ok {
			if o.Verbose {
				_, _ = fmt.Fprintf(o.Stderr, "cache: hit %s\n", appBin)
			}
			return execCmd(o, appBin)
		}
	}

	file, err := script.Parse(source)
	if err != nil {
		return fmt.Errorf("parse %s: %w", o.ScriptPath, err)
	}

	if o.Verbose {
		_, _ = o.Stderr.Write([]byte(o.invocation()))
	}

	// Generation is the validation step: always generate so an invalid script
	// (e.g. a preamble import conflict) is surfaced, even for a plain run.
	gen, err := script.Generate(file)
	if err != nil {
		// A format failure carries the raw, unformatted main file; dump it to
		// stderr for debugging before surfacing the error.
		var genErr *script.GenerateError
		if errors.As(err, &genErr) && genErr.Raw != nil {
			_, _ = fmt.Fprintln(o.Stderr, "--- generated main.go (unformatted) ---")
			_, _ = fmt.Fprint(o.Stderr, string(genErr.Raw))
		}
		return fmt.Errorf("generate: %w", err)
	}

	// The print flags are inspect-only: print and do not run.
	if o.PrintMod || o.PrintMain {
		printArtifacts(o, gen)
		return nil
	}

	entry, err := cacheRoot.Build(source, gen, o.NoCache)
	if err != nil {
		return fmt.Errorf("emit: %w", err)
	}
	if o.NoCache {
		// Bypass build lives in a temp dir; remove it once the run finishes.
		defer func() { _ = entry.Remove() }()
	}
	if o.Verbose {
		if o.NoCache {
			_, _ = fmt.Fprintln(o.Stderr, "cache: bypass (--no-cache)")
		} else {
			_, _ = fmt.Fprintf(o.Stderr, "cache: miss, cached %s\n", entry.AppRoot)
		}
	}

	return execCmd(o, entry.AppPath)
}

func execCmd(o RunOpts, appBin string) error {
	cmd := exec.Command(appBin, o.Args...) //nolint:gosec  // G204
	cmd.Stdout = o.Stdout
	cmd.Stderr = o.Stderr
	// TODO thread in through Opts
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run: %w", err)
	}

	return nil
}

// printArtifacts prints the requested generated artifacts to stdout. When both
// are requested it prefixes headers; a single artifact is emitted raw so it
// stays pipeable.
func printArtifacts(o RunOpts, gen *script.Generated) {
	withHeaders := o.PrintMod && o.PrintMain

	if o.PrintMod {
		if withHeaders {
			_, _ = fmt.Fprintln(o.Stdout, "// --- go.mod ---")
		}
		_, _ = fmt.Fprint(o.Stdout, string(gen.ModGenerated))
	}
	if o.PrintMain {
		if withHeaders {
			_, _ = fmt.Fprintln(o.Stdout, "// --- main.go ---")
		}
		_, _ = fmt.Fprint(o.Stdout, string(gen.MainFileFormatted))
	}
}

func readSource(path string) (*app.Source, error) {
	switch path {
	case "":
		return nil, fmt.Errorf("script path is empty")
	case "-":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return app.New(path, data)
	default:
		data, err := os.ReadFile(path) //nolint:gosec // G304: File path provided by user. Disabling because this is a CLI tool, not a server.
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		return app.New(path, data)
	}
}
