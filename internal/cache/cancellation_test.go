package cache

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gornkit/gorn/internal/app"
	"github.com/gornkit/gorn/internal/buildprofile"
	"github.com/gornkit/gorn/internal/script"
)

func TestCanceledDependencyDownloadRemovesStaging(t *testing.T) {
	started, stop := make(chan struct{}), make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(started) })
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	}))
	defer server.Close()
	defer close(stop)
	t.Setenv("GOPROXY", server.URL)
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("GOFLAGS", "")
	s, err := app.New(filepath.Join(t.TempDir(), "cancel.gorn"), []byte("//gorn:require example.test/stall v1.0.0\npackage main\nimport _ \"example.test/stall\"\nfunc main(){}"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := script.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	g, err := script.Generate(f)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := buildprofile.Discover(g.GoVersion, "")
	if err != nil {
		t.Fatal(err)
	}
	s = s.WithBuildIdentity(profile.Identity())
	root := Root(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := root.BuildContext(ctx, s, g, false, nil, profile); done <- err }()
	select {
	case <-started:
	case <-time.After(8 * time.Second):
		t.Fatal("dependency download did not start")
	}
	// A recognizable live stage is already exclusively protected before
	// dependency work becomes visible. A canceled contender cannot interrupt it.
	stages, err := os.ReadDir(root.TmpDir())
	if err != nil || len(stages) != 1 {
		t.Fatalf("active stages: %v %v", stages, err)
	}
	stageID := stages[0].Name()
	owner, err := readOwner(filepath.Join(root.TmpDir(), stageID))
	if err != nil || owner.StageID != stageID {
		t.Fatalf("stage ownership: %+v %v", owner, err)
	}
	if lock, err := root.lock(context.Background(), "stages", stageID, true, false); !errors.Is(err, errLockBusy) {
		_ = lock.close()
		t.Fatalf("live stage was not protected: %v", err)
	}
	waiter, stopWaiter := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer stopWaiter()
	if _, err := root.BuildContext(waiter, s, g, false, nil, profile); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiter cancellation: %v", err)
	}
	select {
	case err := <-done:
		t.Fatalf("waiter interrupted builder: %v", err)
	default:
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("dependency download did not stop")
	}
	entries, err := os.ReadDir(root.TmpDir())
	if err != nil || len(entries) != 0 {
		t.Fatalf("staging remains: %v %v", entries, err)
	}
	if _, ok := root.Lookup(s); ok {
		t.Fatal("canceled build was published")
	}
}
