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

This is a proposed backlog for review. Every item is **proposed**, including
items described as release blockers. It does not authorize implementation,
publication, or changes to the frozen archive.

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

## Proposed format change: ordinary Go entry point

**Proposal under review, 2026-10-17:** replace `//gorn:main` and the generated
main wrapper with a source-authored `func main()`. Recommendation: also require
`package main`, so the source after an optional shebang is an ordinary Go file.
Retain module/version/require directives as header comments. This changes the
recommended v1 format; it does not describe the current implementation.

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
to split a file into declarations and executable statements. Helpers already
work above today's divider; this proposal removes that special ordering rule
rather than introducing functions for the first time.

| Choice | Assessment |
| --- | --- |
| Keep `//gorn:main` | Smallest tiny-script form, but retains a custom split, wrapper generation, and source semantics/mapping work |
| Require `func main()` and inject `package main` | Removes the divider, but the original source still lacks Go's required package clause |
| Require `package main` and `func main()` | Recommended: a little more source text buys a complete Go file model and simpler generation |

Go's [source-file grammar](https://go.dev/ref/spec#Source_file_organization)
requires a package clause; its [program entry-point contract](https://go.dev/ref/spec#Program_execution)
requires `main` in package `main`, with no arguments or results. Use Go's parser
and compiler rather than detecting functions with text matching. Empty
`func main() {}` is valid; the old empty-main-section rule should disappear.

The expected tooling improvement has limits:

- `.gorn` still needs a Go language association in editors that do not recognize
  it. A standard main alone does not establish out-of-the-box gopls support.
  `.go` input/discovery is a separate decision; do not rename every script as
  part of this change without considering normal Go package discovery.
- A shebang is still outside Go syntax. Gorn can preserve support by replacing
  its first line with a blank line before Go parsing, retaining source numbering.
  Direct Go formatting of a shebang-bearing file still needs preprocessing.
- Keep the optional preamble for this format change, with explicit imports as the
  preferred examples. Ambient imports still require generated source for full
  name resolution. Removing the preamble is a separate decision.
- If the preamble remains, inject its imports into the same generated file
  after the package clause; a companion file cannot supply imported package
  names to the script. Preserve original source mappings around inserted code
  and leave all imports before generated keep-alive declarations. Imports have
  [file scope](https://go.dev/ref/spec#Declarations_and_scope).

Gorn's value remains single-file dependency declarations, generated module
management, cached execution, and binary export. It need not replace Go's entry
point syntax to provide those benefits.

Given the pre-alpha status, prefer one documented format break before v1 over
permanent support for both grammars. No migration tooling or dedicated legacy
diagnostics are required. Bump generation/cache identity so old cached binaries
cannot bypass the new contract.

Track the decision and format change as **25**, before source-format work in 02–03.
If adopted, implement 02 by eliminating body wrapping/indentation, retain its
source-equivalence regressions, and implement 03 through Go comment tokens in
the metadata header. Update maintained examples/tests and the authoritative
format spec together during implementation; keep the archive frozen.

Implementation recommendation: **GPT-6.1 Sol (`gpt-6.1-sol`), high reasoning**,
with independent high-reasoning review of literal preservation, preamble import
insertion, and source mapping. No implementation or format approval is implied
by this recommendation.

## Current state

Assessed checkout: `d045bebd9f57646a5d8624ee97005b4414e97a99`, 2026-10-16.
The worktree was clean before this assessment. Findings below are from the local
code, documentation, tests, and bounded probes; hosted CI and release state were
not queried.

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
  this environment; the unmodified full gate is still failing. Do not treat
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
| M0: Format decision | Review standard main/package proposal before investing in the legacy split | 25; inspection fix 01 can proceed independently |
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

**Recommended next implementation slice:** resolve 25 in Plan mode, then
implement the approved source format with the relevant 02–03 regressions in one
logical change. Avoid spending a preliminary change repairing a wrapper that
the selected format removes. Inspection safety (01) remains an independent fix
that can be pulled forward. Follow the [delivery workflow](v1-next-steps.md);
each implementation commit and push requires the user's review approval.

### GORN-V1-01 — Make inspection incapable of executing a script

**P0 · Fix · S · Depends: none**

On a warm cache, `Run` returns through `execCmd` before checking `--print-mod`
or `--print-main`. An inspection command therefore runs user code.
Evidence: [run dispatch](../../pkg/cli/runcmd.go), lookup before the print branch.

Acceptance: both flags, separately and together, emit only the requested
artifacts on cold, warm, bypass, and corrupt caches. A side-effect fixture proves
that inspection never executes the script. Inspection works without a writable
cache or an installed Go executable and reports output-write errors.

### GORN-V1-02 — Preserve Go source semantics during generation

**P0 · Fix · M · Depends: 25 decision**

The template indents every nonempty main-section line, including lines inside
raw strings. A literal containing `first` then `second` gains a tab before
`second`. Go syntax remains valid while its value changes.
Evidence: [indent helper](../../pkg/script/generator.go) and
[main template](../../pkg/script/main.gotmpl).

Acceptance: multiline raw literals, embedded templates, blank lines, and
multiline comments preserve their Go meaning. Compare generated-program output
with equivalent ordinary Go, including leading whitespace. Source mapping still
points to the original package and main sections after formatting.
If 25 is adopted, remove the wrapper and indentation step instead of repairing
the legacy transformation; keep these behavioral regressions for the new format.

### GORN-V1-03 — Recognize directives only as actual source comments

**P0 · Fix · M · Depends: 25 decision**

The parser examines trimmed lines without lexical context. A `//gorn:go` line
inside a raw literal is rejected as a directive after main; a `//gorn:main`
inside content can also be mistaken for a boundary.
Evidence: [parser](../../pkg/script/parser.go), `Parse` directive recognition.

Acceptance: directive-like text inside raw strings and block comments remains
content in either section. Actual standalone directives retain placement,
cardinality, and original-line errors. Cover CRLF, EOF without a newline,
and ambiguous boundary cases without introducing a second Go grammar.
Apply the selected format's placement rules: the proposed ordinary-Go model
uses metadata header comments and has no package/main section boundary.

### GORN-V1-04 — Define and preserve the CLI execution contract

**P0 · Fix · M · Depends: 01**

A script calling `os.Exit(7)` currently makes gorn exit `1`: its execution error
is wrapped and `main` converts every error to `1`.
Evidence: [execution](../../pkg/cli/runcmd.go) and [CLI entry](../../main.go).

Acceptance: successful scripts exit `0`; ordinary child failures preserve their
exit code. Define signal outcomes and distinct usage/build failures. Script
stdout/stderr remain intact; usage and runner errors go to stderr, with no usage
dump for a valid invocation whose script fails. Explicit and shorthand forms
match for `--`, flag-like arguments, empty args, and `-v run` parse failures.
Propagate failed writes for generated artifacts rather than reporting success.

### GORN-V1-05 — Repair invalid cache entries safely

**P0 · Fix · M · Depends: 06; coordinate with 13**

Checksum failure becomes a miss, but publication cannot replace the occupied
invalid directory. A successful rebuild still fails to install. Existing tests
explicitly expect that publication error. Also, manifest loading does not reject
a mismatched `schema` field.
Evidence: [lookup](../../pkg/cache/cache.go), [manifest](../../pkg/cache/manifest.go),
[publication](../../pkg/cache/emit.go), and [cache tests](../../pkg/cache/cache_test.go).

Acceptance: missing/corrupt manifest, wrong schema, absent binary, and checksum
failure recover without manual deletion. Verify full build identity and supported
target metadata. Concurrent rebuilds, lookup, repair, and cleanup never expose
partial entries or remove another valid publication. Unknown or colliding entries
are handled explicitly. Failed builds leave no executable partial result.

### GORN-V1-06 — Key the cache on the effective build profile

**P0 · Improvement · L · Depends: 07, 09**

The current key uses supplied path, bytes, runner compilation Go version, and
runner OS/architecture. Actual builds invoke `go` from PATH with inherited build
settings. Generator changes and the selected compiler can change without a key
change. Manifest Go/target metadata also describes the runner, not necessarily
the produced binary.
Evidence: [key](../../pkg/app/key.go), [build](../../pkg/cache/emit.go),
and [manifest construction](../../pkg/cache/manifest.go).

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

`require` rejects `latest`, but every cold build runs `go mod tidy`, which can
discover undeclared imports. The preamble imports `github.com/gornkit/gorn/sh`
without generating a corresponding pinned requirement. Cold rebuilds can
therefore resolve code not identified by the script's explicit requirements.
Evidence: [generation](../../pkg/script/generator.go),
[module template](../../pkg/script/mod.gotmpl), and [build](../../pkg/cache/emit.go).

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

Build commands and script execution have no context path. `sh.Process.Kill`
kills only the shell; descendants can retain IO and delay `Wait()`.
Evidence: [build subprocesses](../../pkg/cache/emit.go),
[runner execution](../../pkg/cli/runcmd.go), [command](../../sh/command.go),
and [process](../../sh/process.go).

Acceptance: define cancellation for dependency downloads, builds, scripts, and
`sh` commands. Specify supported process-tree behavior per OS, bounded waiting
on inherited pipes, exit statuses, and reap/temporary-directory ownership.
Ctrl-C and termination stop supported work predictably without publishing
unfinished builds. Test children holding output open and repeated termination.
Document any unavoidable platform differences before stabilizing the API.

### GORN-V1-09 — Normalize source and cache paths at the boundary

**P1 · Fix · M · Depends: none**

Comments describe resolved absolute source paths, but `app.New` retains the
supplied path; CLI input is not made absolute. stdin is represented as
`/dev/stdin` on every platform. A relative `GORN_CACHE` is also accepted, and an
explicit unusable override silently falls back to another root.
Evidence: [source](../../pkg/app/source.go), [source reading](../../pkg/cli/runcmd.go),
and [cache location](../../pkg/cache/cache.go).

Acceptance: choose absolute source identity, a documented symlink policy, and a
portable synthetic stdin name. Diagnostics identify the original source, not a
build-directory-relative path. Cover same-named scripts in different directories,
spaces, Unicode, CRLF, and Windows paths. Runtime cwd remains the invocation cwd.
Normalize or explicitly reject relative cache overrides; an explicit unusable
override returns a useful error. Document stdin-source versus runtime-stdin use.

### GORN-V1-10 — Close directive validation and specification gaps

**P1 · Fix · S · Depends: 03, 07, 25 decision**

The authoritative spec permits one preamble directive; the parser accepts
duplicates. Go/module/version tokens are otherwise largely stored verbatim.
Evidence: [directive handling](../../pkg/script/parser.go) and
[format contract](../specs/gorn-script-format.md).

Acceptance: reject repeated preamble directives; define duplicate/conflicting
requirements and validate supported module/version/Go-version syntax before a
build. Preserve valid pseudo-versions and major-version module paths. Do not
change the documented permissive shebang rule without an explicit format
decision. Update the authoritative contract and add line-specific regressions.

### GORN-V1-11 — Explain slow work and failures while they happen

**P1 · Improvement · M · Depends: 04, 07–08**

`CombinedOutput` buffers tidy/build activity; even verbose cache-miss output is
emitted only after the build. A first run can appear stalled.
Evidence: [build output](../../pkg/cache/emit.go) and [run diagnostics](../../pkg/cli/runcmd.go).

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

Current CLI success tests largely use print flags, cache tests stage fake
binaries, and one generator test compiles via a local module replacement. These
do not exercise the production run/build path end to end.
Evidence: [CLI tests](../../main_test.go), [cache tests](../../pkg/cache/cache_test.go),
and [generator build test](../../pkg/script/generator_test.go).

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
flow. Authoritative format examples call `sh.Glob` and `sh.MustRead`, which do
not exist. Fix those examples using current APIs or the standard library.
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
source model. If 25 is adopted, format one ordinary Go unit without section splitting.

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

Review the standard-main proposal above. Recommended target: optional shebang,
metadata header comments, then `package main` and source-authored `func main()`.
Keep optional preamble behavior as a distinct, documented extension.

Acceptance: record the format decision before changing the current contract.
If adopted, remove section splitting, synthetic main generation, and body
indentation. Parse Go syntax using standard tools; support helpers before and
after main, empty main, and ordinary package declarations. Surface missing,
duplicate, or invalid entry points through source-oriented diagnostics. Recognize
metadata directives only in actual header line comments. Diagnose misplaced
metadata in actual standalone line comments; literals/block comments remain
content. Test shebang, CRLF/EOF,
raw strings, preamble conflicts/import placement,
and source locations before/after generated insertions. Update maintained
fixtures, CLI diagnostics, README, format spec, and generator/parser contracts
together. Cache identity changes invalidate prior generation. Document actual
highlighting/formatting/LSP limits. No migration tooling, dedicated legacy
diagnostics, permanent dual grammar, or automatic extension change is required.

Implementation model: **GPT-6.1 Sol (`gpt-6.1-sol`), high reasoning**. This is
proposed format work, not authorization to implement it.

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

## Decisions to settle before beta

1. **Source format (25):** whether to adopt standard `package main`/`func main()`,
   and the metadata header and preamble rules.
2. **Dependency identity (07):** how a script obtains matching `sh`, what must
   be declared, and whether reproducibility needs a companion artifact.
3. **Build environment (06):** supported Go floor, inherited settings, target
   behavior, and cache identity across gorn upgrades and development builds.
4. **Process behavior (08, 15):** cancellation, process trees, supported shells,
   login startup defaults, and PowerShell native-command failures.
5. **Compatibility and distribution (20–21):** public Go packages, supported
   OS/architecture matrix, install prerequisites, and release policy.

Resolve these in small authoritative specs before implementation spreads their
assumptions across commands. No broad architecture rewrite is required merely
to start fixing inspection or source-semantics bugs.

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
