# Gorn Script Format Specification

Status: reflects the current implementation in [`pkg/gornparser`](../../pkg/gornparser/) (`parser.go` and `generator.go`), which is tested.
Where this spec and [`project/GORN_DESIGN.md`](../../project/GORN_DESIGN.md) diverge, this document and the implementation are authoritative — `project/` holds historical/reference design notes only.

## Implementation status

Read this section first; it sets expectations for the rest of the document.

- **Implemented and tested:** the `.gorn` source file format below (shebang, directives, the package/main split, every parse error), **code generation** — turning a parsed `Script` into a `go.mod` and a formatted `main.go` (both in `pkg/gornparser`) — and the **build/run pipeline**. `gorn run <script>` (and the `gorn <script>` shorthand) parses, generates, builds the module with the Go toolchain, caches the built binary keyed on a content hash, and executes it, forwarding script args. `--print-mod`/`--print-main` print the generated artifacts and exit without running; `--no-cache` builds fresh and bypasses the cache; `--verbose` adds stderr diagnostics (including a `cache: hit/miss/bypass` line).
- **Not yet implemented:** `gorn build`/`gorn cache` (stubs returning "not implemented"), `gorn eject`, fragments/includes (`.gfrag`) and their directives (`//gorn:fragment`, `//gorn:include`), and `//gorn:replace`.

In short: Gorn parses, generates, builds, caches, and runs a `.gorn` file today; the subcommands and fragment/eject features above are still aspirational.

## Overview

A `.gorn` file is a single-file Go automation script. It looks like ordinary Go with a small directive layer on top, and is split into two sections by a required `//gorn:main` marker:

1. **Package section** — everything before `//gorn:main`: optional directives, plus ordinary Go imports and package-level declarations (functions, types, etc.).
2. **Main section** — everything after `//gorn:main`: ordinary Go statements, conceptually compiled into the body of `func main()`.

```gorn
#!/usr/bin/env gorn
//gorn:go 1.26

import "fmt"

//gorn:main

fmt.Println("hello, gorn")
```

There is no new syntax to learn inside either section — it's Go. Gorn only defines the shebang, the directive lines, and where the package/main split happens.

## File structure

Reading a `.gorn` file top to bottom:

