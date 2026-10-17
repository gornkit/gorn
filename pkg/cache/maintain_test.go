//nolint:gosec // Controlled ownership fixtures, intentional corruption, locks and filesystem replacement.
package cache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gornkit/gorn/pkg/app"
)

func setBuildTime(t *testing.T, dir string, when time.Time) {
	t.Helper()
	owner, err := readOwner(dir)
	if err != nil {
		t.Fatal(err)
	}
	owner.BuiltAt = when.UTC().Format(time.RFC3339Nano)
	if err := writeOwner(dir, owner); err != nil {
		t.Fatal(err)
	}
}

func ownedAux(t *testing.T, root Root, source *app.Source, kind string) string {
	t.Helper()
	var dir string
	if kind == "stage" {
		dir = filepath.Join(root.TmpDir(), "build-abandoned")
	} else {
		dir = filepath.Join(root.Path(), "quarantine", string(source.Key())+"-abandoned")
	}
	stageBuild(t, dir, source)
	owner, err := readOwner(dir)
	if err != nil {
		t.Fatal(err)
	}
	if kind == "stage" {
		owner.StageID = filepath.Base(dir)
		owner.Kind = "stage"
	}
	if err := writeOwner(dir, owner); err != nil {
		t.Fatal(err)
	}
	return dir
}

func mustInspect(t *testing.T, root Root) Inventory {
	t.Helper()
	inventory, err := root.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return inventory
}

func hasAction(report MaintenanceReport, path, state string) bool {
	for _, action := range report.Actions {
		if action.Path == path && action.State == state {
			return true
		}
	}
	return false
}

