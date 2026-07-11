---
title: Gorn sh Package Design — v0 Shell-First API
status: archived
type: design
updated: 2026-07-02
summary: v0 shell-first API design for the sh helper package.
---

# Gorn `sh` Package Design — v0 Shell-First API

**Package:** `github.com/gornkit/gorn/sh`  
**Design center:** Start tiny, honest, shell-first, and extensible.

---

## 1. Summary

The v0 `sh` package should begin as a small Go-shaped wrapper around host-shell execution.

The core API should be built around four concepts:

```text
ShellSpec  = shell choice and shell setup
Command    = configured process description
Process    = running process
Result     = execution status
```

The key v0 experience is:

```go
sh.Host.Exec("go test -v ./...").OrExit()
```

and, for reusable shell setup:

```go
strict := sh.Bash.Setup("set -euo pipefail")

strict.Exec("go test -v ./...").OrExitf("tests failed")
strict.Exec("go vet ./...").OrExitf("vet failed")
```

This deliberately does **not** try to be a portable shell replacement in v0. It gives Gorn file apps a practical automation surface immediately, while leaving room for later structured APIs like `Cmd`, `Pipe`, and line streaming.

---

## 2. Design Philosophy

### 2.1 Shell-first, not shell-replacement

Gorn v0 should not pretend that shell snippets are portable.

A shell call means:

```text
The script author is explicitly choosing a host shell.
The shell string is interpreted by that shell.
Portability is the author's responsibility.
```

This is appropriate because most useful automation scripts already shell out, and the first release should prove the `.gorn` file-app model rather than overdesign a complete process DSL.

### 2.2 Output belongs to IO, not `Result`

`Result` should not own stdout or stderr.

Instead:

```text
Unclaimed output goes to the terminal.
Claimed output goes to the writer/stream/buffer the user provided.
Result reports only what happened.
```

So:

```go
sh.Host.Exec("go test ./...").OrExit()
```

prints like a normal shell command.

To capture output, use normal Go IO:

```go
var out bytes.Buffer

sh.Host.
	Shell("git branch --show-current").
	Stdout(&out).
	Exec().
	OrExit()

branch := strings.TrimSpace(out.String())
```

No `Result.Stdout`. No `Result.Stderr`. No capture mode special case.

### 2.3 One lifecycle model

Every shell snippet can be:

```text
Exec'd immediately
or
Start'd as a Process, then Wait'd
```

Porcelain:

```go
sh.Host.Exec("go test -v ./...").OrExit()
```

Configured command:

```go
sh.Bash.
	Setup("set -euo pipefail").
	Shell("go test -v ./...").
	Dir(repo).
	Env("CI", "1").
	Exec().
	OrExitf("tests failed")
```

Split lifecycle:

```go
proc, err := sh.Host.
	Shell("python3 -m http.server 8000").
	Start()

sh.OrExitf(err, "failed to start server")

// do work while the server is running

proc.Kill()
proc.Wait()
```

---

## 3. Goals

### v0 goals

- Provide a tiny useful `sh` package for `.gorn` automation.
- Make host-shell use explicit and ergonomic.
- Keep process lifecycle simple: `Exec`, `Start`, `Wait`.
- Make stdout/stderr behavior shell-like by default.
- Support normal Go IO redirection with `io.Reader` / `io.Writer`.
- Keep `Result` small and output-free.
- Avoid committing to a larger structured command API too early.

### Future goals

- Add structured process execution with `Cmd`.
- Add structured pipelines with `Pipe`.
- Add writer-backed line streaming with `Lines` and `iter.Seq[Line]`.
- Add portable filesystem helpers such as `Glob`, `Read`, `Write`, `Remove`, etc.

---

## 4. Non-goals

The v0 `sh` package should **not**:

- Implement a shell parser.
- Parse shell pipelines into structured stages.
- Emulate POSIX shell semantics.
- Provide portable replacements for `grep`, `sed`, `awk`, `find`, etc.
- Make shell strings safe from injection.
- Make shell snippets portable across operating systems.
- Capture stdout/stderr into `Result`.
- Provide `Cmd` / `Pipe` / `Lines` in v0 unless they fall out naturally later.

---

## 5. Core API

### 5.1 Package sketch

