package process

// This private primitive is duplicated in internal/process to keep sh self-contained
// and the CLI independent of sh. The root consistency test guards against drift.

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"
)

const (
	terminationGrace  = 500 * time.Millisecond
	pipeWaitLimit     = 500 * time.Millisecond
	processPoll       = 10 * time.Millisecond
	ownershipVariable = "GORN_PROCESS_GROUP"
)

var errProcessKilled = errors.New("command killed")

type processPlatform interface {
	observe(*exec.Cmd) (bool, bool, error)
	terminate(*exec.Cmd) error
	ownershipLost(error) bool
	restore() error
	close() error
}

type processOptions struct {
	inherit  bool //nolint:unused // POSIX-only group ownership; shared backend options also compile on Windows.
	mark     bool //nolint:unused // POSIX-only bundled group marker.
	terminal bool //nolint:unused // POSIX-only foreground terminal transfer.
}

type processCore struct {
	cmd      *exec.Cmd
	platform processPlatform
	mu       sync.Mutex
	// Only running permits new cancellation. Reaping ends numeric-ID ownership.
	running   bool
	requested bool
	wake      chan struct{}
	done      chan struct{}
	err       error
}

func startProcess(ctx context.Context, cmd *exec.Cmd, options processOptions) (*processCore, error) {
	if ctx == nil {
		return nil, errors.New("nil command context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	platform, err := prepareProcess(cmd, options)
	if err != nil {
		return nil, err
	}
	cmd.WaitDelay = pipeWaitLimit
	if err := cmd.Start(); err != nil {
		return nil, errors.Join(err, platform.restore())
	}
	if err := platform.started(cmd); err != nil {
		_ = cmd.Process.Kill()
		return nil, errors.Join(err, cmd.Wait(), platform.restore())
	}
	p := &processCore{cmd: cmd, platform: platform, running: true, wake: make(chan struct{}, 1), done: make(chan struct{})}
	go p.control(ctx)
	return p, nil
}

func (p *processCore) wait() error { <-p.done; return p.err }

func (p *processCore) kill() {
	p.mu.Lock()
	if p.running {
		p.requested = true
	}
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *processCore) control(ctx context.Context) {
	ticker := time.NewTicker(processPoll)
	defer ticker.Stop()
	var cause error
	for {
		exited, interrupted, observeErr := p.platform.observe(p.cmd)
		p.mu.Lock()
		cause = ctx.Err()
		if p.requested && cause == nil {
			cause = errProcessKilled
		}
		if observeErr != nil || exited || cause != nil {
			// Commit cleanup/reaping before another Kill. No delayed signal can
			// outlive this controller. Cmd.Wait is called exactly once.
			p.running = false
			p.mu.Unlock()
			if !p.platform.ownershipLost(observeErr) && (cause != nil || interrupted || observeErr != nil) {
				// Keep the leader unreaped through escalation, even if TERM makes
				// it exit while a descendant continues to ignore TERM.
				cause = errors.Join(cause, p.platform.terminate(p.cmd))
			}
			p.err = errors.Join(cause, observeErr, p.cmd.Wait(), p.platform.restore(), p.platform.close())
			close(p.done)
			return
		}
		p.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-p.wake:
		case <-ticker.C:
		}
	}
}
