//nolint:gosec // Controlled self-executable, test fixture paths, and PTY ABI calls.
package process

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

func fixtureCommand(mode string, args ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestProcessFixture$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "GORN_PROCESS_FIXTURE="+mode)
	return cmd
}

func TestProcessFixture(t *testing.T) {
	switch os.Getenv("GORN_PROCESS_FIXTURE") {
	case "echo":
		_ = json.NewEncoder(os.Stdout).Encode(os.Args[3:])
		os.Exit(0)
	case "read":
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	case "wait":
		_, _ = fmt.Fprintln(os.Stdout, "ready")
		for {
			time.Sleep(time.Hour)
		}
	}
}

func TestWaitConcurrentAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := fixtureCommand("wait")
	p, err := Start(ctx, cmd, false)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	var wg sync.WaitGroup
	results := make(chan error, 20)
	for range 20 {
		wg.Go(func() { results <- p.Wait() })
	}
	cancel()
	wg.Wait()
	close(results)
	for err := range results {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Wait = %v", err)
		}
	}
	// Completed ownership cannot be revived by later Kill requests.
	for range 20 {
		p.Kill()
	}
	if !errors.Is(p.Wait(), context.Canceled) {
		t.Fatal("result changed")
	}
}

func TestKillIdempotent(t *testing.T) {
	p, err := Start(context.Background(), fixtureCommand("wait"), false)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(p.Kill)
	}
	wg.Wait()
	if err := p.Wait(); !errors.Is(err, errProcessKilled) {
		t.Fatalf("Wait = %v", err)
	}
}

func TestContextBeforeStartAndDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := fixtureCommand("echo", "must not run")
	if _, err := Start(ctx, cmd, false); !errors.Is(err, context.Canceled) || cmd.Process != nil {
		t.Fatalf("canceled Start: %v", err)
	}
	if _, err := Start(nil, fixtureCommand("echo"), false); err == nil { //nolint:staticcheck // Verify explicit nil rejection.
		t.Fatal("nil accepted")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	p, err := Start(ctx, fixtureCommand("wait"), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline = %v", err)
	}
}

func TestOrdinaryExitAndOutput(t *testing.T) {
	cmd := fixtureCommand("echo", "", "a b", "*", "$HOME", "a|b", "é")
	var out bytes.Buffer
	cmd.Stdout = &out
	p, err := Start(context.Background(), cmd, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err != nil {
		t.Fatal(err)
	}
	if want := "[\"\",\"a b\",\"*\",\"$HOME\",\"a|b\",\"é\"]\n"; out.String() != want {
		t.Fatalf("output = %q", out.String())
	}
	p.Kill()
	if err := p.Wait(); err != nil {
		t.Fatal(err)
	}
}
