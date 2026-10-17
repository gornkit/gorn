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

**Next implementation slice:** versioned releases (21),
following the selected [build/runtime contract](../specs/gorn-build-runtime.md).
Inspection (01), source format (02–03, 25), execution/source identity (04, 09),
cache ownership/repair (05), profile (06), dependency/helper delivery (07),
lifecycle (08), metadata validation (10), progress (11), binary export (12), and cache management (13), shell/exact-argv stabilization (14–15), installed workflows
(18), onboarding (19), compatibility (20), and measured startup (22) are implemented.
Follow the [delivery workflow](v1-next-steps.md) for bounded implementation and
validation.

### GORN-V1-01 — Make inspection incapable of executing a script

**P0 · Fix · S · Depends: none**

**Status: implemented.** Inspection returns after parsing and generation, before
opening or looking up the cache. The previous warm-cache path could execute user
code before checking print flags. Artifact and header write failures, including
short writes, now propagate.

Evidence: [run dispatch](../../internal/cli/runcmd.go),
[write-error tests](../../internal/cli/runcmd_test.go), and
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

Evidence: [generation](../../internal/script/generator.go) and its
[compile/run and diagnostic tests](../../internal/script/generator_test.go).
Acceptance covers raw strings, blank lines, attached comments, CRLF, original
formatting, and equivalence with ordinary Go, including original source locations.

### GORN-V1-03 — Recognize directives only as actual source comments

**P0 · Fix · M · Depends: 25 decision**

**Status: implemented through 25.** The original
line-based parser mistook directive-looking literal/comment contents for metadata.
The parser now uses actual Go comment tokens and accepts standalone metadata only
in the header before the package clause.

Evidence: [parser](../../internal/script/parser.go) and its
[lexical/position regressions](../../internal/script/parser_test.go). Tests cover raw
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
trade-offs, rationale, and verification. Evidence: [status mapping](../../internal/cli/status.go),
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
[publication/acquisition](../../internal/cache/emit.go),
[ownership/cleanup](../../internal/cache/ownership.go),
[behavioral tests](../../internal/cache/ownership_test.go), and
[Windows sharing-violation test](../../internal/cache/cleanup_windows_test.go).
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
[profile tests](../../internal/buildprofile/profile_test.go),
[flag grammar](../../internal/buildprofile/flags_test.go),
[installed-CLI regressions](../../profile_integration_test.go), and
[manifest gates](../../internal/cache/cache_test.go).

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
allowlist delivered under item 06. Evidence: [resolver and file-proxy regressions](../../internal/cache/resolve_test.go),
[bundle tests](../../internal/script/bundle_test.go), and
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
[source tests](../../internal/app/source_test.go),
[cache-location tests](../../internal/cache/cache_test.go), and
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
policy and alternatives. Evidence: [metadata regressions](../../internal/script/metadata_test.go)
cover syntax, cardinality, original physical lines, underlying validation causes,
and operation without Go. Dependency resolution/helper delivery are recorded in item 07.
Evidence: [directive handling](../../internal/script/parser.go) and
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
errors, redaction, and stdout/runtime integrity. Lock-wait phases use this same
reporter through item 05’s cancellable locks.

Acceptance: provide immediate phase information and periodic status during long
work on stderr, with a documented quiet mode for automation. Stream useful Go
errors; identify dependency, toolchain, compilation, and script failures.
Keep stdout usable by pipelines. Explain missing Go, unavailable shells, proxy
failure, and cache permissions. Source errors include the correct script line;
raw generated output is available deliberately rather than dumped by default.

### GORN-V1-12 — Implement standalone binary export

**P1 · Feature · M · Depends: 04–08**

**Status: implemented.** `gorn build <script> -o <path>` shares run's validated
source/profile/dependency/leased-cache pipeline and never executes script code.
Cold, warm and bypass exports are host-native pure-Go binaries; stdin works.
Completed same-directory copies publish with atomic no-replace hard links or
explicit `--force` native rename. Directories, symlinks and source aliases are
refused; failures before publication preserve existing output. Windows adds
`.exe` only without an explicit extension. Output parents must exist; mode 0755
provides Unix execution, and Windows inherits destination-directory ACLs.

