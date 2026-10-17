//nolint:gosec // Controlled native sharing violations and owned-cache deletion fixtures.
package cache

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestWindowsFailedOldestDeletionDoesNotStopSizePruning(t *testing.T) {
	root := Root(t.TempDir())
	old := mustSource(t, "old.gorn", "old")
	next := mustSource(t, "next.gorn", "next")
	stageBuild(t, root.Dir(old), old)
	stageBuild(t, root.Dir(next), next)
	setBuildTime(t, root.Dir(old), time.Now().Add(-48*time.Hour))
	setBuildTime(t, root.Dir(next), time.Now().Add(-24*time.Hour))
	path, err := syscall.UTF16PtrFromString(root.BinPath(old))
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(path, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.CloseHandle(handle) }()
	inventory := mustInspect(t, root)
	var nextBytes int64
	for _, item := range inventory.Items {
		if item.Key == string(next.Key()) {
			nextBytes = item.SizeBytes
		}
	}
	budget := inventory.Totals.Managed.Bytes - nextBytes
	report, err := root.Maintain(context.Background(), MaintenanceOptions{Operation: "prune", Yes: true, MaxBytes: &budget})
	if err == nil || report.UnmetBudget || !hasAction(report, root.Dir(old), "failed") || !hasAction(report, root.Dir(next), "removed") {
		t.Fatal(report, err)
	}
	if _, err := readOwner(root.Dir(old)); err != nil {
		t.Fatal("failed cleanup lost ownership:", err)
	}
	lock, err := root.keyLock(context.Background(), old, "mutation", true, false)
	if err != nil {
		t.Fatal("cleanup failure leaked lock:", err)
	}
	_ = lock.close()
}

func TestWindowsFinalDirectoryFailureRestoresOwnership(t *testing.T) {
	root := Root(t.TempDir())
	src := mustSource(t, "directory.gorn", "body")
	stageBuild(t, root.Dir(src), src)
	before, err := readOwner(root.Dir(src))
	if err != nil {
		t.Fatal(err)
	}
	path, err := syscall.UTF16PtrFromString(root.Dir(src))
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(path, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if handle != syscall.InvalidHandle {
			_ = syscall.CloseHandle(handle)
		}
	}()
	report, err := root.Maintain(context.Background(), MaintenanceOptions{Operation: "clean", All: true, Yes: true})
	if err == nil || !hasAction(report, root.Dir(src), "failed") {
		t.Fatal(report, err)
	}
	after, err := readOwner(root.Dir(src))
	if err != nil || after != before {
		t.Fatal("directory-delete failure lost ownership:", after, err)
	}
	if _, err := os.Stat(filepath.Join(root.Dir(src), "bin")); !os.IsNotExist(err) {
		t.Fatal("fixture did not reach directory finalization", err)
	}
	if err := syscall.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	handle = syscall.InvalidHandle
	report, err = root.Maintain(context.Background(), MaintenanceOptions{Operation: "clean", All: true, Yes: true})
	if err != nil || !hasAction(report, root.Dir(src), "removed") {
		t.Fatal(report, err)
	}
}
