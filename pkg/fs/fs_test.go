package fs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gornkit/gorn/pkg/fs"
)

// TestGenerateAppKeyDeterministic verifies that the same inputs always produce
// the same AppKey.
func TestGenerateAppKeyDeterministic(t *testing.T) {
	source := []byte("//gorn:main\nprintln(\"hello\")\n")
	key1 := fs.GenerateAppKey("/scripts/hello.gorn", source)
	key2 := fs.GenerateAppKey("/scripts/hello.gorn", source)
	if key1 != key2 {
		t.Fatalf("GenerateAppKey not deterministic: %q != %q", key1, key2)
	}
}

// TestGenerateAppKeySensitiveToPath verifies that changing the path changes the key.
func TestGenerateAppKeySensitiveToPath(t *testing.T) {
	source := []byte("//gorn:main\nprintln(\"hello\")\n")
	key1 := fs.GenerateAppKey("/scripts/a.gorn", source)
	key2 := fs.GenerateAppKey("/scripts/b.gorn", source)
	if key1 == key2 {
		t.Fatal("GenerateAppKey: different paths produced the same key")
	}
}

// TestGenerateAppKeySensitiveToSource verifies that changing the source changes the key.
func TestGenerateAppKeySensitiveToSource(t *testing.T) {
	key1 := fs.GenerateAppKey("/scripts/hello.gorn", []byte("//gorn:main\nprintln(\"hello\")\n"))
	key2 := fs.GenerateAppKey("/scripts/hello.gorn", []byte("//gorn:main\nprintln(\"world\")\n"))
	if key1 == key2 {
		t.Fatal("GenerateAppKey: different sources produced the same key")
	}
}

// TestManifestRoundTrip verifies that WriteManifest and LoadManifest are
// inverses of each other.
func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	appKey := fs.GenerateAppKey("/scripts/hello.gorn", []byte("//gorn:main\nprintln(\"hello\")\n"))

	// Use the exported newManifest indirectly via WriteManifest: build one
	// via the Manifest literal so the test controls every field.
	want := fs.Manifest{
		Schema:    1,
		AppKey:    string(appKey),
		GoVersion: "go1.26.0",
		GOOS:      "linux",
		GOARCH:    "amd64",
		Source:    "/scripts/hello.gorn",
		BuiltAt:   "2026-01-01T00:00:00Z",
	}

	if err := fs.WriteManifest(dir, want); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	got, err := fs.LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}

	if got != want {
		t.Fatalf("round-trip mismatch:\ngot  = %+v\nwant = %+v", got, want)
	}
}

// TestLoadManifestMissingReturnsError verifies that LoadManifest errors when
// the manifest file does not exist.
func TestLoadManifestMissingReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, err := fs.LoadManifest(dir)
	if err == nil {
		t.Fatal("LoadManifest: expected error for missing file, got nil")
	}
}

// TestCachedBinMissWhenNoAppDir verifies that CachedBin is a miss when the
// app directory doesn't exist.
func TestCachedBinMissWhenNoAppDir(t *testing.T) {
	root := fs.CacheRoot(t.TempDir())
	appKey := fs.GenerateAppKey("/scripts/hello.gorn", []byte("//gorn:main\nprintln(\"hello\")\n"))

	if bin, ok := root.CachedBin(appKey); ok {
		t.Fatalf("CachedBin: expected miss, got hit with bin=%q", bin)
	}
}

// TestCachedBinMissWhenManifestMissing verifies that CachedBin is a miss when
// the app dir exists but has no manifest.
func TestCachedBinMissWhenManifestMissing(t *testing.T) {
	root := fs.CacheRoot(t.TempDir())
	appKey := fs.GenerateAppKey("/scripts/hello.gorn", []byte("//gorn:main\nprintln(\"hello\")\n"))

	// Create the app dir but no manifest.
	if err := os.MkdirAll(root.AppDir(appKey), 0o700); err != nil {
		t.Fatal(err)
	}

	if bin, ok := root.CachedBin(appKey); ok {
		t.Fatalf("CachedBin: expected miss without manifest, got hit with bin=%q", bin)
	}
}

