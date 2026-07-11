# Gorn Design Notes

**Gorn** is a Go file-app runner for portable automation scripts.

Secret acronym: **Go Right Now**.

Public positioning:

> Gorn lets you write single-file Go automation scripts with a small directive layer, a generated Go module, cached builds, and portable shell-grade helpers.

Gorn is not a new shell language. It is also not a bundled POSIX userland. The core idea is to keep the programming model close to Go while removing the project ceremony that makes Go awkward for small scripts.

---

## Goals

Gorn should make this kind of script pleasant:

```go
#!/usr/bin/env gorn

import (
	"fmt"
	"strings"
)

//gorn:main

for _, file := range sh.Glob("**/*.go") {
	if strings.Contains(sh.MustRead(file), "TODO") {
		fmt.Println(file)
	}
}
```

The main goals are:

- Keep script source recognisably Go.
- Avoid Bash quoting, word splitting, and platform-specific shell behavior.
- Use generated Go modules rather than inventing a package system.
- Use the Go toolchain for compile, build cache, module download, formatting, diagnostics, and cross-platform builds.
- Provide a small `sh` package for portable process, filesystem, glob, path, and file helpers.
- Let users call real external tools with `sh.Cmd` / `sh.Run` when they intentionally depend on those tools.
- Make cached reruns fast enough that Gorn feels script-like after the first build.
- Keep v0 small enough to build.

---

## Non-goals

Gorn should not become:

- A POSIX shell implementation.
- A Fish/Nushell/Elvish competitor.
- A Go-flavoured language with new loops, pipes, lambdas, truthiness, or object semantics.
- A BusyBox-style reimplementation of `grep`, `find`, `sed`, `awk`, `tar`, etc.
- A CLI framework like Cobra, Kong, or `go-flags`.
- A second Go package manager.
- A replacement for ordinary Go modules when reusable code is needed.

If a script needs reusable code, it should use normal Go dependencies.

If a script needs a host command, it should call the host command.

If a script needs serious CLI parsing, it can depend on a normal Go package such as `github.com/alexflint/go-arg`.

---

## File model

A `.gorn` file is a Go file-app source file. It is transformed into a generated Go `main` package before being built.

Gorn source has two sections:

1. **Package section**: imports and package-level declarations.
2. **Main section**: statements compiled into `func main()`.

The `//gorn:main` directive separates the two sections.

```go
#!/usr/bin/env gorn
//gorn:go 1.26
//gorn:require github.com/charmbracelet/lipgloss v1.1.0

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func hasTodo(path string) bool {
	return strings.Contains(sh.MustRead(path), "TODO")
}

//gorn:main

style := lipgloss.NewStyle().Bold(true)

for _, file := range sh.Glob("**/*.go") {
	if hasTodo(file) {
		fmt.Println(style.Render(file))
	}
}
```

Conceptual generated Go:

```go
package main

import "gorn.dev/gorn/sh"

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func hasTodo(path string) bool {
	return strings.Contains(sh.MustRead(path), "TODO")
}

func main() {
	style := lipgloss.NewStyle().Bold(true)

	for _, file := range sh.Glob("**/*.go") {
		if hasTodo(file) {
			fmt.Println(style.Render(file))
		}
	}
}
```

Multiple import declarations are valid Go. Gorn can initially generate a separate `import "gorn.dev/gorn/sh"` rather than trying to merge import blocks. Import normalization can come later if it earns its keep.

---

## Required `//gorn:main`

`//gorn:main` is intentionally explicit. It avoids magic inference and keeps the generated Go model obvious.

Rules:

- Exactly one `//gorn:main` directive is required.
- Everything before `//gorn:main` is package-level Go.
- Everything after `//gorn:main` is compiled into `func main()`.
- Directives are only allowed before `//gorn:main`.
- Imports and declarations belong before `//gorn:main`.
- Top-level executable statements belong after `//gorn:main`.

This makes the file format easy to parse and easy to reason about.

---

## v0 directives

Start with only these directives:

```go
//gorn:go <version>
//gorn:module <module-path>
//gorn:require <module-path> <version>
//gorn:main
```

