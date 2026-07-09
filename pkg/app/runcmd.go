package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/gornkit/gorn/pkg/fs"
	"github.com/gornkit/gorn/pkg/gornparser"
)

type RunOpts struct {
	Stdout     io.Writer
	Stderr     io.Writer
	Verbose    bool
	PrintGen   bool
	PrintMod   bool
	PrintMain  bool
	NoCache    bool
	ScriptPath string
	Args       []string
}

func RunCmd(o RunOpts) error {
	source, err := readSource(o.ScriptPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", o.ScriptPath, err)
	}
	if o.Verbose {
		_, _ = fmt.Fprintf(o.Stderr, "--- script source (%d bytes) ---\n", len(source))
		_, _ = fmt.Fprint(o.Stderr, string(source))
	}
	if len(source) == 0 {
		return fmt.Errorf("script %s is empty", o.ScriptPath)
	}

	appKey := fs.GenerateAppKey(o.ScriptPath, source)

	cacheRoot, err := fs.NewCacheRoot()
	if err != nil {
		return err
	}

	if !o.NoCache {
		if appBin, ok := cacheRoot.CachedBin(appKey); ok {
			if o.Verbose {
				_, _ = fmt.Fprintf(o.Stderr, "using cached bin: %s\n", appBin)
			}
			return execCmd(o, appBin)
		}
	}

	script, err := gornparser.ParseSource(parsePath(o.ScriptPath), source)
	if err != nil {
		return fmt.Errorf("parse %s: %w", o.ScriptPath, err)
	}

	if o.Verbose {
		_, _ = fmt.Fprintln(o.Stderr, "--- invocation ---")
		_, _ = fmt.Fprintf(o.Stderr, "Script: %s\n", o.ScriptPath)
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

	// Cache miss: serialize builders for this key so concurrent runs don't race
	// on the same app dir.
	lock, err := cacheRoot.Lock(appKey)
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer func() { _ = lock.Release() }()

	// Re-check: a peer may have finished the build while we waited for the lock.
	if !o.NoCache {
		if appBin, ok := cacheRoot.CachedBin(appKey); ok {
			if o.Verbose {
				_, _ = fmt.Fprintf(o.Stderr, "using cached bin: %s\n", appBin)
			}
			return execCmd(o, appBin)
		}
	}

	emitted, err := fs.Emit(cacheRoot, script, gen, appKey)
	if err != nil {
		return fmt.Errorf("emit: %w", err)
	}

	return execCmd(o, emitted.AppPath)
}

func execCmd(o RunOpts, appBin string) error {
	cmd := exec.Command(appBin, o.Args...) //nolint:gosec  // G204
	cmd.Stdout = o.Stdout
	cmd.Stderr = o.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run: %w", err)
	}

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

func readSource(path string) ([]byte, error) {
	switch path {
	case "":
		return nil, fmt.Errorf("script path is empty")
	case "-":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return data, nil
	default:
		data, err := os.ReadFile(path) //nolint:gosec // G304: File path provided by user. Disabling because this is a CLI tool, not a server.
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		return data, nil
	}
}

// parsePath maps the "-" stdin convention to a display path for the parser;
// the source bytes are read once by the caller and passed to ParseSource.
func parsePath(path string) string {
	if path == "-" {
		return "/dev/stdin"
	}
	return path
}
