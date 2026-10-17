package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// removeOwnedDir requires the caller's exclusive mutation/activity or stage
// locks. It rechecks private authority and preserves owner until payload success.
func removeOwnedDir(ctx context.Context, dir string, expected ownership, identity os.FileInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(dir) //nolint:gosec // G703: private owned-cache path; location, generation and eager identity checked under exclusive locks.
	if err != nil {
		return err
	}
	if identity == nil || !info.IsDir() || !os.SameFile(info, identity) {
		return errors.New("directory identity changed; refusing cleanup")
	}
	actual, err := readOwner(dir)
	if err != nil || actual != expected {
		return errors.New("ownership changed; refusing cleanup")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ownerName {
			continue
		}
		if err := removePayload(ctx, filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, ownerName)); err != nil { //nolint:gosec // G703: private owned-cache path; location, generation and eager identity checked under exclusive locks.
		return err
	}
	// Finish this short finalization pair once started. If removing the empty
	// directory fails, restore ownership only while it is still our same object.
	if err := os.Remove(dir); err != nil { //nolint:gosec // G703: private owned-cache path; location, generation and eager identity checked under exclusive locks.
		current, statErr := os.Lstat(dir) //nolint:gosec // G703: private owned-cache path; location, generation and eager identity checked under exclusive locks.
		if statErr == nil && current.IsDir() && os.SameFile(current, identity) {
			return errors.Join(err, writeOwner(dir, expected))
		}
		return errors.Join(err, statErr)
	}
	return nil
}

func removePayload(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(path) //nolint:gosec // G703: private owned-cache path; location, generation and eager identity checked under exclusive locks.
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := removePayload(ctx, filepath.Join(path, entry.Name())); err != nil {
				return err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Lstat does not treat a symlink as a directory: unlink the link itself.
	if err := os.Remove(path); err != nil { //nolint:gosec // G703: private owned-cache path; location, generation and eager identity checked under exclusive locks.
		return fmt.Errorf("remove payload: %w", err)
	}
	return nil
}
