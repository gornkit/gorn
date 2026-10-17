//nolint:gosec // Controlled cache paths, executable test helpers, and intentional filesystem corruption.
package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gornkit/gorn/pkg/app"
	"github.com/gornkit/gorn/pkg/buildprofile"
	"github.com/gornkit/gorn/pkg/script"
)

func TestLeasesAndLockOrdering(t *testing.T) {
	root := Root(t.TempDir())
	src := mustSource(t, "leased.gorn", "body")
	stageBuild(t, root.Dir(src), src)
	first, hit, err := root.Acquire(context.Background(), src)
	if err != nil || !hit {
		t.Fatalf("acquire: %v %v", hit, err)
	}
	defer func() { _ = first.Close() }()
	second, hit, err := root.Acquire(context.Background(), src)
	if err != nil || !hit {
		t.Fatalf("shared acquire: %v %v", hit, err)
	}
	if _, err := root.keyLock(context.Background(), src, "activity", true, false); !errors.Is(err, errLockBusy) {
		t.Fatalf("exclusive activity while leased: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	mutation, err := root.keyLock(context.Background(), src, "mutation", true, false)
	if err != nil {
		t.Fatal(err)
	}
	// An independent handle in the same process must not acquire the same lock.
	if _, err := root.keyLock(context.Background(), src, "mutation", true, false); !errors.Is(err, errLockBusy) {
		t.Fatalf("duplicate exclusive lock: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if _, err := root.keyLock(ctx, src, "mutation", true, true); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancel lock wait: %v", err)
	}
	if err := mutation.close(); err != nil {
		t.Fatal(err)
	}
	// A miss must not retain A while the subsequent builder is waiting for M.
	if err := os.Remove(filepath.Join(root.Dir(src), manifestName)); err != nil {
		t.Fatal(err)
	}
	if _, hit, err := root.Acquire(context.Background(), src); err != nil || hit {
		t.Fatalf("miss: %v %v", hit, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	exclusive, err := root.keyLock(context.Background(), src, "activity", true, false)
	if err != nil {
		t.Fatalf("miss leaked activity lease: %v", err)
	}
	_ = exclusive.close()
	if _, err := os.Stat(filepath.Join(root.Path(), "locks", "entries", string(src.Key())+".activity")); err != nil {
		t.Fatal("stable lock removed:", err)
	}
}

func TestRepairOwnedInvalidEntries(t *testing.T) {
	for _, kind := range []string{"manifest", "checksum", "missing binary", "binary directory", "profile", "target"} {
		t.Run(kind, func(t *testing.T) {
			root := Root(t.TempDir())
			src := mustSource(t, "repair.gorn", "body")
			stageBuild(t, root.Dir(src), src)
			switch kind {
			case "manifest":
				if err := os.WriteFile(filepath.Join(root.Dir(src), manifestName), []byte("broken"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "checksum":
				if err := os.WriteFile(root.BinPath(src), []byte("bad"), 0o700); err != nil {
					t.Fatal(err)
				}
			case "missing binary":
				if err := os.Remove(root.BinPath(src)); err != nil {
					t.Fatal(err)
				}
			case "binary directory":
				if err := os.Remove(root.BinPath(src)); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root.BinPath(src), 0o700); err != nil {
					t.Fatal(err)
				}
			default:
				m, err := LoadManifest(root.Dir(src))
				if err != nil {
					t.Fatal(err)
				}
				if kind == "profile" {
					m.Profile.Identity = "bad"
				} else {
					m.GOOS = "other"
				}
				if err := WriteManifest(root.Dir(src), m); err != nil {
					t.Fatal(err)
				}
			}
			candidate := t.TempDir()
			stageBuild(t, candidate, src)
			if err := root.publish(context.Background(), candidate, src); err != nil {
				t.Fatal(err)
			}
			if _, hit := root.Lookup(src); !hit {
				t.Fatal("repair did not publish usable binary")
			}
			quarantined, err := os.ReadDir(filepath.Join(root.Path(), "quarantine"))
			if err != nil || len(quarantined) != 1 {
				t.Fatalf("quarantine: %v %v", quarantined, err)
			}
			if _, err := readOwner(filepath.Join(root.Path(), "quarantine", quarantined[0].Name())); err != nil {
				t.Fatal("lost independent ownership:", err)
			}
		})
	}
}

func TestUnknownOwnershipAndSymlinksArePreserved(t *testing.T) {
	for _, kind := range []string{"missing owner", "corrupt owner", "wrong key", "entry link", "owner link", "manifest link", "binary link", "bin directory link", "apps link", "lock link"} {
		t.Run(kind, func(t *testing.T) {
			root := Root(t.TempDir())
			src := mustSource(t, "unknown.gorn", "body")
			stageBuild(t, root.Dir(src), src)
			external := t.TempDir()
			sentinel := filepath.Join(external, "sentinel")
			if err := os.WriteFile(sentinel, []byte("untouched"), 0o600); err != nil {
				t.Fatal(err)
			}
			makeLink := func(target, path string) {
				t.Helper()
				if err := os.RemoveAll(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			switch kind {
			case "missing owner":
				_ = os.Remove(filepath.Join(root.Dir(src), ownerName))
			case "corrupt owner":
				_ = os.WriteFile(filepath.Join(root.Dir(src), ownerName), []byte("{}"), 0o600)
			case "wrong key":
				owner, err := readOwner(root.Dir(src))
				if err != nil {
					t.Fatal(err)
				}
				owner.Key = strings.Repeat("a", 64)
				if err := writeOwner(root.Dir(src), owner); err != nil {
					t.Fatal(err)
				}
			case "entry link":
				makeLink(external, root.Dir(src))
			case "owner link":
				makeLink(sentinel, filepath.Join(root.Dir(src), ownerName))
			case "manifest link":
				makeLink(sentinel, filepath.Join(root.Dir(src), manifestName))
			case "binary link":
				makeLink(sentinel, root.BinPath(src))
			case "bin directory link":
				makeLink(external, filepath.Join(root.Dir(src), "bin"))
			case "apps link":
				makeLink(external, filepath.Join(root.Path(), "apps"))
			case "lock link":
				if err := root.ensureDir(filepath.Join(root.Path(), "locks", "entries")); err != nil {
					t.Fatal(err)
				}
				makeLink(sentinel, filepath.Join(root.Path(), "locks", "entries", string(src.Key())+".activity"))
			}
			candidate := t.TempDir()
			stageBuild(t, candidate, src)
			if kind == "lock link" {
				if _, _, err := root.Acquire(context.Background(), src); err == nil {
					t.Fatal("followed lock symlink")
				}
			} else {
				if _, hit := root.Lookup(src); hit {
					t.Fatal("lookup accepted substituted ownership/payload")
				}
				err := root.publish(context.Background(), candidate, src)
				// A symlink payload inside independently owned data may be quarantined;
				// its target is never traversed or mutated.
				ownedPayload := kind == "manifest link" || kind == "binary link" || kind == "bin directory link"
				if !ownedPayload && err == nil {
					t.Fatal("replaced unrecognized data")
				}
				if ownedPayload && err != nil {
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(sentinel)
			if err != nil || string(data) != "untouched" {
				t.Fatalf("external target changed: %q %v", data, err)
			}
		})
	}
}

func TestFailedPublicationRetainsQuarantine(t *testing.T) {
	root := Root(t.TempDir())
	src := mustSource(t, "bad.gorn", "body")
	stageBuild(t, root.Dir(src), src)
	if err := os.Remove(filepath.Join(root.Dir(src), manifestName)); err != nil {
		t.Fatal(err)
	}
	err := root.publish(context.Background(), filepath.Join(root.TmpDir(), "absent-candidate"), src)
	if err == nil {
		t.Fatal("missing candidate installed")
	}
	entries, readErr := os.ReadDir(filepath.Join(root.Path(), "quarantine"))
	if readErr != nil || len(entries) != 1 {
		t.Fatalf("lost quarantine: %v %v", entries, readErr)
	}
	if _, err := readOwner(filepath.Join(root.Path(), "quarantine", entries[0].Name())); err != nil {
		t.Fatal(err)
	}
}

func TestEntryClosePrivateAndShared(t *testing.T) {
	root := Root(t.TempDir())
	dir, err := os.MkdirTemp(root.Path(), "build-")
	if err != nil {
		t.Fatal(err)
	}
	id := filepath.Base(dir)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeOwner(dir, ownership{Schema: 1, Layout: manifestSchema, Key: strings.Repeat("a", 64), Kind: "stage", StageID: id, Created: now}); err != nil {
		t.Fatal(err)
	}
	identity, err := captureIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry := makeEntry(dir, nil, func() error { return removeStage(dir, id, identity) }, false)
	copyEntry := entry
	external := t.TempDir()
	copyEntry.AppRoot = external
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Go(func() {
			if err := copyEntry.Remove(); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := entry.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("stage survived:", err)
	}
	if _, err := os.Stat(external); err != nil {
		t.Fatal("exported field redirected deletion:", err)
	}
	// Cached Close/Remove only release their lease.
	src := mustSource(t, "cached.gorn", "body")
	stageBuild(t, root.Dir(src), src)
	cached, hit, err := root.Acquire(context.Background(), src)
	if err != nil || !hit {
		t.Fatal(hit, err)
	}
	if err := cached.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, hit := root.Lookup(src); !hit {
		t.Fatal("Remove deleted a published cache entry")
	}
}

func TestLockProcessHelper(t *testing.T) {
	if os.Getenv("GORN_LOCK_HELPER") != "1" {
		return
	}
	root := Root(os.Getenv("GORN_LOCK_ROOT"))
	lock, err := root.lock(context.Background(), "stages", "build-process", true, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.close() }()
	if err := os.WriteFile(filepath.Join(root.Path(), "ready"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestKilledHolderReleasesStableLock(t *testing.T) {
	root := Root(t.TempDir())
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockProcessHelper$")
	cmd.Env = append(os.Environ(), "GORN_LOCK_HELPER=1", "GORN_LOCK_ROOT="+root.Path())
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root.Path(), "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := root.lock(context.Background(), "stages", "build-process", true, false); !errors.Is(err, errLockBusy) {
		t.Fatal("cross-process exclusion:", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	lock, err := root.lock(context.Background(), "stages", "build-process", true, false)
	if err != nil {
		t.Fatal("dead holder retained lock:", err)
	}
	_ = lock.close()
	if !regular(filepath.Join(root.Path(), "locks", "stages", "build-process")) {
		t.Fatal("lock identity was unlinked")
	}
}

func cacheBuildInput(t *testing.T) (*app.Source, *script.Generated, *buildprofile.Profile) {
	t.Helper()
	fileProxy(t)
	src, err := app.New(filepath.Join(t.TempDir(), "app.gorn"), []byte("package main\nfunc main(){}"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := script.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := script.Generate(parsed)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := buildprofile.Discover(generated.GoVersion, "")
	if err != nil {
		t.Fatal(err)
	}
	return src.WithBuildIdentity(profile.Identity()), generated, profile
}

func TestBuildRepairWaitsForLeaseAndBypassIgnoresMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("builds real Go program")
	}
	src, gen, profile := cacheBuildInput(t)
	root := Root(t.TempDir())
	first, err := root.Build(src, gen, false, nil, profile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	// A leased entry can become invalid; repair must wait for the existing user.
	if err := os.Remove(filepath.Join(first.AppRoot, manifestName)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		entry, err := root.BuildContext(ctx, src, gen, false, nil, profile)
		if err == nil {
			_ = entry.Close()
		}
		done <- err
	}()
	deadline := time.Now().Add(20 * time.Second)
	for {
		manifests, _ := filepath.Glob(filepath.Join(root.TmpDir(), "build-*", manifestName))
		if len(manifests) > 0 {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("repair returned before completed staging: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("repair did not finish staging")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case err := <-done:
		t.Fatalf("repair ignored active lease: %v", err)
	case <-time.After(60 * time.Millisecond):
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("repair did not cancel lock wait: %v", err)
	}

	if _, err := os.Stat(first.AppPath); err != nil {
		t.Fatal("active payload removed:", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	repaired, err := root.Build(src, gen, false, nil, profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := repaired.Close(); err != nil {
		t.Fatal(err)
	}
	mutation, err := root.keyLock(context.Background(), src, "mutation", true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mutation.close() }()
	bypass, err := root.Build(src, gen, true, nil, profile)
	if err != nil {
		t.Fatal(err)
	}
	stageID := filepath.Base(bypass.AppRoot)
	if _, err := root.lock(context.Background(), "stages", stageID, true, false); !errors.Is(err, errLockBusy) {
		t.Fatal("bypass stage not protected:", err)
	}
	if err := bypass.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root.TmpDir())
	if err != nil || len(entries) != 0 {
		t.Fatalf("stages leaked: %v %v", entries, err)
	}
}

func TestColdBuilderProcessHelper(t *testing.T) {
	if os.Getenv("GORN_BUILD_HELPER") != "1" {
		return
	}
	sourcePath := os.Getenv("GORN_BUILD_SOURCE")
	src, err := app.New(sourcePath, []byte("package main\nfunc main(){}"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := script.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	gen, err := script.Generate(file)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := buildprofile.Discover(gen.GoVersion, "")
	if err != nil {
		t.Fatal(err)
	}
	root := Root(os.Getenv("GORN_BUILD_ROOT"))
	if err := os.WriteFile(os.Getenv("GORN_BUILD_READY"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	entry, err := root.Build(src.WithBuildIdentity(profile.Identity()), gen, false, nil, profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("GORN_BUILD_RESULT"), fmt.Appendf(nil, "%t %s", entry.CacheHit, entry.AppRoot), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := entry.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentColdBuildersShareOnePublication(t *testing.T) {
	if testing.Short() {
		t.Skip("builds real Go program in two processes")
	}
	src, _, _ := cacheBuildInput(t)
	root := Root(t.TempDir())
	mutation, err := root.keyLock(context.Background(), src, "mutation", true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mutation.close() }()
	var children []*exec.Cmd
	results := make([]string, 2)
	for i := range 2 {
		ready := filepath.Join(root.Path(), fmt.Sprintf("ready-%d", i))
		results[i] = filepath.Join(root.Path(), fmt.Sprintf("result-%d", i))
		cmd := exec.Command(os.Args[0], "-test.run=^TestColdBuilderProcessHelper$")
		cmd.Env = append(os.Environ(), "GORN_BUILD_HELPER=1", "GORN_BUILD_ROOT="+root.Path(), "GORN_BUILD_SOURCE="+src.Path(), "GORN_BUILD_READY="+ready, "GORN_BUILD_RESULT="+results[i])
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		children = append(children, cmd)
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		deadline := time.Now().Add(20 * time.Second)
		for {
			if _, err := os.Stat(ready); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("builder did not become ready")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	if err := mutation.close(); err != nil {
		t.Fatal(err)
	}
	// The deferred close is idempotent too.
	mutation = nil
	for _, cmd := range children {
		if err := cmd.Wait(); err != nil {
			t.Fatal("child build:", err)
		}
	}
	hits := 0
	for _, path := range results {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(string(data), "true ") {
			hits++
		}
		if !strings.HasSuffix(string(data), root.Dir(src)) {
			t.Fatalf("different publications: %s", data)
		}
	}
	if hits != 1 {
		t.Fatalf("warm peer count=%d, want exactly one", hits)
	}
	stages, err := os.ReadDir(filepath.Join(root.Path(), "locks", "stages"))
	if err != nil || len(stages) != 1 {
		t.Fatalf("duplicate build stages: %v %v", stages, err)
	}
}

func TestStageCleanupRefusesReplacedIdentityAndOwnership(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(fmt.Sprint(replace), func(t *testing.T) {
			root := Root(t.TempDir())
			dir, err := os.MkdirTemp(root.Path(), "build-")
			if err != nil {
				t.Fatal(err)
			}
			stageID := filepath.Base(dir)
			now := time.Now().UTC().Format(time.RFC3339Nano)
			owner := ownership{Schema: 1, Layout: manifestSchema, Key: strings.Repeat("a", 64), Kind: "stage", StageID: stageID, Created: now}
			if err := writeOwner(dir, owner); err != nil {
				t.Fatal(err)
			}
			identity, err := captureIdentity(dir)
			if err != nil {
				t.Fatal(err)
			}
			if replace {
				if err := os.Rename(dir, dir+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := writeOwner(dir, owner); err != nil {
					t.Fatal(err)
				}
			} else {
				owner.StageID = "build-replaced"
				if err := writeOwner(dir, owner); err != nil {
					t.Fatal(err)
				}
			}
			if err := removeStage(dir, stageID, identity); err == nil {
				t.Fatal("removed replacement stage")
			}
			if !regular(filepath.Join(dir, ownerName)) {
				t.Fatal("replacement ownership deleted")
			}
		})
	}
}

func TestCanceledPublicationDoesNotMutateDestination(t *testing.T) {
	root := Root(t.TempDir())
	src := mustSource(t, "canceled.gorn", "body")
	stageBuild(t, root.Dir(src), src)
	if err := os.Remove(filepath.Join(root.Dir(src), manifestName)); err != nil {
		t.Fatal(err)
	}
	candidate := t.TempDir()
	stageBuild(t, candidate, src)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := root.publish(ctx, candidate, src); !errors.Is(err, context.Canceled) {
		t.Fatalf("publication ignored cancellation: %v", err)
	}
	if _, err := os.Stat(root.Dir(src)); err != nil {
		t.Fatal("canceled publication moved old data:", err)
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatal("canceled publication moved candidate:", err)
	}
	if _, err := os.Stat(filepath.Join(root.Path(), "quarantine")); !os.IsNotExist(err) {
		t.Fatal("canceled publication created quarantine:", err)
	}
}

func TestPreviousLayoutIsNeverAdoptedOrRepaired(t *testing.T) {
	root := Root(t.TempDir())
	src := mustSource(t, "old.gorn", "body")
	legacy := filepath.Join(root.Path(), "apps", "2", string(src.Key())[:12]+"-old")
	stageBuild(t, legacy, src)
	old, err := LoadManifest(legacy)
	if err != nil {
		t.Fatal(err)
	}
	old.Schema = 2
	if err := WriteManifest(legacy, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(legacy, ownerName)); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(legacy, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	if _, hit := root.Lookup(src); hit {
		t.Fatal("adopted previous layout")
	}
	candidate := t.TempDir()
	stageBuild(t, candidate, src)
	if err := root.publish(context.Background(), candidate, src); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(legacy, manifestName))
	if err != nil || string(after) != string(original) {
		t.Fatal("previous layout was modified", err)
	}
	if _, hit := root.Lookup(src); !hit {
		t.Fatal("new layout did not publish")
	}
}
