---
title: Gorn User Workflows
status: authoritative
type: spec
updated: 2026-10-17
summary: Tested installation and onboarding, run/build/cache workflows, dependency and offline behavior, and troubleshooting.
---

# User workflows

## Install and first run

The runner's supported Go floor is 1.26.4. Install Go and put both `go` and its
binary output directory on PATH. The output directory is `go env GOBIN`, or
`<go env GOPATH>/bin` when GOBIN is empty. Until a versioned release is approved,
`go install github.com/gornkit/gorn@main` selects the changing development build.
Use `gorn --version` and `gorn --help` to verify the installed command. Source
installs currently may display `gorn dev`; the release workflow will define
version-stamped distribution.

Use the [README first script](../../README.md#your-first-script) or the maintained
[hello example](../../examples/hello.gorn). It is a complete Go main file with
explicit imports. `gorn run hello.gorn -- Ada` and `gorn hello.gorn -- Ada` both
print `hello, Ada`. Go remains necessary for warm runs because installed
build-profile validation precedes cache lookup. The stdlib example needs no module
network access after Gorn/Go installation.

Unix executable scripts may start with `#!/usr/bin/env gorn` and require
`chmod +x hello.gorn`; `gorn` must be on PATH. Use explicit
`gorn run .\hello.gorn -- Ada` on Windows. Shebang text is normalized before Go
parsing and does not alter later source positions. Gorn does not associate file
extensions or install editor integrations.

## Runtime, inspection and export

Scripts run as the invoking user with ordinary environment and permissions, in
the invocation directory. Relative file/command paths follow that directory,
not the source file's directory. Arguments after `--` reach the script literally,
including flag-like strings and empty arguments. Script stdout/stderr and stdin
remain the original streams. Source `-` reads stdin to EOF before runtime, so
runtime stdin is then empty. Child exit status is preserved; usage is status 2,
and source/build/start failures are status 1.

`--print-main` and `--print-mod` validate and print generated artifacts without
running Go or scripts, writing cache data, or resolving dependencies. Both flags
together add artifact headers. They work with missing Go or an unusable cache.
Printing a module plan is not a compile check.

`gorn build hello.gorn -o ./hello` compiles without executing either `init` or
`main`. Run the binary directly without Go/Gorn. It still needs its own external
commands/files, and it targets the current OS/architecture with cgo disabled.
Windows adds `.exe` only when no extension was supplied. Parent directories must
exist. Existing regular outputs need `--force`; symlinks, directories and source
aliases are refused. Completed output is published atomically on supported local
filesystems; default no-overwrite publication requires hard links. See
[export semantics](gorn-build-runtime.md#standalone-binary-export).

Preparation reports phases immediately and a heartbeat every two seconds on
stderr. Quiet suppresses activity, not diagnostics or script stderr. Verbose adds
explicit diagnostics and can be combined with quiet. After startup the script
owns runtime progress. Ctrl-C/termination cancel preparation; POSIX invocation
groups receive bounded cleanup, while Windows tree cleanup is best effort.
Foreground IO/Ctrl-C is supported; full suspend/resume job control and deliberately
escaped descendants are outside the contract. See
[process ownership](gorn-build-runtime.md#cancellation-and-process-ownership).

## Dependencies and offline behavior

The [helper example](../../examples/helper.gorn) uses the fixed preamble and
`sh.Cmd("go", "version")`. Helper source is bundled; this example still needs the
external `go` command at runtime by its own choice. The helper can also be imported
explicitly. The preamble is opt-in and supplies exactly the names in the
[format spec](gorn-script-format.md#preamble).

Declare each direct third-party import's covering module with
`//gorn:require <module> <concrete-version>`. The README's go-arg example is compiled
from its documented source in CI. Go MVS may raise a selected dependency above its
stated minimum. Gorn uses declared graph expansion, Go checksums and read-only
builds; it does not run get/tidy/latest discovery. Go's normal proxy/private-module
configuration controls network access. Do not put credentials in source metadata;
returned diagnostics redact URL authentication/query values.

| Workflow | Network/cache expectation |
| --- | --- |
| Print inspection or cache commands | No Go invocation or module resolution |
| Cold stdlib/bundled-helper build | Installed Go; no external module download |
| Cold dependency build | Reachable configured module sources or populated Go module cache |
| Warm Gorn cache hit | Installed Go/profile validation, no module downloads |
| `--no-cache` | Fresh private Gorn build; still reuses Go's module/build caches |
| Exported binary | No Go/Gorn; program-specific runtime resources still apply |

The selected Go must satisfy the script's Go minimum. Gorn sets GOTOOLCHAIN=local,
GOWORK=off and a host-only pure-Go build profile. Raise `//gorn:go` explicitly if a
dependency requires a higher minimum. Local includes, source `//go:embed`, cgo,
arbitrary GOFLAGS/custom compilers, cross-compilation and remote scripts are
outside v1. Supported flags and identity are specified in the
[build profile](gorn-build-runtime.md). The exported artifact is the portability
boundary for machines without Go.

## Cache maintenance

`GORN_CACHE` selects a root (relative values use invocation cwd); otherwise use the
OS user-cache directory, or temp only when the OS cannot determine that directory.
Creation/permission errors fail instead of selecting a different root.

`cache path` reports it without creating it. `cache list --json` emits schema-1
inventory, separating ownership from validity and complete/incomplete logical
sizes for entries, stages, quarantine, unmanaged data and metadata. Unknown data
and older layouts remain preserved. Go caches are separate and untouched.

Preview one unambiguous lowercase hex key with
`cache clean <key-prefix> --dry-run`, then authorize using `--yes`. `clean --all`
also collects abandoned owned stages/quarantine, while skipping locked objects.
Prune accepts nonnegative `--max-age` durations and/or integer `--max-bytes`;
it removes age-expired entries first and then considers remaining oldest entries.
`--max-bytes 0` requests zero managed entry bytes. Previews do not reserve future
activity. Mutation skips active/changed data, reports partial failures, and fails
status 1 if a requested byte budget remains unmet or cannot be measured.
[The maintenance contract](gorn-build-runtime.md#cache-inspection-and-maintenance)
records bounds, JSON, ownership, concurrency and Windows sharing behavior.

## Troubleshooting

| Symptom | Check/remedy |
| --- | --- |
| `gorn` not found | Add GOBIN or GOPATH/bin to PATH; verify the installed filename on Windows |
| Go missing on a warm run | Install supported Go; use an exported binary on machines without Go |
| Missing package or undeclared import | Add the concrete covering requirement; a transitive import is insufficient |
| Dependency Go minimum failure | Raise `//gorn:go` explicitly and install a compatible local toolchain |
| Proxy/auth/checksum failure | Check normal Go resolver/private-module settings and connectivity; do not disable checksum verification as a workaround |
| Unsupported build input | Compare GOFLAGS/cgo/compiler settings to the profile; inspection still works |
| Cache permission failure | Repair the selected root permissions or deliberately choose GORN_CACHE |
| Corrupt cached program | Rerun for owned-entry repair; inspect validity/provenance with cache list |
| Clean/prune skipped an active object | Let its build/run/export finish, then retry; no process is interrupted |
| Unmet/incomplete byte budget | Read action errors and incomplete totals; inspect permissions/active work; unknown data stays preserved |
| Windows file sharing failure | Close users of the executable/directory, then retry cleanup; owned residue remains diagnosable |
| Shebang fails | Confirm Unix executable permission, line endings and Gorn PATH; use explicit invocation on Windows |
| Shell startup or hidden native failure | Read the helper's startup/Strict limits; prefer exact Cmd for external tools |
| Export path failure | Create its parent, choose an unused output or explicit --force, and use a filesystem supporting required publication operations |

## Product choices and verification

A stdlib-first example teaches ordinary Go and works offline without dependency
setup. The preamble remains an optional convenience. Installing `@latest` before
an approved release would imply a fixed supported version that does not exist;
`@main` describes development honestly, with versioned install deferred to release
delivery. Export serves hosts without Go rather than weakening warm identity
validation. Network effects stay explicit; dependency tests use a controlled
loopback proxy, while release smoke tests separately verify published modules.

[Installed workflow tests](../../workflow_integration_test.go) execute maintained
examples and onboarding commands. [Dependency tests](../../dependency_workflow_test.go)
exercise real CLI cold/warm/bypass/repair/export and concurrent publication with
isolated module caches, refusing proxy traffic after population. Existing
[execution](../../execution_integration_test.go), [cache](../../cache_integration_test.go),
[export](../../build_integration_test.go) and platform process tests cover the
remaining contracts. Ordinary-Go compiler-oracle fixtures initialize private valid
Git roots; this isolates invalid ancestor .git markers without disabling VCS
stamping or requiring a developer's global GOFLAGS/TMPDIR override. Git is a
contributor test prerequisite, not an installed runner requirement.
