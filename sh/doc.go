// Package sh runs executable arguments or shell snippets with normal Go IO and
// status-only results. Cmd passes each argument literally; Shell uses the chosen
// shell's syntax. Command configuration methods return independent copies.
// Environment and unspecified IO inherit from the process; caller IO is never
// closed. Context(nil) is invalid, while an unspecified context is Background.
//
// A started Process has one automatic reaper. Concurrent/repeated Wait returns
// the same result. Kill requests termination and is idempotent; Wait observes
// cleanup. Context cancellation preserves errors.Is with context.Canceled or
// context.DeadlineExceeded. Child exit codes and POSIX 128+signal are retained.
//
// On supported Linux/macOS architectures, standalone commands own a process
// group: termination sends TERM, waits 500ms, sends KILL, then reaps. Deliberately
// escaped groups/sessions are outside that ownership. A verified reserved Gorn
// marker keeps bundled commands in the invocation group; command-local Kill
// then targets only the direct child, and Gorn owns whole-invocation cleanup.
// Windows tree termination uses bounded taskkill with direct kill fallback;
// already detached/reparented descendants are outside its best-effort contract.
//
// Go-owned inherited pipes have a 500ms WaitDelay; an ordinary successful parent
// may leave background work running, and an inherited open pipe can therefore
// produce exec.ErrWaitDelay. Arbitrary blocking caller IO is outside this bound.
// Embedding applications must not separately reap these children or enable
// automatic child reaping. Foreground input and Ctrl-C are supported when the
// caller owns the controlling terminal; full suspend/resume job control is not.
package sh
