package fs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	gp "github.com/gornkit/gorn/pkg/gornparser"
)

type Emitted struct {
	AppRoot      string
	MainFilePath string
	ModFilePath  string
	AppPath      string
}

// Emit builds the generated app for appKey. It builds into a private temp dir
// under the cache and atomically renames the finished dir into place, so a
// concurrent or interrupted run never sees a half-built entry. The manifest is
// written last, marking the entry complete. Callers should hold the per-appKey
// lock (see CacheRoot.Lock) so the rename doesn't race a peer builder.
func Emit(cacheRoot CacheRoot, s *gp.Script, g *gp.Generated, appKey AppKey) (Emitted, error) {
	result := Emitted{}

	tmpParent := cacheRoot.TmpDir()
	if err := os.MkdirAll(tmpParent, 0o700); err != nil {
		return result, err
	}
	// ponytail: temp and final dirs share the cache root, so os.Rename below
	// stays on one filesystem (rename across filesystems fails).
	buildDir, err := os.MkdirTemp(tmpParent, "build-")
	if err != nil {
		return result, err
	}
	// Clean the temp dir on any early return; the successful path renames it
	// away first, so RemoveAll then no-ops.
	defer func() { _ = os.RemoveAll(buildDir) }()

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

	build := exec.Command("go", "build", "-o", filepath.Join("bin", binName())) //nolint:gosec // G204: args are gorn-generated, not user input.
	build.Dir = buildDir
	if out, err := build.CombinedOutput(); err != nil {
		return result, fmt.Errorf("go build: %w\n%s", err, out)
	}

	// Manifest last: its presence means the entry is complete and valid.
	if err := WriteManifest(buildDir, newManifest(appKey, s.SourcePath)); err != nil {
		return result, err
	}

	appRoot := cacheRoot.AppDir(appKey)
	if err := os.MkdirAll(filepath.Dir(appRoot), 0o700); err != nil {
		return result, err
	}
	// Replace any stale entry, then move the fresh build into place.
	if err := os.RemoveAll(appRoot); err != nil {
		return result, err
	}
	if err := os.Rename(buildDir, appRoot); err != nil {
		return result, fmt.Errorf("install cache entry: %w", err)
	}

	result.AppRoot = appRoot
	result.MainFilePath = filepath.Join(appRoot, "main.gorn.go")
	result.ModFilePath = filepath.Join(appRoot, "go.mod")
	result.AppPath = cacheRoot.AppBinFile(appKey)
	return result, nil
}

func Clean(e Emitted) error {
	if e.AppRoot == "" {
		return nil
	}
	return os.RemoveAll(e.AppRoot)
}
