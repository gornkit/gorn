---
title: Gorn v1 Product Assessment and Backlog
status: draft
type: plan
updated: 2026-10-17
summary: Evidence-based assessment, proposed v1 scope, prioritized backlog, and release gates for reliable single-file Go automation.
---

# Gorn v1 product assessment and backlog

## Recommendation

Gorn has a working foundation for single-file Go automation. It is pre-alpha,
not yet ready for a stable v1 promise. The next investment should make running
an ordinary Go script predictable: preserve its meaning, resolve its dependencies
consistently, reuse the correct binary, and behave correctly in terminals and CI.

Ship a focused v1 around **run, build, cache management, and a small stable `sh`
API**. Add exact-argument process execution to that API. Keep the file format
small and use ordinary Go modules and standard library IO. Fragments, a pipeline
DSL, a filesystem utility suite, and a full language server can wait.

GORN-V1-01–04, GORN-V1-09, and GORN-V1-25 are implemented, with linked
verification evidence. Other items remain proposed
unless their status records implementation. The backlog defines scope and
acceptance criteria; it does not authorize implementation or publication.

See [next steps and delivery workflow](v1-next-steps.md) for the recommended
implementation sequence, Plan/Goal handoff, and worktree/PR process.

## Product goal and assumptions

The primary user already knows some Go and wants to keep a small automation
program in one readable file without maintaining a separate module directory.
Initial jobs are repository maintenance, local developer tasks, and CI utilities.
The same script should be runnable locally and buildable into a standalone binary.

The v1 promise should be:

> Write ordinary Go in one file, declare its dependencies, and run it with
> predictable arguments, output, failures, and cached builds.

These are planning assumptions, not findings from user research:

- Local files and stdin are sufficient source inputs for v1. Download-and-run
  URLs, a script registry, and hosted execution are outside this release.
- Go is required to build a script. A binary produced by `gorn build` runs
  without gorn or Go, subject to its own OS, executable, and cgo dependencies.
- Linux, macOS, and Windows are intended supported hosts. The release target
  architectures and required shell combinations must be explicit before beta.
- Scripts execute with the invoking user's permissions. Gorn is an execution
  tool; sandboxing is outside the v1 contract.
- The initial build unit is the script plus declared Go module dependencies.
  Adjacent files for `//go:embed`, cgo includes, or local compilation are outside
  that single-file boundary unless explicitly supported later. Runtime file IO
  remains normal Go behavior; runtime assets are the script author's concern.
- Shell snippets retain the selected shell's behavior. Exact argv execution
  reduces dependence on shell syntax; it does not make host tools universal.

Validate these assumptions with a small beta cohort before expanding scope:
observe a first install, a first dependency-bearing script, a CI invocation,
and a cold versus warm rerun. Treat recurring failures as evidence for priority
changes, rather than adding features from the historical designs by default.

## Source format decision: ordinary Go entry point

**Implemented in GORN-V1-25:** scripts require a source-authored `package main`
and `func main()`. Module/version/require directives remain header comments.
The [format specification](../specs/gorn-script-format.md) defines the contract;
the [implementation record](v1-standard-main-plan.md) captures scope and verification.

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

The main product benefit is a familiar source model: imports and declarations
follow Go rules, helpers can precede or follow `main`, and gorn no longer needs
to split a file into declarations and executable statements. The previous format
allowed helpers only before its divider; the standard format removes that
ordering constraint.

| Choice | Assessment |
| --- | --- |
| Keep `//gorn:main` | Smallest tiny-script form, but retains a custom split, wrapper generation, and source semantics/mapping work |
| Require `func main()` and inject `package main` | Removes the divider, but the original source still lacks Go's required package clause |
| Require `package main` and `func main()` | Selected: a little more source text buys a complete Go file model and simpler generation |

