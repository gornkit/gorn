package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/gornkit/gorn/internal/app"
)

// Input configures runtime stdin and, for script "-", source input. Interrupt
// must promptly unblock a Read and return. Supplying it explicitly authorizes
// its invocation on canceled source collection; Gorn never closes caller IO.
// A nil Reader defaults to os.Stdin. Background-context reads need no Interrupt.
// CollectSource optionally replaces source-stdin collection. It must honor ctx
// and owns cleanup of its work; it never changes runtime Reader. The root CLI
// uses a process-lifetime collector, while embeddings normally supply Interrupt.
type Input struct {
	Reader        io.Reader
	Interrupt     func() error
	CollectSource func(context.Context) ([]byte, error)
}

type sourceReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r sourceReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func readSource(ctx context.Context, path string, input Input) (*app.Source, error) {
	if path == "" {
		return nil, fmt.Errorf("script path is empty")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if path == "-" && input.CollectSource != nil {
		data, err := input.CollectSource(ctx)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			return nil, err
		}
		return app.New(path, data)
	}
	if path != "-" {
		// Opening a FIFO can block before a reader can be interrupted. A source
		// file is a regular single-file unit; stream sources use explicit stdin.
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("source must be a regular file; use - for a stream")
		}
		f, err := os.Open(path) //nolint:gosec // G304: user-selected script source.
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		input = Input{Reader: f, Interrupt: f.Close}
	}
	if input.Reader == nil {
		input.Reader = os.Stdin
	}
	if ctx.Done() != nil && input.Interrupt == nil {
		return nil, fmt.Errorf("cancellable source input requires an explicit Input.Interrupt callback")
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			if input.Interrupt != nil {
				_ = input.Interrupt()
			}
		case <-stop:
		}
	}()
	data, err := io.ReadAll(sourceReader{ctx, input.Reader})
	close(stop)
	<-done
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	return app.New(path, data)
}
