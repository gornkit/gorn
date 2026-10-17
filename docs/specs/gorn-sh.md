---
title: Gorn Shell Helper Contract
status: authoritative
type: spec
updated: 2026-10-17
summary: Exact arguments, supported shell startup and strictness, immutable configuration, IO, and process results.
---

# Shell helper contract

`github.com/gornkit/gorn/sh` is a self-contained, stdlib-only library. It works
outside Gorn and is bundled into generated modules by default. The CLI does not
import the package; the embedded source digest participates in build identity,
so changes to these defaults invalidate affected generated builds.

## Arguments and shells

`sh.Cmd(name, args...)` passes separate arguments through Go's `os/exec`, without
globbing, variable expansion, quoting, pipelines, or a shell. Arguments are copied
when the command is configured. Prefer it for external tools:

```go
result := sh.Cmd("go", "test", "./...").Exec()
if !result.OK() {
    fmt.Fprintln(os.Stderr, result.Error())
}
```

On Windows, native executables using normal Windows argument parsing receive
these arguments literally. Batch files, Cmd and programs with custom command-line
parsers are exceptions to Go's normal quoting algorithm. Use `CmdExe().Shell`
for Cmd syntax and author its quoting explicitly. Gorn does not translate shell
syntax between platforms. See [Go's argument contract](https://pkg.go.dev/os/exec#Command).

| Constructor | Invocation | Required native verification |
| --- | --- | --- |
| `Bash()` | `bash -c <script>` | Linux and macOS |
| `Zsh()` | `zsh -fc <script>` | macOS; optional Linux |
| `Sh()` | `sh -c <script>` | Linux and macOS |
| `CmdExe()` | `cmd.exe /D /S /C <script>` | Windows |
| `Pwsh()` | `pwsh -NoLogo -NoProfile -NonInteractive -Command <script>` | PowerShell 7 on Windows; optional elsewhere |

`Host()` preserves the Unix `SHELL` executable path when its basename is `bash`,
`zsh`, or `sh`, using the corresponding defaults above; otherwise it uses `sh`.
Windows always uses Cmd, irrespective of `SHELL` or `COMSPEC`. Named constructors
look up their executable on the parent process PATH. `NewShell(base, args...)`
preserves a chosen executable and explicit startup arguments, appending the
script last. This is the escape hatch for login shells, profiles or other shells;
custom invocations receive no portable-behavior guarantee. No shell is installed
or silently substituted when a selected executable is missing.

Defaults avoid login/interactive startup. Non-login Bash still honors inherited
`BASH_ENV`; set `.Env("BASH_ENV", "")` when that startup hook is unwanted. Zsh
`-f` skips personal startup files, but its global `zshenv` can still execute. Sh
startup follows the installed implementation. Cmd `/D` suppresses registry
AutoRun; PowerShell suppresses profiles and turns prompt requests into errors.
These are automation defaults, not a hermetic shell environment. References:
[Bash startup](https://www.gnu.org/software/bash/manual/html_node/Bash-Startup-Files.html),
[Zsh startup](https://zsh.sourceforge.io/Doc/Release/Files.html),
[Cmd flags](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/cmd),
[PowerShell startup](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_pwsh?view=powershell-7.6).

## Setup and strictness

`Setup(lines...)` appends setup in call order before each script. `Strict()` adds
setup at its call position; later setup can deliberately override it. Constructors,
setup, argument slices and command configuration return independent copies.
Readers and writers are shared references, so callers coordinate concurrent use.

| Shell | `Strict()` setup | Limit |
| --- | --- | --- |
| Bash / Zsh | `set -euo pipefail` | Native shell exceptions to errexit still apply, including conditional contexts |
| Sh | `set -eu` | No portable pipeline failure detection |
| PowerShell | `$ErrorActionPreference = 'Stop'` | Stops PowerShell errors; does not enable Set-StrictMode or native-command failure policy |
| Cmd / unknown | Unchanged | Use shell control flow and explicit exit status |

Windows executable recognition is case-insensitive, accepts `.exe` and absolute
paths with either separator. Unix basenames retain their case rules. Strictness
does not inspect or rewrite the script.

For PowerShell native commands, inspect `$LASTEXITCODE` and use
`exit $LASTEXITCODE` to preserve a specific native status. A later successful
PowerShell command can hide an earlier native failure; `-Command` otherwise
normalizes a final native failure to status 1. Prefer `sh.Cmd` when shell syntax
is unnecessary. See [PowerShell status semantics](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_pwsh?view=powershell-7.6)
and [native error preferences](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_preference_variables?view=powershell-7.6).

Setup is newline-separated except on Windows Cmd, where fragments are joined
with `&` without adding whitespace. Cmd receives the raw script inside an outer
quote pair, rather than Go's normal argv escaping; custom Cmd invocations ending
in `/C` or `/K` use the same transport. Caller-authored shell syntax is retained.
Cmd's command-string rules apply: `%NAME%` expands before setup executes, `rem`
can consume the remainder, and unquoted `set NAME=value ` retains trailing spaces.
Use quoted `set "NAME=value"`, separate invocations, or an explicit
`NewShell("cmd.exe", "/D", "/V:ON", "/S", "/C")` with `!NAME!` expansion when
setup changes variables used later. Setup does not emulate batch-file line
execution or provide universal fail-fast behavior.

## Configuration, IO and results

`Dir`, `Env`, `Stdin`, `Stdout`, `Stderr` and `Context` configure copies. The default
cwd and environment are inherited at Start; Env overrides inherited values.
Executable lookup happens using the parent PATH when Start constructs the Go
command, before the child Dir/Env take effect. Supply an explicit executable path
when child PATH configuration must not affect lookup expectations. IO defaults
to the caller's standard streams, including stdin; nil IO means inheritance.
Use `io.Discard`, an empty reader, files or buffers explicitly as needed. The
helper never closes caller IO and does not capture output in Result.

`Exec()` starts and waits, returning a status-only `Result`. `Start()` returns a
`Process` or a startup error. A process is reaped automatically; concurrent or
repeated `Wait()` returns the same completed result. `Kill()` requests termination,
is idempotent, and does not wait for IO; `Wait()` observes cleanup. Unspecified
context is Background; explicitly supplied nil is invalid. Cancellation errors
retain `errors.Is` for context cancellation/deadline errors. Startup/non-exit
failures use status 1; child statuses and POSIX `128+signal` are preserved.

`Result.OK`, `Code`, and `Error` inspect status. `Result.OrExit` prints an error
and exits with its nonzero status; `OrExitf` adds a caller message. Package-level
`OrExit(error)` and `OrExitf(error, ...)` handle ordinary Go errors with status 1.
They are explicit process-exit conveniences, not cleanup scopes. Code needing
deferred cleanup should inspect the result and return before its main exits.

The [process ownership contract](gorn-build-runtime.md#cancellation-and-process-ownership)
defines groups, terminal ownership, escalation, pipe bounds and Windows limits.
This slice changes shell transport/defaults, not that lifecycle.

## Decisions, alternatives and evidence

Login/profile defaults make interactive tools convenient but can add output,
change PATH/cwd, run arbitrary initialization and slow every snippet. Non-login
startup favors automation; NewShell retains explicit access to profiles. Scrubbing
every environment startup hook would contradict environment inheritance and still
could not remove global Zsh startup, so residual behavior is documented instead.

Universal Strict would require script parsing/rewriting and version-sensitive
native-status policy. Bounded named-shell setup preserves each language's control
flow and exposes the limits. A universal Windows argv quoting rule is incompatible
with Cmd; raw transport is scoped to explicit Cmd shell snippets, preserving Cmd's
syntax while keeping `sh.Cmd` on Go's ordinary executable argument path.

[Named-shell tests](../../sh/shell_behavior_test.go) exercise required shells,
absolute paths, setup copies/order, environment/cwd/IO, failures, strictness and
startup files. Missing required shells fail; optional shells skip with a reason.
[Windows tests](../../sh/shell_behavior_windows_test.go) exercise case/extension
recognition, quoted executable paths and arguments, delayed expansion, native
PowerShell status and preservation of process attributes.
[Exact-argument tests](../../sh/command_test.go) and existing lifecycle/terminal
integration tests remain regression gates. Native three-platform CI is required
before landing; target lint and cross-compilation are not runtime verification.