```go
package sh

func OrExit(err error)
func OrExitf(err error, format string, args ...any)

type ShellSpec struct {
	// unexported or mostly unexported fields
}

var (
	Host       ShellSpec
	Sh         ShellSpec
	Bash       ShellSpec
	CmdExe     ShellSpec
	Pwsh       ShellSpec
)

func (s ShellSpec) Setup(lines ...string) ShellSpec
func (s ShellSpec) Shell(script string) Command
func (s ShellSpec) Exec(script string) Result

type Command struct {
	// unexported command config
}

func (c Command) Dir(path string) Command
func (c Command) Env(key, value string) Command
func (c Command) Stdin(r io.Reader) Command
func (c Command) Stdout(w io.Writer) Command
func (c Command) Stderr(w io.Writer) Command

func (c Command) Exec() Result
func (c Command) Start() (*Process, error)

type Process struct {
	// unexported process state
}

func (p *Process) Wait() Result
func (p *Process) Kill() error

type Result struct {
	// execution status only
}

func (r Result) OK() bool
func (r Result) Code() int
func (r Result) Error() error
func (r Result) OrExit() Result
func (r Result) OrExitf(format string, args ...any) Result
```

---

## 6. `ShellSpec`

`ShellSpec` represents a shell profile: executable, invocation arguments, and setup lines.

Examples:

```go
sh.Host.Exec("go test -v ./...").OrExit()
```

```go
strict := sh.Bash.Setup("set -euo pipefail")
strict.Exec("go test -v ./...").OrExitf("tests failed")
```

```go
sh.Pwsh.Exec("Get-ChildItem").OrExit()
```

### 6.1 Built-in specs

Suggested built-ins:

```go
var (
	Host       ShellSpec // platform default
	Sh         ShellSpec // sh -c
	Bash       ShellSpec // bash -lc
	CmdExe     ShellSpec // cmd.exe /C
	PowerShell ShellSpec // powershell.exe -NoProfile -Command
	Pwsh       ShellSpec // pwsh -NoProfile -Command
)
```

Suggested default `Host` behavior:

| Platform | Host shell |
|---|---|
| Unix-like | `/bin/sh -c` or `sh -c` |
| Windows | `cmd.exe /C` |

The `Host` default should be documented as platform-dependent and explicitly non-portable.

### 6.2 Setup lines

`Setup` keeps shell setup separate from the actual script.

Instead of:

```go
sh.Bash.Exec("set -euo pipefail; go test -v ./...")
```

prefer:

```go
sh.Bash.
	Setup("set -euo pipefail").
	Exec("go test -v ./...").
	OrExitf("tests failed")
```

A setup-enabled shell invocation should concatenate setup and script with newlines.

Example:

```go
sh.Bash.Setup("set -euo pipefail").Exec("go test -v ./...")
```

lowers conceptually to:

```sh
bash -lc 'set -euo pipefail
go test -v ./...'
```

### 6.3 Why not generic shell options?

Avoid generic APIs like:

```go
sh.Bash.Option("pipefail")
```

because shell options are not portable and are not always invocation flags. For v0, use explicit setup lines:

```go
sh.Bash.Setup("set -o pipefail")
```

Future sugar may add:

```go
sh.Bash.Strict()
sh.Sh.Strict()
sh.Pwsh.Strict()
```

but v0 should keep the primitive obvious.

---

## 7. `Command`

`Command` is an inert description of a configured process.

For v0, a `Command` is usually produced by:

```go
cmd := sh.Host.Shell("go test -v ./...")
```

It can then be configured:

```go
cmd = cmd.
	Dir(repo).
	Env("CI", "1").
	Stdout(os.Stdout).
	Stderr(os.Stderr)
```

and then run:

```go
cmd.Exec().OrExit()
```

or started:

```go
proc, err := cmd.Start()
```

### 7.1 Command config methods

```go
func (c Command) Dir(path string) Command
func (c Command) Env(key, value string) Command
func (c Command) Stdin(r io.Reader) Command
func (c Command) Stdout(w io.Writer) Command
func (c Command) Stderr(w io.Writer) Command
```

These should return a new `Command` value, not mutate in place. This makes reuse safe:

```go
base := sh.Bash.Setup("set -euo pipefail")

base.Exec("go test ./...").OrExit()
base.Exec("go vet ./...").OrExit()
```

### 7.2 Default IO policy

The default should match shell-script expectations:

