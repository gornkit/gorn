# gorn

[![CI](https://github.com/gornkit/gorn/actions/workflows/ci.yml/badge.svg)](https://github.com/gornkit/gorn/actions/workflows/ci.yml)

**Go Right Now** — a runner for single-file Go automation scripts.

> Gorn lets you write single-file Go scripts with a small directive layer,
> generated modules, cached builds, and a portable `sh` helper package.

## Status

Pre-alpha, but the runner works end to end: `gorn run <script>` (or the
`gorn <script>` shorthand) parses the script, generates a module, builds it
with the Go toolchain, caches the binary keyed on a content hash, and runs it,
forwarding args. The `sh` package has a usable shell-exec v0. The separate
`gorn build` and `gorn cache` subcommands are still stubs.

## `sh` package

`github.com/gornkit/gorn/sh` runs shell snippets with normal Go IO and
status-only results:

```go
sh.Host().Exec("go test ./...").OrExit()

sh.Bash().
	Strict().
	Shell("go test ./...").
	Env("CI", "1").
	Exec().
	OrExit()
```

It is shell-first, not a portable shell replacement. Structured commands,
globbing, and file helpers are future work.

## Design

- [`docs/specs/gorn-script-format.md`](docs/specs/gorn-script-format.md) — the `.gorn` script format, as currently implemented (authoritative).
- [`docs/planning/v1-backlog.md`](docs/planning/v1-backlog.md) — current-state assessment and proposed v1 backlog (draft).
- [`docs/archive/GORN_DESIGN.md`](docs/archive/GORN_DESIGN.md) — overall design and non-goals (historical/reference).
- [`docs/archive/sh-package-design-v0.md`](docs/archive/sh-package-design-v0.md) — the `sh` helper package (historical/reference).
- [`docs/archive/IMPLEMENTATION_PLAN.md`](docs/archive/IMPLEMENTATION_PLAN.md) — build order (historical/reference).

## Development

Dev environment is pinned with [mise](https://mise.jdx.dev/):

```sh
mise trust && mise install
```

See [`AGENTS.md`](./AGENTS.md) for build/lint/test commands and conventions.

Current CLI:

```sh
gorn <script.gorn> [-- args...]        # shorthand for run
gorn run [flags] <script.gorn> [-- args...]
gorn build                             # stub (not implemented)
gorn cache                             # stub (not implemented)
gorn --version

# run flags:
#   --print-mod     print the generated go.mod, then exit (does not run)
#   --print-main    print the generated main.go, then exit (does not run)
#   --no-cache      build fresh and bypass the cache
#   -v, --verbose   diagnostics on stderr
```

Inspection parses and generates source without running the script, accessing the
build cache, or invoking Go. Output-write failures are reported as errors.

Use `--` before script arguments that look like gorn flags. Child exit codes and
stdout/stderr are preserved; usage errors go to stderr with status 2. Scripts run
from the invocation directory. Source supplied through `-` consumes stdin before
the program starts. Relative `GORN_CACHE` paths resolve from the invocation
directory; an unusable explicit cache path reports an error.

The build cache lives under `$GORN_CACHE` (falling back to the user cache dir,
then a temp dir).

## Writing scripts

Scripts declare `package main` and an explicit `func main()`. Helpers can appear
before or after main. Ordinary Go imports and declarations work as usual:

```go
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

An optional first-line shebang (`#!/usr/bin/env gorn`) is supported. Gorn
preserves your Go source formatting and literal values while generating the
module and any preamble additions. There is no `//gorn:main` divider.

When a script needs real I/O or common helpers, `//gorn:preamble` makes a
small, fixed set of packages ambient — `fmt`, `os`, `path/filepath`,
`strconv`, `strings`, `time`, and gorn's own `sh` — so you can use them
without writing the imports:

```go
//gorn:preamble
package main

func main() {
	fmt.Println("real output, on stdout")
}
```

Note: `print`/`println` are Go debug builtins — they write to **stderr** and
their format isn't guaranteed. Use `fmt.Println` (with an explicit import or the preamble) for output
a human or pipeline will read. See
[`docs/specs/gorn-script-format.md`](docs/specs/gorn-script-format.md) for the
full format, the preamble package list, and its rules.

For third-party imports, declare a concrete module minimum with
`//gorn:require <module-path> <version>`. Gorn resolves only the declared graph;
it never discovers an undeclared latest dependency. Direct imports need their
own covering declaration even when another dependency brings the module in.

The preamble and explicit `sh` imports use production helper source bundled with
the CLI, so stdlib/helper scripts build offline. To select a published helper,
declare `//gorn:require github.com/gornkit/gorn <concrete-version>`. Mixing a
transitive parent module with the bundled helper is rejected with that remedy.
Builds retain the effective module, checksums, and selected graph in the cache.
Other offline cold builds require populated Go caches; warm Gorn hits avoid
downloads. Dependencies requiring a higher Go minimum need an explicit
`//gorn:go` update.

Before cache lookup, Gorn identifies installed Go and freezes a host-only pure-Go
profile. GOFLAGS supports tags, trimpath, and linker `-s`, `-w`, and
`-X importpath.name=value`. Other flags, cgo/custom compilers, experiments, FIPS,
and source embed/local compilation inputs are rejected. Identity includes the
selected compiler, CLI generation, helper bytes, and supported settings; provenance
hashes private linker/resolver values. Warm runs still require installed Go for
profile discovery. Scripts keep ordinary runtime environment and arguments. See
the [build/runtime contract](docs/specs/gorn-build-runtime.md) for syntax,
alternatives, and limits.

## License

[Apache 2.0](LICENSE).
