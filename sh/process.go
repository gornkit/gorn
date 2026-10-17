package sh

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// Process is a running command. Its child is reaped independently of Wait.
// Wait and Kill may be called concurrently or repeatedly.
type Process struct {
	core *processCore
}

// Wait waits for the process and returns its exit status.
func (p *Process) Wait() Result {
	if p == nil || p.core == nil {
		return Result{code: 1, err: os.ErrInvalid}
	}

	err := p.core.wait()
	if err == nil {
		return Result{}
	}

	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		code := max(exitErr.ExitCode(), 1)
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			code = 128 + int(status.Signal())
		}
		return Result{code: code, err: err}
	}

	return Result{code: 1, err: err}
}

// Kill requests bounded termination. It is idempotent and does not wait for IO.
// Wait observes completion. Caller-owned IO is never closed.
func (p *Process) Kill() error {
	if p == nil || p.core == nil {
		return os.ErrInvalid
	}
	p.core.kill()
	return nil
}