Examples:

```go
//gorn:go 1.26
//gorn:module example.com/scripts/todo
//gorn:require github.com/charmbracelet/lipgloss v1.1.0
```

`//gorn:require` should require explicit versions. Do not allow `latest` in source. A future command such as `gorn add <module>@latest` can resolve `latest` once and write a concrete version.

Defer:

```go
//gorn:replace
//gorn:fragment
//gorn:include
//gorn:prelude
```

---

## Dependency model

The root `.gorn` file owns its generated module.

A generated `go.mod` is produced from directives:

```go
module gorn.local/app/<hash>

go 1.26

require github.com/charmbracelet/lipgloss v1.1.0
```

If no `//gorn:module` is supplied, Gorn should generate a stable local module path, for example:

```text
gorn.local/app/<short-source-hash>
```

Reusable code should be ordinary Go code in ordinary Go modules.

Example:

```go
//gorn:require github.com/charles/automation v0.1.0

import "github.com/charles/automation/report"
```

Gorn should not invent remote script includes or script dependency version solving.

---

## Fragments and includes

Fragments/includes are deferred.

The likely future model is:

```text
.gorn  = one runnable Gorn app root
.gfrag = app-local source fragment, if this feature is added
Go modules = reusable code
```

A fragment, if added later, should be app-local organization rather than reusable package distribution.

Possible future rules:

- `.gorn` contains one `//gorn:main`.
- `.gfrag` contains imports and declarations only.
- `.gfrag` has no top-level statements.
- `.gfrag` has no dependency directives.
- One `.gorn`/`.gfrag` source unit generates one `.go` file in package `main`.
- Do not hoist or rewrite import aliases across files.

This avoids turning includes into a second package system.

---

## Relationship to Go

Gorn should preserve Go gravity:

- Use `:=`, `range`, `if`, `for`, `func`, `type`, `map`, slices, structs, and ordinary imports.
- Use normal Go dependencies for libraries.
- Use normal Go packages for real logic.
- Use the Go toolchain to build generated code.

Avoid:

- New loop syntax.
- New variable declaration syntax.
- New pipe operators.
- New lambda syntax.
- Dynamic truthiness.
- Shell-style word splitting.
- Implicit glob expansion.

The programming language is Go. Gorn only provides a file-app source format and shell automation helpers.

---

## Cross-platform principle

Anything implemented by `sh` should be cross-platform unless the name clearly indicates host dependence.

Portable:

```go
sh.Glob("**/*.go")
sh.Copy("assets", "dist/assets", sh.CopyRecursive)
sh.Remove("dist", sh.RemoveRecursive, sh.RemoveForce)
sh.Read("config.json")
```

Host-dependent:

```go
sh.Cmd("grep", "-q", "TODO", file)
sh.Run("go", "test", "./...")
```

This distinction is central to Gorn.

---

## CLI shape

Initial commands:

```text
gorn run <script.gorn> [-- args...]
gorn build <script.gorn> -o <path>
gorn cache path
gorn cache clean
gorn cache prune
gorn version
```

Useful development flags:

```text
--rebuild       ignore the Gorn binary cache
--no-cache      build in a temp dir and discard output
--keep          keep generated project and print its path
--print-gen     print generated Go
--print-mod     print generated go.mod
```

Later convenience:

```text
gorn <script.gorn>
```

---

## Eject

A future `gorn eject` command should convert a `.gorn` file app into a normal Go module.

```text
gorn eject todo.gorn ./todo
```

Output:

```text
todo/
  go.mod
  main.go
```

This is the escape hatch when a script grows into a proper program.

---

## v0 product test

Gorn v0 is successful if this feels good:

```go
#!/usr/bin/env gorn

import (
	"fmt"
	"strings"
)

//gorn:main

for _, file := range sh.Glob("**/*.go") {
	if strings.Contains(sh.MustRead(file), "TODO") {
		fmt.Println(file)
	}
}
```

Run:

```sh
gorn run todo.gorn
```

The first run may compile. Subsequent runs should feel immediate.
