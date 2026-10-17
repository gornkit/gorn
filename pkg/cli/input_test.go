package cli

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestSourceInputCancellationOwnership(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := readSource(ctx, "-", Input{Reader: reader}); err == nil || !strings.Contains(err.Error(), "Input.Interrupt") {
		t.Fatalf("missing ownership contract = %v", err)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := readSource(ctx, "-", Input{Reader: notifyReader{reader, started}, Interrupt: reader.Close})
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("source read did not stop")
	}
}

type notifyReader struct {
	io.Reader
	started chan struct{}
}

func (r notifyReader) Read(p []byte) (int, error) { close(r.started); return r.Reader.Read(p) }

func TestSourceInputNormalCompletionDoesNotInterrupt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	interrupted := false
	source, err := readSource(ctx, "-", Input{Reader: strings.NewReader("package main; func main(){}"), Interrupt: func() error { interrupted = true; return nil }})
	cancel()
	if err != nil || source == nil || interrupted {
		t.Fatalf("source=%v, err=%v, interrupted=%t", source, err, interrupted)
	}
}
