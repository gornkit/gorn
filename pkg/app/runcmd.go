package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gornkit/gorn/pkg/fs"
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
	source, pathLabel, absPath, err := readSource(o.Script)
	if err != nil {
		return fmt.Errorf("read %s: %w", o.Script, err)
	}

	script, err := gornparser.ParseSource(pathLabel, source)
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

	cacheRoot, err := fs.NewCacheRoot()
	if err != nil {
		return fmt.Errorf("cache root: %w", err)
	}

	appKey := fs.GenerateAppKey(absPath, source)

	// Fast path: binary already cached and valid.
	if bin, ok := cacheRoot.CachedBin(appKey); ok {
		return execCmd(bin, o)
	}

	// Slow path: acquire the per-app lock before building.
	lock, err := cacheRoot.Lock(appKey)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer func() { _ = lock.Release() }()

	// Re-check after acquiring the lock: a concurrent builder may have
	// finished while we were waiting.
	if bin, ok := cacheRoot.CachedBin(appKey); ok {
		return execCmd(bin, o)
	}

	emitted, err := fs.Emit(cacheRoot, script, gen, appKey)
	if err != nil {
		return fmt.Errorf("emit: %w", err)
	}

	return execCmd(emitted.BinFile, o)
}

// execCmd runs the compiled binary, forwarding stdout/stderr from o and
// passing any extra args.
func execCmd(bin string, o RunOpts) error {
	cmd := exec.Command(bin, o.Args...) //nolint:gosec // bin is a path we just compiled
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

// readSource reads the script source bytes and returns them along with:
//   - pathLabel: the label to use for parse error messages ("-" for stdin)
//   - absPath: the absolute path used to generate the app cache key (empty
//     string for stdin, which uses "-")
func readSource(path string) (source []byte, pathLabel, absPath string, err error) {
	if path == "-" {
		data, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			return nil, "", "", readErr
		}
		return data, "-", "-", nil
	}

	abs, absErr := filepath.Abs(path)
	if absErr != nil {
		return nil, "", "", absErr
	}

	data, readErr := os.ReadFile(abs) //nolint:gosec // path is user-provided script
	if readErr != nil {
		return nil, "", "", readErr
	}

	return data, abs, abs, nil
}
