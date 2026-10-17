package cache

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gornkit/gorn/pkg/app"
)

const ownerName = "ownership.json"

type ownership struct {
	Schema  int    `json:"schema"`
	Layout  int    `json:"layout"`
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	StageID string `json:"stage_id"`
	Source  string `json:"source"`
	Created string `json:"created"`
	BuiltAt string `json:"built_at,omitempty"`
}

func validKey(key string) bool {
	if len(key) != 64 || key != strings.ToLower(key) {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}

func validStage(id string) bool {
	return strings.HasPrefix(id, "build-") && len(id) > 6 && filepath.Base(id) == id
}

func regular(path string) bool {
	info, err := os.Lstat(path) //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
	return err == nil && info.Mode().IsRegular()
}

func walkDirs(root, rel string, create bool) error {
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return errors.New("path outside cache")
	}
	path := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		path = filepath.Join(path, part)
		info, err := os.Lstat(path) //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
		if os.IsNotExist(err) && create {
			err = os.Mkdir(path, 0o700) //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
			if err != nil && !os.IsExist(err) {
				return err
			}
			info, err = os.Lstat(path) //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("cache directory is not a plain directory: %s", path)
		}
	}
	return nil
}

func readOwner(dir string) (ownership, error) {
	var owner ownership
	path := filepath.Join(dir, ownerName)
	info, err := os.Lstat(path) //nolint:gosec // G703: internal cache ownership path, validated before mutation.
	if err != nil {
		return owner, err
	}
	if !info.Mode().IsRegular() {
		return owner, errors.New("nonregular cache ownership record")
	}
	data, err := os.ReadFile(path) //nolint:gosec // validated internal cache path
	if err != nil {
		return owner, err
	}
	if err := json.Unmarshal(data, &owner); err != nil {
		return owner, err
	}
	if owner.Schema != 1 || owner.Layout != manifestSchema || !validKey(owner.Key) || !validStage(owner.StageID) || (owner.Kind != "stage" && owner.Kind != "entry") {
		return owner, errors.New("invalid cache ownership record")
	}
	if _, err := time.Parse(time.RFC3339Nano, owner.Created); err != nil {
		return owner, err
	}
	if owner.Kind == "entry" {
		if _, err := time.Parse(time.RFC3339Nano, owner.BuiltAt); err != nil {
			return owner, err
		}
	}
	return owner, nil
}

func writeOwner(dir string, owner ownership) error {
	data, err := json.MarshalIndent(owner, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, ownerName+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil { //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, ownerName)) //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
}

func (r Root) ownedEntry(s *app.Source) error {
	dir := r.Dir(s)
	if err := r.checkDir(dir); err != nil {
		return err
	}
	owner, err := readOwner(dir)
	if err != nil {
		return err
	}
	if owner.Kind != "entry" || owner.Key != string(s.Key()) {
		return errors.New("cache ownership does not match entry")
	}
	return nil
}

// Entry is a leased executable. Close after execution/export. Copies share the
// same private, idempotent cleanup state; exported paths confer no deletion right.
type Entry struct {
	AppRoot      string
	MainFilePath string
	ModFilePath  string
	AppPath      string
	CacheHit     bool
	state        *entryState
}
type entryState struct {
	once    sync.Once
	err     error
	lease   *fileLock
	cleanup func() error
}

func makeEntry(dir string, lock *fileLock, cleanup func() error, hit bool) Entry {
	return Entry{AppRoot: dir, MainFilePath: filepath.Join(dir, "main.gorn.go"), ModFilePath: filepath.Join(dir, "go.mod"), AppPath: filepath.Join(dir, "bin", binName), CacheHit: hit, state: &entryState{lease: lock, cleanup: cleanup}}
}

// Close releases the lease and removes a private bypass stage, retaining its
// ownership record if payload deletion fails. Cached entries are never deleted.
func (e Entry) Close() error {
	if e.state == nil {
		return nil
	}
	e.state.once.Do(func() {
		if e.state.cleanup != nil {
			e.state.err = e.state.cleanup()
		}
		e.state.err = errors.Join(e.state.err, e.state.lease.close())
	})
	return e.state.err
}

// Remove is retained as an alias for Close. It never deletes a published entry.
func (e Entry) Remove() error { return e.Close() }

// Capture through a live handle: Windows pathname Stat loads identity lazily,
// which would otherwise compare a replacement to itself after a rename.
func captureIdentity(dir string) (os.FileInfo, error) {
	file, err := os.Open(dir) //nolint:gosec // G304: private, validated staging directory.
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	return info, errors.Join(err, file.Close())
}

func removeStage(dir, stageID string, identity os.FileInfo) error {
	info, err := os.Lstat(dir) //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || !os.SameFile(info, identity) {
		return errors.New("stage identity changed; refusing cleanup")
	}
	owner, err := readOwner(dir)
	if err != nil || owner.StageID != stageID {
		return errors.New("stage ownership changed; refusing cleanup")
	}
	return removeOwnedDir(context.Background(), dir, owner, identity)
}
