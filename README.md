# gorn

**Go Right Now** — a runner for single-file Go automation scripts.

> Gorn lets you write single-file Go scripts with a small directive layer,
> generated modules, cached builds, and a portable `sh` helper package.

## Status

Pre-alpha. The design is written; the runner is not. Nothing here builds
into a usable tool yet. If you're looking for something to run, come back
later.

## Design

- [`project/GORN_DESIGN.md`](project/GORN_DESIGN.md) — overall design and non-goals.
- [`project/SH_PACKAGE_DESIGN.md`](project/SH_PACKAGE_DESIGN.md) — the `sh` helper package.
- [`project/IMPLEMENTATION_PLAN.md`](project/IMPLEMENTATION_PLAN.md) — build order.

## Development

Dev environment is pinned with [mise](https://mise.jdx.dev/):

```sh
mise trust && mise install
```

See [`AGENTS.md`](AGENTS.md) for build/lint/test commands and conventions.

## License

[Apache 2.0](LICENSE).
