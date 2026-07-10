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

	cacheRoot, err := fs.NewCacheRoot()
	if err != nil {
		return fmt.Errorf("cache root: %w", err)
	}

	appKey := fs.GenerateAppKey(absPath, source)

	// The print flags are inspect-only. On a cache hit, print directly from
	// cached generated files; otherwise parse and generate in-memory.
	if o.PrintGen || o.PrintMod || o.PrintMain {
		if cached, ok := cacheRoot.CachedGeneratedFiles(appKey); ok {
			printArtifactsFromBytes(o, cached.ModGenerated, cached.MainFileFormatted)
			return nil
		}

		script, parseErr := gornparser.ParseSource(pathLabel, source)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", o.Script, parseErr)
		}
		gen, genErr := generateSource(o, script)
		if genErr != nil {
			return genErr
		}
		printArtifacts(o, gen)
		return nil
	}

	// Fast path: binary already cached and valid — skip parse and generate.
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

	// Cache miss: parse and generate are deferred to here to avoid the work
	// on a cache hit.
	script, err := gornparser.ParseSource(pathLabel, source)
	if err != nil {
		return fmt.Errorf("parse %s: %w", o.Script, err)
	}

	if o.Verbose {
		_, _ = fmt.Fprintln(o.Stderr, "--- invocation ---")
		_, _ = fmt.Fprintf(o.Stderr, "Script: %s\n", o.Script)
		_, _ = fmt.Fprintf(o.Stderr, "Args:   %v\n", o.Args)
		script.Dump(o.Stderr)
	}

	gen, err := generateSource(o, script)
	if err != nil {
		return err
	}

	emitted, err := fs.Emit(cacheRoot, script, gen, appKey)
	if err != nil {
		return fmt.Errorf("emit: %w", err)
	}

	return execCmd(emitted.BinFile, o)
}

// generateSource runs gornparser.Generate, dumping the unformatted output to
// stderr on a format failure before returning the error.
func generateSource(o RunOpts, script *gornparser.Script) (*gornparser.Generated, error) {
	gen, err := gornparser.Generate(script)
	if err != nil {
		var genErr *gornparser.GenerateError
		if errors.As(err, &genErr) && genErr.Raw != nil {
			_, _ = fmt.Fprintln(o.Stderr, "--- generated main.go (unformatted) ---")
			_, _ = fmt.Fprint(o.Stderr, string(genErr.Raw))
		}
		return nil, fmt.Errorf("generate: %w", err)
	}
	return gen, nil
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
	printArtifactsFromBytes(o, gen.ModGenerated, gen.MainFileFormatted)
}

func printArtifactsFromBytes(o RunOpts, modGenerated, mainFileFormatted []byte) {
	printMod := o.PrintGen || o.PrintMod
	printMain := o.PrintGen || o.PrintMain
	withHeaders := printMod && printMain

	if printMod {
		if withHeaders {
			_, _ = fmt.Fprintln(o.Stdout, "// --- go.mod ---")
		}
		_, _ = fmt.Fprint(o.Stdout, string(modGenerated))
	}
	if printMain {
		if withHeaders {
			_, _ = fmt.Fprintln(o.Stdout, "// --- main.go ---")
		}
		_, _ = fmt.Fprint(o.Stdout, string(mainFileFormatted))
	}
}

// readSource reads the script source bytes and returns them along with:
//   - pathLabel: the label to use for parse error messages
//   - absPath: the absolute path used to generate the app cache key
func readSource(path string) (source []byte, pathLabel, absPath string, err error) {
	if path == "-" {
		data, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			return nil, "", "", readErr
		}
		return data, "/dev/stdin", "/dev/stdin", nil
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
