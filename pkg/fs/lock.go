package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Lock is a cross-process advisory lock backed by an atomic directory create.
// os.Mkdir is atomic on all supported platforms, so it needs no third-party
// dependency (keeps fs stdlib-only).
type Lock struct {
	dir string
}

const (
	lockPoll    = 20 * time.Millisecond
	lockTimeout = 60 * time.Second
	// ponytail: a crashed holder leaves the lock dir behind. Time-based steal
	// is the ceiling; if two processes steal at once one loses the race
	// harmlessly. Per-holder PID/heartbeat files if this ever matters.
	lockStaleAfter = 120 * time.Second
)

// Lock acquires the per-appKey build lock, blocking until it is free, the
// holder's lock looks stale, or lockTimeout elapses.
func (c CacheRoot) Lock(appKey AppKey) (*Lock, error) {
	locksDir := c.LocksDir()
	if err := os.MkdirAll(locksDir, 0o700); err != nil {
		return nil, err
	}
	lockDir := filepath.Join(locksDir, string(appKey)[:12]+".lock")

	deadline := time.Now().Add(lockTimeout)
	for {
		err := os.Mkdir(lockDir, 0o700)
		if err == nil {
			return &Lock{dir: lockDir}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if info, statErr := os.Stat(lockDir); statErr == nil {
			if time.Since(info.ModTime()) > lockStaleAfter {
				_ = os.Remove(lockDir)
				continue
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("acquire lock %s: timed out after %s", lockDir, lockTimeout)
		}
		time.Sleep(lockPoll)
	}
}

// Release frees the lock. Safe to call on a nil or already-released Lock.
func (l *Lock) Release() error {
	if l == nil || l.dir == "" {
		return nil
	}
	dir := l.dir
	l.dir = ""
	return os.Remove(dir)
}
