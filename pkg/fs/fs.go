package fs

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	so "github.com/gornkit/gorn/pkg/source"
)

const keyPrefixLength int = 12

var binName = func() string {
	if runtime.GOOS == "windows" {
		return "app.exe"
	}
	return "app"
}()

// CacheRoot is the root directory of the gorn build cache.
type CacheRoot string

// NewCacheRoot resolves and creates the cache root (GORN_CACHE, else the user
// cache dir, else a temp dir).
func NewCacheRoot() (CacheRoot, error) {
	root, err := getRoot()
	if err != nil {
		return "", err
	}
	return CacheRoot(root), nil
}

// Path returns the cache root directory.
func (c CacheRoot) Path() string { return string(c) }

// TmpDir returns the directory used for in-progress builds.
func (c CacheRoot) TmpDir() string { return filepath.Join(string(c), "tmp") }

// AppDir returns the install directory for s, namespaced by manifest schema so
// a schema bump lands in a fresh namespace instead of colliding with stale
// entries. The dir is named on a 12-hex-char (48-bit) prefix of the full key
// plus the script's Slug for human browsability; the manifest still validates
// the full key, so a prefix collision never serves the wrong binary. It does,
// however, wedge the losing script (its build can't publish over the
// incumbent's dir) — astronomically unlikely, and the fix is to rename the
// script.
func (c CacheRoot) AppDir(s *so.Source) string {
	name := string(s.AppKey())[:keyPrefixLength]
	if slug := s.Slug(); slug != "" {
		name += "-" + slug
	}
	return filepath.Join(string(c), "apps", strconv.Itoa(manifestSchema), name)
}

// AppBinDir returns the bin subdirectory of s's install directory.
func (c CacheRoot) AppBinDir(s *so.Source) string {
	return filepath.Join(c.AppDir(s), "bin")
}

// AppBinFile returns the path to s's built binary.
func (c CacheRoot) AppBinFile(s *so.Source) string {
	return filepath.Join(c.AppBinDir(s), binName)
}

// CachedBin returns the path to a usable cached binary for s, and true, only
// when the app dir holds a manifest that parses and matches the full appKey,
// and the binary is a regular file whose checksum matches the manifest. Any
// failure is a cache miss, never an error: the caller rebuilds.
func (c CacheRoot) CachedBin(s *so.Source) (string, bool) {
	dir := c.AppDir(s)
	m, err := LoadManifest(dir)
	if err != nil {
		return "", false
	}
	if m.AppKey != string(s.AppKey()) {
		return "", false
	}
	bin := c.AppBinFile(s)
	info, err := os.Stat(bin)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	// ponytail: re-hashes the whole binary on every run to catch post-publish
	// storage corruption. Gate on size+mtime if this shows up in startup latency.
	sum, err := fileSHA256(bin)
	if err != nil || sum != m.BinSHA256 {
		return "", false
	}
	return bin, true
}

// fileSHA256 returns the hex SHA-256 of the file at path.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // G304: path is a gorn-computed cache path, not user input.
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func getRoot() (string, error) {
	// 1. GORN_CACHE
	// 2. UserCacheDir/gorn
	// 3. /tmp/gorn

	if cacheRoot := os.Getenv("GORN_CACHE"); cacheRoot != "" {
		if err := os.MkdirAll(cacheRoot, 0o700); err == nil { //nolint:gosec // G703: cacheRoot is an operator-set env var, not remote input.
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

	tmpGorn := filepath.Join(os.TempDir(), "gorn", cmp.Or(os.Getenv("USER"), os.Getenv("USERNAME"), "shared"))
	if err := os.MkdirAll(tmpGorn, 0o700); err == nil { //nolint:gosec // G703: USER/USERNAME are local env vars, not remote input.
		return tmpGorn, nil
	}

	return "", errors.New("failed to determine cache root")
}
