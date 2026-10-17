package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/gornkit/gorn/internal/app"
	"github.com/gornkit/gorn/internal/buildprofile"
	"github.com/gornkit/gorn/internal/cache"
	"github.com/gornkit/gorn/internal/progress"
	"github.com/gornkit/gorn/internal/script"
)

func generateScript(ctx, sourceContext context.Context, o RunOpts) (*app.Source, *script.Generated, error) {
	source, err := readSource(sourceContext, o.ScriptPath, o.Input)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", o.ScriptPath, err)
	}

	file, err := script.Parse(source)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", o.ScriptPath, err)
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
		if o.Verbose && errors.As(err, &genErr) && genErr.Raw != nil {
			_, _ = fmt.Fprintln(o.Stderr, "--- invalid generated main.go ---")
			_, _ = fmt.Fprint(o.Stderr, string(genErr.Raw))
		}
		return nil, nil, fmt.Errorf("generate: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	return source, gen, nil
}

func compileScript(ctx context.Context, o RunOpts, source *app.Source, gen *script.Generated) (cache.Entry, error) {
	var err error
	var bundle *script.HelperBundle
	helperDigest := ""
	if gen.BundleHelper {
		bundle, err = script.LoadHelper(o.HelperSources)
		if err != nil {
			return cache.Entry{}, err
		}
		helperDigest = bundle.Digest()
	}
	if len(gen.EmbedLines) > 0 {
		return cache.Entry{}, fmt.Errorf("%s: source //go:embed is unsupported in a single-file app at line %d", gen.SourcePath, gen.EmbedLines[0])
	}
	for _, imp := range gen.Imports {
		if imp.Path == "C" {
			return cache.Entry{}, fmt.Errorf("%s: import C is unsupported by the pure-Go build profile at line %d", gen.SourcePath, imp.Line)
		}
	}
	profile, err := buildprofile.DiscoverContext(ctx, gen.GoVersion, helperDigest)
	if err != nil {
		return cache.Entry{}, fmt.Errorf("build profile: %w", err)
	}
	source = source.WithBuildIdentity(profile.Identity())
	if err := ctx.Err(); err != nil {
		return cache.Entry{}, err
	}
	if err := progress.Phase(ctx, "checking build cache"); err != nil {
		return cache.Entry{}, err
	}

	cacheRoot, err := cache.Open()
	if err != nil {
		return cache.Entry{}, err
	}

	entry, err := cacheRoot.BuildContext(ctx, source, gen, o.NoCache, bundle, profile)
	if err != nil {
		return cache.Entry{}, fmt.Errorf("emit: %w", err)
	}
	return entry, nil
}

func closeEntry(entry cache.Entry, stderr io.Writer, resultErr *error) {
	if err := entry.Close(); err != nil {
		cleanupErr := fmt.Errorf("close cache entry: %w", err)
		if *resultErr == nil {
			*resultErr = cleanupErr
		} else {
			_, _ = fmt.Fprintln(stderr, cleanupErr)
		}
	}
}
