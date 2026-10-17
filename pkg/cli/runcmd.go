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

	"github.com/gornkit/gorn/pkg/buildprofile"
	"github.com/gornkit/gorn/pkg/cache"
	"github.com/gornkit/gorn/pkg/process"
	"github.com/gornkit/gorn/pkg/script"
)

// RunOpts configures a single Run invocation.
type RunOpts struct {
	Input          Input
	HelperSources  fs.FS
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
	return RunContext(context.Background(), o)
}

// RunContext executes an invocation with cancellation. Cancellable stdin source
// reads require Input.Interrupt; the caller retains IO ownership.
func RunContext(ctx context.Context, o RunOpts) error {
	if ctx == nil {
		return fmt.Errorf("nil invocation context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	source, err := readSource(ctx, o.ScriptPath, o.Input)
	if err != nil {
		return fmt.Errorf("read %s: %w", o.ScriptPath, err)
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
		// Internal generation failures carry the generated file for debugging.
		var genErr *script.GenerateError
		if errors.As(err, &genErr) && genErr.Raw != nil {
			_, _ = fmt.Fprintln(o.Stderr, "--- invalid generated main.go ---")
			_, _ = fmt.Fprint(o.Stderr, string(genErr.Raw))
		}
		return fmt.Errorf("generate: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// The print flags are inspect-only: print and do not run.
	if o.PrintMod || o.PrintMain {
		return printArtifacts(o, gen)
	}

	var bundle *script.HelperBundle
	helperDigest := ""
	if gen.BundleHelper {
		bundle, err = script.LoadHelper(o.HelperSources)
		if err != nil {
			return err
		}
		helperDigest = bundle.Digest()
	}
	if len(gen.EmbedLines) > 0 {
		return fmt.Errorf("%s: source //go:embed is unsupported in a single-file app at line %d", gen.SourcePath, gen.EmbedLines[0])
	}
	for _, imp := range gen.Imports {
		if imp.Path == "C" {
			return fmt.Errorf("%s: import C is unsupported by the pure-Go build profile at line %d", gen.SourcePath, imp.Line)
		}
	}
	profile, err := buildprofile.DiscoverContext(ctx, gen.GoVersion, helperDigest)
	if err != nil {
		return fmt.Errorf("build profile: %w", err)
	}
	source = source.WithBuildIdentity(profile.Identity())
	if err := ctx.Err(); err != nil {
		return err
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
			return execCmd(ctx, o, appBin)
		}
	}

	entry, err := cacheRoot.BuildContext(ctx, source, gen, o.NoCache, bundle, profile)
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

	return execCmd(ctx, o, entry.AppPath)
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