func TestCleanSkipsLeasedDataAndPreservesUnknown(t *testing.T) {
	root := Root(t.TempDir())
	src := mustSource(t, "active.gorn", "body")
	stageBuild(t, root.Dir(src), src)
	stage := ownedAux(t, root, src, "stage")
	quarantine := ownedAux(t, root, src, "quarantine")
	unknown := filepath.Join(root.Path(), "unrelated")
	if err := os.WriteFile(unknown, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	entry, hit, err := root.Acquire(context.Background(), src)
	if err != nil || !hit {
		t.Fatal(hit, err)
	}
	defer func() { _ = entry.Close() }()
	stageLock, err := root.lock(context.Background(), "stages", filepath.Base(stage), true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stageLock.close() }()
	report, err := root.Maintain(context.Background(), MaintenanceOptions{Operation: "clean", All: true, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root.Dir(src), stage, quarantine} {
		if !hasAction(report, path, "skipped-active") {
			t.Fatal("active object not skipped:", path, report.Actions)
		}
	}
	if err := entry.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stageLock.close(); err != nil {
		t.Fatal(err)
	}
	report, err = root.Maintain(context.Background(), MaintenanceOptions{Operation: "clean", All: true, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root.Dir(src), stage, quarantine} {
		if !hasAction(report, path, "removed") {
			t.Fatal(report.Actions)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("owned data survived", path, err)
		}
	}
	if data, err := os.ReadFile(unknown); err != nil || string(data) != "keep" {
		t.Fatal("unknown data modified", err)
	}
	if report.After.Managed.Bytes != 0 || report.After.Quarantine.Bytes != 0 || report.After.Stages.Bytes != 0 || report.After.Unmanaged.Bytes != 4 {
		t.Fatal(report.After)
	}
	if !regular(filepath.Join(root.Path(), "locks", "stages", filepath.Base(stage))) {
		t.Fatal("stable lock removed")
	}
}

func TestDryRunIsNonmutatingAndPredictive(t *testing.T) {
	root := Root(t.TempDir())
	src := mustSource(t, "preview.gorn", "body")
	stageBuild(t, root.Dir(src), src)
	before := mustInspect(t, root)
	report, err := root.Maintain(context.Background(), MaintenanceOptions{Operation: "clean", All: true, DryRun: true})
	if err != nil || report.Predicted == nil || report.Predicted.Managed.Bytes != 0 || report.After.Managed != before.Totals.Managed {
		t.Fatal(report, err)
	}
	if _, hit := root.Lookup(src); !hit {
		t.Fatal("dry-run deleted binary")
	}
	if _, err := os.Stat(filepath.Join(root.Path(), "locks")); !os.IsNotExist(err) {
		t.Fatal("dry-run created lock metadata", err)
	}
}

func TestSizePruneContinuesPastBusyOldest(t *testing.T) {
	root := Root(t.TempDir())
	sources := []*app.Source{mustSource(t, "old.gorn", "old"), mustSource(t, "mid.gorn", "mid"), mustSource(t, "new.gorn", "new")}
	now := time.Now()
	for i, src := range sources {
		stageBuild(t, root.Dir(src), src)
		setBuildTime(t, root.Dir(src), now.Add(-time.Duration(3-i)*time.Hour))
	}
	entry, hit, err := root.Acquire(context.Background(), sources[0])
	if err != nil || !hit {
		t.Fatal(hit, err)
	}
	defer func() { _ = entry.Close() }()
	inventory := mustInspect(t, root)
	var middleBytes int64
	for _, item := range inventory.Items {
		if item.Key == string(sources[1].Key()) {
			middleBytes = item.SizeBytes
		}
	}
	budget := inventory.Totals.Managed.Bytes - middleBytes
	report, err := root.Maintain(context.Background(), MaintenanceOptions{Operation: "prune", Yes: true, MaxBytes: &budget})
	if err != nil || report.UnmetBudget || report.After.Managed.Bytes > budget {
		t.Fatal(report, err)
	}
	if !hasAction(report, root.Dir(sources[0]), "skipped-active") || !hasAction(report, root.Dir(sources[1]), "removed") {
		t.Fatal(report.Actions)
	}
	if _, hit := root.Lookup(sources[2]); !hit {
		t.Fatal("pruned beyond satisfied budget")
	}
}

func TestUnmetBudgetReportsActiveBytes(t *testing.T) {
	root := Root(t.TempDir())
	src := mustSource(t, "busy.gorn", "body")
	stageBuild(t, root.Dir(src), src)
	lock, err := root.keyLock(context.Background(), src, "mutation", true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.close() }()
	zero := int64(0)
	report, err := root.Maintain(context.Background(), MaintenanceOptions{Operation: "prune", Yes: true, MaxBytes: &zero})
	if err == nil || !report.UnmetBudget || report.After.Managed.Bytes == 0 || !report.After.Managed.Complete {
		t.Fatal(report, err)
	}
	if !hasAction(report, root.Dir(src), "skipped-active") {
		t.Fatal(report.Actions)
	}
}

func TestPruneAgeUsesOwnerEvidenceAndExcludesAuxiliaryData(t *testing.T) {
	root := Root(t.TempDir())
	old := mustSource(t, "old.gorn", "old")
	future := mustSource(t, "future.gorn", "new")
	stageBuild(t, root.Dir(old), old)
	stageBuild(t, root.Dir(future), future)
	setBuildTime(t, root.Dir(old), time.Now().Add(-48*time.Hour))
	setBuildTime(t, root.Dir(future), time.Now().Add(48*time.Hour))
	if err := os.Remove(filepath.Join(root.Dir(old), manifestName)); err != nil {
		t.Fatal(err)
	}
	stage := ownedAux(t, root, old, "stage")
	quarantine := ownedAux(t, root, old, "quarantine")
	age := 24 * time.Hour
	report, err := root.Maintain(context.Background(), MaintenanceOptions{Operation: "prune", Yes: true, MaxAge: &age})
	if err != nil || !hasAction(report, root.Dir(old), "removed") {
		t.Fatal(report, err)
	}
	for _, path := range []string{root.Dir(future), stage, quarantine} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal("age pruning selected wrong namespace/time:", path, err)
		}
	}
}

func TestCleanupRejectsReplacedSnapshotGeneration(t *testing.T) {
	for _, copiedOwner := range []bool{false, true} {
		t.Run(map[bool]string{false: "new owner", true: "copied owner"}[copiedOwner], func(t *testing.T) {
			root := Root(t.TempDir())
			src := mustSource(t, "stale.gorn", "old")
			stageBuild(t, root.Dir(src), src)
			_, candidates, err := root.inspect(context.Background())
			if err != nil || len(candidates) != 1 {
				t.Fatal(candidates, err)
			}
			selected := candidates[0]
			if err := os.Rename(root.Dir(src), filepath.Join(t.TempDir(), "old")); err != nil {
				t.Fatal(err)
			}
			stageBuild(t, root.Dir(src), src)
			if copiedOwner {
				if err := writeOwner(root.Dir(src), selected.owner); err != nil {
					t.Fatal(err)
				}
			}
			action, err := root.removeCandidate(context.Background(), selected, false)
			if err != nil || action.State != "skipped-changed" {
				t.Fatal(action, err)
			}
			if _, hit := root.Lookup(src); !hit {
				t.Fatal("stale selection deleted replacement")
			}
		})
	}
}

func TestCleanSelectorAndIntentValidation(t *testing.T) {
	for _, options := range []MaintenanceOptions{
		{Operation: "clean", All: true},
		{Operation: "clean", Yes: true},
		{Operation: "clean", All: true, KeyPrefix: "a", Yes: true},
		{Operation: "clean", KeyPrefix: "../", Yes: true},
		{Operation: "prune", Yes: true},
		{Operation: "prune", Yes: true, MaxBytes: new(int64(-1))},
		{Operation: "prune", Yes: true, MaxAge: new(time.Duration(-1))},
	} {
		if err := options.Validate(); !errors.Is(err, ErrMaintenanceRequest) {
			t.Fatal(options, err)
		}
	}
	root := Root(t.TempDir())
	prefixes := map[byte]*app.Source{}
	var first, second *app.Source
	for i := 0; i < 100; i++ {
		src := mustSource(t, "selector.gorn", string(rune('a'+i)))
		prefix := string(src.Key())[0]
		if previous := prefixes[prefix]; previous != nil {
			first, second = previous, src
			break
		}
		prefixes[prefix] = src
	}
	if first == nil {
		t.Fatal("failed to construct ambiguous prefix")
	}
	stageBuild(t, root.Dir(first), first)
	stageBuild(t, root.Dir(second), second)
	if _, err := root.Maintain(context.Background(), MaintenanceOptions{Operation: "clean", KeyPrefix: string(first.Key())[:1], Yes: true}); err == nil {
		t.Fatal("ambiguous key accepted")
	}
	report, err := root.Maintain(context.Background(), MaintenanceOptions{Operation: "clean", KeyPrefix: string(first.Key()), Yes: true})
	if err != nil || !hasAction(report, root.Dir(first), "removed") {
		t.Fatal(report, err)
	}
	if _, hit := root.Lookup(second); !hit {
		t.Fatal("exact selector removed another key")
	}
}

func TestIncompleteManagedBytesRefuseSizeDeletion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission fixture")
	}
	root := Root(t.TempDir())
	blocked := mustSource(t, "blocked.gorn", "new")
	old := mustSource(t, "old.gorn", "old")
	stageBuild(t, root.Dir(blocked), blocked)
	stageBuild(t, root.Dir(old), old)
	setBuildTime(t, root.Dir(old), time.Now().Add(-48*time.Hour))
	hidden := filepath.Join(root.Dir(blocked), "hidden")
	if err := os.Mkdir(hidden, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "payload"), []byte("unknown"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(hidden, 0); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(hidden, 0o700) }()
	if _, err := os.ReadDir(hidden); err == nil {
		t.Skip("host bypasses fixture permission restrictions")
	}
	zero := int64(0)
	report, err := root.Maintain(context.Background(), MaintenanceOptions{Operation: "prune", Yes: true, MaxBytes: &zero})
	if err == nil || !report.UnmetBudget || len(report.Actions) != 0 {
		t.Fatal(report, err)
	}
	if _, hit := root.Lookup(old); !hit {
		t.Fatal("unknown size drove deletion")
	}
	age := 24 * time.Hour
	report, err = root.Maintain(context.Background(), MaintenanceOptions{Operation: "prune", Yes: true, MaxBytes: &zero, MaxAge: &age})
	if err == nil || !hasAction(report, root.Dir(old), "removed") || !report.UnmetBudget {
		t.Fatal(report, err)
	}
	if _, err := os.Stat(root.Dir(blocked)); err != nil {
		t.Fatal("unknown size selected recent entry", err)
	}
}

