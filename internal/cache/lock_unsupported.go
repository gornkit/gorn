//go:build !linux && !darwin && !windows

package cache

import (
	"fmt"
	"os"
)

func tryFileLock(_ *os.File, _ bool) error {
	return fmt.Errorf("cache locking is unsupported on this operating system")
}
func unlockFile(_ *os.File) error { return nil }
