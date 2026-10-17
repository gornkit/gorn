//nolint:gosec // Controlled cache fixtures, intentional corruption, symlinks and metadata credentials.
package cache

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gornkit/gorn/internal/app"
)

func TestReadOnlyResolveAndInventoryDoNotCreateRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent", "cache")
	t.Setenv("GORN_CACHE", path)
	root, err := Resolve()
	if err != nil || root.Path() != path {
		t.Fatal(root, err)
	}
	inventory, err := root.Inspect(context.Background())
	if err != nil || len(inventory.Items) != 0 || !inventory.Totals.Managed.Complete {
		t.Fatal(inventory, err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("read-only resolution created directories:", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inventory, err = root.Inspect(ctx)
	if !errors.Is(err, context.Canceled) || inventory.Totals.Managed.Complete {
		t.Fatalf("canceled inventory claimed complete empty cache: %+v %v", inventory, err)
	}
}

func TestRootSelectionDoesNotFallbackAfterCreationFailure(t *testing.T) {
	// OS-path selection is read-only and shared with Open. A selected path must
	// not silently redirect when a filesystem object prevents its creation.
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GORN_CACHE", filepath.Join(blocked, "cache"))
	if _, err := Open(); err == nil || !strings.Contains(err.Error(), "GORN_CACHE") {
		t.Fatal(err)
	}
}

func TestInventorySeparatesOwnershipAndValidity(t *testing.T) {
	root := Root(t.TempDir())
	valid := mustSource(t, "valid.gorn", "one")
	invalid := mustSource(t, "invalid.gorn", "two")
	unknown := mustSource(t, "unknown.gorn", "three")
	for _, src := range []*app.Source{valid, invalid, unknown} {
		stageBuild(t, root.Dir(src), src)
	}
	if err := os.Remove(filepath.Join(root.Dir(invalid), manifestName)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root.Dir(unknown), ownerName)); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(root.Path(), "apps", "2", "legacy")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "payload"), []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), make([]byte, 1000), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(root.Path(), "linked")
	linkedOK := os.Symlink(outside, linked) == nil
	inventory, err := root.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]Item{}
	for _, item := range inventory.Items {
		byPath[item.Path] = item
	}
	if byPath[root.Dir(valid)].State != "valid" || byPath[root.Dir(invalid)].State != "invalid" || byPath[root.Dir(invalid)].Kind != "entry" || byPath[root.Dir(unknown)].Kind != "unmanaged" {
		t.Fatal(byPath)
	}
	if byPath[filepath.Dir(legacy)].Kind != "unmanaged" || byPath[filepath.Dir(legacy)].SizeBytes != 6 {
		t.Fatal(byPath[filepath.Dir(legacy)])
	}
	if linkedOK && (byPath[linked].SizeBytes != 0 || byPath[linked].Kind != "unmanaged") {
		t.Fatal("followed outside link:", byPath[linked])
	}
	if inventory.Totals.Managed.Bytes != byPath[root.Dir(valid)].SizeBytes+byPath[root.Dir(invalid)].SizeBytes {
		t.Fatal("managed budget included unknown data", inventory.Totals)
	}
	if inventory.Totals.Unmanaged.Bytes != 6+byPath[root.Dir(unknown)].SizeBytes {
		t.Fatal("unmanaged bytes missing/linked target counted", inventory.Totals)
	}
}

func TestInventoryProvenanceIsGoIndependentAndSanitized(t *testing.T) {
	root := Root(t.TempDir())
	src := mustSource(t, "profile.gorn", "body")
	stageBuild(t, root.Dir(src), src)
	owner, err := readOwner(root.Dir(src))
	if err != nil {
		t.Fatal(err)
	}
	owner.Source = "https://user:password@example.test/source?token=secret"
	if err := writeOwner(root.Dir(src), owner); err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadManifest(root.Dir(src))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Profile.GoPath = "https://user:password@example.test/go?token=secret"
	manifest.Profile.Identity = manifest.Profile.Fingerprint()
	if err := WriteManifest(root.Dir(src), manifest); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	inventory, err := root.Inspect(context.Background())
	if err != nil || len(inventory.Items) != 1 {
		t.Fatal(inventory, err)
	}
	item := inventory.Items[0]
	if item.Profile == nil || item.State != "valid" {
		t.Fatal(item)
	}
	for _, text := range []string{item.Source, item.Profile.GoPath} {
		if strings.Contains(text, "password") || strings.Contains(text, "secret") {
			t.Fatal("credential leaked:", text)
		}
	}
}

func TestIncompleteAndOverflowedByteAccounting(t *testing.T) {
	total := Size{Complete: true}
	addSize(&total, math.MaxInt64, true)
	addSize(&total, 1, true)
	if total.Complete || total.Bytes != math.MaxInt64 {
		t.Fatal(total)
	}
	total = Size{Complete: true}
	addSize(&total, 4, false)
	if total.Complete || total.Bytes != 4 {
		t.Fatal(total)
	}
}

func TestDefaultRootCreationFailureIsExplicit(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GORN_CACHE", "")
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LocalAppData", blocked)
	case "darwin":
		t.Setenv("HOME", blocked)
	default:
		t.Setenv("XDG_CACHE_HOME", blocked)
	}
	if _, err := Open(); err == nil {
		t.Fatal("default cache silently redirected after creation failure")
	}
}

func TestUnreadableOwnershipMembershipRemainsIncomplete(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission denial fixture")
	}
	for _, location := range []string{"apps", "layout", "entry", "owner", "stage", "quarantine"} {
		t.Run(location, func(t *testing.T) {
			root := Root(t.TempDir())
			source := mustSource(t, "hidden.gorn", "body")
			stageBuild(t, root.Dir(source), source)
			path := root.Dir(source)
			category := "entry"
			switch location {
			case "apps":
				path = filepath.Join(root.Path(), "apps")
			case "layout":
				path = filepath.Dir(path)
			case "owner":
				path = filepath.Join(path, ownerName)
			case "stage", "quarantine":
				path = ownedAux(t, root, source, location)
				category = location
			}
			if err := os.Chmod(path, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(path, 0o700) })
			var denied error
			if location == "owner" {
				_, denied = os.ReadFile(path)
			} else {
				_, denied = os.ReadDir(path)
			}
			if denied == nil {
				t.Skip("host ignores permission denial")
			}
			inventory := mustInspect(t, root)
			complete := inventory.Totals.Managed.Complete
			if category == "stage" {
				complete = inventory.Totals.Stages.Complete
			}
			if category == "quarantine" {
				complete = inventory.Totals.Quarantine.Complete
			}
			if complete {
				t.Fatalf("hidden ownership claimed complete membership: %+v", inventory.Totals)
			}
			if category == "entry" {
				zero := int64(0)
				report, err := root.Maintain(context.Background(), MaintenanceOptions{Operation: "prune", MaxBytes: &zero, Yes: true})
				if err == nil || !report.UnmetBudget || report.After.Managed.Complete {
					t.Fatalf("hidden managed bytes satisfied budget: %+v %v", report, err)
				}
				if len(report.Actions) != 0 {
					t.Fatal("size-driven deletion with unknown membership", report.Actions)
				}
			}
		})
	}
}
