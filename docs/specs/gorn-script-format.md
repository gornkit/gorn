---
title: Gorn Script Format Specification
status: authoritative
type: spec
updated: 2026-10-17
summary: Complete Go main files with optional shebangs, header metadata, source-preserving generation, and an opt-in preamble.
---

# Gorn Script Format Specification

This document and the implementation define the current format. Documents in
`docs/archive/` are frozen historical references.

## Source format

A `.gorn` script is a complete Go file declaring `package main` and exactly one
package-level `func main()` with a body, no parameters, no results, and no type
parameters. An empty main body is valid. Imports, helpers, types, variables,
constants, and methods follow ordinary Go rules. Helpers can precede or follow
main; executable statements belong inside functions.

```go
#!/usr/bin/env gorn
//gorn:go 1.26
package main

import "fmt"

func main() {
	greet("gorn")
}

func greet(name string) {
	fmt.Println("hello,", name)
}
```

There is no section divider and gorn does not generate a main wrapper.
`//gorn:main` is not a supported directive. Unknown directives receive the normal
invalid-directive error; there is no legacy format or migration mechanism.

### Shebang and whitespace

An optional first line beginning with `#!` is accepted without validating its
target. For Go parsing and generation, its non-line-ending bytes become spaces;
line endings and subsequent source offsets stay intact. A `#!` sequence elsewhere
is ordinary source and must satisfy Go's syntax rules. A leading UTF-8 BOM is
similarly replaced with spaces in generated Go.

Go source formatting is preserved, including comments, multiline literal values,
blank lines, CRLF, and an absent final newline. Generation does not run gofmt on
the user's source. A final separator newline may be added before generated
preamble declarations.

## Header directives

Metadata is a standalone line comment whose text begins exactly with `//gorn:`
and has only whitespace before it on its line. Directives must appear before the
`package` keyword. Ordinary comments and blank lines may appear between them.
Names are case-sensitive; arguments are separated by whitespace without quoting.

| Directive | Syntax | Cardinality | Behavior |
| --- | --- | --- | --- |
| Go version | `//gorn:go <version>` | At most once | Sets the generated module's Go version; defaults to `1.26` |
| Module | `//gorn:module <path>` | At most once | Sets the generated module path; defaults to `gorn.local/app` |
| Requirement | `//gorn:require <path> <version>` | One per module path | Adds a concrete module minimum under Go MVS; queries and duplicates are rejected |
| Preamble | `//gorn:preamble` | At most once | Supplies the fixed ambient imports below |

Go versions must satisfy Go module syntax and recognized Go release/beta/RC
version syntax. Main module paths follow Go import-path syntax, including local
names such as `task`. Requirement paths/versions use Go module validation,
including major-version path compatibility, canonical semantic versions,
prereleases, and valid pseudo-version timestamps and bases. Versions are retained as
supplied after validation. Queries, branch names, bare revisions, partial versions,
and noncanonical build metadata are rejected before invoking Go.

Require entries retain their order; repeating a module path is an error even
when the versions match. Errors identify the repeated line and the first
declaration line. A concrete requirement is a minimum under Go MVS, not an upper
bound. Existence, checksum validation, graph resolution, and helper delivery are
build responsibilities; syntax validation requires no Go executable,
network, or writable cache.

Standalone metadata comments after the package clause are errors, including
comments inside functions. A trailing comment after Go code is ordinary Go, not
metadata. Text inside raw strings or block comments is never a directive:

```go
package main

func main() {
	text := `this is literal text
//gorn:go 1.99
//gorn:main
`
	println(text)
}
```

## Preamble

`//gorn:preamble` supplies these packages without explicit imports:

| Import path | Identifier |
| --- | --- |
| `fmt` | `fmt` |
| `os` | `os` |
| `path/filepath` | `filepath` |
| `strconv` | `strconv` |
| `strings` | `strings` |
| `time` | `time` |
| `github.com/gornkit/gorn/sh` | `sh` |

Do not also import a preamble package. Conflicts are detected by import path,
including aliased, blank, and dot imports, and reported at the original import
line. Other packages must still be imported normally. Local identifiers shadow
ambient imports according to ordinary Go scoping rules.

