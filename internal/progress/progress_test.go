package progress

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}
func (b *lockedBuffer) text() string { b.mu.Lock(); defer b.mu.Unlock(); return b.String() }

func TestImmediateHeartbeatQuietAndJoin(t *testing.T) {
	for _, quiet := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "quiet"}[quiet], func(t *testing.T) {
			var out lockedBuffer
			ctx, r := newReporter(context.Background(), &out, quiet, 10*time.Millisecond)
			defer func() { _ = r.Finish() }()
			if err := Phase(ctx, "waiting for test work"); err != nil {
				t.Fatal(err)
			}
			if !quiet && !strings.Contains(out.text(), "gorn: waiting for test work...\n") {
				t.Fatal("initial phase was not synchronous")
			}
			if !quiet {
				deadline := time.Now().Add(time.Second)
				for !strings.Contains(out.text(), "gorn: still waiting") {
					if time.Now().After(deadline) {
						t.Fatal("missing heartbeat")
					}
					time.Sleep(time.Millisecond)
				}
			} else {
				time.Sleep(30 * time.Millisecond)
				if out.text() != "" {
					t.Fatal("quiet progress emitted")
				}
			}
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			before := out.text()
			time.Sleep(30 * time.Millisecond)
			if before != out.text() {
				t.Fatal("heartbeat survived Close")
			}
			if err := Phase(ctx, "must not restart"); err != nil {
				t.Fatal(err)
			}
			if before != out.text() {
				t.Fatal("phase survived Close")
			}
			if _, err := io.WriteString(r.Writer(), "compiler error\n"); err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(out.text(), "compiler error\n") {
				t.Fatal("diagnostics suppressed")
			}
		})
	}
}

type brokenWriter struct{ err error }

func (w brokenWriter) Write([]byte) (int, error) { return 0, w.err }

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestOutputFailureCancelsPendingWork(t *testing.T) {
	failure := errors.New("output closed")
	for _, tc := range []struct {
		writer io.Writer
		want   error
	}{{brokenWriter{failure}, failure}, {shortWriter{}, io.ErrShortWrite}} {
		ctx, r := newReporter(context.Background(), tc.writer, false, time.Hour)
		if err := Phase(ctx, "testing"); !errors.Is(err, tc.want) {
			t.Fatalf("phase=%v", err)
		}
		if ctx.Err() == nil {
			t.Fatal("IO failure did not cancel work")
		}
		if err := r.Finish(); !errors.Is(err, tc.want) {
			t.Fatalf("Finish=%v", err)
		}
	}
}

func TestDiagnosticsFrameBeforeSanitizing(t *testing.T) {
	var out bytes.Buffer
	ctx, r := New(context.Background(), &out, true)
	defer func() { _ = r.Finish() }()
	if err := Phase(ctx, "quiet"); err != nil {
		t.Fatal(err)
	}
	d := r.Diagnostics(func(s string, _ bool) string { return strings.ReplaceAll(s, "user:secret@", "[redacted]@") })
	for _, part := range []string{"go: https://user:", "secret", "@example.test/file\nnext", " fragment"} {
		if _, err := io.WriteString(d, part); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "secret") {
			t.Fatalf("secret escaped: %q", out.String())
		}
	}
	if out.String() != "go: https://[redacted]@example.test/file\n" {
		t.Fatalf("early output=%q", out.String())
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out.String(), "next fragment") {
		t.Fatalf("EOF lost: %q", out.String())
	}
	d = r.Diagnostics(func(s string, _ bool) string { return s })
	if _, err := io.WriteString(d, strings.Repeat("private", maxDiagnosticLine)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(d, "\nsmall\n"); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "private") || strings.Count(out.String(), "omitted Go diagnostic") != 1 || !strings.HasSuffix(out.String(), "small\n") {
		t.Fatalf("oversized line leaked: %q", out.String())
	}
}

func TestConcurrentDiagnosticsDoNotInterleaveLines(t *testing.T) {
	var out bytes.Buffer
	ctx, r := newReporter(context.Background(), &out, false, time.Millisecond)
	if err := Phase(ctx, "concurrent work"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 50 {
			_ = Phase(ctx, "concurrent work")
		}
	})
	for range 4 {
		wg.Go(func() {
			for range 50 {
				_, _ = io.WriteString(r.Writer(), "whole diagnostic line\n")
			}
		})
	}
	wg.Wait()
	if err := r.Finish(); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line != "whole diagnostic line" && line != "gorn: concurrent work..." && line != "gorn: still concurrent work..." {
			t.Fatalf("interleaved line %q", line)
		}
	}
}
