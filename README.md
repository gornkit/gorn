# gorn

**Go Right Now** — a runner for single-file Go automation scripts.

> Gorn lets you write single-file Go scripts with a small directive layer,
> generated modules, cached builds, and a portable `sh` helper package.

## Status

Pre-alpha. The `sh` package has a usable shell-exec v0; the runner is not
usable yet.

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

- [`project/GORN_DESIGN.md`](project/GORN_DESIGN.md) — overall design and non-goals.
- [`project/sh-package-design-v0.md`](project/sh-package-design-v0.md) — the `sh` helper package.
- [`project/IMPLEMENTATION_PLAN.md`](project/IMPLEMENTATION_PLAN.md) — build order.

## Development

Dev environment is pinned with [mise](https://mise.jdx.dev/):

```sh
mise trust && mise install
```

See [`AGENTS.md`](./AGENTS.md) for build/lint/test commands and conventions.

Current CLI skeleton:

```sh
gorn <script.go> [-- args...]      # shorthand for run
gorn run <script.go> [-- args...]
gorn build
gorn cache
gorn --version
```

## License

[Apache 2.0](LICENSE).
