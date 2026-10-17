// Package progress provides invocation-scoped phase/heartbeat reporting. It
// serializes preparation diagnostics; runtime children keep their original IO.
package progress

import (
	"context"
	"io"
	"sync"
	"time"
)

const heartbeatInterval = 2 * time.Second

type contextKey struct{}

type Reporter struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	out    io.Writer
	quiet  bool
	mu     sync.Mutex
	phase  string
	err    error
	closed bool
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
}

// New returns a context carrying the reporter. Output failures cancel that
// context so pending Go work can stop; Finish returns the underlying IO error.
func New(ctx context.Context, out io.Writer, quiet bool) (context.Context, *Reporter) {
	return newReporter(ctx, out, quiet, heartbeatInterval)
}

func newReporter(ctx context.Context, out io.Writer, quiet bool, interval time.Duration) (context.Context, *Reporter) {
	ctx, cancel := context.WithCancelCause(ctx)
	r := &Reporter{ctx: ctx, cancel: cancel, out: out, quiet: quiet, stop: make(chan struct{}), done: make(chan struct{})}
	go r.heartbeat(interval)
	return context.WithValue(ctx, contextKey{}, r), r
}

func FromContext(ctx context.Context) *Reporter {
	r, _ := ctx.Value(contextKey{}).(*Reporter)
	return r
}

// Phase announces a fixed, non-sensitive phase before its potentially slow work.
// Contexts without a reporter retain ordinary library behavior.
func Phase(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r := FromContext(ctx)
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r.err
	}
	r.phase = name
	if !r.quiet {
		_, _ = r.writeLocked(r.out, []byte("gorn: "+name+"...\n"))
	}
	return r.err
}

func (r *Reporter) heartbeat(interval time.Duration) {
	defer close(r.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.mu.Lock()
			if !r.closed && !r.quiet && r.phase != "" {
				_, _ = r.writeLocked(r.out, []byte("gorn: still "+r.phase+"...\n"))
			}
			r.mu.Unlock()
		}
	}
}

func (r *Reporter) writeLocked(out io.Writer, p []byte) (int, error) {
	n, err := out.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		if r.err == nil {
			r.err = err
		}
		r.cancel(err)
	}
	return n, err
}

type synchronizedWriter struct {
	reporter *Reporter
	out      io.Writer
}

func (w synchronizedWriter) Write(p []byte) (int, error) {
	w.reporter.mu.Lock()
	defer w.reporter.mu.Unlock()
	return w.reporter.writeLocked(w.out, p)
}

// Synchronize is for preparation/artifact writers. Do not wrap runtime *os.File
// streams: os/exec would replace a terminal descriptor with a Go-owned pipe.
func (r *Reporter) Synchronize(out io.Writer) io.Writer { return synchronizedWriter{r, out} }
func (r *Reporter) Writer() io.Writer                   { return r.Synchronize(r.out) }

// Close stops and joins the heartbeat. Diagnostic writers remain usable, but
// no phase/heartbeat can steal a runtime child's foreground terminal afterward.
func (r *Reporter) Close() error {
	r.once.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()
		close(r.stop)
	})
	<-r.done
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// Finish joins reporting and releases its context after all invocation work.
func (r *Reporter) Finish() error {
	err := r.Close()
	r.cancel(nil)
	return err
}
