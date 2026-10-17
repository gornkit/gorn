//go:build linux || darwin

package cache

import (
	"errors"
	"os"
	"syscall"
)

func tryFileLock(file *os.File, exclusive bool) error {
	mode := syscall.LOCK_SH
	if exclusive {
		mode = syscall.LOCK_EX
	}
	err := syscall.Flock(int(file.Fd()), mode|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return errLockBusy
	}
	return err
}
func unlockFile(file *os.File) error { return syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }
