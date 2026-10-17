//nolint:gosec // Controlled private fixtures, native handles, and a copied test executable.
package cache

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWindowsCleanupRetainsOwnershipWhenPayloadIsInUse(t *testing.T) {
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
	payload := filepath.Join(dir, "payload")
	if err := os.WriteFile(payload, []byte("held"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := syscall.UTF16PtrFromString(payload)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(path, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if handle != syscall.InvalidHandle {
			_ = syscall.CloseHandle(handle)
		}
	}()
	identity, err := captureIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := root.lock(context.Background(), "stages", id, true, false)
	if err != nil {
		t.Fatal(err)
	}
	entry := makeEntry(dir, lock, func() error { return removeStage(dir, id, identity) }, false)
	if err := entry.Close(); err == nil {
		t.Fatal("cleanup ignored locked Windows payload")
	}
	if !regular(filepath.Join(dir, ownerName)) || !regular(payload) {
		t.Fatal("failed cleanup lost payload or ownership evidence")
	}
	// Close releases the stage lock even when cleanup failed; later maintenance
	// can retry after the OS releases the payload. Entry Close itself is idempotent.
	retry, err := root.lock(context.Background(), "stages", id, true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = retry.close() }()
	if err := syscall.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	handle = syscall.InvalidHandle
	if err := removeStage(dir, id, identity); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsRunningExecutableKeepsStageRecoverable(t *testing.T) {
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
	payload := filepath.Join(dir, "child.exe")
	executable, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(payload, executable, 0o700); err != nil {
		t.Fatal(err)
	}
	identity, err := captureIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := root.lock(context.Background(), "stages", id, true, false)
	if err != nil {
		t.Fatal(err)
	}
	entry := makeEntry(dir, lock, func() error { return removeStage(dir, id, identity) }, false)
	defer func() { _ = entry.Close() }()
	child := exec.Command(payload, "-test.run=^TestLockProcessHelper$")
	child.Env = append(os.Environ(), "GORN_LOCK_HELPER=1", "GORN_LOCK_ROOT="+root.Path())
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root.Path(), "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("executable did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := entry.Close(); err == nil {
		t.Fatal("cleanup removed a running Windows executable")
	}
	if !regular(filepath.Join(dir, ownerName)) {
		t.Fatal("cleanup lost stage ownership")
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	if err := removeStage(dir, id, identity); err != nil {
		t.Fatal("stage not recoverable after process exit:", err)
	}
}
