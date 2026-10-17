package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Locks live outside removable data and are never unlinked: an open handle and
// a newly created pathname must never identify different coordination objects.
type fileLock struct {
	file *os.File
	once sync.Once
	err  error
}

func (r Root) lock(ctx context.Context, group, name string, exclusive, wait bool) (*fileLock, error) {
	if ctx == nil {
		return nil, errors.New("nil lock context")
	}
	if filepath.Base(name) != name || name == "." || name == "" {
		return nil, errors.New("invalid lock name")
	}
	dir := filepath.Join(r.Path(), "locks", group)
	if err := r.ensureDir(dir); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, name)
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() { //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
		return nil, fmt.Errorf("cache lock is not a regular file: %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // private cache coordination path
	if err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		err = tryFileLock(file, exclusive)
		if err == nil {
			lock := &fileLock{file: file}
			if err := ctx.Err(); err != nil {
				_ = lock.close()
				return nil, err
			}
			return lock, nil
		}
		if !errors.Is(err, errLockBusy) || !wait {
			_ = file.Close()
			return nil, err
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

var errLockBusy = errors.New("cache entry is busy")

func (l *fileLock) close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() { l.err = errors.Join(unlockFile(l.file), l.file.Close()) })
	return l.err
}

// ensureDir permits an operator-selected root alias but rejects symlinks below
// it. This is cooperative cache integrity, not an adversarial same-UID sandbox.
func (r Root) ensureDir(path string) error {
	rel, err := filepath.Rel(r.Path(), path)
	if err != nil || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("path outside cache: %s", path)
	}
	if err := os.MkdirAll(r.Path(), 0o700); err != nil { //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
		return err
	}
	return walkDirs(r.Path(), rel, true)
}

func (r Root) checkDir(path string) error {
	rel, err := filepath.Rel(r.Path(), path)
	if err != nil {
		return err
	}
	return walkDirs(r.Path(), rel, false)
}