```go
//gorn:preamble
package main

import "bytes"

func main() {
	var output bytes.Buffer
	output.WriteString(strings.ToUpper("hello, gorn"))
	fmt.Println(output.String())
}
```

Generation inserts the preamble import block after the complete package clause
in the same generated file. Original import/declaration comments stay attached.
Keep-alive declarations are appended after the original source so unused
ambient packages are accepted and all imports precede declarations. There is no
rewriting of package references and no source-wide indentation or formatting.

The preamble's `sh` import defaults to the CLI's bundled production helper source.
Explicit `sh` imports use that same bundle. A concrete parent-module requirement
selects the published helper instead. The CLI embeds source as data without
importing `sh`; `sh` remains a stdlib-only library.

## Dependencies

Declare direct module requirements in the header and import their packages
using normal Go syntax:

```go
//gorn:require github.com/alexflint/go-arg v1.6.1
package main

import (
	"fmt"

	"github.com/alexflint/go-arg"
)

func main() {
	var opts struct {
		Name string `arg:"positional"`
	}
	arg.MustParse(&opts)
	fmt.Println("hello,", opts.Name)
}
```

Every direct nonstandard import needs a covering requirement (the module path
itself or a slash-delimited descendant), except bundled `github.com/gornkit/gorn/sh`.
The selected Go toolchain classifies standard packages at build time. An
incidental transitive requirement does not authorize a direct import. Missing
declarations identify the original import line and `//gorn:require` remedy.
Declaring a requirement does not make its package ambient.

Bundled helper mode generates `require github.com/gornkit/gorn/sh v0.0.0` and
`replace github.com/gornkit/gorn/sh => ./gorn-runtime`. Builds materialize exact
production source and its license in that private child module; source imports
are unchanged. `//gorn:require github.com/gornkit/gorn <concrete-version>` opts
into the published parent module with no bundle/replacement. Explicit child-module
requirements are reserved and rejected. A transitive parent module in bundled
mode fails with a remedy to select the parent explicitly.

Builds download and expand only the declared graph to a fixed point, using Go's
MVS and checksum validation, then run `go build -mod=readonly -buildvcs=false
-pgo=off`. They never run `tidy`, `get`, or undeclared latest-version discovery.
A higher dependency Go minimum fails with a remedy to raise `//gorn:go`; the
source minimum cannot silently change during resolution. Installed local Go and
`GOWORK=off` keep workspace/toolchain discovery outside the generated module.
Before lookup, Gorn freezes a host-only pure-Go profile. Supported GOFLAGS are
`-tags=<list>`, `-trimpath[=<bool>]`, and quoted `-ldflags` limited to `-s`, `-w`,
and `-X importpath.name=value`. Unsupported inputs fail on warm runs too. Real
source `//go:embed` and `import "C"` are unsupported, with original source lines;
inspection remains independent of this build check.

Completed builds retain effective `go.mod`, `go.sum`, and `dependencies.json`
(schema 1: original `declared` requirements, `selected` path/version/Go-minimum
records, and optional `helper_digest`). Bundled file names and exact bytes feed
the helper digest and cache identity; generation version 4 invalidates older
policy output. No companion lock file is required. Warm Gorn hits do not
redownload modules; cold offline builds need populated Go caches except for
stdlib/bundled-helper programs. Concrete requirements are minimums under MVS;
reproducibility still depends on the selected toolchain and Go's trusted caches
and module sources. See the [build/runtime contract](gorn-build-runtime.md) for
alternatives, rationale, the supported profile, and process ownership.

`//gorn:replace`, fragments/includes, and remote source inputs are not part of
the format. Private-module settings continue through Go's resolver; credential
userinfo and URL query values are redacted from returned Go diagnostics.

## Diagnostics and generation interfaces

`script.Parse` accepts an `app.Source` and returns a `script.File` containing
metadata and the complete normalized `Content`. Its private syntax and position
information belong to that content; callers must not modify `Content` after
parsing. Go syntax errors retain their underlying `scanner.ErrorList` with source
filename, line, and column. Other Go semantic errors remain compiler errors.

