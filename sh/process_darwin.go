//go:build darwin && (amd64 || arm64)

//nolint:gosec // Platform-bounded syscall ABI: fixed-size aligned buffers and OS-sized PIDs.
package sh

import (
	"syscall"
	"unsafe"
)

func observeExit(pid int) (int, int, int, error) {
	// Darwin siginfo_t has no Linux alignment word before si_pid.
	var info [16]uint64
	for {
		_, _, errno := syscall.Syscall6(syscall.SYS_WAITID, 1, uintptr(pid), uintptr(unsafe.Pointer(&info)), syscall.WEXITED|syscall.WNOWAIT|syscall.WNOHANG, 0, 0)
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			return 0, 0, 0, errno
		}
		words := (*[32]int32)(unsafe.Pointer(&info))
		return int(words[3]), int(words[2]), int(words[5]), nil
	}
}

type signalMask uint32

func blockTerminalStop() (signalMask, error) {
	mask := signalMask(1 << (uint(syscall.SIGTTOU) - 1))
	var old signalMask
	_, _, errno := syscall.RawSyscall(syscall.SYS___PTHREAD_SIGMASK, 1, uintptr(unsafe.Pointer(&mask)), uintptr(unsafe.Pointer(&old)))
	if errno != 0 {
		return old, errno
	}
	return old, nil
}

func restoreSignalMask(old signalMask) error {
	_, _, errno := syscall.RawSyscall(syscall.SYS___PTHREAD_SIGMASK, 3, uintptr(unsafe.Pointer(&old)), 0)
	if errno != 0 {
		return errno
	}
	return nil
}
