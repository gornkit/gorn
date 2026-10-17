package cache

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	cacheKernel     = syscall.NewLazyDLL("kernel32.dll")
	cacheLockFile   = cacheKernel.NewProc("LockFileEx")
	cacheUnlockFile = cacheKernel.NewProc("UnlockFileEx")
)

func tryFileLock(file *os.File, exclusive bool) error {
	flags := uintptr(1) // LOCKFILE_FAIL_IMMEDIATELY, including shared requests.
	if exclusive {
		flags |= 2
	}
	var overlapped syscall.Overlapped
	result, _, err := cacheLockFile.Call(file.Fd(), flags, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped))) //nolint:gosec // G103: synchronous Win32 call; live zeroed OVERLAPPED retained through return.
	runtime.KeepAlive(&overlapped)
	if result != 0 {
		return nil
	}
	if errors.Is(err, syscall.Errno(33)) {
		return errLockBusy
	}
	return err
}

func unlockFile(file *os.File) error {
	var overlapped syscall.Overlapped
	result, _, err := cacheUnlockFile.Call(file.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&overlapped))) //nolint:gosec // G103: synchronous Win32 call; live zeroed OVERLAPPED retained through return.
	runtime.KeepAlive(&overlapped)
	if result != 0 {
		return nil
	}
	return err
}