// TestCachedBinMissWhenKeyMismatch verifies that CachedBin is a miss when
// the manifest's app_key does not match.
func TestCachedBinMissWhenKeyMismatch(t *testing.T) {
	root := fs.CacheRoot(t.TempDir())
	appKey := fs.GenerateAppKey("/scripts/hello.gorn", []byte("//gorn:main\nprintln(\"hello\")\n"))
	otherKey := fs.GenerateAppKey("/scripts/other.gorn", []byte("//gorn:main\nprintln(\"other\")\n"))

	appDir := root.AppDir(appKey)
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Write a manifest with a different key.
	m := fs.Manifest{
		Schema:    1,
		AppKey:    string(otherKey),
		GoVersion: "go1.26.0",
		GOOS:      "linux",
		GOARCH:    "amd64",
		Source:    "/scripts/hello.gorn",
		BuiltAt:   "2026-01-01T00:00:00Z",
	}
	if err := fs.WriteManifest(appDir, m); err != nil {
		t.Fatal(err)
	}

	if bin, ok := root.CachedBin(appKey); ok {
		t.Fatalf("CachedBin: expected miss on key mismatch, got hit with bin=%q", bin)
	}
}

// TestCachedBinMissWhenBinaryMissing verifies that CachedBin is a miss when
// the manifest matches but the binary file doesn't exist.
func TestCachedBinMissWhenBinaryMissing(t *testing.T) {
	root := fs.CacheRoot(t.TempDir())
	appKey := fs.GenerateAppKey("/scripts/hello.gorn", []byte("//gorn:main\nprintln(\"hello\")\n"))

	appDir := root.AppDir(appKey)
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		t.Fatal(err)
	}

	m := fs.Manifest{
		Schema:    1,
		AppKey:    string(appKey),
		GoVersion: "go1.26.0",
		GOOS:      "linux",
		GOARCH:    "amd64",
		Source:    "/scripts/hello.gorn",
		BuiltAt:   "2026-01-01T00:00:00Z",
	}
	if err := fs.WriteManifest(appDir, m); err != nil {
		t.Fatal(err)
	}
	// No binary written — expect miss.

	if bin, ok := root.CachedBin(appKey); ok {
		t.Fatalf("CachedBin: expected miss without binary, got hit with bin=%q", bin)
	}
}

// TestCachedBinHit verifies that CachedBin returns a hit when the manifest
// matches and the binary file exists.
func TestCachedBinHit(t *testing.T) {
	root := fs.CacheRoot(t.TempDir())
	appKey := fs.GenerateAppKey("/scripts/hello.gorn", []byte("//gorn:main\nprintln(\"hello\")\n"))

	appDir := root.AppDir(appKey)
	binDir := root.AppBinDir(appKey)
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Write a stub binary file.
	binPath := root.AppBinFile(appKey)
	if err := os.WriteFile(binPath, []byte("stub"), 0o700); err != nil {
		t.Fatal(err)
	}

	m := fs.Manifest{
		Schema:    1,
		AppKey:    string(appKey),
		GoVersion: "go1.26.0",
		GOOS:      "linux",
		GOARCH:    "amd64",
		Source:    "/scripts/hello.gorn",
		BuiltAt:   "2026-01-01T00:00:00Z",
	}
	if err := fs.WriteManifest(appDir, m); err != nil {
		t.Fatal(err)
	}

	bin, ok := root.CachedBin(appKey)
	if !ok {
		t.Fatal("CachedBin: expected hit, got miss")
	}
	if bin != filepath.Clean(binPath) && bin != binPath {
		t.Fatalf("CachedBin: bin = %q, want %q", bin, binPath)
	}
}

// TestLockAcquireAndRelease verifies the basic acquire-release cycle.
func TestLockAcquireAndRelease(t *testing.T) {
	root := fs.CacheRoot(t.TempDir())
	appKey := fs.GenerateAppKey("/scripts/lock.gorn", []byte("//gorn:main\n"))

	lock, err := root.Lock(appKey)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// After release, a second acquire must succeed.
	lock2, err := root.Lock(appKey)
	if err != nil {
		t.Fatalf("second Lock after release: %v", err)
	}
	_ = lock2.Release()
}

// TestLockReleaseNil verifies that releasing a nil lock is a no-op.
func TestLockReleaseNil(t *testing.T) {
	var l *fs.Lock
	if err := l.Release(); err != nil {
		t.Fatalf("nil Release: %v", err)
	}
}
