//go:build (linux || darwin) && (amd64 || arm64)

//nolint:gosec // Controlled self-executable, test fixture paths, and PTY ABI calls.
package process

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func posixFixture(mode string, args ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestPOSIXFixture$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "GORN_POSIX_FIXTURE="+mode)
	return cmd
}

func TestPOSIXFixture(t *testing.T) {
	switch os.Getenv("GORN_POSIX_FIXTURE") {
	case "grandchild":
		signal.Ignore(syscall.SIGTERM)
		for {
			_ = os.WriteFile(os.Args[3], []byte(strconv.FormatInt(time.Now().UnixNano(), 10)), 0o600)
			time.Sleep(10 * time.Millisecond)
		}
	case "leader", "leader-ignore":
		term := make(chan os.Signal, 1)
		signal.Notify(term, syscall.SIGTERM)
		cmd := posixFixture("grandchild", os.Args[3])
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Start(); err != nil {
			panic(err)
		}
		<-term
		if os.Getenv("GORN_POSIX_FIXTURE") == "leader" {
			os.Exit(0)
		}
		for {
			time.Sleep(time.Hour)
		}
	case "background", "background130", "background143":
		cmd := posixFixture("grandchild", os.Args[3])
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Start(); err != nil {
			panic(err)
		}
		_ = os.WriteFile(os.Args[4], []byte(strconv.Itoa(cmd.Process.Pid)), 0o600)
		for {
			if data, _ := os.ReadFile(os.Args[3]); len(data) > 0 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		code := 0
		if mode := os.Getenv("GORN_POSIX_FIXTURE"); mode != "background" {
			code, _ = strconv.Atoi(strings.TrimPrefix(mode, "background"))
		}
		os.Exit(code)
	case "pty-child", "pty-handled":
		var interrupts chan os.Signal
		if os.Getenv("GORN_POSIX_FIXTURE") == "pty-handled" {
			interrupts = make(chan os.Signal, 1)
			signal.Notify(interrupts, syscall.SIGINT)
		}
		var group int32
		if err := terminalGroup(os.Stdin, &group, false); err != nil || int(group) != syscall.Getpgrp() {
			panic("child lacks foreground")
		}
		_, _ = fmt.Fprintln(os.Stdout, "child-ready")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil || line != "input\n" {
			panic(fmt.Sprintf("input %q: %v", line, err))
		}
		_, _ = fmt.Fprintln(os.Stdout, "child-read")
		if interrupts != nil {
			<-interrupts
			os.Exit(0)
		}
		for {
			time.Sleep(time.Hour)
		}
	case "pty-owner":
		mode := os.Args[3]
		cmd := posixFixture(mode)
		if mode == "bad-exec" {
			path := filepath.Join(os.Args[4], "bad-exec")
			if err := os.WriteFile(path, []byte("invalid executable"), 0o700); err != nil {
				panic(err)
			}
			cmd = exec.Command(path)
		}
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		p, err := Start(context.Background(), cmd, true)
		if mode == "bad-exec" {
			if err == nil {
				panic("invalid executable started")
			}
		} else {
			if err != nil {
				panic(err)
			}
			err = p.Wait()
			if mode == "pty-handled" && err != nil {
				panic(err)
			}
			if mode == "pty-child" {
				var exited *exec.ExitError
				if !errors.As(err, &exited) || !exited.Sys().(syscall.WaitStatus).Signaled() {
					panic(fmt.Sprintf("Ctrl-C: %v", err))
				}
			}
		}
		var group int32
		if err := terminalGroup(os.Stdin, &group, false); err != nil || int(group) != syscall.Getpgrp() {
			panic(fmt.Sprintf("foreground not restored %d: %v", group, err))
		}
		_, _ = fmt.Fprintln(os.Stdout, "owner-restored")
		os.Exit(0)
	}
}

func awaitFile(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return data
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
	return nil
}

func TestCancellationKillsGroupAfterLeaderExits(t *testing.T) {
	for _, mode := range []string{"leader", "leader-ignore"} {
		t.Run(mode, func(t *testing.T) {
			heartbeat := filepath.Join(t.TempDir(), "heartbeat")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p, err := Start(ctx, posixFixture(mode, heartbeat), false)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Kill()
			awaitFile(t, heartbeat)
			started := time.Now()
			cancel()
			if err := p.Wait(); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if elapsed := time.Since(started); elapsed < terminationGrace || elapsed > 3*time.Second {
				t.Fatalf("termination took %s", elapsed)
			}
			before, _ := os.ReadFile(heartbeat)
			time.Sleep(80 * time.Millisecond)
			after, _ := os.ReadFile(heartbeat)
			if string(before) != string(after) {
				t.Fatal("descendant survived cancellation")
			}
		})
	}
}

func TestStoppedChildIsNotReaped(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = read.Close() }()
	defer func() { _ = write.Close() }()
	cmd := fixtureCommand("read")
	cmd.Stdin = read
	p, err := Start(context.Background(), cmd, false)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Kill()
	if err := cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	select {
	case <-p.core.done:
		t.Fatalf("stopped child treated as exited: %v", p.Wait())
	default:
	}
	if err := cmd.Process.Signal(syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	_ = write.Close()
	if err := p.Wait(); err != nil {
		t.Fatal(err)
	}
}

type failingObserver struct{ processPlatform }

func (f failingObserver) observe(*exec.Cmd) (bool, bool, error) { return false, false, syscall.ENOSYS }

func TestObservationFailureStillTerminatesOwnedChild(t *testing.T) {
	cmd := fixtureCommand("wait")
	platform, err := prepareProcess(cmd, processOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cmd.WaitDelay = pipeWaitLimit
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := platform.started(cmd); err != nil {
		t.Fatal(err)
	}
	p := &processCore{cmd: cmd, platform: failingObserver{platform}, running: true, wake: make(chan struct{}, 1), done: make(chan struct{})}
	go p.control(context.Background())
	select {
	case <-p.done:
		if !errors.Is(p.wait(), syscall.ENOSYS) {
			t.Fatalf("Wait = %v", p.wait())
		}
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("observer failure abandoned a live child")
	}
	if !platform.ownershipLost(syscall.ECHILD) || platform.ownershipLost(syscall.ENOSYS) {
		t.Fatal("ownership-loss classification")
	}
}

func TestOrdinaryBackgroundAndPipeBound(t *testing.T) {
	dir := t.TempDir()
	heartbeat, pidFile := filepath.Join(dir, "heartbeat"), filepath.Join(dir, "pid")
	cmd := posixFixture("background", heartbeat, pidFile)
	// A non-file writer makes os/exec own the stdout pipe inherited by the
	// background child. It must not hold Wait indefinitely.
	cmd.Stdout = io.Discard
	p, err := Start(context.Background(), cmd, false)
	if err != nil {
		t.Fatal(err)
	}
	pidData := awaitFile(t, pidFile)
	pid, _ := strconv.Atoi(string(pidData))
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
	awaitFile(t, heartbeat)
	started := time.Now()
	if err := p.Wait(); !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("Wait = %v", err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("inherited pipe was not bounded")
	}
	before, _ := os.ReadFile(heartbeat)
	time.Sleep(80 * time.Millisecond)
	after, _ := os.ReadFile(heartbeat)
	if string(before) == string(after) {
		t.Fatal("ordinary exit killed background child")
	}
}

func TestConventionalInterruptionStatusCleansGroup(t *testing.T) {
	for _, code := range []int{130, 143} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			dir := t.TempDir()
			heartbeat, pidFile := filepath.Join(dir, "heartbeat"), filepath.Join(dir, "pid")
			p, err := Start(context.Background(), posixFixture("background"+strconv.Itoa(code), heartbeat, pidFile), false)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Kill()
			awaitFile(t, heartbeat)
			var exited *exec.ExitError
			if err := p.Wait(); !errors.As(err, &exited) || exited.ExitCode() != code {
				t.Fatalf("Wait=%v", err)
			}
			before, _ := os.ReadFile(heartbeat)
			time.Sleep(80 * time.Millisecond)
			after, _ := os.ReadFile(heartbeat)
			if string(before) != string(after) {
				t.Fatal("conventional interruption left descendant running")
			}
		})
	}
}

