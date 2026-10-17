//go:build windows

package sh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

type platformProcess struct{ handle syscall.Handle }

func prepareProcess(_ *exec.Cmd, _ processOptions) (*platformProcess, error) {
	return &platformProcess{}, nil
}

func (p *platformProcess) started(cmd *exec.Cmd) error {
	handle, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(cmd.Process.Pid)) //nolint:gosec // G115: Windows process IDs are native DWORDs.
	p.handle = handle
	return err
}

func (p *platformProcess) observe(_ *exec.Cmd) (bool, bool, error) {
	status, err := syscall.WaitForSingleObject(p.handle, 0)
	return status == syscall.WAIT_OBJECT_0, false, err
}

func (p *platformProcess) terminate(cmd *exec.Cmd) error {
	return terminateWindows(cmd, filepath.Join(os.Getenv("SystemRoot"), "System32", "taskkill.exe"))
}

func terminateWindows(cmd *exec.Cmd, taskkill string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	killer := exec.CommandContext(ctx, taskkill, "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F") //nolint:gosec // G204: native tree termination for this owned child.
	killer.WaitDelay = pipeWaitLimit
	if err := killer.Run(); err == nil {
		return nil
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("direct-process termination fallback: %w", err)
	}
	return nil
}

func (p *platformProcess) restore() error           { return nil }
func (p *platformProcess) ownershipLost(error) bool { return false } // Cmd retains the native child handle.
func (p *platformProcess) close() error             { return syscall.CloseHandle(p.handle) }
