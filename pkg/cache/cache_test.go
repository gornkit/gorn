package cache

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gornkit/gorn/pkg/app"
)

func mustSource(t *testing.T, name, data string) *app.Source {
	t.Helper()
	s, err := app.New(name, []byte(data))
	if err != nil {
		t.Fatalf("app.New(%q): %v", name, err)
	}
	return s
}

// stageBuild populates dir as a finished build for src: a bin plus a manifest
// whose checksum matches, so it satisfies Lookup once it lives at Dir.
func stageBuild(t *testing.T, dir string, src *app.Source) {
	t.Helper()
	binPath := filepath.Join(dir, "bin", binName)
	if err := os.MkdirAll(filepath.Dir(binPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binPath, []byte("bin:"+string(src.Key())), 0o700); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	sum, err := fileSHA256(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(dir, newManifest(src, sum)); err != nil {
		t.Fatal(err)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := newManifest(mustSource(t, "/src/x.gorn", "body"), "deadbeef")
	if err := WriteManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != m {
		t.Fatalf("round-trip = %+v, want %+v", got, m)
	}
}

// TestAppDirNamespacedBySchema guards the change that made the build lock
// unnecessary: app dirs are namespaced by manifest schema, so a schema bump
// lands in a fresh namespace instead of forcing a destructive replace of a
// stale entry.
func TestAppDirNamespacedBySchema(t *testing.T) {
	c := Root(t.TempDir())
	src := mustSource(t, "app.gorn", "body")
	dir := c.Dir(src)
	want := filepath.Join("apps", strconv.Itoa(manifestSchema))
	if rel, err := filepath.Rel(c.Path(), dir); err != nil || filepath.Dir(rel) != want {
		t.Fatalf("Dir = %q, want under %q", dir, want)
	}
	if !strings.HasSuffix(filepath.Base(dir), "-app") {
		t.Fatalf("Dir base %q missing slug suffix", filepath.Base(dir))
	}
}

func TestCachedBinGate(t *testing.T) {
	c := Root(t.TempDir())
	src := mustSource(t, "app.gorn", "body")
	dir := c.Dir(src)
	stageBuild(t, dir, src)

	// Valid entry: manifest matches key + bin exists + checksum matches.
	if bin, ok := c.Lookup(src); !ok || bin != c.BinPath(src) {
		t.Fatalf("valid entry: bin=%q ok=%v", bin, ok)
	}

	// Checksum mismatch → miss (guards post-publish corruption).
	if err := os.WriteFile(c.BinPath(src), []byte("tampered"), 0o700); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	if _, ok := c.Lookup(src); ok {
		t.Fatal("checksum mismatch should be a miss")
	}
	stageBuild(t, dir, src) // restore a consistent entry

	// Key mismatch → miss (guards the 12-char prefix collision: a different
	// full key sharing the dir must not be served).
	if err := WriteManifest(dir, newManifest(mustSource(t, "other.gorn", "other"), "x")); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Lookup(src); ok {
		t.Fatal("key mismatch should be a miss")
	}

	// Missing manifest → miss.
	if err := os.Remove(filepath.Join(dir, manifestName)); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Lookup(src); ok {
		t.Fatal("missing manifest should be a miss")
	}
}

func TestCachedBinMissWhenBinAbsent(t *testing.T) {
	c := Root(t.TempDir())
	src := mustSource(t, "app.gorn", "body")
	dir := c.Dir(src)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(dir, newManifest(src, "abc")); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Lookup(src); ok {
		t.Fatal("manifest present but no bin should be a miss")
	}
}

// TestPublish covers the lock-free install: a bare rename is the publish point,
// and a rename that loses to a peer yields to their identical entry, while any
// other rename failure surfaces.
func TestPublish(t *testing.T) {
	t.Run("absent dest installs", func(t *testing.T) {
		c := Root(t.TempDir())
		src := mustSource(t, "app.gorn", "body")
		build := t.TempDir()
		stageBuild(t, build, src)
		if err := c.publish(build, src); err != nil {
			t.Fatalf("publish into empty cache: %v", err)
		}
		if _, ok := c.Lookup(src); !ok {
			t.Fatal("published entry not usable")
		}
	})

	t.Run("valid dest yields to peer", func(t *testing.T) {
		c := Root(t.TempDir())
		src := mustSource(t, "app.gorn", "body")
		stageBuild(t, c.Dir(src), src) // peer already published

		build := t.TempDir()
		stageBuild(t, build, src)
		if err := c.publish(build, src); err != nil {
			t.Fatalf("losing publisher should yield, got: %v", err)
		}
		if _, ok := c.Lookup(src); !ok {
			t.Fatal("peer entry not usable after yield")
		}
	})

	t.Run("occupied but invalid dest errors", func(t *testing.T) {
		c := Root(t.TempDir())
		src := mustSource(t, "app.gorn", "body")
		dest := c.Dir(src)
		if err := os.MkdirAll(dest, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dest, "junk"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}

		build := t.TempDir()
		stageBuild(t, build, src)
		if err := c.publish(build, src); err == nil {
			t.Fatal("publish over occupied non-yielding dir should error")
		}
	})
}
