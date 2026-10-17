//nolint:gosec // Controlled test executables, release markers, and private fixture paths.
package cache

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gornkit/gorn/pkg/progress"
)

type diagnosticBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *diagnosticBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}
func (b *diagnosticBuffer) text() string { b.mu.Lock(); defer b.mu.Unlock(); return b.String() }

func TestStreamingDiagnosticFixture(t *testing.T) {
	if os.Getenv("GORN_STREAM_FIXTURE") != "1" {
		return
	}
	for _, part := range []string{"go: https://user:", "private-token", "@example.test/file?query=hidden\n"} {
		_, _ = io.WriteString(os.Stderr, part)
		time.Sleep(10 * time.Millisecond)
	}
	for {
		if _, err := os.Stat(os.Getenv("GORN_STREAM_RELEASE")); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, _ = io.WriteString(os.Stderr, "final EOF diagnostic https://user:truncated-secret")
	os.Exit(2)
}

func TestResolverStreamsBeforeExitAndDoesNotReplay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output diagnosticBuffer
	ctx, report := progress.New(ctx, &output, true)
	defer func() { _ = report.Finish() }()
	release := filepath.Join(t.TempDir(), "release")
	r := &resolver{ctx: ctx, goPath: os.Args[0], env: append(os.Environ(), "GORN_STREAM_FIXTURE=1", "GORN_STREAM_RELEASE="+release)}
	done := make(chan error, 1)
	completed := make(chan struct{})
	go func() {
		defer close(completed)
		_, err := r.command("-test.run=^TestStreamingDiagnosticFixture$")
		done <- err
	}()
	defer func() {
		cancel()
		select {
		case <-completed:
		case <-time.After(3 * time.Second):
			t.Error("diagnostic fixture did not reap")
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(output.text(), "[redacted]@example.test/file?[redacted]\n") {
		if time.Now().After(deadline) {
			t.Fatalf("diagnostic not streamed:\n%s", output.text())
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case err := <-done:
		t.Fatalf("command exited before release: %v", err)
	default:
	}
	if strings.Contains(output.text(), "private-token") || strings.Contains(output.text(), "hidden") {
		t.Fatalf("credential leaked:\n%s", output.text())
	}
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || strings.Contains(err.Error(), "example.test") || errors.Is(err, context.Canceled) {
			t.Fatalf("summary replayed diagnostics or lost failure: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("fixture did not exit")
	}
	if strings.Contains(output.text(), "truncated-secret") || !strings.HasSuffix(output.text(), "final EOF diagnostic [redacted URL fragment]") {
		t.Fatalf("EOF diagnostic lost:\n%s", output.text())
	}
}
