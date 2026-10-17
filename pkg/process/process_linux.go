//go:build linux && (amd64 || arm64)

//nolint:gosec // Platform-bounded syscall ABI: fixed-size aligned buffers and OS-sized PIDs.
package process

import (
	"syscall"
	"unsafe"
)

func observeExit(pid int) (int, int, int, error) {
	// Linux 64-bit siginfo_t: signum/errno/code, alignment, pid/uid/status.
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
		return int(words[4]), int(words[2]), int(words[6]), nil
	}
}

type signalMask uint64

func blockTerminalStop() (signalMask, error) {
	mask := signalMask(1 << (uint(syscall.SIGTTOU) - 1))
	var old signalMask
	_, _, errno := syscall.RawSyscall6(syscall.SYS_RT_SIGPROCMASK, 0, uintptr(unsafe.Pointer(&mask)), uintptr(unsafe.Pointer(&old)), unsafe.Sizeof(mask), 0, 0)
	if errno != 0 {
		return old, errno
	}
	return old, nil
}

func restoreSignalMask(old signalMask) error {
	_, _, errno := syscall.RawSyscall6(syscall.SYS_RT_SIGPROCMASK, 2, uintptr(unsafe.Pointer(&old)), 0, unsafe.Sizeof(old), 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
