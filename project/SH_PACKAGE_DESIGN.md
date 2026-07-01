# Gorn `sh` Package Design

The `sh` package is Gorn's portable automation helper library.

It should make Go scripts better at common shell-script jobs without becoming a shell, a POSIX userland, or a CLI framework.

Import path for v0:

```go
import "gorn.dev/gorn/sh"
```

Generated `.gorn` files should import `sh` automatically.

---

## Design principles

1. **Portable helpers should use Go/OS primitives.**
   - Prefer `os`, `io`, `fs`, `path/filepath`, `runtime`, and `os/exec`.
   - Make Linux/macOS/Windows work by default.

2. **External commands are explicit host dependencies.**
   - `sh.Cmd("grep", ...)` calls whatever `grep` the user has.
   - Gorn does not promise `grep` exists on Windows.

3. **Do not bundle a Unix userland.**
   - No `sh.Grep`, `sh.Sed`, `sh.Awk`, `sh.Find`, `sh.Tar`, etc. in v0.

4. **Avoid shell flags for native helpers.**
   - Prefer typed Go options over strings like `"-rf"`.

5. **Keep the common path compact.**
   - Gorn scripts should be explicit, but not verbose.

6. **General Go libraries remain user choices.**
   - Argument parsing, JSON querying, terminal UI, HTTP clients, etc. should be normal Go dependencies unless there is a very strong reason to include them.

---

## Core API sketch

```go
package sh

// Process execution.
func Cmd(name string, args ...any) Result
func Run(name string, args ...any) Result
func Command(name string, args ...any) *Command
func Spread(values ...string) ArgList

// Globbing and paths.
func Glob(pattern string, opts ...GlobOption) []string
func MustGlob(pattern string, opts ...GlobOption) []string
func GlobWith(pattern string, opts GlobOptions) []string // future/advanced
func Home() string
func Expand(path string) string
func Abs(path string) string
func Join(parts ...string) string

// Filesystem.
func Exists(path string) bool
func IsFile(path string) bool
func IsDir(path string) bool
func Mkdir(path string, opts ...MkdirOption) Result
func Remove(path string, opts ...RemoveOption) Result
func Copy(src, dst string, opts ...CopyOption) Result
func Move(src, dst string, opts ...MoveOption) Result

// Text files.
func Read(path string) (string, error)
func MustRead(path string) string
func ReadLines(path string) ([]string, error)
func MustReadLines(path string) []string
func Write(path, text string) Result
func Append(path, text string) Result

// Failure helpers.
func OrExit(err error)
func OrExitf(err error, format string, args ...any)
func Fatal(args ...any)
func Fatalf(format string, args ...any)
```

---

## `Result`

Most script failures should flow through `sh.Result`.

```go
type Result struct {
	Op   string   // "cmd", "copy", "remove", etc.
	Name string   // command name for external commands
	Args []string // command args for external commands

	Code int

	Stdout string
	Stderr string
	Err    error

	StdoutCaptured bool
	StderrCaptured bool
}
```

Success:

```go
func (r Result) OK() bool
```

Failure conversion:

```go
func (r Result) Error() error
```

Script exits:

```go
func (r Result) OrExit() Result
func (r Result) OrExitf(format string, args ...any) Result
```

Example:

```go
sh.Run("go", "test", "./...").OrExitf("tests failed")

status := sh.Cmd("git", "status", "--short").
	OrExitf("could not read git status")

fmt.Print(status.Stdout)
```

`OrExit` and `OrExitf` should exit only on failure. They should return the original `Result` on success so chaining remains useful.

`Exit` is intentionally not the method name because it is ambiguous. `OrExit` communicates conditional behavior.

---

## Result failure output

For captured command failure:

```text
could not read git status

command: git status --short
exit status: 128

fatal: not a git repository
```

For streamed command failure:

```text
tests failed

command: go test ./...
exit status: 1
```

For native helper failure:

```text
failed to copy README

operation: copy README.md -> dist/README.md
cause: permission denied
```

If stderr was streamed by `Run`, do not duplicate it in `OrExitf` output.

Exit code normalization:

- Success returns without exiting.
- External command exit codes should be preserved where sensible.
- Internal execution errors should exit with `1`.
- Do not pass negative codes to `os.Exit`.

---

## Process execution

`Cmd` captures stdout/stderr.

```go
r := sh.Cmd("git", "status", "--short")
if r.OK() {
	fmt.Print(r.Stdout)
}
```

`Run` streams stdout/stderr to the current process.

```go
sh.Run("go", "test", "./...").OrExit()
```

`Command` is the future configurable builder:

```go
r := sh.Command("git", "status", "--short").
	Dir(repo).
	Env("GIT_PAGER", "cat").
	Capture().
	Run()
```

Do not run command strings through a shell by default.

Good:

```go
sh.Cmd("git", "status", "--short")
```

Avoid:

```go
sh.Cmd("git status --short")
```

If shell evaluation is ever added, name it explicitly:

```go
sh.Shell("echo *.go")
```

This should be deferred.

---

## Argument spreading for external commands

