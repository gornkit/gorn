package cache

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gornkit/gorn/pkg/app"
	"github.com/gornkit/gorn/pkg/buildprofile"
)

func mustSource(t *testing.T, name, data string) *app.Source {
	t.Helper()
	s, err := app.New(name, []byte(data))
	if err != nil {
		t.Fatalf("app.New(%q): %v", name, err)
	}
	return s.WithBuildIdentity(fixtureProfile().Identity)
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
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeOwner(dir, ownership{Schema: 1, Layout: manifestSchema, Key: string(src.Key()), Kind: "entry", StageID: "build-fixture", Source: src.Path(), Created: now, BuiltAt: now}); err != nil {
		t.Fatal(err)
	}
	sum, err := fileSHA256(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(dir, newManifest(src, sum, fixtureProfile())); err != nil {
		t.Fatal(err)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := newManifest(mustSource(t, "/src/x.gorn", "body"), "deadbeef", fixtureProfile())
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

// Layout changes use a fresh namespace; complete keys avoid prefix collisions.
func TestAppDirNamespacedBySchema(t *testing.T) {
	c := Root(t.TempDir())
	src := mustSource(t, "app.gorn", "body")
	dir := c.Dir(src)
	want := filepath.Join("apps", strconv.Itoa(manifestSchema))
	if rel, err := filepath.Rel(c.Path(), dir); err != nil || filepath.Dir(rel) != want {
		t.Fatalf("Dir = %q, want under %q", dir, want)
	}
	if filepath.Base(dir) != string(src.Key()) {
		t.Fatalf("Dir base %q is not the full key", filepath.Base(dir))
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

	// Key mismatch → miss even if someone changes the manifest in place.
	if err := WriteManifest(dir, newManifest(mustSource(t, "other.gorn", "other"), "x", fixtureProfile())); err != nil {
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
	if err := WriteManifest(dir, newManifest(src, "abc", fixtureProfile())); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Lookup(src); ok {
		t.Fatal("manifest present but no bin should be a miss")
	}
}

// TestPublish exercises publication under the caller-owned lock protocol.
func TestPublish(t *testing.T) {
	t.Run("absent dest installs", func(t *testing.T) {
		c := Root(t.TempDir())
		src := mustSource(t, "app.gorn", "body")
		build := t.TempDir()
		stageBuild(t, build, src)
		if err := c.publish(context.Background(), build, src); err != nil {
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
		if err := c.publish(context.Background(), build, src); err != nil {
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
		if err := c.publish(context.Background(), build, src); err == nil {
			t.Fatal("publish over occupied non-yielding dir should error")
		}
	})
}

func TestExplicitCacheRoot(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("GORN_CACHE", "relative-cache")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := Open()
	if err != nil || root.Path() != filepath.Join(cwd, "relative-cache") {
		t.Fatalf("relative cache: root=%q err=%v", root, err)
	}
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GORN_CACHE", filepath.Join(blocked, "cache"))
	if _, err := Open(); err == nil || !strings.Contains(err.Error(), "GORN_CACHE") || !strings.Contains(err.Error(), blocked) {
		t.Fatalf("explicit unusable cache must not fall back: %v", err)
	}
}

func fixtureProfile() buildprofile.Record {
	r := buildprofile.Record{Schema: 1, GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	r.Identity = r.Fingerprint()
	return r
}

func TestCacheRejectsChangedProfileAndTargetMetadata(t *testing.T) {
	c := Root(t.TempDir())
	source := mustSource(t, "profile.gorn", "body")
	for name, mutate := range map[string]func(*Manifest){
		"schema":           func(m *Manifest) { m.Schema-- },
		"profile schema":   func(m *Manifest) { m.Profile.Schema++ },
		"profile identity": func(m *Manifest) { m.Profile.Identity = "wrong" },
		"profile contents": func(m *Manifest) { m.Profile.GoSHA256 = "changed" },
		"target":           func(m *Manifest) { m.GOOS = "foreign" },
		"compiler":         func(m *Manifest) { m.GoVersion = "go1.99" },
	} {
		t.Run(name, func(t *testing.T) {
			stageBuild(t, c.Dir(source), source)
			m, err := LoadManifest(c.Dir(source))
			if err != nil {
				t.Fatal(err)
			}
			mutate(&m)
			if err := WriteManifest(c.Dir(source), m); err != nil {
				t.Fatal(err)
			}
			if _, hit := c.Lookup(source); hit {
				t.Fatal("accepted mismatched profile/target metadata")
			}
		})
	}
}
