# gorn

Go module `github.com/gornkit/gorn`. Tagline: "Go file apps you can run right now."

## Layout

- Root `package main` → `go install github.com/gornkit/gorn` installs the CLI as `gorn`.
- `sh/` → library package, imported as `github.com/gornkit/gorn/sh`.
- The root is not importable as a library; consumers import subpackages.
- `project/` holds the design docs. Read these before touching architecture:
  - `project/GORN_DESIGN.md` — overall design.
  - `project/SH_PACKAGE_DESIGN.md` — `sh` package design.
  - `project/IMPLEMENTATION_PLAN.md` — build order / status.

## Toolchain

- Everything pinned via [mise](https://mise.jdx.dev/) in `.config/mise.toml` (lockfile on): Go, `gopls`, `golangci-lint`.
- Contributor bootstrap: `mise trust && mise install`. Do not rely on system Go.
- Copilot CLI LSP: `.github/lsp.json` launches gopls via `mise exec -- gopls` so the mise-pinned version is used. Don't hardcode an absolute path.
- Zed workspace LSP config in `.zed/settings.json` invokes `golangci-lint` through `mise exec` for the same reason; keep it committed.

## Commands

- Build: `go build ./...`
- Test all: `go test ./...`
- Single test: `go test ./path/to/pkg -run TestName`
- Vet: `go vet ./...`
- Lint: `mise run lint` (config: `.config/golangci.yaml`, not auto-detected without `--config`)
- Format: `mise run fmt` (formatters only) or `mise run fix` (format + lint --fix)

Mise tasks are inline in `.config/mise.toml`. If you add more, either keep them inline or move to `.config/mise/tasks/` — don't mix.

## Conventions

- Module path is `github.com/gornkit/gorn`; use it for internal imports.
- No dependencies yet — prefer stdlib. Adding a dep needs justification.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `chore:`, `docs:`, `refactor:`, `test:`, etc.). Prefer a single commit per logical change over splitting scaffolding into many small ones.
- User style (`@trippwill`): buildable commits, small reviewable slices, no
  half-baked feature branches pushed. Pushing requires explicit sign-off.
