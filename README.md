# gorn

[![CI](https://github.com/gornkit/gorn/actions/workflows/ci.yml/badge.svg)](https://github.com/gornkit/gorn/actions/workflows/ci.yml)

**Go file apps you can run right now.** Write one Go main file, run it with
arguments and normal IO, then rerun its cached executable or export a binary.

Gorn is preparing for v1. The current development build supports `run`, `build`
and explicit cache management. Scripts use `package main` and `func main()`;
`.gorn` remains the filename convention.

## Install

Install Go **1.26.4 or newer**, with `go` on PATH, then install the development
build:

```sh
go install github.com/gornkit/gorn@main
gorn --version
```

Put Go's binary directory on PATH: `go env GOBIN` if set, otherwise the `bin`
directory under `go env GOPATH`. These Go commands work in Unix shells and
PowerShell. `@main` follows development and can change; an approved versioned
release will provide a fixed install target. Source installs report Go's selected
module version; explicit release stamps take precedence. Downloading Go modules
normally needs network access on first install. The [release contract](docs/specs/gorn-release.md)
defines binary targets, checksum verification and candidate promotion.

The runner needs installed Go on every run, including warm hits, to validate its
build profile. An exported executable needs neither Go nor Gorn, but still needs
any commands/data its script uses. Builds target the current host with cgo
disabled. Scripts execute with your permissions.

## Your first script

Save this as `hello.gorn` ([maintained example](examples/hello.gorn)):

```go
package main

import (
    "fmt"
    "os"
)

func main() {
    name := "world"
    if len(os.Args) > 1 {
        name = os.Args[1]
    }
    fmt.Println("hello,", name)
}
```

```sh
gorn run hello.gorn -- Ada
gorn hello.gorn -- Ada
gorn build hello.gorn -o ./hello
```

Both runs print `hello, Ada`. The second can use the cached build. Run `./hello`
on Unix or `.\hello.exe` in PowerShell. Windows adds `.exe` to an extensionless
output name; explicit extensions stay as written. The output parent must exist;
replace an existing regular file with `--force`.

On Unix, optionally add `#!/usr/bin/env gorn` as the first line, run
`chmod +x hello.gorn`, then `./hello.gorn Ada`. Windows uses explicit
`gorn run .\hello.gorn -- Ada`; a shebang is not a Windows executable association.
Go tooling may need an editor language association for `.gorn`.

## Run, inspect and manage builds

```sh
gorn run hello.gorn -- --script-flag "two words"
gorn run --no-cache hello.gorn
gorn run --print-main hello.gorn
gorn run --print-mod hello.gorn
gorn cache path
gorn cache list --json
gorn cache clean <key-prefix> --dry-run
gorn cache clean <key-prefix> --yes
gorn cache prune --max-age 168h --max-bytes 100000000 --yes
gorn --help
gorn cache --help
```

Use `--` before script arguments that resemble Gorn flags. Runtime cwd stays the
invocation directory, and stdin/stdout/stderr are inherited. Source `-` consumes
stdin before the script starts. Child exit codes are preserved; usage errors
return 2 and source/build/start errors return 1.

Inspection preserves source text and never invokes Go, opens the build cache or
executes the script. Cache inspection also needs no Go. Mutation requires
`--yes`; preview writes no cache files and does not reserve activity. Cleanup
skips active work and preserves unknown data. Prune uses build age and logical
entry bytes; incomplete counts or unmet byte budgets return status 1. Go's module
and build caches are separate and untouched. `GORN_CACHE` selects another root.

Gorn reports phases and two-second heartbeats on stderr during preparation.
`--quiet` suppresses activity; diagnostics and script stderr remain visible.
`--verbose` adds diagnostics. Progress stops when the script starts so its normal
terminal IO stays in control. See the [workflow guide](docs/specs/gorn-workflows.md)
for offline behavior, dependencies, cancellation and troubleshooting.

## Optional helpers and dependencies

[The helper example](examples/helper.gorn) opts into a small fixed preamble:

```go
//gorn:preamble
package main

func main() {
    sh.Cmd("go", "version").Exec().OrExit()
}
```

The CLI bundles `github.com/gornkit/gorn/sh`, so helper source needs no separate
module download. `sh.Cmd` passes executable arguments literally; `.Shell(script)`
uses a chosen shell's syntax. Shells use non-login automation defaults.
[The helper contract](docs/specs/gorn-sh.md) covers supported shells, setup,
Strict, environment/IO, Wait/Kill and status-only results.

For third-party packages, declare a concrete module minimum before `package`,
then use normal Go imports:

```go
//gorn:require github.com/alexflint/go-arg v1.6.1
package main

import (
    "fmt"
    "github.com/alexflint/go-arg"
)

func main() {
    var opts struct { Name string `arg:"positional,required"` }
    arg.MustParse(&opts)
    fmt.Println("hello,", opts.Name)
}
```

Save it as `greet.gorn` and run `gorn greet.gorn -- Ada`. Gorn resolves the
declared graph using Go MVS and checksums. It never selects undeclared latest
dependencies. Direct imports need covering declarations even when transitively
available. Warm hits download no dependencies; cold dependency builds need network
or populated Go caches. Stdlib and bundled-helper source build offline when Go is
installed. To select a published helper, pin the parent module explicitly as
[documented in the format](docs/specs/gorn-script-format.md#dependencies).

## Contracts and development

- [Releases and upgrades](docs/specs/gorn-release.md): archives, checksums and verified installation.

- [Startup measurements](docs/specs/gorn-startup.md): reference distributions, budget and reproduction.
- [Compatibility](docs/specs/gorn-compatibility.md): supported public surfaces and upgrades.
- [Script format](docs/specs/gorn-script-format.md): metadata, preamble, preserved source and diagnostics.
- [Build/runtime contract](docs/specs/gorn-build-runtime.md): toolchain/profile inputs, dependencies, process ownership, cache and export.
- [Workflow guide](docs/specs/gorn-workflows.md): install, run, build, cache and troubleshooting.
- [Helper contract](docs/specs/gorn-sh.md): exact arguments and named shells.
- [v1 roadmap](docs/planning/v1-backlog.md): delivered scope and release gates.

Contributors need Git and [mise](https://mise.jdx.dev/):

```sh
mise trust && mise install
mise run test
```

The mise lock pins Go, gopls and lint. `mise run test` runs lint, stamped build and
uncached tests. See [AGENTS.md](AGENTS.md) for repository conventions. Archived
designs remain historical; authoritative specs and implementation describe the
current product.

[Apache 2.0](LICENSE).
