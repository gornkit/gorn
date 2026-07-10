package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// CacheRoot is the root directory of the gorn cache. All per-app
// subdirectories live beneath it.
type CacheRoot string

// NewCacheRoot returns the CacheRoot, resolved from the GORN_CACHE environment
// variable if set, or the OS-appropriate user cache directory otherwise.
func NewCacheRoot() (CacheRoot, error) {
	if dir := os.Getenv("GORN_CACHE"); dir != "" {
		return CacheRoot(dir), nil
	}

	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("gorn cache dir: %w", err)
	}

	return CacheRoot(filepath.Join(base, "gorn")), nil
}

// Path returns the raw string path of the cache root.
func (c CacheRoot) Path() string { return string(c) }

// TmpDir returns the directory used for temporary build artefacts.
// It is always a child of the cache root so that os.Rename from a temp build
// dir to AppDir is always on the same filesystem.
func (c CacheRoot) TmpDir() string { return filepath.Join(string(c), "tmp") }

// LocksDir returns the directory that holds per-app lock entries.
func (c CacheRoot) LocksDir() string { return filepath.Join(string(c), "locks") }

// AppDir returns the directory for a specific app's artefacts. The directory
// name is the first 12 hex characters of the AppKey — long enough to be
// collision-resistant for typical workloads, short enough for readability. The
// manifest inside the dir stores the full key for exact verification in
// CachedBin.
func (c CacheRoot) AppDir(appKey AppKey) string {
	return filepath.Join(string(c), "apps", string(appKey)[:12])
}

// AppBinDir returns the directory inside AppDir where the compiled binary lives.
func (c CacheRoot) AppBinDir(appKey AppKey) string {
	return filepath.Join(c.AppDir(appKey), "bin")
}

// AppBinFile returns the full path to the compiled binary for appKey.
func (c CacheRoot) AppBinFile(appKey AppKey) string {
	return filepath.Join(c.AppBinDir(appKey), binName())
}

// AppModFile returns the full path to the cached go.mod for appKey.
func (c CacheRoot) AppModFile(appKey AppKey) string {
	return filepath.Join(c.AppDir(appKey), "go.mod")
}

// AppMainFile returns the full path to the cached generated main file for appKey.
func (c CacheRoot) AppMainFile(appKey AppKey) string {
	return filepath.Join(c.AppDir(appKey), "main.gorn.go")
}

// binName returns the platform-appropriate name for the compiled app binary.
func binName() string {
	if runtime.GOOS == "windows" {
		return "app.exe"
	}
	return "app"
}

// CachedBin returns the path to a valid cached binary and true when the cache
// is a hit for appKey, or ("", false) on any miss (missing directory, missing
// manifest, schema mismatch, key mismatch, or missing binary). It never
// returns an error: any failure is treated as a cache miss so the caller
// rebuilds cleanly.
func (c CacheRoot) CachedBin(appKey AppKey) (string, bool) {
	m, err := LoadManifest(c.AppDir(appKey))
	if err != nil {
		return "", false
	}
	if m.Schema != manifestSchema {
		return "", false
	}
	if m.AppKey != string(appKey) {
		return "", false
	}

	bin := c.AppBinFile(appKey)
	if _, err := os.Stat(bin); err != nil {
		return "", false
	}

	return bin, true
}

// CachedGenerated holds cached generated source files from a valid app slot.
type CachedGenerated struct {
	ModGenerated      []byte
	MainFileFormatted []byte
}

// CachedGeneratedFiles returns generated source files when appKey is a valid
// cache hit; otherwise it returns ({}, false).
func (c CacheRoot) CachedGeneratedFiles(appKey AppKey) (CachedGenerated, bool) {
	if _, ok := c.CachedBin(appKey); !ok {
		return CachedGenerated{}, false
	}

	modFile, err := os.ReadFile(c.AppModFile(appKey)) //nolint:gosec // cache root is internal and key-gated
	if err != nil {
		return CachedGenerated{}, false
	}
	mainFile, err := os.ReadFile(c.AppMainFile(appKey)) //nolint:gosec // cache root is internal and key-gated
	if err != nil {
		return CachedGenerated{}, false
	}

	return CachedGenerated{
		ModGenerated:      modFile,
		MainFileFormatted: mainFile,
	}, true
}
