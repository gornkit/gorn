package sh

import (
	"errors"
	"os"
	"os/exec"
)

// Process is a running shell command.
type Process struct {
	cmd *exec.Cmd
}

// Wait waits for the process and returns its exit status.
func (p *Process) Wait() Result {
	if p == nil || p.cmd == nil {
		return Result{code: 1, err: os.ErrInvalid}
	}

	err := p.cmd.Wait()
	if err == nil {
		return Result{}
	}

	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		code := max(exitErr.ExitCode(), 1)
		return Result{code: code, err: err}
	}

	return Result{code: 1, err: err}
}

// Kill terminates the process.
func (p *Process) Kill() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return os.ErrInvalid
	}
	return p.cmd.Process.Kill()
}