Use explicit spreading for multiple argv items.

```go
files := sh.Glob("**/*.go")
sh.Run("rm", "-f", sh.Spread(files...)).OrExit()
```

Rules:

```text
string        -> one argv item
sh.Spread(...) -> many argv items
[]string directly -> reject/panic/error; user should be explicit
```

Do not name this helper `Args`; reserve that concept for script argument parsing if it is ever added.

---

## Native helper options

Use variadic typed constants for simple boolean options.

This gives good completion discovery:

```go
sh.Remove("dist", sh.RemoveRecursive, sh.RemoveForce).OrExit()
sh.Copy("assets", "dist/assets", sh.CopyRecursive, sh.CopyOverwrite).OrExit()
files := sh.Glob("**/*.go", sh.GlobDotfiles, sh.GlobFollowSymlinks)
```

Use function-prefixed option names:

```go
type RemoveOption uint32

const (
	RemoveForce RemoveOption = 1 << iota
	RemoveRecursive
)

type CopyOption uint32

const (
	CopyOverwrite CopyOption = 1 << iota
	CopyRecursive
)

type GlobOption uint32

const (
	GlobDotfiles GlobOption = 1 << iota
	GlobFollowSymlinks
	GlobUnsorted
	GlobFailOnNoMatch
)
```

Avoid one global option type. API-specific option types let the compiler reject invalid combinations.

---

## Struct options escape hatch

When options need values, add `With` variants and struct options.

Common case:

```go
files := sh.Glob("**/*.go", sh.GlobDotfiles)
```

Advanced/value-bearing case:

```go
files := sh.GlobWith("**/*.go", sh.GlobOptions{
	Root:            "src",
	Dotfiles:        true,
	FollowSymlinks: true,
	FailOnNoMatch:  true,
})
```

Rule:

> Use variadic typed option constants for simple boolean behavior. Add `With` + `Options` struct variants only when the operation needs value-bearing or advanced configuration.

Avoid functional options in v0. They add ceremony without enough value for this package.

---

## Glob semantics

`sh.Glob` should be better and more predictable than shell globbing.

Default behavior:

- `**` recurses.
- Results are lexicographically sorted.
- No match returns an empty slice.
- Dotfiles are excluded unless enabled with `GlobDotfiles` or the pattern segment explicitly starts with `.`.
- Symlink directories are not followed unless enabled with `GlobFollowSymlinks`.
- Patterns should use slash-style paths and be normalized internally for Windows.

Example:

```go
for _, file := range sh.Glob("**/*.go") {
	fmt.Println(file)
}
```

Advanced:

```go
files := sh.Glob("**/*.go", sh.GlobDotfiles, sh.GlobFollowSymlinks)
```

---

## Filesystem helpers

Native helpers should be cross-platform and not shell-flag compatible.

Good:

```go
sh.Mkdir("dist/bin", sh.MkdirParents).OrExit()
sh.Remove("dist", sh.RemoveRecursive, sh.RemoveForce).OrExit()
sh.Copy("assets", "dist/assets", sh.CopyRecursive, sh.CopyOverwrite).OrExit()
```

Avoid:

```go
sh.Rm("-rf", "dist")
sh.Cp("-R", "assets", "dist/assets")
```

If the user wants POSIX `rm` or `cp`, they can call the host command:

```go
sh.Run("rm", "-rf", "dist").OrExit()
```

That is intentionally host-dependent.

---

## Text file helpers

Text helpers are script IO, not userland replacements.

```go
text := sh.MustRead("README.md")
sh.Write("out.txt", text).OrExit()
sh.Append("out.txt", "\n").OrExit()
```

Error-returning variants should exist for normal Go style:

```go
text, err := sh.Read("config.json")
sh.OrExitf(err, "failed to read config.json")
```

---

## Argument parsing policy

Do not include an argument parser in `sh` v0.

For simple scripts, users can use `os.Args`.

For typed flags and positional arguments, users can depend on an ordinary Go package.

Example using `github.com/alexflint/go-arg`:

```go
#!/usr/bin/env gorn
//gorn:require github.com/alexflint/go-arg v1.6.1

import (
	"fmt"

	arg "github.com/alexflint/go-arg"
)

type Args struct {
	Output string   `arg:"-o,--output" default:"todo.txt"`
	Files  []string `arg:"positional,required"`
}

//gorn:main

var args Args
arg.MustParse(&args)

fmt.Println("output:", args.Output)
fmt.Println("files:", args.Files)
```

Gorn should not own this API unless real usage proves it belongs in core.

---

## Native userland policy

Do not add wrappers like these in v0:

```go
sh.Grep(...)
sh.Find(...)
sh.Sed(...)
sh.Awk(...)
sh.Tar(...)
sh.Curl(...)
```

Use host tools explicitly:

```go
sh.Cmd("grep", "-q", "TODO", file)
```

Or use Go stdlib / dependencies:

```go
if strings.Contains(sh.MustRead(file), "TODO") {
	fmt.Println(file)
}
```

This keeps Gorn portable, small, and honest.
