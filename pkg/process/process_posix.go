//go:build (linux || darwin) && (amd64 || arm64)

//nolint:gosec // Platform-bounded syscall ABI: fixed-size aligned buffers and OS-sized PIDs.
package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type platformProcess struct {
	group      bool
	tty        *os.File
	foreground int32
	child      int32
}

func prepareProcess(cmd *exec.Cmd, options processOptions) (*platformProcess, error) {
	p := &platformProcess{group: true}
	if options.inherit {
		marker, _ := strconv.Atoi(os.Getenv(ownershipVariable))
		p.group = marker <= 0 || marker != syscall.Getpgrp()
		// Command.Env cannot forge or override the inherited ownership marker.
		setProcessMarker(cmd, "")
		if !p.group {
			setProcessMarker(cmd, strconv.Itoa(marker))
		}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: p.group}
	if p.group && options.terminal {
		if tty, ok := cmd.Stdin.(*os.File); ok {
			var foreground int32
			if err := terminalGroup(tty, &foreground, false); err == nil && int(foreground) == syscall.Getpgrp() {
				p.tty, p.foreground = tty, foreground
				cmd.SysProcAttr.Foreground = true
				cmd.SysProcAttr.Ctty = int(tty.Fd())
			}
		}
	}
	if options.mark {
		// Bundled sh binds this reserved marker to its actual group at init.
		// The new group PID cannot be known before Start/exec.
		setProcessMarker(cmd, "self")
	}
	return p, nil
}

func setProcessMarker(cmd *exec.Cmd, value string) {
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	for i := len(cmd.Env) - 1; i >= 0; i-- {
		if strings.HasPrefix(cmd.Env[i], ownershipVariable+"=") {
			cmd.Env = append(cmd.Env[:i], cmd.Env[i+1:]...)
		}
	}
	if value != "" {
		cmd.Env = append(cmd.Env, ownershipVariable+"="+value)
	}
}

func (p *platformProcess) started(cmd *exec.Cmd) error { p.child = int32(cmd.Process.Pid); return nil }

func (p *platformProcess) observe(cmd *exec.Cmd) (bool, bool, error) {
	pid, code, status, err := observeExit(cmd.Process.Pid)
	if err != nil {
		return false, false, fmt.Errorf("observe child without reaping (exclusive child-reaper ownership required): %w", err)
	}
	if pid == 0 || code < 1 || code > 3 {
		return false, false, nil
	}
	if pid != cmd.Process.Pid {
		return false, false, fmt.Errorf("%w: unexpected child identity from waitid", errProcessOwnershipLost)
	}
	interrupted := code == 1 && (status == 130 || status == 143) || (code == 2 || code == 3) && (status == int(syscall.SIGINT) || status == int(syscall.SIGTERM))
	return true, interrupted, nil
}

func (p *platformProcess) terminate(cmd *exec.Cmd) error {
	pid := cmd.Process.Pid
	if p.group {
		pid = -pid
	}
	signal := func(sig syscall.Signal) error {
		err := syscall.Kill(pid, sig)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	termErr := signal(syscall.SIGTERM)
	time.Sleep(terminationGrace)
	return errors.Join(termErr, signal(syscall.SIGKILL))
}

func (*platformProcess) ownershipLost(err error) bool {
	return errors.Is(err, syscall.ECHILD) || errors.Is(err, errProcessOwnershipLost)
}

func (p *platformProcess) close() error { return nil }

func terminalGroup(tty *os.File, group *int32, set bool) error {
	request := uintptr(syscall.TIOCGPGRP)
	if set {
		request = syscall.TIOCSPGRP
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, tty.Fd(), request, uintptr(unsafe.Pointer(group)))
	if errno != 0 {
		return errno
	}
	return nil
}

func (p *platformProcess) restore() error {
	if p.tty == nil {
		return nil
	}
	// A locked thread retires if restoring its signal mask fails, rather than
	// returning a changed mask to the embedding application's thread pool.
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		old, err := blockTerminalStop()
		if err != nil {
			runtime.UnlockOSThread()
			done <- err
			return
		}
		var current int32
		err = terminalGroup(p.tty, &current, false)
		// Start can fail after foreground transfer but before exposing a PID.
		// Restore an orphaned group then, while respecting another live owner.
		orphan := p.child == 0 && current > 0 && errors.Is(syscall.Kill(-int(current), 0), syscall.ESRCH)
		if err == nil && (current == p.child || current == p.foreground || orphan) {
			err = terminalGroup(p.tty, &p.foreground, true)
		}
		maskErr := restoreSignalMask(old)
		if maskErr == nil {
			runtime.UnlockOSThread()
		}
		done <- errors.Join(err, maskErr)
	}()
	return <-done
}
