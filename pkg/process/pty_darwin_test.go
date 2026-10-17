//go:build darwin && (amd64 || arm64)

//nolint:gosec // Controlled self-executable, test fixture paths, and PTY ABI calls.
package process

import (
	"bytes"
	"os"
	"syscall"
	"testing"
	"unsafe"
)

func openPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []uintptr{syscall.TIOCPTYGRANT, syscall.TIOCPTYUNLK} {
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), request, 0)
		if errno != 0 {
			_ = master.Close()
			t.Fatal(errno)
		}
	}
	var name [128]byte
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name)))
	if errno != 0 {
		_ = master.Close()
		t.Fatal(errno)
	}
	slave, err := os.OpenFile(string(bytes.TrimRight(name[:], "\x00")), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		t.Fatal(err)
	}
	return master, slave
}
