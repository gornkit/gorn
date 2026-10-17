//nolint:gosec // Controlled Windows sharing-violation fixture and publication paths.
package cli

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestWindowsFailedNativeRenamePreservesIncumbent(t *testing.T) {
	dir := t.TempDir()
	staged := filepath.Join(dir, "candidate")
	destination := filepath.Join(dir, "existing")
	for path, text := range map[string]string{staged: "candidate", destination: "incumbent"} {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	path, err := syscall.UTF16PtrFromString(staged)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(path, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.CloseHandle(handle) }()
	if err := publishExecutable(staged, destination, true); err == nil {
		t.Fatal("renamed candidate despite denied delete sharing")
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "incumbent" {
		t.Fatal("failed native rename changed destination:", string(data), err)
	}
}