func TestPayloadCleanupNeverTraversesLinks(t *testing.T) {
	root := Root(t.TempDir())
	src := mustSource(t, "links.gorn", "body")
	stageBuild(t, root.Dir(src), src)
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "sentinel")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root.Dir(src), "linked")); err != nil {
		t.Skip(err)
	}
	report, err := root.Maintain(context.Background(), MaintenanceOptions{Operation: "clean", All: true, Yes: true})
	if err != nil || !hasAction(report, root.Dir(src), "removed") {
		t.Fatal(report, err)
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "keep" {
		t.Fatal("outside target deleted", err)
	}
}

func TestCanceledMaintenanceNeverReportsCompleteZero(t *testing.T) {
	root := Root(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	zero := int64(0)
	report, err := root.Maintain(ctx, MaintenanceOptions{Operation: "prune", Yes: true, MaxBytes: &zero})
	if !errors.Is(err, context.Canceled) || report.After.Managed.Complete || !report.UnmetBudget {
		t.Fatal(report, err)
	}
	if !strings.Contains(report.Error, "canceled") {
		t.Fatal(report)
	}
}

type cleanupObserver struct {
	context.Context
	marker string
	cancel context.CancelFunc
}

func (c cleanupObserver) Err() error {
	if _, err := os.Stat(c.marker); os.IsNotExist(err) {
		c.cancel()
	}
	return c.Context.Err()
}

func TestCancellationBetweenPayloadRemovalsRetainsAuthority(t *testing.T) {
	root := Root(t.TempDir())
	src := mustSource(t, "cancel-clean.gorn", "body")
	stageBuild(t, root.Dir(src), src)
	marker := filepath.Join(root.Dir(src), "a-first")
	if err := os.WriteFile(marker, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observed := cleanupObserver{Context: ctx, marker: marker, cancel: cancel}
	report, err := root.Maintain(observed, MaintenanceOptions{Operation: "clean", All: true, Yes: true})
	if !errors.Is(err, context.Canceled) || report.After.Managed.Complete {
		t.Fatal(report, err)
	}
	if _, err := readOwner(root.Dir(src)); err != nil {
		t.Fatal("partial cancellation lost ownership:", err)
	}
	if _, err := os.Stat(root.BinPath(src)); err != nil {
		t.Fatal("cleanup continued after cancellation:", err)
	}
	lock, err := root.keyLock(context.Background(), src, "mutation", true, false)
	if err != nil {
		t.Fatal("canceled cleanup leaked lock:", err)
	}
	_ = lock.close()
	if _, err := root.Maintain(context.Background(), MaintenanceOptions{Operation: "clean", All: true, Yes: true}); err != nil {
		t.Fatal("cannot retry canceled cleanup:", err)
	}
}
