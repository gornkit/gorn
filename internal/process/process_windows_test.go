//go:build windows

//nolint:gosec // Controlled self-executable and test fixture files.
package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func windowsFixture(mode string, args ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestWindowsFixture$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "GORN_WINDOWS_FIXTURE="+mode)
	return cmd
}

func TestWindowsFixture(t *testing.T) {
	switch os.Getenv("GORN_WINDOWS_FIXTURE") {
	case "grandchild":
		for {
			_ = os.WriteFile(os.Args[3], []byte(strconv.FormatInt(time.Now().UnixNano(), 10)), 0o600)
			time.Sleep(10 * time.Millisecond)
		}
	case "leader":
		cmd := windowsFixture("grandchild", os.Args[3])
		if err := cmd.Start(); err != nil {
			panic(err)
		}
		for {
			time.Sleep(time.Hour)
		}
	}
}

func TestWindowsTreeAndDirectFallback(t *testing.T) {
	heartbeat := filepath.Join(t.TempDir(), "heartbeat")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := Start(ctx, windowsFixture("leader", heartbeat), false)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Kill()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if data, _ := os.ReadFile(heartbeat); len(data) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("grandchild did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := p.Wait(); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(heartbeat)
	time.Sleep(80 * time.Millisecond)
	after, _ := os.ReadFile(heartbeat)
	if string(before) != string(after) {
		t.Fatal("taskkill left grandchild running")
	}
	cmd := fixtureCommand("wait")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := terminateWindows(cmd, filepath.Join(t.TempDir(), "absent-taskkill.exe")); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("fallback did not kill/reap child")
	}
}