1. **Line 1 may be a shebang.** Any line 1 beginning with `#!` is discarded verbatim and does not count as package content. Gorn does not validate or require a specific shebang target — `#!/usr/bin/env gorn`, `#!gorn`, or any other `#!`-prefixed first line is accepted equally. This is a deliberate, permissive design choice, not an oversight.
2. **Directives**, in any order relative to each other but always before any real (non-blank) package-section content. See [Directives](#directives) below.
3. **Package-section content** — imports and declarations, ordinary Go. Optional; a script may have no package section at all.
4. **Exactly one `//gorn:main` directive**, required.
5. **Main-section content** — ordinary Go statements. At least one non-blank line is required; an empty main section is an error.

## Directives

A directive is a line whose trimmed content starts with `//gorn:`. Directive names are matched exactly and case-sensitively — `//gorn:mainly`, `//gorn:gopher`, `//gorn:requirement`, and `//gorn:modules` are all invalid directives, not aliases.

Directive arguments are split on whitespace (`bytes.Fields`); there is no quoting mechanism, so directive arguments must not themselves contain whitespace.

| Directive | Syntax | Cardinality | Notes |
|---|---|---|---|
| `//gorn:go` | `//gorn:go <version>` | at most once | Sets `Script.GoVersion`. The version token is stored as-is; it is not validated against real Go release versions at parse time. |
| `//gorn:module` | `//gorn:module <module-path>` | at most once | Sets `Script.Module`. The path token is stored as-is; it is not validated as a real module path at parse time. |
| `//gorn:require` | `//gorn:require <module-path> <version>` | any number of times | Appends to `Script.Requires`. `latest` is rejected as a version — an explicit version is always required. |
| `//gorn:preamble` | `//gorn:preamble` | at most once | Opts the script into ambient stdlib imports (see [Preamble](#preamble)). Takes no arguments. |
| `//gorn:main` | `//gorn:main` | exactly once | Marks the package/main boundary. Required in every script. |

### Directive placement rules

- Directives (other than `//gorn:main`) must appear before any real, non-blank package-section content. Once a non-blank package-section line has been seen, a further directive is an error (`ErrDirectiveAfterPackage`), because letting a directive appear in the middle of package content would create a gap in the line numbering Gorn uses to map generated code back to the original source.
- **Blank lines do not count as content for this rule.** Leading blank lines — including blank lines used purely to visually separate directives — are dropped and never recorded, so directives may freely appear after blank-only spacing:

  ```gorn
  //gorn:go 1.26

  //gorn:module example.com/scripts/spaced

  //gorn:require github.com/example/tool v1.0.0

  import "fmt"

  //gorn:main

  fmt.Println("blank lines between directives are fine")
  ```

- No directive (including `//gorn:main` again) may appear after `//gorn:main` (`ErrDirectiveAfterMain`, or `ErrMultipleMain` specifically for a second `//gorn:main`).
- `//gorn:go` and `//gorn:module` may each appear at most once; a second occurrence is `ErrDuplicateGo` / `ErrDuplicateModule`. `//gorn:require` is intentionally repeatable — one per dependency.

## Package section

The package section holds ordinary Go: `import` blocks, `func`/`type`/`var`/`const` declarations, and comments. Gorn does not parse or validate arbitrary Go syntax itself; it slices out the raw lines and lets the Go toolchain judge correctness when the generated module is built. (The one exception is [preamble](#preamble) conflict detection, which inspects imports.)

Leading blank lines before the first real package-section line are dropped entirely and are not part of the recorded package section, for the same reason directives may follow blank spacing: there is nothing meaningful to anchor a line-number reference to.

A script may have an empty package section — no imports, no declarations — if the main section doesn't need any.

## Main section

The main section holds ordinary Go statements, conceptually forming the body of `func main()`.

- **At least one non-blank line is required.** A script where `//gorn:main` is the last line, or where only blank lines follow it, is rejected (`ErrEmptyMain`).
- Leading blank lines immediately after `//gorn:main` are dropped, not preserved — Gorn never needs to reconstruct the original source byte-for-byte, so this formatting detail is intentionally lost.
- No directives are permitted anywhere in the main section.

## Preamble

`//gorn:preamble` opts a script into a small, fixed set of **ambient imports**: the generated `main.go` imports them and keeps them alive, so the script may use these packages without writing the imports itself. It is opt-in precisely because ambient imports are non-obvious; scripts that don't use the directive behave like ordinary Go (write your own imports).

The preamble set (stdlib plus gorn's own `sh`):

| Import path | Made available as |
|---|---|
| `fmt` | `fmt` |
| `os` | `os` |
| `path/filepath` | `filepath` |
| `strconv` | `strconv` |
| `strings` | `strings` |
| `time` | `time` |
| `github.com/gornkit/gorn/sh` | `sh` |

Rules and caveats:

- **Don't also import a preamble package.** In a preamble script, importing any package whose path is in the set above is an error (`ErrPreambleImportConflict`), reported at the offending import's original source line. Detection is by import **path**, so an aliased import (e.g. `import f "fmt"`) is flagged too — the package is already provided, aliased or not.
- **Only these packages are ambient.** Anything else — including other stdlib packages and all `//gorn:require` dependencies — must still be imported explicitly.
- **Shadowing disables the ambient import in scope.** Declaring a local identifier named after a preamble package (e.g. a variable `strings`) shadows the import for that scope, as in ordinary Go. Rare, but worth knowing since the import is invisible in the source.

## Errors

Parsing (`ParseSource`/`ParseFile`) returns a `*gornparser.Error` wrapping one of the sentinels below, carrying the 1-based source line where the problem was detected. Generation (`Generate`) returns a `*gornparser.GenerateError`; for a preamble conflict it wraps a line-carrying `*Error`, and for a formatting failure it carries the raw unformatted output in `Raw`. `errors.Is` and `errors.As` work against all of these.

| Error | Raised by | Meaning | Example |
|---|---|---|---|
| `ErrFailedToReadFile` | parse | `ParseFile` could not read the file from disk. | Path does not exist. |
| `ErrEmptyScript` | parse | The source is zero-length. | `""` |
| `ErrMissingMain` | parse | No `//gorn:main` directive found. | `import "fmt"\n` |
| `ErrMultipleMain` | parse | More than one `//gorn:main` directive. | `//gorn:main\nx()\n//gorn:main\n` |
| `ErrDirectiveAfterMain` | parse | A `//gorn:` directive appears after `//gorn:main`. | `//gorn:main\n//gorn:go 1.26\n` |
| `ErrDirectiveAfterPackage` | parse | A directive appears after real package-section content started. | `import "fmt"\n//gorn:require x v1\n//gorn:main\n` |
| `ErrInvalidDirective` | parse | Unknown directive, wrong arg count for `go`/`module`/`preamble`, or an empty directive. | `//gorn:unknown value\n` |
| `ErrInvalidRequire` | parse | `//gorn:require` with the wrong argument count, or a `latest` version. | `//gorn:require github.com/example/tool\n` |
| `ErrDuplicateGo` | parse | A second `//gorn:go` directive. | `//gorn:go 1.26\n//gorn:go 1.27\n` |
| `ErrDuplicateModule` | parse | A second `//gorn:module` directive. | `//gorn:module a\n//gorn:module b\n` |
| `ErrEmptyMain` | parse | The main section has no non-blank content. | `//gorn:main\n` (nothing after it) |
| `ErrPreambleImportConflict` | generate | A preamble script imports a preamble package itself. | `//gorn:preamble` + `import "fmt"` |

## Examples

### Minimal script

```gorn
#!/usr/bin/env gorn
//gorn:go 1.26

import "fmt"

//gorn:main

fmt.Println("hello, gorn")
```

### Script with a module path and a dependency

```gorn
#!/usr/bin/env gorn
//gorn:go 1.26
//gorn:module example.com/scripts/todo
//gorn:require github.com/charmbracelet/lipgloss v1.1.0

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gornkit/gorn/sh"
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

Note: the `sh` import uses the real current module path (`github.com/gornkit/gorn/sh`). This example imports everything explicitly; the [preamble](#preamble) example below shows how `//gorn:preamble` lets a script drop the `fmt`, `strings`, and `sh` imports.

### Script using the preamble

With `//gorn:preamble`, the `fmt`, `strings`, and `sh` imports become ambient and must not be written explicitly. Only the non-preamble dependency (`lipgloss`) is imported:

```gorn
#!/usr/bin/env gorn
//gorn:go 1.26
//gorn:module example.com/scripts/todo
//gorn:require github.com/charmbracelet/lipgloss v1.1.0
//gorn:preamble

import "github.com/charmbracelet/lipgloss"

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

Adding `import "fmt"` to this script would fail with `ErrPreambleImportConflict` — `fmt` is already provided by the preamble.

### Script with no package section

A script needs no imports or declarations if the main section is self-contained:

```gorn
//gorn:main

println("no imports needed for a builtin function")
```

### Invalid: directive after real content

```gorn
import "fmt"
//gorn:require github.com/example/tool v1.0.0
//gorn:main
```

Fails with `ErrDirectiveAfterPackage` at line 2 — the `import` line already started the package section, so the `require` directive that follows is rejected. Moving the `require` directive above the `import` fixes it.

### Invalid: empty main section

```gorn
//gorn:main
```

Fails with `ErrEmptyMain` at line 1 — there is nothing to run.

## For tool authors: line-number tracking

`Script.PackageStart`/`Script.MainStart` record the 1-based source line of the first line in `Script.PackageContent`/`Script.MainContent`, respectively. Both are contiguous runs of the original source (no gaps from filtered-out directive or leading-blank lines), so a single `//line <path>:<N>` directive at `PackageStart`/`MainStart`, followed by emitting the corresponding content verbatim, maps generated-code compiler errors back to the original `.gorn` source — no per-line tracking needed. This is exactly what the generator's `main.gotmpl` template does.

`MainStart` is a plain `int` and is always ≥ 1 on a successful parse (an empty main section is rejected with `ErrEmptyMain`). `PackageStart` is a `*int`, nil when the script has no non-blank package-section content. See the doc comments on `Script` in `pkg/gornparser/parser.go` for the exact guarantees.

## Not yet implemented

The following are part of the longer-term design (see [`project/GORN_DESIGN.md`](../../project/GORN_DESIGN.md)) but do not exist in the codebase today:

- The `gorn build`/`gorn cache` commands (currently stubs returning "not implemented"). Note: `gorn run` already builds and caches; these separate subcommands are not wired up.
- `gorn eject` — converting a `.gorn` file into a normal Go module.
- Fragments/includes (`.gfrag` files) and their directives (`//gorn:fragment`, `//gorn:include`).
- `//gorn:replace`.

Treat any of the above as aspirational until this document (or a successor) is updated to say otherwise.
