package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/gornkit/gorn/pkg/progress"
)

// BuildOpts configures standalone host-native binary export. Build never runs
// user code, including main/init. Output is required; its parent must exist.
type BuildOpts struct {
	Input         Input
	HelperSources fs.FS
	Stdout        io.Writer
	Stderr        io.Writer
	Verbose       bool
	Quiet         bool
	ScriptPath    string
	Output        string
	Force         bool
	NoCache       bool
}

func Build(o BuildOpts) error { return BuildContext(context.Background(), o) }

// BuildContext shares source validation, profile, resolver, and leased cache
// construction with RunContext, then publishes a private executable copy.
func BuildContext(ctx context.Context, o BuildOpts) (resultErr error) {
	if ctx == nil {
		return fmt.Errorf("nil build context")
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
	destination, err := outputPath(o.Output)
	if err != nil {
		return err
	}
	sourcePath := o.ScriptPath
	if sourcePath == "-" {
		sourcePath = ""
	}
	if err := checkDestination(destination, sourcePath, o.Force); err != nil {
		return err
	}
	sourceContext := ctx
	ctx, report := progress.New(ctx, o.Stderr, o.Quiet)
	if o.ScriptPath != "-" || sourceContext.Done() != nil || o.Input.Interrupt != nil || o.Input.CollectSource != nil {
		sourceContext = ctx
	}
	defer func() {
		if err := report.Finish(); err != nil {
			resultErr = &StatusError{Code: 1, Err: fmt.Errorf("write output: %w", errors.Join(err, resultErr))}
		}
	}()
	prep := RunOpts{Input: o.Input, HelperSources: o.HelperSources, Stdout: report.Synchronize(o.Stdout), Stderr: report.Writer(), Verbose: o.Verbose, Quiet: o.Quiet, ScriptPath: o.ScriptPath, NoCache: o.NoCache}
	if err := progress.Phase(ctx, "reading source"); err != nil {
		return err
	}
	source, gen, err := generateScript(ctx, sourceContext, prep)
	if err != nil {
		return err
	}
	entry, err := compileScript(ctx, prep, source, gen)
	if err != nil {
		return err
	}
	defer closeEntry(entry, report.Writer(), &resultErr)
	if o.Verbose {
		state := "miss"
		if o.NoCache {
			state = "bypass"
		} else if entry.CacheHit {
			state = "hit"
		}
		_, _ = fmt.Fprintf(prep.Stderr, "cache: %s %s\n", state, entry.AppRoot)
	}
	if err := progress.Phase(ctx, "exporting binary"); err != nil {
		return err
	}
	if err := exportExecutable(ctx, entry.AppPath, destination, sourcePath, o.Force); err != nil {
		return fmt.Errorf("export binary: %w", err)
	}
	if o.Verbose {
		_, _ = fmt.Fprintf(prep.Stderr, "output: %s\n", destination)
	}
	return nil
}
