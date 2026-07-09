package fs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	gp "github.com/gornkit/gorn/pkg/gornparser"
)

// Emitted holds the paths of artefacts produced by a successful Emit call.
type Emitted struct {
	// AppDir is the directory holding the binary and manifest.
	AppDir string
	// BinFile is the path to the compiled binary.
	BinFile string
}

// Emit builds the script described by s and g, caches the result under
// cacheRoot, and returns the paths of the produced artefacts. The build is
// atomic: the binary is compiled into a temporary directory and renamed into
// place only after the build succeeds, so a concurrent reader never observes a
// partial cache slot.
//
// The manifest is written last so that CachedBin's key-equality check acts as
// a validity gate: a slot without a manifest (or with a mismatched key) is
// treated as a miss.
//
// Callers are responsible for acquiring a CacheRoot.Lock before calling Emit
// and releasing it after, to avoid redundant concurrent builds.
//
// Note: the tmp dir and the app dir must be on the same filesystem so that the
// rename at the end of Emit is atomic. Both live under the same CacheRoot, so
// this is always satisfied.
func Emit(cacheRoot CacheRoot, s *gp.Script, g *gp.Generated, appKey AppKey) (Emitted, error) {
	// Ensure the tmp directory exists under the cache root so that
	// os.MkdirTemp places the build directory on the same filesystem as the
	// final app directory. This makes the later os.Rename atomic.
	if err := os.MkdirAll(cacheRoot.TmpDir(), 0o700); err != nil {
		return Emitted{}, fmt.Errorf("create tmp dir: %w", err)
	}

	buildDir, err := os.MkdirTemp(cacheRoot.TmpDir(), "build-")
	if err != nil {
		return Emitted{}, fmt.Errorf("create build dir: %w", err)
	}
	// Always clean up the temp dir on error; on success the rename moves it
	// away so RemoveAll is a no-op.
	defer func() { _ = os.RemoveAll(buildDir) }()

	// Write go.mod and the generated main file.
	if err := os.WriteFile(filepath.Join(buildDir, "go.mod"), g.ModGenerated, 0o600); err != nil {
		return Emitted{}, fmt.Errorf("write go.mod: %w", err)
	}
	if err := os.WriteFile(filepath.Join(buildDir, "main.gorn.go"), g.MainFileFormatted, 0o600); err != nil {
		return Emitted{}, fmt.Errorf("write main.gorn.go: %w", err)
	}

	// go mod tidy resolves any indirect dependencies.
	tidy := exec.Command("go", "mod", "tidy") //nolint:gosec // arguments are literals
	tidy.Dir = buildDir
	if out, err := tidy.CombinedOutput(); err != nil {
		return Emitted{}, fmt.Errorf("go mod tidy: %w\n%s", err, out)
	}

	// Compile the binary into a bin/ subdirectory of the build dir.
	binDir := filepath.Join(buildDir, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return Emitted{}, fmt.Errorf("create bin dir: %w", err)
	}
	outBin := filepath.Join(binDir, binName())
	build := exec.Command("go", "build", "-o", outBin, ".") //nolint:gosec // arguments are literals
	build.Dir = buildDir
	if out, err := build.CombinedOutput(); err != nil {
		return Emitted{}, fmt.Errorf("go build: %w\n%s", err, out)
	}

	// Write the manifest last — CachedBin treats a missing/mismatched manifest
	// as a cache miss, so writing it last means a partial build dir is never
	// mistaken for a valid cache hit.
	m := newManifest(appKey, s.SourcePath)
	if err := WriteManifest(buildDir, m); err != nil {
		return Emitted{}, fmt.Errorf("write manifest: %w", err)
	}

	// Atomically replace the app slot: remove any existing slot, then rename
	// the completed build dir into place. The rename is atomic on the same
	// filesystem (guaranteed because buildDir is under cacheRoot.TmpDir()).
	appDir := cacheRoot.AppDir(appKey)
	if err := os.RemoveAll(appDir); err != nil {
		return Emitted{}, fmt.Errorf("remove old app dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(appDir), 0o700); err != nil {
		return Emitted{}, fmt.Errorf("create apps dir: %w", err)
	}
	if err := os.Rename(buildDir, appDir); err != nil {
		return Emitted{}, fmt.Errorf("install app dir: %w", err)
	}

	return Emitted{
		AppDir:  appDir,
		BinFile: cacheRoot.AppBinFile(appKey),
	}, nil
}

// Clean removes all artefacts associated with e. It is safe to call on a
// zero-value Emitted (no-op).
func Clean(e Emitted) error {
	if e.AppDir == "" {
		return nil
	}
	return os.RemoveAll(e.AppDir)
}
