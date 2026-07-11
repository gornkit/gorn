package cache

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gornkit/gorn/pkg/app"
	"github.com/gornkit/gorn/pkg/script"
)

// Entry records the on-disk paths of a freshly installed cache entry.
type Entry struct {
	AppRoot      string
	MainFilePath string
	ModFilePath  string
	AppPath      string
}

// Build builds the generated app for s and returns its on-disk paths. It builds
// into a private temp dir. When noCache is false it installs the build into the
// cache: writing the manifest last and atomically renaming the finished dir
// into place, so a concurrent or interrupted run never sees a half-built entry.
// No lock is needed: the app dir is keyed on the content hash (and manifest
// schema), so concurrent builders produce identical output and the first rename
// wins; a builder whose rename loses the race yields to the peer's
// already-published, equivalent entry. When noCache is true it bypasses the
// cache entirely — no manifest, no publish — and hands the temp build dir back
// to the caller, who must call [Entry.Remove] after running.
func (r Root) Build(s *app.Source, g *script.Generated, noCache bool) (Entry, error) {
	result := Entry{}

	tmpParent := r.TmpDir()
	if err := os.MkdirAll(tmpParent, 0o700); err != nil {
		return result, err
	}
	// ponytail: temp and final dirs share the cache root, so os.Rename below
	// stays on one filesystem (rename across filesystems fails).
	buildDir, err := os.MkdirTemp(tmpParent, "build-")
	if err != nil {
		return result, err
	}
	// Clean the temp dir on error. On the cache path a successful publish
	// renames it away first (so RemoveAll no-ops); on the bypass path we hand it
	// to the caller and set handoff so it survives.
	handoff := false
	defer func() {
		if !handoff {
			_ = os.RemoveAll(buildDir)
		}
	}()

	if err := os.WriteFile(filepath.Join(buildDir, "main.gorn.go"), g.MainFileFormatted, 0o600); err != nil {
		return result, err
	}
	if err := os.WriteFile(filepath.Join(buildDir, "go.mod"), g.ModGenerated, 0o600); err != nil {
		return result, err
	}

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = buildDir
	if out, err := tidy.CombinedOutput(); err != nil {
		return result, fmt.Errorf("go mod tidy: %w\n%s", err, out)
	}

	build := exec.Command("go", "build", "-o", filepath.Join("bin", binName)) //nolint:gosec // G204: args are gorn-generated, not user input.
	build.Dir = buildDir
	if out, err := build.CombinedOutput(); err != nil {
		return result, fmt.Errorf("go build: %w\n%s", err, out)
	}

	if noCache {
		// Bypass: run straight from the temp dir; caller calls Entry.Remove.
		handoff = true
		result.AppRoot = buildDir
		result.MainFilePath = filepath.Join(buildDir, "main.gorn.go")
		result.ModFilePath = filepath.Join(buildDir, "go.mod")
		result.AppPath = filepath.Join(buildDir, "bin", binName)
		return result, nil
	}

	binSHA, err := fileSHA256(filepath.Join(buildDir, "bin", binName))
	if err != nil {
		return result, err
	}

	// Manifest last: its presence means the entry is complete and valid.
	if err := WriteManifest(buildDir, newManifest(s, binSHA)); err != nil {
		return result, err
	}

	if err := r.publish(buildDir, s); err != nil {
		return result, err
	}

	appRoot := r.Dir(s)
	result.AppRoot = appRoot
	result.MainFilePath = filepath.Join(appRoot, "main.gorn.go")
	result.ModFilePath = filepath.Join(appRoot, "go.mod")
	result.AppPath = r.BinPath(s)
	return result, nil
}

// publish atomically installs the finished buildDir as s's cache entry. The
// rename is the publish point. If it fails but a valid entry for s already
// exists, a peer published byte-identical content first, so we yield to theirs;
// any other rename failure is a real install error.
func (c Root) publish(buildDir string, s *app.Source) error {
	appRoot := c.Dir(s)
	if err := os.MkdirAll(filepath.Dir(appRoot), 0o700); err != nil {
		return err
	}
	if err := os.Rename(buildDir, appRoot); err != nil {
		if _, ok := c.Lookup(s); !ok {
			return fmt.Errorf("install cache entry: %w", err)
		}
	}
	return nil
}

// Remove removes the installed cache entry recorded in e. A zero Entry is a
// no-op.
func (e Entry) Remove() error {
	if e.AppRoot == "" {
		return nil
	}
	return os.RemoveAll(e.AppRoot)
}
