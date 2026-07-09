package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	lockPoll       = 20 * time.Millisecond
	lockTimeout    = 60 * time.Second
	lockStaleAfter = 120 * time.Second
)

// Lock is a held directory-based mutex for an app's cache slot. The mutex is
// implemented with os.Mkdir: the directory either exists (held) or does not
// (free). This is atomic on all filesystems that gorn's cache can live on.
//
// Acquire with CacheRoot.Lock; release with (*Lock).Release.
type Lock struct {
	dir string
}

// Lock acquires the per-app directory mutex for appKey. It spins with
// lockPoll sleeps, steals stale locks (held longer than lockStaleAfter), and
// returns an error if lockTimeout elapses without acquiring.
func (c CacheRoot) Lock(appKey AppKey) (*Lock, error) {
	if err := os.MkdirAll(c.LocksDir(), 0o700); err != nil {
		return nil, fmt.Errorf("create locks dir: %w", err)
	}

	// Lock dir name matches the AppDir prefix — same 12-char key slice.
	lockDir := filepath.Join(c.LocksDir(), string(appKey)[:12]+".lock")

	deadline := time.Now().Add(lockTimeout)
	for {
		err := os.Mkdir(lockDir, 0o700)
		if err == nil {
			// Acquired.
			return &Lock{dir: lockDir}, nil
		}

		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire lock: %w", err)
		}

		// Lock dir already exists — check for staleness.
		info, statErr := os.Stat(lockDir)
		if statErr == nil && time.Since(info.ModTime()) > lockStaleAfter {
			// Stale lock: remove and retry immediately.
			_ = os.Remove(lockDir)
			continue
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("acquire lock: timed out after %s", lockTimeout)
		}

		time.Sleep(lockPoll)
	}
}

// Release removes the lock directory, freeing the mutex. It is safe to call
// on a nil *Lock (no-op).
func (l *Lock) Release() error {
	if l == nil {
		return nil
	}
	return os.Remove(l.dir)
}