Go's [source-file grammar](https://go.dev/ref/spec#Source_file_organization)
requires a package clause; its [program entry-point contract](https://go.dev/ref/spec#Program_execution)
requires `main` in package `main`, with no arguments or results. Gorn uses Go's
parser for format validation and leaves remaining semantic checks to the
compiler. Empty `func main() {}` is valid.

The expected tooling improvement has limits:

- `.gorn` still needs a Go language association in editors that do not recognize
  it. A standard main alone does not establish out-of-the-box gopls support.
  `.go` input/discovery is a separate decision; do not rename every script as
  part of this change without considering normal Go package discovery.
- A shebang is still outside Go syntax. Gorn replaces its first-line characters
  with whitespace before Go parsing, retaining offsets and source numbering.
  Direct Go formatting of a shebang-bearing file still needs preprocessing.
- The optional preamble is retained, with explicit imports as the preferred
  examples. Ambient imports still require generated source for full
  name resolution. Removing the preamble is a separate decision.
- Preamble imports are injected into the same generated file after the package
  clause; a companion file cannot supply imported package names to the script.
  Original source mappings resume after inserted code, and all imports precede
  generated keep-alive declarations. Imports have
  [file scope](https://go.dev/ref/spec#Declarations_and_scope).

Gorn's value remains single-file dependency declarations, generated module
management, cached execution, and binary export. It need not replace Go's entry
point syntax to provide those benefits.

The pre-alpha format break replaces the legacy grammar without migration
tooling or dedicated legacy diagnostics. Generation/cache identity advances to
version 2 so old cached binaries cannot bypass the new contract.

The format change is tracked as **25**, with the relevant 02–03 fixes delivered
by removing wrapping/indentation and using Go comment tokens for header metadata.
Source-equivalence and diagnostic regressions cover the new format. Maintained
examples, tests, and the authoritative specification change together; the archive
remains frozen.

Implementation recommendation: **GPT-6.1 Sol (`gpt-6.1-sol`), high reasoning**,
with independent high-reasoning review of literal preservation, preamble import
insertion, and source mapping.

## Assessment baseline

Assessed checkout: `d045bebd9f57646a5d8624ee97005b4414e97a99`, 2026-10-16.
This snapshot records the code, documentation, tests, and bounded probes before
GORN-V1-25. Item statuses and the implementation record describe subsequent
changes. Hosted CI and release state were outside the assessment.

| Area | What works | Gap before v1 |
| --- | --- | --- |
| Script format | Explicit package/main split, shebang, version/module/require directives, optional preamble, line-carrying parse errors | Go content can be changed or mistaken for directives; directive validation and spec agreement need work |
| Generation | Generated module and formatted Go; `//line` mapping; inspect flags | Multiline literal semantics and source paths need correction; inspection depends on cache state |
| Runner | Explicit and shorthand run, stdin source, argument forwarding, inherited runtime IO | Exit status, cancellation, argument edge cases, build progress, and cold/warm parity lack a stable contract |
| Cache | Content-derived key, checksum verification, private staging directories, atomic initial publication, concurrent-winner handling | Corrupt entries cannot repair in place; identity omits effective build inputs; schema is not checked on load |
| Dependencies | Explicit require directives; standard Go tooling performs resolution and build | `go mod tidy` can discover undeclared dependencies; generated `sh` dependency is not automatically pinned |
| `sh` | Small stdlib-only package, explicit shell choice, copy-style configuration, standard IO, status-only results | Exact argv and context support absent; shell/child lifecycle and Windows behavior need stronger contracts and coverage |
| Delivery | Mise lockfile; lint/build/test CI configured for three operating systems | No release workflow in this checkout; no installed-CLI end-to-end suite; limited user onboarding and authoring guidance |

Preserve the good boundaries: Go remains the language and dependency system;
`sh` stays stdlib-only; CLI packages do not import `sh`; output belongs to Go IO,
not a growing result container. Root `package main` remains the installable CLI.

### Verification performed

- `mise run test`, using pinned Go `1.26.4`: lint reported **0 issues**, and
  the stamped CLI built. All packages passed except one generator build test.
- `TestGeneratePreambleOutputCompiles` failed with
  `error obtaining VCS status: exit status 128`. A focused rerun with
  `GOFLAGS=-buildvcs=false` passed. This isolates a VCS-stamping interaction in
  the assessment environment; the unmodified full gate failed there. Do not treat
  the override as a release fix or infer that hosted CI is failing.
- Isolated probes reproduced warm-cache inspection executing the script,
  lost child exit status, failed cache repair, altered raw strings, and
  directive-looking text inside a raw string being rejected.
- Changing a linker value through `GOFLAGS` reused the previous cached value.
  Sending SIGTERM only to gorn left the compiled child running. Both were
  reproduced on Linux; the child was cleaned up after the probe.
- A Linux `sh` probe killed a shell with a two-second child holding copied
  stdout open. `Wait()` still took approximately two seconds. This confirms
  the current direct-process limitation; it is not a cross-platform result.

Runtime probes used `GOFLAGS=-buildvcs=false` where necessary to separate those
behaviors from the VCS error. Print-only probes did not need that override.
No product implementation was changed during the assessment.

## Priorities and delivery order

- **P0 / required:** correctness or reliability blockers for the proposed v1 promise.
- **P1 / required:** the bounded feature, validation, and delivery work needed
  to support that promise.
- **P2 / optional:** useful additions that may ship if the required work is
  complete. They do not hold v1 open.

Size is a planning signal: **S** is a localized change, **M** spans a workflow
or several packages, **L** needs a cross-cutting or platform-specific contract.
Sizes are not calendar estimates. All dependencies below refer to item IDs.

| Milestone | Outcome | Items |
| --- | --- | --- |
| M0: Standard source format | Complete: source-authored package/main, preserved source, Go-aware metadata | 25, with 02–03 |
| M1: Correct execution | Go semantics, inspection, source identity, and failures are reliable | 01–04, 09; start 18 immediately |
| M2: Predictable builds | Build identity, dependency policy, repair, cancellation, validation, and progress work together | 05–08, 10–11 |
| M3: Complete core workflows | Export binaries, manage cache, execute exact argv, document supported shells | 12–15 |
| M4: Release candidate | Real workflow tests, usable docs, supported public API, measured startup, installable release | 18–22 |
| Optional after core | Check, eject, format, and dependency convenience | 16–17, 23–24 |

### Implementation model and reasoning recommendation

Use **GPT-6.1 Sol (`gpt-6.1-sol`), high reasoning**, as the default implementation
model. The following are task-specific starting points, not measured performance
claims for this repository. Keep changes to one reviewable behavior at a time.

| Work | Recommended model | Reasoning | Reason |
| --- | --- | --- | --- |
| Build identity, dependency policy, process lifecycle (06–08), including design review | GPT-6 Astra (`gpt-6-astra`) | high | Cross-cutting invariants and platform-dependent failure paths |
| Source semantics, runner/cache fixes, command workflows, integration coverage, API review | GPT-6.1 Sol (`gpt-6.1-sol`) | high | Multi-package code reasoning with behavioral verification |
| Localized inspection/cardinality fixes and settled documentation or release plumbing (01, 10, 19, 21) | GPT-6.1 Sol (`gpt-6.1-sol`) | medium | Bounded work once the relevant contracts are decided |

Use an independent reviewer at high reasoning for P0 changes. Raise effort only
when concrete failures justify it. These choices follow the general approach in
OpenAI's [model-selection guidance](https://developers.openai.com/api/docs/guides/model-selection)
and [reasoning-effort guidance](https://developers.openai.com/api/docs/guides/deployment-checklist).

**Next implementation slice:** binary export and cache management (12–13),
following the selected [build/runtime contract](../specs/gorn-build-runtime.md).
Inspection (01), source format (02–03, 25), execution/source identity (04, 09),
cache ownership/repair (05), profile (06), dependency/helper delivery (07),
lifecycle (08), metadata validation (10), and progress (11) are implemented.
Follow the [delivery workflow](v1-next-steps.md) for bounded implementation and
validation.

### GORN-V1-01 — Make inspection incapable of executing a script

**P0 · Fix · S · Depends: none**

**Status: implemented.** Inspection returns after parsing and generation, before
opening or looking up the cache. The previous warm-cache path could execute user
code before checking print flags. Artifact and header write failures, including
short writes, now propagate.

Evidence: [run dispatch](../../pkg/cli/runcmd.go),
[write-error tests](../../pkg/cli/runcmd_test.go), and
[cache-state inspection tests](../../run_integration_test.go). The regressions
failed on the unchanged runner and passed after the fix. The full `mise run test`
gate passed on Linux using the documented valid Git fixture as TMPDIR, with
normal VCS stamping. Independent P0 review found no actionable issues.

Acceptance: both flags, separately and together, emit only the requested
artifacts on cold, warm, bypass, and corrupt caches. A side-effect fixture proves
that inspection never executes the script. Inspection works without a writable
cache or an installed Go executable and reports output-write errors.

### GORN-V1-02 — Preserve Go source semantics during generation

**P0 · Fix · M · Depends: 25 decision**

**Status: implemented through 25.** The original
wrapper altered multiline raw-string values by indenting every body line. The
wrapper and indentation helper are removed. Generation now preserves source
text and uses source anchors around inserted preamble code.

Evidence: [generation](../../pkg/script/generator.go) and its
[compile/run and diagnostic tests](../../pkg/script/generator_test.go).
Acceptance covers raw strings, blank lines, attached comments, CRLF, original
formatting, and equivalence with ordinary Go, including original source locations.

### GORN-V1-03 — Recognize directives only as actual source comments

**P0 · Fix · M · Depends: 25 decision**

**Status: implemented through 25.** The original
line-based parser mistook directive-looking literal/comment contents for metadata.
The parser now uses actual Go comment tokens and accepts standalone metadata only
in the header before the package clause.

Evidence: [parser](../../pkg/script/parser.go) and its
[lexical/position regressions](../../pkg/script/parser_test.go). Tests cover raw
strings, block comments, trailing comments, placement, cardinality, and physical
source lines, without a second Go grammar.

### GORN-V1-04 — Define and preserve the CLI execution contract

**P0 · Fix · M · Depends: 01**

**Status: implemented.** Ordinary child exits are preserved; POSIX signal exits
map to `128 + signal`. Usage errors exit 2 on stderr; runner failures exit 1.
Valid invocations never dump usage, and child failures retain their stdout/stderr
without an added runner error line. Explicit and shorthand forms share help,
version, separator, and verbose-flag behavior.

The [execution/source identity decisions](v1-execution-plan.md) record alternatives,
trade-offs, rationale, and verification. Evidence: [status mapping](../../pkg/cli/status.go),
[CLI entry](../../main.go), and [installed-CLI tests](../../execution_integration_test.go).

Acceptance: successful scripts exit `0`; ordinary child failures preserve their
exit code. Define signal outcomes and distinct usage/build failures. Script
stdout/stderr remain intact; usage and runner errors go to stderr, with no usage
dump for a valid invocation whose script fails. Explicit and shorthand forms
match for `--`, flag-like arguments, empty args, and `-v run` parse failures.
Propagate failed writes for generated artifacts rather than reporting success.

### GORN-V1-05 — Repair invalid cache entries safely

**P0 · Fix · M · Depends: 06; coordinate with 13**

**Status: implemented.** Full-key layout/manifest schema 3 separates independent
ownership records from the validity gate. OS-backed mutation/activity/stage locks
serialize builders and lease executables through use. Repair waits for active
leases, quarantines invalid owned entries and atomically installs completed
candidates. Unknown ownership and symlink substitutions are preserved/refused.
Bypass builds avoid entry locks but retain private stage protection. Entry copies
share idempotent private cleanup; payload-first deletion preserves ownership after
Windows sharing violations. Prior schema-2 entries remain untouched.

Evidence: [selected protocol, alternatives and boundaries](../specs/gorn-build-runtime.md#cache-publication-repair-and-management),
[publication/acquisition](../../pkg/cache/emit.go),
[ownership/cleanup](../../pkg/cache/ownership.go),
[behavioral tests](../../pkg/cache/ownership_test.go), and
[Windows sharing-violation test](../../pkg/cache/cleanup_windows_test.go).
Focused tests cover real two-process cold builds with exactly one compilation,
coexisting readers, killed-holder lock release, canceled publication waits,
corrupt/missing manifest and payload/profile/target repair, unknown records,
symlinks, failed final publication, and safe copied-entry cleanup. Cache command
enumeration, age/size selection and maintenance skip/report behavior remain 13;
export uses the same lease contract under 12. Local validation passes
`mise run test` in the documented private Git/TMPDIR fixture, focused race tests,
Linux/macOS/Windows lint, both dependency invariants, and documentation checks.
Independent P0 review corrected cancellation at publication and eager Windows
file-identity capture; hosted execution verifies the OS-specific tests before
landing.

### GORN-V1-06 — Key the cache on the effective build profile

**P0 · Improvement · L · Depends: 07, 09**

**Status: implemented.** One frozen profile validates before warm/cold lookup,
keys actual installed Go and executing CLI bytes, normalized supported flags,
helper identity, and resolver settings, and configures resolver/build subprocesses.
The host-only pure-Go policy rejects unsupported flags, compiler hooks,
experiments/FIPS, and source embed/local inputs. SDK/user/process precedence is
explicit; mutable user GOENV is disabled afterward. Manifest schema 3 records the
actual compiler/profile with private values hashed; generation version 4 excludes
old identity.

The [build/runtime contract](../specs/gorn-build-runtime.md) records alternatives,
rationale, and installed-input trust assumptions. Evidence:
[profile tests](../../pkg/buildprofile/profile_test.go),
[flag grammar](../../pkg/buildprofile/flags_test.go),
[installed-CLI regressions](../../profile_integration_test.go), and
[manifest gates](../../pkg/cache/cache_test.go).

Acceptance: define supported inputs once and use them for execution, identity,
and provenance: generator identity, dependency identity, actual selected Go
toolchain, target, and supported flags/cgo settings. Inputs are either accounted
for or rejected with a useful error. Define source-adjacent compilation inputs;
for the initial single-file scope, report unsupported embed/local-file inputs
clearly rather than silently copying untracked assets. `run` builds for the host
or rejects a foreign target before executing it. Relevant changes miss the cache; runtime args and
ordinary runtime env changes do not. Use a development-build identity that cannot
silently reuse another generator revision's binary.

### GORN-V1-07 — Make dependency and helper resolution explicit

**P0 · Improvement · L · Depends: none; decision required before 06**

**Status: implemented.** Direct nonstandard imports require covering concrete
declarations. The CLI embeds maintained production `sh` source and license as
data, materializes a private child module by default, and keys bundled bytes.
An explicit parent requirement selects the published helper; reserved child
requirements and mixed transitive-parent graphs fail with remedies.

Go expands only the declared graph to a fixed point and builds readonly, without
tidy/get/latest discovery or silently raising the source Go minimum. Effective
module/checksums, original declarations, selected versions, and helper identity
are saved. No companion lock artifact is required. The [build/runtime design](../specs/gorn-build-runtime.md)
records alternatives, trade-offs, reproducibility limits, and the supported GOFLAGS
allowlist delivered under item 06. Evidence: [resolver and file-proxy regressions](../../pkg/cache/resolve_test.go),
[bundle tests](../../pkg/script/bundle_test.go), and
[installed-CLI offline cold/warm/bypass tests](../../dependency_integration_test.go).
Private-proxy transport failures verify URL credential/query redaction. Actual
toolchain/generator identity is delivered under 06; lifecycle remains 08.

Acceptance: require concrete direct dependency versions and reject undeclared
third-party imports with a source-oriented remedy instead of silently selecting
them. Choose a versioned `sh` delivery strategy compatible with the module's
release version and development builds. Define conflicts with explicit gorn
requirements. Preserve resolved module/checksum provenance and decide whether
single-file declarations suffice or a companion lock artifact is necessary.
Document the resulting reproducibility limits. Test preamble, explicit `sh`,
third-party, transitive, and private-module cases without exposing credentials.
Network-disabled runs succeed when the required caches are populated; missing
dependencies fail clearly. Use Go's resolver and checksum mechanisms.

### GORN-V1-08 — Make cancellation and process cleanup predictable

**P0 · Improvement · L · Depends: 04; coordinate with 15**

**Status: implemented.** Context reaches source collection, installed-Go
discovery, resolution/builds, runtime execution, and helper commands. One automatic
reaper serves repeated/concurrent waits. Linux/macOS group termination retains
the leader through bounded TERM/KILL escalation; Windows uses bounded native
tree termination with direct fallback. Go-owned pipe waits are bounded. Root
interrupt/termination statuses, explicit embedding IO ownership, reserved bundled
helper coordination, foreground terminal restoration, and platform/job-control
limits are documented with alternatives and rationale in the
[lifecycle contract and linked evidence](../specs/gorn-build-runtime.md#cancellation-and-process-ownership).
Cache lock/lease cancellation is delivered with the locks in item 05; phase
progress remains item 11.

Acceptance: define cancellation for dependency downloads, builds, scripts, and
`sh` commands. Specify supported process-tree behavior per OS, bounded waiting
on inherited pipes, exit statuses, and reap/temporary-directory ownership.
Ctrl-C and termination stop supported work predictably without publishing
unfinished builds. Test children holding output open and repeated termination.
Document any unavoidable platform differences before stabilizing the API.

### GORN-V1-09 — Normalize source and cache paths at the boundary

**P1 · Fix · M · Depends: none**

**Status: implemented.** Source paths are absolute and lexically cleaned while
retaining symlink spelling. stdin uses a distinct identity and synthetic absolute
`stdin.gorn` diagnostic path in the invocation cwd. Source bytes are immutable
after construction. Relative `GORN_CACHE` values resolve against invocation cwd;
unusable explicit roots return their path and cause without fallback. Runtime
cwd is preserved, and source stdin is consumed before runtime begins.

See [decisions and verification](v1-execution-plan.md). Evidence:
[source tests](../../pkg/app/source_test.go),
[cache-location tests](../../pkg/cache/cache_test.go), and
[installed-CLI tests](../../execution_integration_test.go).

Acceptance: choose absolute source identity, a documented symlink policy, and a
portable synthetic stdin name. Diagnostics identify the original source, not a
build-directory-relative path. Cover same-named scripts in different directories,
spaces, Unicode, CRLF, and Windows paths. Runtime cwd remains the invocation cwd.
Normalize or explicitly reject relative cache overrides; an explicit unusable
override returns a useful error. Document stdin-source versus runtime-stdin use.

### GORN-V1-10 — Close directive validation and specification gaps

**P1 · Fix · S · Depends: 03, 07, 25 decision**

**Status: implemented.** GORN-V1-25 enforces the single-preamble rule. Metadata
validation now rejects invalid Go/module/version syntax and all duplicate
requirement paths, with both declaration lines. Canonical prereleases, valid
pseudo-versions, major-version module paths, and dotless main-module names are
preserved. Go module validation uses the justified CLI-only `golang.org/x/mod`
dependency; `sh` remains stdlib-only.

The [build/runtime contract](../specs/gorn-build-runtime.md) records the selected
policy and alternatives. Evidence: [metadata regressions](../../pkg/script/metadata_test.go)
cover syntax, cardinality, original physical lines, underlying validation causes,
and operation without Go. Dependency resolution/helper delivery are recorded in item 07.
Evidence: [directive handling](../../pkg/script/parser.go) and
[format contract](../specs/gorn-script-format.md).

Acceptance: reject repeated preamble directives; define duplicate/conflicting
requirements and validate supported module/version/Go-version syntax before a
build. Preserve valid pseudo-versions and major-version module paths. Do not
change the documented permissive shebang rule without an explicit format
decision. Update the authoritative contract and add line-specific regressions.

### GORN-V1-11 — Explain slow work and failures while they happen

**P1 · Improvement · M · Depends: 04, 07–08**

**Status: implemented.** Immediate preparation phases and two-second heartbeats
go to stderr. Quiet suppresses Gorn activity while preserving sanitized Go
diagnostics and script stderr. Go errors stream as complete sanitized lines,
including conservative EOF redaction and bounded oversized-line handling, without
final-error replay. Reporting stops and joins before runtime terminal handoff;
original runtime IO and background embedding-reader ownership are retained.
Output failures preserve their IO cause and stop pending Go work. Internal
generated-source dumps require verbose; print flags remain deliberate inspection.
The [selected decisions, alternatives, trade-offs, and behavioral evidence](../specs/gorn-build-runtime.md#progress-and-verification)
include real installed-CLI cold/warm/bypass, slow source/dependency work, quiet
errors, redaction, and stdout/runtime integrity. Lock-wait phases join this same
reporter when item 05 implements locks.

Acceptance: provide immediate phase information and periodic status during long
work on stderr, with a documented quiet mode for automation. Stream useful Go
errors; identify dependency, toolchain, compilation, and script failures.
Keep stdout usable by pipelines. Explain missing Go, unavailable shells, proxy
failure, and cache permissions. Source errors include the correct script line;
raw generated output is available deliberately rather than dumped by default.

### GORN-V1-12 — Implement standalone binary export

**P1 · Feature · M · Depends: 04–08**

Implement `gorn build <script> -o <path>` using the same validated build pipeline
as run. [The current command](../../pkg/cli/buildcmd.go) is a stub.

Acceptance: build never executes the script; cold and cached exports work;
output has the correct platform name and permissions. Publish output atomically
with an explicit overwrite policy; failures preserve an existing destination.
The binary works from another directory without gorn installed. Define target
selection and cgo limits; cross-compilation is optional, not a prerequisite for
host-target export. Help contains no advertised stub workflow.

### GORN-V1-13 — Implement inspectable, bounded cache management

**P1 · Feature · M · Depends: 05–06, 09**

Implement `cache path`, `cache list`, `cache clean`, and `cache prune`.
[The current command](../../pkg/cli/cachecmd.go) is a stub. Start with explicit
cleanup; automatic background eviction is unnecessary for v1.

Acceptance: list reports identity, source, size, and recorded build provenance;
provide stable JSON where results are intended for automation. Define clean and
age/size-based prune semantics, including whether recency is build time or usage.
Support dry-run and explicit noninteractive deletion. Delete only gorn-owned
entries under the resolved cache root, never Go's caches or unrelated paths.
Coordinate active builds/runs and Windows executable locking; collect abandoned
temporary directories safely. Bad entries remain diagnosable.

### GORN-V1-14 — Add exact argv execution to `sh`

**P1 · Feature · M · Depends: 08, 15**

Add a bounded `sh.Cmd(name string, args ...string)` entry point using the existing
command configuration and result model. Ordinary commands should not require
constructing shell strings. Keep shell execution available for shell syntax.

**Status: core API delivered with 08.** Exact arguments use the same immutable
configuration, IO/env/dir, cancellation, status, and startup path as shell snippets.
[Tests](../../sh/command_test.go) cover spaces, Unicode, quotes, empty arguments,
metacharacters, executable paths, independent copies, IO/env/dir, and failures.
Final supported-shell/API verification remains coordinated with 15 and 20.

Acceptance: arguments containing spaces, quotes, wildcard/metacharacters, and
empty values arrive literally without shell interpretation. IO/env/dir,
cancellation, start failure, and nonzero status follow the same documented
contract. Test Windows argv and executable paths. Retain stdlib-only dependencies;
do not add implicit globbing, argument flattening, or a pipeline abstraction.

### GORN-V1-15 — Stabilize and verify the small shell API

**P1 · Improvement · M · Depends: 08; review API before 14**

Document host-shell selection, login/profile startup, setup ordering, environment
inheritance, `Strict`, IO ownership, `Wait`, and `Kill`. Most behavioral tests use
Bash and skip Windows. `Strict` identifies exact executable basenames, missing
paths such as `pwsh.exe`.
Evidence: [shell selection](../../sh/shellspec.go), [package docs](../../sh/doc.go),
and [shell tests](../../sh/shellspec_test.go).

Acceptance: decide login versus non-login defaults before v1. Verify the named
supported shells on appropriate CI hosts, including CmdExe and PowerShell;
unsupported optional shells skip explicitly. Cover absolute executable paths,
Windows extensions/casing, missing shells, env override, setup immutability,
and exit behavior. State what PowerShell strictness means for native commands.
Keep status-only results and standard Go IO as the output extension points.

### GORN-V1-16 — Add a non-executing check workflow

**P2 · Feature · M · Depends: 01–03, 07, 11**

Provide `gorn check <script>` for CI and authoring. Parse, generate, resolve
declared dependencies, and compile-check without executing user code.

Acceptance: a side-effect fixture never runs; type errors point to original
lines. Document network/cache effects and the difference between check and
print-only inspection. Reuse build rules rather than a separate validator.
An editor can invoke the command without requiring a new language server.

### GORN-V1-17 — Provide an escape hatch into ordinary Go

**P2 · Feature · M · Depends: 02, 07, 09, 12**

Implement `gorn eject <script> <directory>` if core work leaves capacity. This
reduces adoption risk when a file grows into a normal application.

Acceptance: export readable Go, `go.mod`, and applicable checksums with explicit
imports and pinned helper dependencies. Ordinary Go builds and runs the result
after the original script/cache are removed. Source mapping does not depend on
the original `.gorn` path. Refuse unsafe overwrites; document external runtime
assets and any provenance files. Multi-file includes are unnecessary.

### GORN-V1-18 — Test the real installed-CLI workflows

**P1 · Validation · L · Depends: start now; complete after 01–15**

GORN-V1-25 adds real CLI cold/warm/bypass execution, stdin, arguments, IO,
and cold inspection coverage. GORN-V1-01 extends inspection coverage across
warm, bypass, and corrupt caches, missing Go, unusable cache paths, and output
failures. Generator tests compile plain, preamble, and
documentation examples using a local module replacement where needed.
Installed-CLI and cross-platform coverage of the remaining workflows is still
required. Evidence: [execution tests](../../run_integration_test.go),
[CLI tests](../../main_test.go), [cache tests](../../pkg/cache/cache_test.go),
and [generator tests](../../pkg/script/generator_test.go).

Acceptance: use a built CLI in isolated directories for plain, preamble, and
explicit dependency scripts. Cover cold/warm/bypass, stdin, args, IO, status,
inspection safety, corruption repair, concurrent builds, cancellation, and
exported binaries. Use controlled local dependencies for deterministic tests and
a separate release smoke test for actual published dependency resolution.
Run relevant behavior on all supported OSes and race checks where supported.
Investigate the observed VCS-stamping failure; keep the ordinary project gate
passing without requiring a developer's global `GOFLAGS` override.

### GORN-V1-19 — Make the first-run documentation executable

**P1 · Improvement · M · Depends: 07, 12, 14–15, 18**

The README provides contributor setup but no complete end-user install/start
flow. GORN-V1-25 replaces obsolete helper calls in the authoritative format
examples with existing APIs or standard-library code and compile-checks those
examples. Complete the end-user onboarding and workflow documentation below.
Evidence: [README](../../README.md) and [format examples](../specs/gorn-script-format.md).

Acceptance: install instructions, Go/PATH prerequisites, a working first script,
Unix shebang and Windows invocation, dependencies, build/cache usage, and a
troubleshooting path are tested. Explain host dependence, permissions, expected
network access, cwd, and stdio. Compile/run maintained examples in CI. Publish
current runner/cache/`sh` contracts outside the archive; keep archived docs frozen.

### GORN-V1-20 — Define the v1 compatibility surface

**P1 · Architecture · M · Depends: 07, 10, 14–15, 25 decision**

Choose what v1 promises to keep stable: script directives and preamble names,
CLI flags/output/status, public `sh`, and cache format transitions. The `pkg/`
tree is described as internal but Go permits imports of those package paths.

Acceptance: either move implementation packages behind `internal/` before v1
or explicitly support the intended public API; resolve the naming ambiguity.
Document the supported Go floor and upgrade/deprecation policy. Changes to
generator/preamble/helper behavior invalidate affected cached builds. Enforce
the two package dependency invariants with a meaningful dependency-graph check:
stdlib-only `sh`, and no root/CLI import of `sh`.

### GORN-V1-21 — Deliver and smoke-test versioned releases

**P1 · Delivery · M · Depends: 12, 18–20, 22**

This checkout has a CI workflow but no release workflow. Add the release process
only after the supported targets and helper version strategy are settled.

Acceptance: prepare tagged, version-stamped binaries and checksums for the
declared target matrix. Verify the installed command and `go install` path,
version output, permissions, and first-run examples in clean environments.
Test actual release-module resolution of `sh`, not a local replacement. Use an
approved candidate/module tag first where needed; verify its availability and
installed-CLI behavior before promoting final release assets. State the residual
ordering risk if final-version resolution can only be verified after publishing
the final module tag. Publish release notes and upgrade instructions.
Package-manager integrations can follow after the direct install paths work.
Publishing remains a separately approved
delivery action.

### GORN-V1-22 — Measure whether reruns feel immediate

**P1 · Validation/improvement · M · Depends: 05–07, 18**

The promise depends on startup experience, but no performance baseline is
recorded. Lookup hashes the entire binary on every hit; measure before changing
that integrity check. Dependency resolution and compilation need separate timing.

Acceptance: record cold with empty caches, cold with Go caches, warm, and bypass
timings for tiny, preamble, and dependency-bearing fixtures. Separate runner
overhead from script work and record OS, hardware, Go, and sample distribution.
Set a numerical warm-start budget from beta measurements before the release
candidate; do not invent a portable threshold here. Prove warm runs cause no
dependency downloads. Optimize measured bottlenecks and retain integrity guarantees.

### GORN-V1-23 — Format `.gorn` files without changing their meaning

**P2 · Feature · M · Depends: 02–03, 10, 25 decision**

Consider `gorn fmt` with write and check-only modes after source-preservation
work is solid. Preserve shebang/directives and use Go's formatter for the chosen
source model: one ordinary Go unit without section splitting.

Acceptance: formatting is idempotent, preserves comments and literal values,
does not execute scripts, and produces useful check-mode failures in CI.
Document lightweight editor use. A generated-file/gopls bridge is future work.

### GORN-V1-24 — Reduce dependency setup friction

**P2 · Feature · M · Depends: 07, 10, 18**

Consider a helper to add or update a requirement, and/or prefetch dependencies
for CI, only after the dependency contract is settled.

Acceptance: a request for `latest` resolves once and writes a concrete version;
source is edited atomically with comments preserved. Preview changes, retain
explicit requirements, and make offline preparation distinguishable from running
the script. Do not add a registry or independent dependency solver.

### GORN-V1-25 — Decide and adopt a standard Go entry point

**P1 · Architecture/feature · M · Depends: none; decision before 02–03**

**Status: implemented.**

The source-authored `package main` and `func main()` contract, optional shebang
and preamble, source-preserving generation, and key-version invalidation are
implemented. The wrapper template and section representation are removed.

The [implementation record](v1-standard-main-plan.md) records acceptance
coverage, validation commands, environment handling, and the source-preservation
review correction.
No migration tooling, dedicated legacy diagnostics, dual grammar, or automatic
extension change was introduced. Full build identity, dependency pinning, and
other runner fixes remain separate backlog items.

Implementation model recommendation: **GPT-6.1 Sol (`gpt-6.1-sol`), high reasoning**.

## Scope deliberately deferred

| Candidate | Why it waits | Revisit when |
| --- | --- | --- |
| Fragments/includes and `.gfrag` | Adds source/dependency/mapping complexity before the single-file contract is stable | Real scripts need local organization and ordinary modules/eject do not suffice |
| `//gorn:replace` and workspace integration | Introduces local build inputs and portability rules | A concrete local-module workflow justifies it after cache/dependency identity is settled |
| Structured pipelines and line-event iterators | Adds lifecycle, backpressure, and failure semantics | Exact argv plus ordinary Go IO is demonstrably insufficient |
| Recursive globbing and filesystem helper suite | Expands public API into existing standard library territory | Repeated user tasks show a concise, portable API with clear semantics |
| Remote scripts, registry, URL execution, self-update | Adds distribution and trust workflows | Local execution and versioned releases have reliable adoption evidence |
| Full `.gorn` language server/debug adapter | Large continuing tooling commitment | Check/format/eject and normal Go tooling leave a measured authoring problem |
| Task DAGs, watch mode, shell emulation | Changes the product into an orchestrator or language runtime | Separate product research supports that direction |

## Design decisions and beta validation

The source-format decision (25) is implemented in the
[authoritative format specification](../specs/gorn-script-format.md). The
[build/runtime design](../specs/gorn-build-runtime.md) selects dependency, build,
and process contracts with alternatives and rationale. Implement and verify those
contracts before beta, then settle the remaining release surface:

1. **Dependency identity (07):** delivered bundled helper bytes, explicit parent
   requirements for published helper mode, pinned-graph resolution, and saved
   module/checksum provenance. Offline/private-proxy regressions are in place;
   release-wide verification remains part of 18–22.
2. **Build environment (06):** delivered the local installed toolchain, pure-Go
   host profile, supported flag allowlist, and generator/profile cache identity.
3. **Process behavior (08, 15):** implement the selected ownership/cancellation,
   foreground terminal handoff, non-login defaults, and shell status contracts.
   Verify real POSIX terminals and Windows process behavior.
4. **Compatibility and distribution (20–21):** select the public package and
   release target surfaces, installation prerequisites, and release policy before
   stabilization and publication.

The design constrains implementation; behavioral verification establishes when
its backlog items are complete. Release-wide validation remains required even
when focused implementation tests pass.

## v1 release gate

All P0 and P1 items are complete, with linked implementation and validation
evidence. Optional work can be omitted without delaying the release. Specifically:

- Ordinary Go content retains its meaning, inspection cannot execute it, and
  explicit/shorthand execution agree on args, cwd, IO, status, and cancellation.
- The same supported build inputs resolve consistently; all cache-affecting
  changes invalidate reuse; damaged caches recover and concurrent operations work.
- Advertised run/build/cache and `sh` workflows work on the declared platforms;
  no advertised command is a stub.
- The normal mise gate and installed-CLI tests pass, executable docs match the
  release, and candidate published-module resolution is verified before final
  release promotion. Final-version resolution is checked as soon as its approved
  module tag is available.
- Cold/warm performance is recorded and the agreed warm-start budget is met.
- Beta users can install, run, diagnose a failure, and export a script using the
  maintained documentation. Release notes explain compatibility commitments and
  remaining platform limits.
