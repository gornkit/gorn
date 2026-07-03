# gorn

Go module `github.com/gornkit/gorn`. Tagline: "Go file apps you can run right now."

## Layout

- Root `package main` → `go install github.com/gornkit/gorn` installs the CLI as `gorn`.
- `sh/` → library package, imported as `github.com/gornkit/gorn/sh`. Current v0 is shell-exec only: `Host()/Bash()/Zsh()/Sh()/CmdExe()/Pwsh()`, `Setup()/Strict()/Shell()/Exec()`, command IO/env/dir, process `Wait()/Kill()`, and status-only `Result`.
- The root is not importable as a library; consumers import subpackages.
- `project/` holds historical/reference design docs. They are useful context, but the implementation and root docs are authoritative when they diverge.

## Toolchain

- Everything pinned via [mise](https://mise.jdx.dev/) in `.config/mise.toml` (lockfile on): Go, `gopls`, `golangci-lint`.
- Contributor bootstrap: `mise trust && mise install`. Do not rely on system Go.
- Copilot CLI LSP: `.github/lsp.json` launches gopls via `mise exec -- gopls` so the mise-pinned version is used. Don't hardcode an absolute path.
- Zed workspace LSP config in `.zed/settings.json` invokes `golangci-lint` through `mise exec` for the same reason; keep it committed. Because the lint config lives under `.config/`, keep `run.relative-path-mode: gomod` in `.config/golangci.yaml` so Zed diagnostics match Go file paths.

## Commands

- Build all packages: `go build ./...`
- Build stamped CLI: `mise run build` (override with `GORN_VERSION=...`)
- Test all: `go test ./...` or `mise run test` for verbose uncached tests
- Single test: `go test ./path/to/pkg -run TestName`
- Vet: `go vet ./...`
- Lint: `mise run lint:run` (config: `.config/golangci.yaml`, not auto-detected without `--config`)
- Format: `mise run lint:fmt` (formatters only) or `mise run lint:fix` (format + lint --fix)

Mise tasks are inline in `.config/mise.toml`. If you add more, either keep them inline or move to `.config/mise/tasks/` — don't mix.

## Conventions

- Module path is `github.com/gornkit/gorn`; use it for internal imports.
- CLI parsing uses `github.com/alexflint/go-arg`; otherwise prefer stdlib. Adding another dep needs justification.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `chore:`, `docs:`, `refactor:`, `test:`, etc.). Prefer a single commit per logical change over splitting scaffolding into many small ones.
- User style (`@trippwill`): buildable commits, small reviewable slices, no
  half-baked feature branches pushed. Pushing requires explicit sign-off.
