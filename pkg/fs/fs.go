package fs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

type CacheRoot string

func NewCacheRoot() (CacheRoot, error) {
	root, err := getRoot()
	if err != nil {
		return "", err
	}
	return CacheRoot(root), nil
}

func (c CacheRoot) Path() string { return string(c) }

func (c CacheRoot) TmpDir() string { return filepath.Join(string(c), "tmp") }

func (c CacheRoot) LocksDir() string { return filepath.Join(string(c), "locks") }

func (c CacheRoot) AppDir(appKey AppKey) string {
	return filepath.Join(string(c), "apps", string(appKey)[:12])
}

func (c CacheRoot) AppBinDir(appKey AppKey) string {
	return filepath.Join(c.AppDir(appKey), "bin")
}

func binName() string {
	if runtime.GOOS == "windows" {
		return "app.exe"
	}
	return "app"
}

func (c CacheRoot) AppBinFile(appKey AppKey) string {
	return filepath.Join(c.AppBinDir(appKey), binName())
}

// CachedBin returns the path to a usable cached binary for appKey, and true,
// only when the app dir holds a manifest that parses, matches the full appKey,
// and the binary exists. Any failure is a cache miss, never an error: the
// caller rebuilds.
func (c CacheRoot) CachedBin(appKey AppKey) (string, bool) {
	dir := c.AppDir(appKey)
	m, err := LoadManifest(dir)
	if err != nil {
		return "", false
	}
	if m.Schema != manifestSchema || m.AppKey != string(appKey) {
		return "", false
	}
	bin := c.AppBinFile(appKey)
	if _, err := os.Stat(bin); err != nil {
		return "", false
	}
	return bin, true
}

func getRoot() (string, error) {
	// 1. GORN_CACHE
	// 2. UserCacheDir/gorn
	// 3. /tmp/gorn

	if cacheRoot := os.Getenv("GORN_CACHE"); cacheRoot != "" {
		if err := os.MkdirAll(cacheRoot, 0o700); err == nil {
			return cacheRoot, nil
		}
	}

	userCacheDir, err := os.UserCacheDir()
	if err == nil {
		gornCacheDir := filepath.Join(userCacheDir, "gorn")
		if err := os.MkdirAll(gornCacheDir, 0o700); err == nil {
			return gornCacheDir, nil
		}
	}

	tmpGorn := filepath.Join(os.TempDir(), "gorn")
	if err := os.MkdirAll(tmpGorn, 0o700); err == nil {
		return tmpGorn, nil
	}

	return "", errors.New("failed to determine cache root")
}
