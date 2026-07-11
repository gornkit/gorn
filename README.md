# gorn

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
- [`project/GORN_DESIGN.md`](project/GORN_DESIGN.md) — overall design and non-goals (historical/reference).
- [`project/sh-package-design-v0.md`](project/sh-package-design-v0.md) — the `sh` helper package (historical/reference).
- [`project/IMPLEMENTATION_PLAN.md`](project/IMPLEMENTATION_PLAN.md) — build order (historical/reference).

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

The build cache lives under `$GORN_CACHE` (falling back to the user cache dir,
then a temp dir).

## Writing scripts

The smallest scripts need no imports at all — Go's builtins (`len`, `append`,
`make`, `min`/`max`, `clear`, `range`-over-int, and the debug-only
`print`/`println`) are always available:

```go
//gorn:main
for i := range 3 {
	println("tick", i) // debug builtin: writes to stderr
}
```

When a script needs real I/O or common helpers, `//gorn:preamble` makes a
small, fixed set of packages ambient — `fmt`, `os`, `path/filepath`,
`strconv`, `strings`, `time`, and gorn's own `sh` — so you can use them
without writing the imports:

```go
//gorn:preamble
//gorn:main
fmt.Println("real output, on stdout")
```

Note: `print`/`println` are Go debug builtins — they write to **stderr** and
their format isn't guaranteed. Use `fmt.Println` (via the preamble) for output
a human or pipeline will read. See
[`docs/specs/gorn-script-format.md`](docs/specs/gorn-script-format.md) for the
full format, the preamble package list, and its rules.

## License

[Apache 2.0](LICENSE).
