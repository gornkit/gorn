package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"

	"github.com/gornkit/gorn/internal/process"
	"github.com/gornkit/gorn/internal/progress"
	"github.com/gornkit/gorn/internal/script"
)

// RunOpts configures a single Run invocation.
type RunOpts struct {
	Input          Input
	HelperSources  fs.FS
	Stdout         io.Writer
	Stderr         io.Writer
	Verbose        bool
	Quiet          bool
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
		if r.Quiet {
			flags = append(flags, "--quiet")
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
	return RunContext(context.Background(), o)
}

// RunContext executes an invocation with cancellation. Cancellable stdin source
// reads require Input.Interrupt or CollectSource; the caller retains IO ownership.
func RunContext(ctx context.Context, o RunOpts) (resultErr error) {
	if ctx == nil {
		return fmt.Errorf("nil invocation context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	runtimeOpts := o
	sourceContext := ctx
	ctx, report := progress.New(ctx, o.Stderr, o.Quiet)
	if o.ScriptPath != "-" || sourceContext.Done() != nil || o.Input.Interrupt != nil || o.Input.CollectSource != nil {
		sourceContext = ctx
	}
	// A legacy background stdin read has no interruption authorization. Do not
	// turn it into a cancellable reader merely by attaching internal reporting.
	defer func() {
		if err := report.Finish(); err != nil {
			resultErr = &StatusError{Code: 1, Err: fmt.Errorf("write output: %w", errors.Join(err, resultErr))}
		}
	}()
	o.Stdout, o.Stderr = report.Synchronize(o.Stdout), report.Writer()
	if err := progress.Phase(ctx, "reading source"); err != nil {
		return err
	}
	run := func(bin string) error {
		if err := progress.Phase(ctx, "running script"); err != nil {
			return err
		}
		if err := report.Close(); err != nil {
			return err
		}
		// Direct *os.File streams preserve tty identity. No parent heartbeat
		// remains to race script output or write from a background tty group.
		return execCmd(ctx, runtimeOpts, bin)
	}
	source, gen, err := generateScript(ctx, sourceContext, o)
	if err != nil {
		return err
	}

	// The print flags are inspect-only: print and do not run.
	if o.PrintMod || o.PrintMain {
		return printArtifacts(o, gen)
	}

	entry, err := compileScript(ctx, o, source, gen)
	if err != nil {
		return err
	}
	defer closeEntry(entry, runtimeOpts.Stderr, &resultErr)

	if o.Verbose {
		switch {
		case o.NoCache:
			_, _ = fmt.Fprintln(o.Stderr, "cache: bypass (--no-cache)")
		case entry.CacheHit:
			_, _ = fmt.Fprintf(o.Stderr, "cache: hit %s\n", entry.AppPath)
		default:
			_, _ = fmt.Fprintf(o.Stderr, "cache: miss, cached %s\n", entry.AppRoot)
		}
	}

	return run(entry.AppPath)
}

func execCmd(ctx context.Context, o RunOpts, appBin string) error {
	cmd := exec.Command(appBin, o.Args...) //nolint:gosec  // G204
	cmd.Stdout = o.Stdout
	cmd.Stderr = o.Stderr
	cmd.Stdin = o.Input.Reader
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}

	p, err := process.Start(ctx, cmd, true)
	if err != nil {
		return scriptError(err)
	}
	if err := p.Wait(); err != nil {
		return scriptError(err)
	}

	return nil
}

// printArtifacts prints the requested generated artifacts to stdout. When both
// are requested it prefixes headers; a single artifact is emitted raw so it
// stays pipeable.
func printArtifacts(o RunOpts, gen *script.Generated) error {
	withHeaders := o.PrintMod && o.PrintMain
	for _, artifact := range []struct {
		enabled bool
		name    string
		data    []byte
	}{
		{o.PrintMod, "go.mod", gen.ModGenerated},
		{o.PrintMain, "main.go", gen.MainGenerated},
	} {
		if !artifact.enabled {
			continue
		}
		if withHeaders {
			header := fmt.Sprintf("// --- %s ---\n", artifact.name)
			n, err := io.WriteString(o.Stdout, header)
			if err == nil && n != len(header) {
				err = io.ErrShortWrite
			}
			if err != nil {
				return fmt.Errorf("write %s header: %w", artifact.name, err)
			}
		}
		n, err := o.Stdout.Write(artifact.data)
		if err == nil && n != len(artifact.data) {
			err = io.ErrShortWrite
		}
		if err != nil {
			return fmt.Errorf("write %s: %w", artifact.name, err)
		}
	}
	return nil
}