```text
stdin  = os.Stdin
stdout = os.Stdout
stderr = os.Stderr
```

So:

```go
sh.Host.Exec("go test ./...").OrExit()
```

prints output directly to the terminal.

If users want capture, they provide a writer:

```go
var out bytes.Buffer

sh.Host.
	Shell("git rev-parse --show-toplevel").
	Stdout(&out).
	Exec().
	OrExit()
```

If users want silence:

```go
sh.Host.
	Shell("noisy-tool").
	Stdout(io.Discard).
	Stderr(io.Discard).
	Exec().
	OrExit()
```

### 7.3 Writer ownership

If the user passes an `io.Writer`, Gorn writes to it but does not close it.

```text
Gorn never closes user-owned readers or writers.
```

The caller owns file handles, buffers, and custom writers.

---

## 8. `Process`

`Process` represents a running process.

```go
proc, err := sh.Host.Shell("dev-server").Start()
sh.OrExitf(err, "failed to start dev server")

// do other work

proc.Kill()
proc.Wait()
```

### 8.1 Process methods

```go
func (p *Process) Wait() Result
func (p *Process) Kill() error
```

Optional later:

```go
func (p *Process) Signal(sig os.Signal) error
```

`Signal` is useful but platform-sensitive, so it can wait.

### 8.2 `Exec` is `Start` + `Wait`

Conceptually:

```go
func (c Command) Exec() Result {
	p, err := c.Start()
	if err != nil {
		return startErrorResult(c, err)
	}
	return p.Wait()
}
```

---

## 9. `Result`

`Result` reports execution status. It does not store output.

### 9.1 Suggested shape

```go
type Result struct {
	Command string // display command, maybe redacted/quoted form
	Code    int
	Err     error
}
```

For future structured pipelines, this may become:

```go
type Result struct {
	Stages []StageResult
	Err    error
}

type StageResult struct {
	Name string
	Args []string
	Code int
	Err  error
}
```

For v0 shell execution, one stage is enough.

### 9.2 Methods

```go
func (r Result) OK() bool
func (r Result) Code() int
func (r Result) Error() error
func (r Result) OrExit() Result
func (r Result) OrExitf(format string, args ...any) Result
```

`OrExit` prints a useful diagnostic and exits the process if the command failed.

Example:

```go
sh.Host.Exec("go test ./...").OrExitf("tests failed")
```

Possible failure output:

```text
tests failed

command: sh -c go test ./...
exit status: 1
```

Because stdout/stderr normally went directly to the terminal, `OrExitf` does not need to print captured stderr.

---

## 10. Error Helpers

Useful for split lifecycle:

```go
proc, err := sh.Host.Shell("dev-server").Start()
sh.OrExitf(err, "failed to start dev server")

proc.Wait().OrExit()
```

Suggested helpers:

```go
func OrExit(err error)
func OrExitf(err error, format string, args ...any)
```

These are for ordinary `error` values, separate from `Result.OrExit`.

---

## 11. Examples

### 11.1 Basic test script

```go
#!/usr/bin/env gorn

//gorn:main

sh.Host.Exec("go test -v ./...").OrExitf("tests failed")
sh.Host.Exec("go vet ./...").OrExitf("vet failed")
```

### 11.2 Strict Bash

```go
#!/usr/bin/env gorn

//gorn:main

strict := sh.Bash.Setup("set -euo pipefail")

strict.Exec("go test -v ./...").OrExitf("tests failed")
strict.Exec("go vet ./...").OrExitf("vet failed")
strict.Exec("go build ./...").OrExitf("build failed")
```

### 11.3 Capture with `bytes.Buffer`

```go
#!/usr/bin/env gorn

import (
	"bytes"
	"fmt"
	"strings"
)

//gorn:main

var out bytes.Buffer

sh.Host.
	Shell("git branch --show-current").
	Stdout(&out).
	Exec().
	OrExitf("failed to get branch")

branch := strings.TrimSpace(out.String())
fmt.Println("branch:", branch)
```

### 11.4 Start a temporary server

```go
#!/usr/bin/env gorn

import "time"

//gorn:main

proc, err := sh.Host.
	Shell("python3 -m http.server 8000").
	Start()

sh.OrExitf(err, "failed to start server")

time.Sleep(2 * time.Second)

sh.Host.Exec("curl -f http://localhost:8000").OrExitf("server probe failed")

proc.Kill()
proc.Wait()
```