func TestThreadSignalMaskRestoresExistingBits(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	original, err := blockTerminalStop()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := restoreSignalMask(original); err != nil {
			t.Error(err)
		}
	}()
	existing := original | signalMask(1<<(uint(syscall.SIGUSR1)-1))
	if err := restoreSignalMask(existing); err != nil {
		t.Fatal(err)
	}
	saved, err := blockTerminalStop()
	if err != nil || saved != existing {
		t.Fatalf("saved mask=%x want=%x err=%v", saved, existing, err)
	}
	if err := restoreSignalMask(saved); err != nil {
		t.Fatal(err)
	}
	again, err := blockTerminalStop()
	if err != nil || again != existing {
		t.Fatalf("restored mask=%x want=%x err=%v", again, existing, err)
	}
}

func TestForegroundPTY(t *testing.T) {
	for _, mode := range []string{"pty-child", "pty-handled", "bad-exec"} {
		t.Run(mode, func(t *testing.T) {
			master, slave := openPTY(t)
			defer func() { _ = master.Close() }()
			defer func() { _ = slave.Close() }()
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			owner := posixFixture("pty-owner", mode, t.TempDir())
			owner.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
			owner.Stdin, owner.Stdout, owner.Stderr = slave, slave, slave
			if err := owner.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = owner.Process.Kill() }()
			lines := make(chan string, 32)
			go func() {
				scanner := bufio.NewScanner(master)
				for scanner.Scan() {
					lines <- scanner.Text()
				}
				close(lines)
			}()
			restored := false
			var transcript strings.Builder
		loop:
			for {
				select {
				case line, ok := <-lines:
					if !ok {
						break loop
					}
					transcript.WriteString(line + "\n")
					if strings.TrimSpace(line) == "child-ready" {
						if _, err := master.Write([]byte("input\n")); err != nil {
							t.Fatalf("PTY input: %v", err)
						}
					}
					if strings.TrimSpace(line) == "child-read" {
						if _, err := master.Write([]byte{3}); err != nil {
							t.Fatalf("PTY interrupt: %v", err)
						}
					}
					if strings.HasSuffix(strings.TrimSpace(line), "owner-restored") {
						restored = true
						break loop
					}
				case <-ctx.Done():
					t.Fatalf("PTY timeout:\n%s", transcript.String())
				}
			}
			if err := owner.Wait(); err != nil || !restored {
				t.Fatalf("PTY owner: %v, restored=%t\n%s", err, restored, transcript.String())
			}
		})
	}
}
