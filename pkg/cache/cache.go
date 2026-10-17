package cache

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/gornkit/gorn/pkg/app"
)

var binName = func() string {
	if runtime.GOOS == "windows" {
		return "app.exe"
	}
	return "app"
}()

// Root is the root directory of the gorn build cache.
type Root string

// Resolve selects the cache root without creating directories or probing
// writability. Existing aliases resolve physically; absent roots stay absolute.
func Resolve() (Root, error) {
	path := getRoot()
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", rootLabel(), err)
	}
	physical, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return Root(physical), nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("%s %q: %w", rootLabel(), absolute, err)
	}
	return Root(absolute), nil
}

// Open creates the deterministically selected root. A creation failure never
// silently redirects builds away from the location reported by Resolve.
func Open() (Root, error) {
	root, err := Resolve()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root.Path(), 0o700); err != nil {
		return "", fmt.Errorf("%s %q: %w", rootLabel(), root.Path(), err)
	}
	return root, nil
}

func rootLabel() string {
	if os.Getenv("GORN_CACHE") != "" {
		return "GORN_CACHE"
	}
	return "cache root"
}

// Path returns the cache root directory.
func (c Root) Path() string { return string(c) }

// TmpDir returns the directory used for in-progress builds.
func (c Root) TmpDir() string { return filepath.Join(string(c), "tmp") }

// Dir returns the full-key directory in the current layout namespace.
func (c Root) Dir(s *app.Source) string {
	return filepath.Join(string(c), "apps", strconv.Itoa(manifestSchema), string(s.Key()))
}

// binDir returns the bin subdirectory of s's install directory.
func (c Root) binDir(s *app.Source) string {
	return filepath.Join(c.Dir(s), "bin")
}

// BinPath returns the path to s's built binary.
func (c Root) BinPath(s *app.Source) string {
	return filepath.Join(c.binDir(s), binName)
}

// Lookup returns the path to a usable cached binary for s, and true, only
// when the app dir holds a manifest that parses and matches the full appKey,
// and the binary is a regular file whose checksum matches the manifest. Any
// failure is a cache miss. Lookup is read-only and does not protect lifetime;
// execution and export callers must use Acquire instead.
func (c Root) Lookup(s *app.Source) (string, bool) {
	if !validKey(string(s.Key())) || c.ownedEntry(s) != nil {
		return "", false
	}
	dir := c.Dir(s)
	if c.checkDir(filepath.Join(dir, "bin")) != nil || !regular(filepath.Join(dir, manifestName)) {
		return "", false
	}
	m, err := LoadManifest(dir)
	if err != nil {
		return "", false
	}
	if m.Schema != manifestSchema || m.Key != string(s.Key()) || m.Profile.Schema != 1 || m.Profile.Identity == "" || m.Profile.Identity != s.BuildIdentity() || m.Profile.Fingerprint() != m.Profile.Identity ||
		m.GOOS != runtime.GOOS || m.GOARCH != runtime.GOARCH || m.Profile.GOOS != m.GOOS || m.Profile.GOARCH != m.GOARCH || m.Profile.GoVersion != m.GoVersion {
		return "", false
	}
	bin := c.BinPath(s)
	info, err := os.Lstat(bin) //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
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

func getRoot() string {
	if path := os.Getenv("GORN_CACHE"); path != "" {
		return path
	}
	if path, err := os.UserCacheDir(); err == nil {
		return filepath.Join(path, "gorn")
	}
	return filepath.Join(os.TempDir(), "gorn", cmp.Or(os.Getenv("USER"), os.Getenv("USERNAME"), "shared"))
}