[The selected export contract](../specs/gorn-build-runtime.md#standalone-binary-export)
records alternatives, filesystem/native-runtime assumptions, cancellation,
commit-point semantics, permission and naming decisions. Evidence includes
[unit policy and concurrent publication tests](../../internal/cli/export_test.go) and
[installed-CLI cold/warm/bypass, standalone and bundled-helper tests](../../build_integration_test.go).
Inspection still returns before the extracted build preparation path. Public
help advertises implemented build flags; cross-compilation remains outside v1.
`mise run test` passes in the documented private Git/TMPDIR fixture; focused race
coverage, target lint for Windows/macOS, dependency invariants and documentation
checks pass. Independent review of the P0 preparation extraction found no
behavioral regression. Native publication tests run in hosted CI before landing.

### GORN-V1-13 — Implement inspectable, bounded cache management

**P1 · Feature · M · Depends: 05–06, 09**

**Status: implemented.** `cache path/list/clean/prune` use shared read-only root
selection and require no Go. Schema-1 JSON reports ownership separately from
validity, sanitized recorded provenance, complete/incomplete logical sizes, and
separate entry/stage/quarantine/unmanaged/metadata totals. Clean resolves one key
or explicitly selects all owned data; age/size prune uses build time, finite
oldest-first selection, explicit `--yes`, read-only prediction, nonblocking
activity protection, generation revalidation and final unmet-budget reporting.
Unknown data and old layouts remain untouched. Windows sharing failures retain
ownership for later retry.

[The authoritative contract](../specs/gorn-build-runtime.md#cache-inspection-and-maintenance)
records alternatives, trade-offs and rationale for deterministic root selection,
logical bytes, build age, explicit mutation, preview limits, ownership authority,
nonblocking cleanup and bounded concurrent maintenance. Evidence includes
[inventory](../../internal/cache/inventory_test.go),
[maintenance](../../internal/cache/maintain_test.go),
[Windows deletion failures](../../internal/cache/maintain_windows_test.go),
[CLI contracts](../../internal/cli/cachecmd_test.go), and
[installed no-Go/active-run tests](../../cache_integration_test.go).
Independent destructive-path review found and corrected unreadable-namespace
membership accounting: hidden owned bytes cannot appear as a satisfied zero-byte
budget. `mise run test` passes in the documented private Git/TMPDIR environment;
focused race checks, Windows/macOS target lint, dependency invariants, frontmatter
and local-link checks pass. Native three-platform CI remains the landing gate.

### GORN-V1-14 — Add exact argv execution to `sh`

**P1 · Feature · M · Depends: 08, 15**

Add a bounded `sh.Cmd(name string, args ...string)` entry point using the existing
command configuration and result model. Ordinary commands should not require
constructing shell strings. Keep shell execution available for shell syntax.

**Status: implemented; core API delivered with 08, final verification with 15.** Exact arguments use the same immutable
configuration, IO/env/dir, cancellation, status, and startup path as shell snippets.
[Tests](../../sh/command_test.go) cover spaces, Unicode, quotes, empty arguments,
metacharacters, executable paths, independent copies, IO/env/dir, and failures.
Supported-shell verification is delivered with 15; compatibility policy remains
in 20.

Acceptance: arguments containing spaces, quotes, wildcard/metacharacters, and
empty values arrive literally without shell interpretation. IO/env/dir,
cancellation, start failure, and nonzero status follow the same documented
contract. Test Windows argv and executable paths. Retain stdlib-only dependencies;
do not add implicit globbing, argument flattening, or a pipeline abstraction.

### GORN-V1-15 — Stabilize and verify the small shell API

**P1 · Improvement · M · Depends: 08; review API before 14**

**Status: implemented.** The [authoritative helper contract](../specs/gorn-sh.md)
selects non-login automation defaults, explicit NewShell profile opt-in,
shell-specific Strict, setup ordering and limits, environment/PATH/cwd/IO,
status-only results and Wait/Kill/exit behavior. Windows recognition handles
absolute paths, extensions and casing. Explicit Cmd snippets use native raw
script transport; exact Cmd arguments retain Go's normal argv path. Embedded
helper identity invalidates generated builds after this source change.

[Named-shell tests](../../sh/shell_behavior_test.go) require Bash/Sh on Linux/macOS,
Zsh on macOS, and Cmd/PowerShell 7 on Windows; optional shells skip explicitly.
[Windows tests](../../sh/shell_behavior_windows_test.go) cover quoting with spaced
executable paths, delayed expansion, PowerShell native exit behavior and process
attribute preservation. Existing exact-argument and process-lifecycle coverage
remains intact. The specification records the alternatives and trade-offs for
login startup, inherited hooks, strictness and Windows quoting. `mise run test`
passes with the documented private Git/TMPDIR fixture, including embedded-helper
execution and lifecycle regression tests. Race checks, Windows/macOS target lint,
dependency invariants and documentation checks pass. Independent P0 transport
review found no remaining issue; native three-platform CI is the landing gate.

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

**Status: implemented.** Installed tests cover plain and bundled-helper scripts,
explicit dependency cold/warm/bypass builds, arguments/stdin/cwd/status,
inspection safety, corrupt-manifest/binary repair, concurrent publication,
cache maintenance and standalone exports. Controlled loopback dependencies keep
resolution deterministic. After cache population, the proxy deliberately fails;
warm/bypass/repair/export still succeed without requests. Concurrent installed
CLIs report one miss and one hit. Saved graph and repair quarantine are asserted.

Evidence: [dependency workflows](../../dependency_workflow_test.go),
[onboarding workflows](../../workflow_integration_test.go),
[execution](../../execution_integration_test.go),
[export](../../build_integration_test.go), [cache](../../cache_integration_test.go),
[profile](../../profile_integration_test.go),
[progress](../../progress_integration_test.go), and existing process/cache tests.
POSIX installed cancellation/terminal tests and native Windows backend tree/fallback
and sharing-failure tests run on their supported hosts; their guarantees remain
platform-specific. Actual published-helper resolution remains release gate 21. Default `mise run test`
passes without environment overrides; focused installed-workflow race checks,
Windows/macOS target lint, dependency invariants and documentation checks pass.
Native three-platform CI remains the landing gate.

Ordinary-Go compiler-oracle fixtures now initialize private valid Git roots.
This resolves the observed invalid ancestor .git issue at its test boundary,
retains normal VCS stamping, and removes the need for caller TMPDIR/global
GOFLAGS overrides. The runner does not require Git for stdlib/bundled execution.

### GORN-V1-19 — Make the first-run documentation executable

**P1 · Improvement · M · Depends: 07, 12, 14–15, 18**

**Status: implemented.** README now leads with Go/PATH installation, a stdlib-only
first script, explicit Windows invocation and Unix shebang, then run/build/cache
and optional helpers/dependencies. The authoritative
[workflow guide](../specs/gorn-workflows.md) documents permissions, network/offline
behavior, host dependence, stdio/cwd/status, cancellation and troubleshooting.
Alternatives and rationale explain stdlib-first onboarding, mutable development
installation before an approved release, export as the no-Go runtime boundary,
and deterministic tests versus published release smoke.

[Maintained examples](../../examples/hello.gorn) and their
[helper counterpart](../../examples/helper.gorn) match README source and execute
through the built CLI. Tests also exercise bypass, inspection, JSON cache preview,
export, no-Go standalone execution and a real Unix shebang. All complete Go blocks
in README and the format specification compile in
[documentation tests](../../internal/script/generator_test.go).
Current format, runner/cache, workflow and helper contracts live outside the
frozen archive. Versioned distribution and source-install version reporting
remain in 21; supporting implementation packages and API policy remain in 20.

### GORN-V1-20 — Define the v1 compatibility surface

**P1 · Architecture · M · Depends: 07, 10, 14–15, 25 decision**

**Status: implemented.** [The compatibility spec](../specs/gorn-compatibility.md)
selects source/metadata/preamble, documented CLI behavior, schema-1 cache JSON,
standalone export and public `sh` as the v1 surface. Implementation packages move
from `pkg/` behind Go's enforced `internal/` boundary without pre-v1 forwarding
packages. Supported human diagnostic meaning/streams/status remain distinct from
unstable wording/layout; raw cache formats and implementation structs stay private.
The spec records alternatives, trade-offs, deprecation/upgrade policy and identity
invalidation for generator/helper/profile changes.

The installed compiler floor now enforces the existing module/install minimum
Go 1.26.4 before identity/cache lookup. Script/bundled language defaults remain
1.26; source minima are never silently raised for dependencies. Tests exercise
an older-reporting Go launcher and keep the compiler floor aligned with go.mod.
The [real dependency-graph check](../../dependency_boundary_test.go) enforces
stdlib-only `sh` and no root/internal graph import of `sh`. Maintained imports,
links, layout instructions and mirrored-process comments move together; archived
documents stay frozen. Existing CLI/helper digests invalidate affected cached
identities without adopting old layouts. Default `mise run test`, focused race
checks, Windows/macOS target lint, dependency-graph/embedding/mirror checks and
documentation checks pass. Independent P0 review verified every moved production
file after import/comment normalization and found only the intended floor check
as a behavior change. Native three-platform CI remains the landing gate.

### GORN-V1-21 — Deliver and smoke-test versioned releases

**P1 · Delivery · M · Depends: 12, 18–20, 22**

**Status: implementation in progress.** The [release contract](../specs/gorn-release.md)
selects candidate-first `v1.0.0-rc.1` then `v1.0.0`, five archive targets, native
three-platform verification, immutable tags/assets and gated Latest promotion.
It records alternatives, trade-offs, the unavoidable public-module tag ordering
risk and recovery. Contributor tooling, reusable CI, published-asset/module
smokes and [upgrade notes](../releases/v1.0.0.md) are delivered together. Actual
candidate and stable publication evidence remains required before completion.
Local preview cross-builds cover all five targets; Linux downloaded-archive-shaped
verification covers checksums, modes, version, arguments and no-Go export. Stripped
candidate-flag preview warm p95 is 50.31–51.88 ms across the three fixtures (50
samples each), below the 100 ms reference budget. Focused version/identity,
archive/checksum, poisoned resolver-environment, preview/wrong-revision and real
dependency-boundary tests pass. Independent P0 review closed four gate gaps:
checksum bypass settings, tag provenance, dispatch CI and compiled helper origin.
An existing inspection test exposed transient Linux ETXTBSY when truncating a
just-exited executable; its corruption fixture now replaces that inode and twenty
repeats pass. Final default `mise run test` passes in 28.80 s; Windows/macOS target
lint, dependency/embedding/mirror boundaries, documentation checks and frozen
archives pass. Hosted native CI and real published-module/asset smokes remain
explicit delivery gates. Initial native CI exposed Go's clean-checkout module
version derivation, which the local worktree does not carry. Version selection now
ignores VCS-derived local labels without an explicit stamp, preserving `dev` while
retaining actual source-install tag/pseudoversion labels; both paths have coverage.
A clean private Git directory reproduces the hosted build conditions and its real
CLI/version/cache-identity checks pass; the final full gate passes in 28.74 s.

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

**Status: implemented.** [The startup spec](../specs/gorn-startup.md) records
reference hardware/OS/Go, exact CLI digest and checkout state, raw distributions,
reproduction commands, alternatives, limitations and the selected **100 ms p95
full quiet-CLI warm budget on the reference host**. Observed warm p95 is
53.56–54.21 ms across tiny, preamble/helper and dependency fixtures, with fifty
warm samples each. Direct-program medians are 0.78–0.96 ms. Empty-Go-cache,
populated-Go-cache cold and bypass distributions separate compilation from reuse.

[The internal benchmark tool](../../internal/startupbench/main.go) owns private
Go/Gorn caches and a deterministic local module proxy. Warm runs make zero
requests and retain the same publication; actual public-module transport remains
release smoke work. Means, p95, min/max and all samples are saved in
[the reference data](../benchmarks/2026-10-17-startup-linux-amd64.json).
The measured budget leaves headroom without weakening full integrity validation;
no optimization is selected. Shared CI does not assert a portable timing limit.
Candidate flags/bytes require a warm reference recheck before release. The full
benchmark and short smoke pass, raw statistics are independently recomputed, and
default `mise run test`, Windows/macOS target lint, dependency boundaries and
documentation checks pass. Native three-platform CI remains the landing gate.

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
| Check/eject commands (16–17) | Build already compiles without execution, and standard source plus inspection provides an ordinary-Go escape path; extra authoring/export UX is not required for v1 | Repeated CI or growing-script workflows justify dedicated commands |
| Format/dependency convenience (23–24) | Preserved ordinary Go and explicit concrete requirements cover the core workflow; rewriting/prefetch commands add another user surface | Authoring evidence shows enough recurring friction to justify source mutation helpers |
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
3. **Process behavior (08, 15):** delivered ownership/cancellation, foreground
   terminal handoff, non-login defaults and shell status contracts, with real
   POSIX terminals and native Windows process/sharing tests.
4. **Compatibility (20):** selected public source/CLI/JSON/helper surfaces and
   enforced private implementation packages, dependency invariants and Go floor.
   **Distribution (21)** remains the final required release delivery slice.
5. **Startup (22):** measured all cache states and selected a reference-host
   budget from recorded distributions, retaining integrity guarantees.

The design constrains implementation; behavioral verification establishes when
its backlog items are complete. Release-wide validation remains required even
when focused implementation tests pass.

## v1 release gate

**Pass condition:** all P0 and P1 items are complete, with linked implementation
and validation evidence. Optional work can be omitted without delaying the
release. Specifically:

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
