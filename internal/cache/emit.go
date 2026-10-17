package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gornkit/gorn/internal/app"
	"github.com/gornkit/gorn/internal/buildprofile"
	"github.com/gornkit/gorn/internal/progress"
	"github.com/gornkit/gorn/internal/script"
)

// Build creates or acquires a leased executable. Close the returned Entry.
// NoCache builds have only a private stage lease and never acquire a key lock.
func (r Root) Build(s *app.Source, g *script.Generated, noCache bool, bundle *script.HelperBundle, profile *buildprofile.Profile) (Entry, error) {
	return r.BuildContext(context.Background(), s, g, noCache, bundle, profile)
}

// BuildContext cancels resolver/build children and removes unpublished staging.
func (r Root) BuildContext(ctx context.Context, s *app.Source, g *script.Generated, noCache bool, bundle *script.HelperBundle, profile *buildprofile.Profile) (result Entry, resultErr error) {
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, result.Close())
			result = Entry{}
		}
	}()
	if ctx == nil {
		return result, fmt.Errorf("nil build context")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if profile == nil || s.BuildIdentity() != profile.Identity() {
		return result, fmt.Errorf("source must be bound to the selected build profile")
	}
	var mutation *fileLock
	if !noCache {
		if entry, hit, err := r.Acquire(ctx, s); err != nil {
			return result, err
		} else if hit {
			return entry, nil
		}
		if err := progress.Phase(ctx, "waiting for cache builder"); err != nil {
			return result, err
		}
		var err error
		mutation, err = r.keyLock(ctx, s, "mutation", true, true)
		if err != nil {
			return result, err
		}
		defer func() { resultErr = errors.Join(resultErr, mutation.close()) }()
		if entry, hit, err := r.Acquire(ctx, s); err != nil {
			return result, err
		} else if hit {
			return entry, nil
		}
	}
	if err := progress.Phase(ctx, "preparing build"); err != nil {
		return result, err
	}

	if err := r.ensureDir(r.TmpDir()); err != nil {
		return result, err
	}
	buildDir, err := os.MkdirTemp(r.TmpDir(), "build-")
	if err != nil {
		return result, err
	}
	stageID := filepath.Base(buildDir)
	stageLock, err := r.lock(ctx, "stages", stageID, true, true)
	if err != nil {
		_ = os.Remove(buildDir) //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
		return result, err
	}
	owner := ownership{Schema: 1, Layout: manifestSchema, Key: string(s.Key()), Kind: "stage", StageID: stageID, Source: s.Path(), Created: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := writeOwner(buildDir, owner); err != nil {
		_ = stageLock.close()
		_ = os.RemoveAll(buildDir) //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
		return result, err
	}
	identity, err := captureIdentity(buildDir)
	if err != nil {
		_ = stageLock.close()
		return result, err
	}
	handoff := false
	defer func() {
		if !handoff {
			resultErr = errors.Join(resultErr, removeStage(buildDir, stageID, identity), stageLock.close())
		}
	}()

	if err := os.WriteFile(filepath.Join(buildDir, "main.gorn.go"), g.MainGenerated, 0o600); err != nil { //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
		return result, err
	}
	if err := os.WriteFile(filepath.Join(buildDir, "go.mod"), g.ModGenerated, 0o600); err != nil { //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
		return result, err
	}

	if g.BundleHelper {
		if bundle == nil {
			return result, fmt.Errorf("bundled helper source is unavailable")
		}
		if err := bundle.WriteTo(filepath.Join(buildDir, script.HelperDir)); err != nil {
			return result, err
		}
	}

	resolver := newResolver(buildDir, profile)
	resolver.ctx = ctx
	resolution, err := resolver.resolve(g, bundle)
	if err != nil {
		return result, fmt.Errorf("resolve dependencies: %w", err)
	}
	if err := resolver.saveResolution(resolution); err != nil {
		return result, err
	}
	if _, err := resolver.command(profile.BuildArgs(filepath.Join("bin", binName))...); err != nil {
		if strings.Contains(err.Error(), "cannot find module providing package") || strings.Contains(err.Error(), "no required module provides package") || strings.Contains(err.Error(), "missing go.sum entry") {
			return result, fmt.Errorf("%w\nOnly the declared graph is resolved; add or correct //gorn:require <module> <concrete-version> for the missing dependency", err)
		}
		return result, err
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}
	if noCache {
		handoff = true
		return makeEntry(buildDir, stageLock, func() error { return removeStage(buildDir, stageID, identity) }, false), nil
	}

	binSHA, err := fileSHA256(filepath.Join(buildDir, "bin", binName))
	if err != nil {
		return result, err
	}

	owner.Kind = "entry"
	owner.BuiltAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeOwner(buildDir, owner); err != nil {
		return result, err
	}
	// Manifest last: its presence means the candidate is complete.
	if err := WriteManifest(buildDir, newManifest(s, binSHA, profile.Record())); err != nil {
		return result, err
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := progress.Phase(ctx, "waiting to publish cache entry"); err != nil {
		return result, err
	}
	activity, err := r.keyLock(ctx, s, "activity", true, true)
	if err != nil {
		return result, err
	}
	if err := r.publish(ctx, buildDir, s); err != nil {
		_ = activity.close()
		return result, err
	}
	// Never upgrade a shared lock. Release EX and acquire SH while mutation is
	// still held, so no cooperative mutator can intervene during this downgrade.
	if err := activity.close(); err != nil {
		return result, err
	}
	activity, err = r.keyLock(ctx, s, "activity", false, true)
	if err != nil {
		return result, err
	}
	return makeEntry(r.Dir(s), activity, nil, false), nil
}

// Acquire validates under a shared activity lease. On a miss the lease is
// released before any builder waits on mutation, preventing a lock inversion.
func (r Root) Acquire(ctx context.Context, s *app.Source) (Entry, bool, error) {
	activity, err := r.keyLock(ctx, s, "activity", false, true)
	if err != nil {
		return Entry{}, false, err
	}
	if _, hit := r.Lookup(s); hit {
		if err := ctx.Err(); err != nil {
			_ = activity.close()
			return Entry{}, false, err
		}
		return makeEntry(r.Dir(s), activity, nil, true), true, nil
	}
	return Entry{}, false, activity.close()
}

func (r Root) keyLock(ctx context.Context, s *app.Source, kind string, exclusive, wait bool) (*fileLock, error) {
	key := string(s.Key())
	if !validKey(key) {
		return nil, errors.New("invalid cache key")
	}
	return r.lock(ctx, "entries", key+"."+kind, exclusive, wait)
}

// publish requires the mutation and exclusive activity locks. Unknown data is
// never adopted. Keep quarantined invalid data if installing its replacement
// fails, so publication errors cannot destroy the previous ownership evidence.
func (r Root) publish(ctx context.Context, buildDir string, s *app.Source) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dest := r.Dir(s)
	if err := r.ensureDir(filepath.Dir(dest)); err != nil {
		return err
	}
	if _, hit := r.Lookup(s); hit {
		return nil
	}
	if _, err := os.Lstat(dest); err == nil { //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
		if err := r.ownedEntry(s); err != nil {
			return fmt.Errorf("refuse to replace unrecognized cache entry %s: %w", dest, err)
		}
		quarantine := filepath.Join(r.Path(), "quarantine")
		if err := r.ensureDir(quarantine); err != nil {
			return err
		}
		slot, err := os.MkdirTemp(quarantine, string(s.Key())+"-")
		if err != nil {
			return err
		}
		if err := os.Remove(slot); err != nil { //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Rename(dest, slot); err != nil { //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
			return fmt.Errorf("quarantine invalid cache entry: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(buildDir, dest); err != nil { //nolint:gosec // G703: internal cache path, bounded by ownership and plain-directory validation.
		return fmt.Errorf("install cache entry: %w", err)
	}
	return nil
}