Gorn format errors use `*script.LineError`, preserving `errors.Is` and
`errors.As` against these sentinels:

| Error | Meaning |
| --- | --- |
| `ErrInvalidPackage` | Package name is not `main` |
| `ErrMissingMain` | No package-level main function exists; a method named main does not count |
| `ErrMultipleMain` | More than one package-level main function |
| `ErrInvalidMain` | Main lacks a body or has parameters, results, or type parameters |
| `ErrDirectiveAfterPackage` | Standalone metadata occurs after the package keyword |
| `ErrInvalidDirective` | Unknown directive or invalid argument count |
| `ErrInvalidImport` | Invalid Go import path, including relative/local paths |
| `ErrReservedHelperModule` | Explicit child-module requirement; pin the published parent instead |
| `ErrInvalidRequire` | Missing path/version pair or invalid/noncanonical concrete module version |
| `ErrInvalidGoVersion` | Invalid module Go version or unrecognized Go version syntax |
| `ErrInvalidModule` | Invalid main module path |
| `ErrDuplicateRequire` | Repeated requirement module path; reports the first declaration line |
| `ErrDuplicateGo` | Repeated Go directive |
| `ErrDuplicateModule` | Repeated module directive |
| `ErrDuplicatePreamble` | Repeated preamble directive |
| `ErrPreambleImportConflict` | Explicit import duplicates a preamble package; raised during generation |

Source construction rejects empty paths and empty scripts before parsing.
Metadata and format errors use physical source line numbers, even if the source
contains Go `//line` directives.

`script.Generate` returns `MainGenerated` and `ModGenerated`. It preserves user
formatting and adds a generated-file comment and `//line` anchors. File-backed
anchors use absolute original paths; resumed source segments restore original
line and column positions around preamble insertions. Go may shorten filenames
when presenting diagnostics. Explicit Go source-location directives retain their
normal compiler behavior.

Generation errors use `*script.GenerateError`. Preamble conflicts wrap a
`LineError`; `Raw` is reserved for generated source that fails internal syntax
validation. Invalid user Go is rejected during parsing, without a generated-source
dump. Cache-key generation version `4` and manifest schema `3` invalidate earlier
format/dependency/profile output and record actual build provenance.

## Runner status and tooling

`gorn run <script>` and `gorn <script>` parse, generate, build, cache, and execute
a script with forwarded arguments. stdin source uses `-`. `--no-cache` builds
fresh without publishing the result. `--print-main` and `--print-mod` expose the
generated artifacts; both together add artifact headers. Inspection validates the
source and generates the requested artifacts without opening the build cache,
invoking Go, or executing the script. It works independently of cache contents
and writability; failed or short output writes return errors.

Ordinary child exit codes are preserved; POSIX child signals use `128 + signal`.
Usage errors exit 2 with usage on stderr; source/build/start failures exit 1.
Valid invocations do not print usage, and a child failure does not add a runner
error line. Flags before `--` belong to gorn; use the separator to forward
flag-like arguments, empty strings, and literal separators.

Source identity uses absolute, cleaned paths without resolving symlinks. stdin
source uses a distinct identity and synthetic `<invocation cwd>/stdin.gorn`
diagnostic path; it consumes stdin before runtime begins. Runtime cwd stays the
invocation cwd. Relative `GORN_CACHE` values resolve there, and unusable explicit
overrides report an error rather than selecting another cache. The
[execution/source identity record](../planning/v1-execution-plan.md) explains
these decisions and their trade-offs. The [build/runtime contract](gorn-build-runtime.md)
defines implemented cancellation, profile identity, ownership and repair.
`gorn build` exports a standalone host binary without executing the source;
`gorn cache` supports path/list/clean/prune with explicit mutation and activity
protection. See the [workflow guide](gorn-workflows.md). Check, format, and eject
commands remain optional backlog items.

A complete Go body improves compatibility with Go syntax tooling, but `.gorn`
may still require an editor language association. Shebangs need preprocessing
for direct Go formatting, and preamble scripts require the generated imports
for complete name resolution. This format does not add a language server,
formatter command, or automatic extension change.