### 11.5 Explicit PowerShell

```go
#!/usr/bin/env gorn

//gorn:main

sh.Pwsh.Exec("Get-ChildItem").OrExit()
```

### 11.6 Conditional host behavior

```go
#!/usr/bin/env gorn

import "runtime"

//gorn:main

switch runtime.GOOS {
case "windows":
	sh.Pwsh.Exec("Get-ChildItem").OrExit()
default:
	sh.Bash.Exec("ls -la").OrExit()
}
```

---

## 12. Security and Quoting

Shell strings are interpreted by the selected shell.

This is fine for literal snippets:

```go
sh.Host.Exec("go test -v ./...").OrExit()
```

This is risky:

```go
sh.Host.Exec("grep " + pattern + " file.txt").OrExit()
```

If `pattern` comes from user input, the shell may interpret it as syntax.

The docs should say:

```text
Use ShellSpec.Exec for literal host-shell snippets.
Do not build shell strings from untrusted input.
Future structured commands should be preferred for dynamic arguments.
```

Future structured form:

```go
sh.Cmd("grep", pattern, "file.txt").Exec().OrExit()
```

---

## 13. Implementation Sketch

### 13.1 Shell lowering

`ShellSpec.Shell(script)` creates a `Command` whose executable is the shell.

Conceptually:

```go
func (s ShellSpec) Shell(script string) Command {
	fullScript := joinSetupAndScript(s.setup, script)
	args := append([]string{}, s.args...)
	args = append(args, fullScript)
	return commandFromArgv(s.name, args...)
}
```

Examples:

```go
sh.Bash.Shell("go test")
```

lowers to:

```text
bash -lc "go test"
```

```go
sh.Bash.Setup("set -euo pipefail").Shell("go test")
```

lowers to:

```text
bash -lc "set -euo pipefail\ngo test"
```

### 13.2 Command execution

For v0, `Start` can assign configured IO directly to `exec.Cmd`:

```go
cmd.Stdout = stdoutOrDefault
cmd.Stderr = stderrOrDefault
cmd.Stdin  = stdinOrDefault
```

Default values:

```go
stdin  := os.Stdin
stdout := os.Stdout
stderr := os.Stderr
```

Later, when `Lines` is added, Gorn may need to own output-copy goroutines to detect EOF and close streams cleanly.

### 13.3 Exit code extraction

When `cmd.Wait()` returns an `*exec.ExitError`, extract the exit code:

```go
if exitErr, ok := err.(*exec.ExitError); ok {
	code := exitErr.ExitCode()
}
```

If process start fails, `Result` should represent a start error rather than an exit code.

---

## 14. Future: Structured Commands

Later, Gorn can add:

```go
sh.Cmd("go", "test", "-v", "./...").Exec().OrExit()
```

This should mean:

```text
No shell interpretation.
Arguments are passed exactly.
Dynamic values are safe from shell injection.
```

The future `Cmd` should return the same `Command` type used by `ShellSpec.Shell`.

So the v0 design already prepares for:

```go
sh.Host.Shell("go test -v ./...").Exec()
sh.Cmd("go", "test", "-v", "./...").Exec()
```

Both are just `Command` values.

---

## 15. Future: Pipelines

Structured pipelines can come later:

```go
sh.Cmd("git", "status", "--short").
	Pipe(sh.Cmd("grep", "^ M")).
	Pipe(sh.Cmd("wc", "-l")).
	Exec().
	OrExit()
```

This is distinct from:

```go
sh.Bash.Exec("git status --short | grep '^ M' | wc -l")
```

The shell version is opaque to Gorn.

The structured version can eventually provide:

```text
- stage-aware results
- pipefail-by-default semantics
- safe dynamic arguments
- consistent IO policy
```

---

## 16. Future: Lines and `iter.Seq`

Line streaming can be added later as a writer-backed adapter.

Possible API:

```go
proc, lines, err := sh.Cmd("go", "test", "./...").StartLines()
sh.OrExit(err)

for line := range lines.Seq() {
	fmt.Println(line.Text)
}

proc.Wait().OrExit()
```

Or the lower-level writer form:

```go
lines := sh.Lines()

proc, err := sh.Host.
	Shell("go test ./...").
	Stdout(lines.Stdout()).
	Stderr(lines.Stderr()).
	Start()

sh.OrExit(err)

for line := range lines.Seq() {
	fmt.Println(line.Text)
}

proc.Wait().OrExit()
```

Design direction:

```text
io.Writer is the extension point.
goroutines copy process output.
channels queue line events internally.
iter.Seq[Line] is the public range surface.
Result still does not own output.
```

Core type:

```go
type Line struct {
	Stream Stream
	Text   string
	Stage  int
	Cmd    string
}
```

Prefer `iter.Seq[Line]` over `iter.Seq2[Stream, string]`, because a struct can grow to include stage, command, timestamp, or other metadata.

---

## 17. Phased Roadmap

### v0.0

Ship only the shell-first core:

```text
ShellSpec
Host/Sh/Bash/CmdExe/PowerShell/Pwsh
ShellSpec.Setup
ShellSpec.Shell
ShellSpec.Exec
Command Dir/Env/Stdin/Stdout/Stderr
Command Exec/Start
Process Wait/Kill
Result OK/Code/Error/OrExit/OrExitf
sh.OrExit/sh.OrExitf
```

### v0.1

Consider quality-of-life helpers:

```go
sh.Bash.Strict()
sh.Sh.Strict()
sh.Pwsh.Strict()
```

Possibly add simple filesystem/text helpers if needed by real scripts.

### v0.2

Add structured command execution:

```go
sh.Cmd("go", "test", "./...")
```

### v0.3

Add structured pipelines:

```go
cmd.Pipe(next).Exec()
```

### v0.4

Add line streaming:

```go
StartLines
Lines
iter.Seq[Line]
```

---

## 18. Recommended v0 Design Sentence

```text
Gorn v0 sh is a Go-shaped host-shell runner: ShellSpec chooses and configures a shell, Command configures process IO/env/dir, Process manages lifecycle, and Result reports status only.
```

---

## 19. Open Questions

### 19.1 Should `Host` use `/bin/sh` or lookup `sh` on PATH?

Possible Unix default:

```text
/bin/sh -c
```

Pros: conventional.  
Cons: not always present in unusual environments.

Alternative:

```text
sh -c
```

Pros: respects PATH.  
Cons: potentially less predictable.

### 19.2 Should `Bash` use `-c` or `-lc`?

`bash -lc` runs a login shell, which may load startup files and cause side effects. For Gorn, `bash -c` may be more predictable.

Possible definitions:

```go
Bash = ShellSpec{Name: "bash", Args: []string{"-c"}}
BashLogin = ShellSpec{Name: "bash", Args: []string{"-lc"}}
```

This may be preferable to making `Bash` login by default.

### 19.3 Should `Pwsh` use `-Command` or `-File`?

`-Command` is direct for inline snippets. `-File` requires writing a temporary script file but can have cleaner quoting behavior.

For v0, `-Command` is simpler.

### 19.4 Should `Kill` call `Process.Kill` only, or kill process groups?

For shell snippets that launch children, killing only the shell may leave child processes alive.

This is a real issue for:

```go
proc, err := sh.Host.Shell("npm run dev").Start()
```

Possible v0 behavior:

```text
Kill kills the direct process only.
```

Document the limitation.

Future behavior may add process group support:

```go
Command.ProcessGroup()
Process.KillGroup()
```

### 19.5 Should setup lines be joined with `\n` or `;`?

Prefer newline:

```text
setup line 1
setup line 2
script body
```

It keeps script diagnostics and readability closer to real shell scripts.

---

## 20. Final v0 Example

```go
#!/usr/bin/env gorn
//gorn:go 1.26

import (
	"bytes"
	"fmt"
	"strings"
)

//gorn:main

strict := sh.Bash.Setup("set -euo pipefail")

strict.Exec("go test -v ./...").OrExitf("tests failed")
strict.Exec("go vet ./...").OrExitf("vet failed")

var branchOut bytes.Buffer

sh.Host.
	Shell("git branch --show-current").
	Stdout(&branchOut).
	Exec().
	OrExitf("failed to get current branch")

branch := strings.TrimSpace(branchOut.String())
fmt.Println("current branch:", branch)
```

This is small, readable, and useful. It proves the Gorn file-app idea before committing to the larger portable `sh` surface.
