// Package process owns a subprocess from Start through bounded cancellation and
// its sole Wait. Callers must not separately wait/reap the supplied exec.Cmd.
package process

import (
	"context"
	"os/exec"
)

// Process is an owned child. Wait and Kill are safe concurrently and repeatedly.
type Process struct{ core *processCore }

// Start starts cmd. Interactive runtime children receive terminal foreground
// when appropriate, and a reserved bundled-helper process-group marker.
func Start(ctx context.Context, cmd *exec.Cmd, interactive bool) (*Process, error) {
	core, err := startProcess(ctx, cmd, processOptions{mark: interactive, terminal: interactive})
	if err != nil {
		return nil, err
	}
	return &Process{core: core}, nil
}

func (p *Process) Wait() error { return p.core.wait() }
func (p *Process) Kill()       { p.core.kill() }

// Run starts and waits for a noninteractive compiler/resolver child.
func Run(ctx context.Context, cmd *exec.Cmd) error {
	p, err := Start(ctx, cmd, false)
	if err != nil {
		return err
	}
	return p.Wait()
}
