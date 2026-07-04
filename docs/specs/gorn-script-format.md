# Gorn Script Format Specification

Status: reflects the current implementation in [`pkg/gornparser`](../../pkg/gornparser/parser.go), which is fully tested.
Where this spec and [`project/GORN_DESIGN.md`](../../project/GORN_DESIGN.md) diverge, this document and the implementation are authoritative — `project/` holds historical/reference design notes only.

## Implementation status

Read this section first; it sets expectations for the rest of the document.

- **Implemented and tested:** the `.gorn` source file format described below — shebang handling, directives, the package/main split, and every error condition — is fully implemented and covered by tests in `pkg/gornparser`.
- **Not yet implemented:** code generation, the build pipeline, and script execution. `gorn run <script>` currently parses its CLI arguments but does not invoke the parser, generate Go code, or execute the script — it only prints the resolved script path and arguments (see `pkg/app/runcmd.go`). `gorn build` and `gorn cache` are unimplemented stubs. `gorn eject`, fragments/includes (`.gfrag`), and directives such as `//gorn:replace`, `//gorn:fragment`, `//gorn:include`, and `//gorn:prelude` do not exist yet.

In short: this spec defines the file format Gorn already understands and validates. It does not yet describe a runnable system end to end.

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

The package section holds ordinary Go: `import` blocks, `func`/`type`/`var`/`const` declarations, and comments. Gorn does not parse or validate Go syntax itself; it only slices out the raw lines. Syntactic and semantic correctness of the Go code is a build-time concern (not yet implemented).

Leading blank lines before the first real package-section line are dropped entirely and are not part of the recorded package section, for the same reason directives may follow blank spacing: there is nothing meaningful to anchor a line-number reference to.

A script may have an empty package section — no imports, no declarations — if the main section doesn't need any.

## Main section

The main section holds ordinary Go statements, conceptually forming the body of `func main()`.

- **At least one non-blank line is required.** A script where `//gorn:main` is the last line, or where only blank lines follow it, is rejected (`ErrEmptyMain`).
- Leading blank lines immediately after `//gorn:main` are dropped, not preserved — Gorn never needs to reconstruct the original source byte-for-byte, so this formatting detail is intentionally lost.
- No directives are permitted anywhere in the main section.

## Errors

`ParseSource`/`ParseFile` return a `*gornparser.Error` wrapping one of these sentinels, with the 1-based source line where the problem was detected (`errors.Is`/`errors.As` both work against the returned error):

| Error | Meaning | Example |
|---|---|---|
| `ErrFailedToReadFile` | `ParseFile` could not read the file from disk. | Path does not exist. |
| `ErrEmptyScript` | The source is zero-length. | `""` |
| `ErrMissingMain` | No `//gorn:main` directive was found anywhere in the file. | `import "fmt"\n` |
| `ErrMultipleMain` | More than one `//gorn:main` directive. | `//gorn:main\nx()\n//gorn:main\n` |
| `ErrDirectiveAfterMain` | A `//gorn:` directive appears after `//gorn:main`. | `//gorn:main\n//gorn:go 1.26\n` |
| `ErrDirectiveAfterPackage` | A directive appears after real package-section content has already started. | `import "fmt"\n//gorn:require x v1\n//gorn:main\n` |
| `ErrInvalidDirective` | Unknown directive name, or wrong argument count for `go`/`module`, or an empty directive. | `//gorn:unknown value\n` |
| `ErrInvalidRequire` | `//gorn:require` with the wrong argument count, or a `latest` version. | `//gorn:require github.com/example/tool\n` or `... latest` |
| `ErrDuplicateGo` | A second `//gorn:go` directive. | `//gorn:go 1.26\n//gorn:go 1.27\n` |
| `ErrDuplicateModule` | A second `//gorn:module` directive. | `//gorn:module a\n//gorn:module b\n` |
| `ErrEmptyMain` | The main section has no non-blank content. | `//gorn:main\n` (nothing after it) |

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

Note: the `sh` import above uses the real current module path (`github.com/gornkit/gorn/sh`). Once a code generator exists, it may auto-inject this import so scripts don't have to write it themselves — see [`project/GORN_DESIGN.md`](../../project/GORN_DESIGN.md) for that (unimplemented) design intent. Today, the parser has no opinion on imports at all; it only slices lines.

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

`Script.PackageStart`/`Script.MainStart` record the 1-based source line of the first line in `Script.PackageLines`/`Script.MainLines`, respectively. Both slices are guaranteed contiguous runs of the original source (no gaps from filtered-out directive or leading-blank lines), so a single `//line <path>:<N>` directive at `PackageStart`/`MainStart`, followed by emitting the corresponding lines verbatim, is sufficient to map generated-code compiler errors back to the original `.gorn` source — no per-line tracking is needed. See the doc comments on `Script` in `pkg/gornparser/parser.go` for the exact contiguity guarantees.

## Not yet implemented

The following are part of the longer-term design (see [`project/GORN_DESIGN.md`](../../project/GORN_DESIGN.md)) but do not exist in the parser, CLI, or anywhere else in the codebase today:

- Code generation from a parsed `Script` into a runnable Go module.
- The `gorn build`/`gorn cache` commands (currently stub implementations that return "not implemented").
- Actually executing a script via `gorn run`/`gorn <script>` (currently prints the script path and arguments only).
- `gorn eject` — converting a `.gorn` file into a normal Go module.
- Fragments/includes (`.gfrag` files) and their associated directives (`//gorn:fragment`, `//gorn:include`, `//gorn:prelude`).
- `//gorn:replace`.
- Build caching semantics and cache invalidation.

Treat any of the above as aspirational until this document (or a successor) is updated to say otherwise.
