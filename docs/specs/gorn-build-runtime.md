---
title: Gorn v1 Build and Runtime Contract
status: authoritative
type: design
updated: 2026-10-17
summary: Selected dependency, build identity, cancellation, cache ownership, and progress contracts for the remaining v1 implementation.
---

# Gorn v1 build and runtime contract

This document selects the contracts for GORN-V1-05–08 and 10–15. The
[backlog](../planning/v1-backlog.md) records implementation and verification;
selecting a contract does not mark its implementation complete. The
[source format](gorn-script-format.md) and
[execution decisions](../planning/v1-execution-plan.md) remain applicable.

Implementation recommendation: GPT-6.1 Sol, high reasoning, with GPT-6 Astra at
high reasoning for cross-cutting design review and independent P0 review of
identity, dependency resolution, and process ownership.

## Delivered dependency policy

GORN-V1-07 implements the dependency/helper policy below. Evidence includes
[bundle identity/materialization tests](../../pkg/script/bundle_test.go),
[maintained-source embedding checks](../../helper_embed_test.go),
[real file-proxy graph tests](../../pkg/cache/resolve_test.go), and
[installed-CLI offline runs](../../dependency_integration_test.go). These cover
lazy transitives, MVS upgrades, module cycles, missing direct/graph declarations,
published/bundled helper modes and conflicts, source Go floors, populated offline
rebuilds, and private-proxy transport diagnostic redaction. Completed builds save
the effective module/checksums and schema-1 `dependencies.json`. Generation key
version 4 and manifest schema 2 also bind the delivered profile below.

The full local gate passes in the documented private Git/TMPDIR environment.
Cancellation, progress, and cache ownership remain separate implementation items.
Workspace/toolchain control, readonly builds, and generated VCS/PGO policy are
applied through the frozen profile.

## Delivered build profile

GORN-V1-06 discovers and validates one immutable profile before every lookup,
including warm runs. It fingerprints the installed Go executable, reported
version/GOROOT/architecture settings, normalized supported flags, source Go floor,
helper bytes, dependency policy, resolver configuration, and executing CLI bytes.
Provenance describes the actual compiler, with opaque digests for resolver
settings and linker values. Manifest schema 2 checks the complete profile
fingerprint and host target; generation version 4 excludes older identity.
Occupied-entry repair/leases remain item 05.

[Profile tests](../../pkg/buildprofile/profile_test.go),
[Go flag grammar tests](../../pkg/buildprofile/flags_test.go), and
[installed-CLI behavior](../../profile_integration_test.go) cover frozen GOENV,
SDK/user/process precedence, normalized flag reuse, private linker values,
same-version generator changes, selected compiler-path changes, foreign targets,
unsupported inputs on warm runs, and real versus literal embed directives.
[Manifest regressions](../../pkg/cache/cache_test.go) reject changed schemas,
profile contents, identities, and targets. The full local gate passes with the
documented Git/TMPDIR isolation and normal project VCS stamping.

Configuration decisions:

- A PATH launcher may identify an SDK through `go env`; subsequent commands use
  the actual SDK directly. A real Go executable selected directly from PATH
  retains its identity when sharing another GOROOT. Re-running a mutable launcher
  for every phase could select different compilers.
- Read SDK `GOROOT/go.env` defaults, then user GOENV entries, then nonempty
  process overrides. Empty process values follow Go's fallback semantics;
  whitespace GOFLAGS explicitly means no flags. Disable user GOENV after discovery
  and pass captured settings. Later configuration edits cannot redirect the
  operation. Installed SDK/CLI files are trusted, immutable inputs during discovery
  and execution.
  Re-query every captured setting after freezing; if an empty value would revive
  an SDK default (for example saved `GOPROXY=`), reject the profile with the setting
  name and an explicit-value/reset remedy. A private frozen GOENV file could
  preserve those unusual empty overrides, but adds secret-bearing temporary
  files and lifecycle ownership to every warm run. Bounded rejection preserves
  the supported-value contract without silently changing configuration.
- Support a single `-tags=<list>`, `-trimpath[=<bool>]`, and `-ldflags=<flags>`.
  Sort/deduplicate tags; duplicate top-level options fail instead of adopting Go's
  last-value override. Linker flags are positive `-s`, `-w`, and
  `-X importpath.name=value`/`-X=importpath.name=value`. Sort independent assignments,
  reject conflicting assignments, and preserve literal values using Go quoting.
  Shell escaping is not applied. Broader linker options admit external inputs
  that this profile cannot identify.
- Reject explicit cgo/custom compiler settings, external-link forcing, compiler
  debug hooks, SSA dumps, experiments, FIPS, and external cache programs. Set cgo
  off and internal linking explicitly. Discovery queries stable fields and
  disables GOFLAGS so unsupported build hooks cannot execute during validation.
  Windows environment names are case insensitive.
- GODEBUG remains inherited script runtime configuration. Go build commands clear
  it, keeping compiler/debug behavior outside ambient runtime settings. Runtime
  arguments and unrelated environment do not enter identity.
- Reject source `import "C"` and real `//go:embed` comments with original paths
  and physical lines. Directive-looking strings/block comments remain ordinary
  Go. Inspection prints source/module plans without requiring Go or a supported
  compiler profile. Dependency modules may embed assets.

Alternatives: inheriting mutable configuration risks a different build after
lookup; ignoring requested flags silently changes intent; accepting every flag
requires tracking arbitrary files, compilers, and linkers. The bounded profile
supports useful pure-Go builds while rejecting unidentifiable requests. Opaque
resolver settings can invalidate reuse after authentication/configuration changes;
this conservative cost avoids assuming differently configured module sources are
interchangeable and keeps credentials out of provenance.

## Dependencies and helper delivery

Every direct nonstandard import needs a concrete `//gorn:require` module
requirement covering that import, except the bundled `github.com/gornkit/gorn/sh`
package described below. Direct-import coverage is checked during the build
phase, after toolchain discovery. Coverage means the module path itself or a
slash-delimited descendant. An incidental transitive requirement does not replace
an explicit declaration for a direct import. The selected installed toolchain
identifies standard packages. Missing declarations report the original import
line and the requirement syntax needed to correct them.

Requirements are Go module minimums under MVS, with concrete semantic versions,
prereleases, or pseudo-versions. Reject queries, branches, bare revisions,
`latest`, malformed paths/versions, incompatible major paths, and duplicate
requirements, including identical duplicates. Duplicate errors identify both
lines. Header and import validation uses Go comment/syntax positions and keeps
print-only inspection independent of Go, network, and cache access. Standard
package classification using the compiler belongs to the build phase.

Use the Go project's `golang.org/x/mod` module/modfile/semver APIs in the CLI for
validation and generated module editing. This justified CLI dependency avoids
maintaining a partial Go module grammar. It must be pinned to a version supported
by the repository toolchain. It is never a dependency of `sh`.

The CLI embeds the maintained production `sh` source as data, including platform
files and excluding tests. It does not import `sh`. For default helper use,
materialize a local child module named `github.com/gornkit/gorn/sh`, require its
synthetic concrete version, and add a generated relative replacement. Hash the
ordered file names and exact bytes; record that digest as bundled dependency
identity. Copying assets leaves the user's source and imports intact.

An explicit concrete requirement of the parent `github.com/gornkit/gorn` module
selects external helper mode. Use Go to resolve that published module without a
bundled replacement. Reserve the child module requirement for bundled delivery;
reject explicit child requirements with a remedy to pin the parent. If bundled
mode's transitive graph selects the parent module, fail with the same remedy.
This prevents ambiguous imports and silently mixing helper APIs. Compiler errors
report an external helper version that lacks the requested API.

Resolve only the declared pinned graph. Download module metadata, enumerate
selected modules, add selected graph requirements to the generated root, and
repeat until the selected path/version map reaches a fixed point. Check errors
before consuming partial JSON. Preserve the source Go language floor and reject
a dependency/helper whose required Go floor is higher, with a remedy to raise
`//gorn:go`. Check generated go.mod after resolver commands: Go can raise its go
directive during downloads, so readonly final build alone is insufficient. Do
not silently accept that change or an introduced toolchain directive. Preserve
direct declarations separately from expanded requirements and selected versions. Build with `-mod=readonly`.
Never run `tidy`, `get`, or missing-import discovery that can select an undeclared
latest dependency. Missing graph dependencies fail with a declaration remedy.
Go remains the resolver, version selector, and checksum authority.

Save effective go.mod, go.sum, selected modules, declared requirements, and helper
identity with completed builds. No companion lock artifact is required in v1.
Concrete references and Go checksums constrain resolution, but are not a promise
of bit-for-bit output across arbitrary Go versions, platforms, private-module
servers, or mutable local caches. Cold/offline builds require populated Go caches;
a valid Gorn cache hit does not redownload modules. Warm reuse still requires
installed Go to establish the effective build profile. Exported binaries require
neither Go nor Gorn.

Alternatives considered:

- Published helper only: simple module resolution, but unreleased development
  and offline helper use would depend on a published version.
- Replacing the entire parent module with a sh-only copy: short wiring, but
  shadows unrelated packages and transitive parent-module requirements.
- Rewriting helper imports or copying functions into user source: reduces local
  module setup while violating source preservation and ordinary package scope.
- A portable lock companion: stronger cross-machine provenance, but adds another
  author-managed file. Save resolved provenance first and revisit only if beta
  workflows need portable exact resolution.

The chosen bundle supports development and offline use while explicit parent
requirements provide a deliberate published-version escape hatch. Expanding the
whole selected graph can download host-unused modules; this cost is preferred to
an independent package resolver or implicit import discovery.

## Effective build profile

Use one immutable, versioned profile for cache identity, subprocess configuration,
and provenance. Discovery finishes before lookup. It records the actual installed
Go executable path and digest, reported version, GOROOT, host/architecture feature
settings, supported effective flags, dependency policy identity, bundled helper
digest, and generator identity. Hash the executing CLI binary so development
builds with the same version string cannot share another revision's generation.

Freeze supported configuration after discovery. Do not reread mutable GOENV
settings for each subprocess. Preserve Go's configured module/cache/authentication
mechanisms while keeping configured credentials out of progress and provenance.
Sanitize resolver diagnostics before streaming rather than echoing credential-bearing URLs.
Record sensitive resolver configuration through opaque digests or redacted
identifiers. Ordinary script arguments and runtime environment are excluded.

The initial profile is deliberately bounded:

- Use installed Go with `GOTOOLCHAIN=local`; enforce the source and dependency Go
  minimums. A newer installed compiler does not authorize raising the declared
  source language floor to satisfy a dependency. The bundled child module uses
  the minimum Go language version its source needs; CLI toolchain patch minimums
  are a separate prerequisite. Automatic toolchain downloads are outside this
  profile.
- Use module mode and `GOWORK=off`. Generated modules build with `-buildvcs=false`
  because incidental ancestor repositories are not script inputs. This is a
  product build policy, not a global test-environment override. The project gate
  itself retains normal VCS stamping.
- Run and export for the host GOOS/GOARCH. Reject foreign target requests before
  execution/export. Architecture feature settings are explicit identity inputs;
  the author remains responsible for selecting an ISA the host CPU supports.
- Build pure Go with controlled `CGO_ENABLED=0`. Reject explicit enabling,
  `import "C"`, cgo/custom-compiler settings, race instrumentation and sanitizers.
  Standard packages may select their pure-Go implementations.
- Support normalized `-tags`, `-trimpath`, and linker flags limited to `-s`, `-w`,
  and `-X importpath.name=value`. Parse Go's quoting rules, preserve values, and
  reject malformed or conflicting inputs. Key the complete effective values;
  avoid displaying linker values that may contain application secrets.
- Reject compiler/assembler overrides, overlays, modfiles, alternate package
  directories, toolexec, external linker/PGO files, and other unsupported flags.
  Set PGO off. Reject nondefault experiments/FIPS settings until explicitly
  supported and fingerprinted.
- Reject genuine source `//go:embed` directives and relative/local imports with
  source locations. Versioned dependency modules may embed their own assets.
  Source-adjacent compilation files are outside the single-file unit; runtime
  file IO remains ordinary Go behavior.

Toolchain installations and Go's module/build caches are trusted installed build
inputs. In-place mutation of compiler/linker/stdlib files without a toolchain
change is outside the supported cache contract; hashing only the Go launcher
cannot establish arbitrary toolchain-tree immutability.

Alternatives considered: honoring all Go flags and cgo requires identity for
external headers, libraries, compiler programs, and file inputs; ignoring ambient
settings silently changes requested builds. An explicit allowlist with useful
errors keeps supported inputs executable, identifiable, and explainable. Local
only toolchain selection avoids a different compiler appearing after lookup.

## Cancellation and process ownership

GORN-V1-08 implements this lifecycle through `cli.RunContext`,
`buildprofile.DiscoverContext`, `cache.Root.BuildContext`, and immutable
`sh.Command.Context`. Existing entry points use a background context. Root CLI
signal handling is separate from embedding APIs; those APIs install no global
handlers. The exact-argument `sh.Cmd` entry point shares the same process path.
Shell startup/Strict behavior remains item 15.

Evidence: [process and real PTY tests](../../pkg/process/process_posix_test.go),
[concurrent waits and deadlines](../../pkg/process/process_test.go),
[Windows tree/fallback tests](../../pkg/process/process_windows_test.go),
[stalled launcher](../../pkg/buildprofile/context_test.go),
[stalled dependency download/staging cleanup](../../pkg/cache/cancellation_test.go),
[source-reader ownership](../../pkg/cli/input_test.go),
[installed nested-helper and input cancellation](../../lifecycle_integration_posix_test.go),
and [exact argv/IO/configuration tests](../../sh/command_test.go).

Carry context through source collection, discovery, lock/lease acquisition,
downloads, builds, execution, and `sh` commands. Source stdin collection must be
interruptible; the root CLI owns its process input and may close that input on
cancellation. Embedding APIs require an explicit interruptible-reader/ownership
contract and must not silently close caller-owned IO.
`cli.Input` supplies a Reader and an explicit `Interrupt` callback that promptly
unblocks source reads. A cancellable stdin-source invocation without Interrupt or
CollectSource fails before reading. Embedding callers choose their interruption mechanism;
completed reads join that interruption worker before returning. An explicit
`CollectSource(ctx)` callback can instead own source collection separately from
runtime Reader. The root CLI alone uses a process-lifetime collector: on canceled
stdin collection it returns immediately and exits the process, allowing the OS to
retire a blocked inherited-stdin read. This occurs before any cache/build/runtime
resources exist. Inherited blocking stdin's Close does not reliably unblock Read.
Duplicating/nonblocking its shared descriptor could change runtime stdin; native
Windows thread cancellation adds a larger platform/race surface. The root-only
collector avoids both, and is deliberately not the embedding default. File sources
must be regular files; use `-` with the explicit
input contract for streams. Routine filesystem calls and arbitrary caller-owned
IO cannot provide a universal cancellation bound.
CLI interrupt and termination cancel supported work, clean uncommitted staging,
and return 130/143 respectively on POSIX. Never publish a canceled build.
Programmatic cancellation preserves `errors.Is` with context cancellation or
deadline errors; ordinary child failures retain their exit status.

Use immutable `Command.Context(ctx)` configuration and exact-argv
`sh.Cmd(name, args...)` with the same IO/env/dir/result model as shell snippets.
Reject a nil supplied context at Start. Copy argv/configuration. Start one reaper
per process; repeat/concurrent Wait returns the same result. Kill is idempotent
and does not wait while holding a lock needed by the reaper. Caller-owned IO is
never closed. Bound waiting on Go-owned inherited pipes; arbitrary user IO whose
Read/Write blocks forever is outside that bound.

On Linux/macOS, Gorn owns a runtime process group and cancels its members with
TERM followed by bounded KILL escalation. Descendants that deliberately create
another group/session are outside this guarantee. Group lifetime and escalation
are tied to the active operation; do not schedule an unguarded delayed kill after
ownership has ended. Cancellation remains active through group escalation even
if the leader exits on TERM; a same-group descendant ignoring TERM must still be
killed. Leader-reaper completion alone cannot end cancellation ownership.
Ordinary successful script exit permits ordinary background
process behavior; cancellation owns cleanup of the invocation group.

The sole controller observes terminal child exit without reaping using
`waitid(WEXITED|WNOWAIT|WNOHANG)`, polling every 10ms. It retains the leader PID
through a 500ms TERM grace and group KILL, then performs the sole `exec.Cmd.Wait`.
This avoids a delayed negative-PID signal after numeric identity could be reused.
It validates matching PID and terminal `si_code`; stopped/continued reports are
not exit, including on Darwin. Ordinary observation errors terminate the still
owned child before waiting. ECHILD or an unexpected identity means ownership was
lost: report that failure without signaling a potentially reused numeric group.
Embedding programs must not reap these children or enable automatic child reaping.
Kill/context requests arriving after the controller commits to normal reaping
cannot revive ownership. Go-owned pipe waiting is bounded by a 500ms WaitDelay;
caller IO that blocks forever is outside that bound.

Native syscall layouts are bounded to Linux/macOS amd64 and arm64. Other POSIX
architectures reject subprocess startup rather than use an unverified ABI.
Windows uses native process handles. The CLI and self-contained `sh` duplicate
the private stdlib primitive; a consistency test enforces matching source. A
shared runtime import would violate the helper's delivery/dependency boundary.
Polling adds at most approximately one poll interval to normal exit observation,
in exchange for a single serialized controller without an unjoined wait worker.

Bundled `sh` checks a reserved inherited ownership marker against its actual
process group. Inside a Gorn-owned group it keeps commands in that group, so
nested helpers remain subject to whole-invocation cancellation. Per-command
Kill/context cancellation in this mode targets the direct command and bounds
inherited-pipe waiting; whole-invocation cleanup belongs to Gorn. Standalone `sh`
owns a group per command and cancels its members. The marker is coordination,
not isolation. An explicitly selected external helper may create groups outside
the runner's ownership and retains its documented lifecycle behavior.

The reserved variable is `GORN_PROCESS_GROUP`. The runner installs `self` before
exec because the new child PID is not yet known. Bundled `sh` initialization binds
it to a numeric PGID only when that script is the actual group leader. Every Start
rechecks that value against the current group, then propagates the canonical
marker after Command.Env overrides. Missing, malformed, or mismatched values use
standalone ownership. This reserved environment normalization runs no subprocesses
and changes no signal handlers. It also covers ordinary os/exec descendants after
bundle initialization, before the first helper command. A root script that never
imports the bundle cannot bind `self`; separately installed/external helpers in
its descendants retain their own lifecycle contract. Standalone helper commands
own their own groups; this runner handshake is not a general process-tree registry.
Lazy normalization at the first helper call would avoid init-time mutation but
miss earlier raw os/exec descendants. A supervisor/bootstrap protocol would cover
more arbitrary chains at the cost of a new executable protocol and distribution
surface; v1 uses this bounded bundled-helper coordination.

When stdin is the controlling terminal and the caller owns its foreground group,
transfer terminal foreground to the child's owned group and restore it after
completion. Use thread-scoped signal masking for restoration; do not replace the
embedding application's global signal handlers. Verify real terminal input,
Ctrl-C, and restoration with PTYs on both supported POSIX hosts.

Restore only the expected/orphaned foreground group, including when Start fails
after the child changes foreground but before exec succeeds. Restoration masks
SIGTTOU on a dedicated locked thread, restores its exact previous mask, and retires
that thread if mask restoration fails. It never installs global ignore handlers.
The background parent cannot observe a terminal Ctrl-C handled or ignored by a
foreground application. An application handling SIGINT and exiting zero keeps its
successful semantics. Observed SIGINT/SIGTERM termination, and conventional exit
130/143, trigger group cleanup while preserving the child status. Treating those
exit values as interruption is a convention: an intentional exit130/143 receives
the same cleanup. Full Ctrl-Z suspend/resume job control is outside v1; implementing
it would require coordinated stop detection, parent suspension, and re-handoff.

On Windows, use bounded native taskkill tree termination with direct-process kill
fallback and reap the direct child. This is best effort, not Job Object
containment, and makes no promise for already detached/reparented descendants.
There is no POSIX graceful SIGTERM guarantee. Test actual Windows child/grandchild
behavior and fallback paths.

Alternatives considered: per-command groups everywhere allow nested helpers to
escape Gorn cancellation; inheritance everywhere prevents standalone helpers
from owning a command group. Arbitrary tree registration or platform-specific
containment adds a larger API/operational surface. Direct-only terminal cleanup
would be simpler but weaken interactive invocation ownership. The selected
conditional ownership and foreground handoff preserve normal interactive IO and
make the supported cancellation boundary explicit.

Shell defaults are non-login Bash/Zsh (`-c`) for predictable automation. Explicit
NewShell arguments can request login/profile behavior. Strict recognizes known
basenames across absolute paths, Windows extensions, and casing. Document setup
order, immutable configuration, inherited environment and IO, and PowerShell's
handling of native nonzero exits. Keep status-only results.

## Cache publication, repair, and management

Use full content/build keys in cache directory names and a new manifest schema
for the effective profile and dependency provenance. Reject wrong schema, key,
profile, target, nonregular binary, checksum, and unsupported layout. Missing or
invalid entries rebuild without exposing partial output.

Use stable per-entry OS-backed locks outside removable entry directories. Hold
run/export leases from validated lookup through execution/copy; clean/prune must
skip active entries. Never unlink a live lock file and create a second namespace.
Under the mutation lock, revalidate: yield to a valid matching peer; otherwise
quarantine an invalid owned entry and atomically publish a completed candidate.
Failed builds preserve valid publications. Handle Windows executable locking as
an explicit skip/retry/error outcome. Unknown entries and symlinks are diagnosable
and cannot become deletion authority.

Cache path reports the resolved Gorn root. Cache list supports stable JSON
(versioned schema) with identity, source, size, build time, and
sanitized provenance, including corrupt entries. Age pruning uses build time;
usage recency would add a write to every run. Select the management commands:

- `cache clean <key-prefix>` requires one unambiguous key, or `--all` selects all
  owned entries. Mutation requires `--yes`; `--dry-run` previews without it.
- `cache prune --max-age <duration>` selects entries older than the threshold.
  `--max-bytes <bytes>` evicts oldest eligible entries until managed bytes meet
  the limit. With both, remove age-expired entries first, then apply the size
  budget. Require at least one bound, and `--yes` unless previewing with
  `--dry-run`. Report skipped active entries and an unmet budget.
- Size means regular-file bytes under owned entries, excluding Go caches and
  unrelated paths. Report unmanaged/quarantined bytes separately; do not imply
  they satisfy a managed-entry budget. Corrupt entries require independent
  ownership/build-time evidence before automatic mutation or age selection.
- Never follow symlinks for enumeration/deletion. Keep stable lock metadata in
  place; identify abandoned stages through ownership evidence and locks, not
  age alone. Active entries are skipped by cleanup rather than interrupted.

Delete only owned entries under the resolved Gorn root and leave Go caches
untouched. Binary export uses the same validated pipeline and never executes the
script. Require `build <script> -o <path>`; refuse an existing destination by
default. `--force` can replace a regular file, while symlinks, directories, and the
source file itself are always refused. Preserve explicit output paths, adding
`.exe` on Windows only when no extension was supplied. Publish a completed
same-directory temporary file using the platform's atomic publication primitive;
failed builds/publication preserve an existing destination.

Alternatives considered: bare rename handles first publication but not corrupt
replacement or cleanup races; PID/mkdir locks need a separate abandonment and
identifier-reuse protocol. OS-backed lifetime locks plus private staging give
publication, repair, execution, and cleanup one ownership model. Build-time age
keeps warm cache reads free of recency writes. Explicit `--yes` supports
noninteractive automation without a blocking confirmation prompt. Refusing
overwrite by default protects existing output; `--force` provides a deliberate
regular-file replacement policy.

## Progress and verification

GORN-V1-11 implements immediate preparation phases and a two-second stderr
heartbeat for pending source collection, discovery, cache access, dependency
resolution/downloads, and compilation. `-q`/`--quiet` suppresses Gorn's activity
messages, including during inspection. It preserves sanitized Go diagnostics,
script stderr, and explicitly requested verbose diagnostics. Help/version do not
start reporting. Missing Go, source, dependency, compiler, and script failures keep
their phase/cause; internal generated-source dumps require `--verbose`, while
`--print-main` remains the explicit inspect-only artifact path.

[Reporter tests](../../pkg/progress/progress_test.go) cover synchronous initial
output, heartbeat/quiet behavior, joined shutdown, synchronized writers, IO
failure cancellation, complete-line framing, EOF, and oversized diagnostics.
[Resolver streaming tests](../../pkg/cache/progress_test.go) prove redacted
diagnostics appear before a still-running child exits and are not replayed in
its final error. [Installed CLI tests](../../progress_integration_test.go) cover
cold/warm/bypass phases, quiet aliases, compiler failures, blocked-source
heartbeats, slow dependency failures/redaction, stdout integrity, and runtime
handoff. [Background stdin compatibility](../../pkg/cli/runcmd_test.go) keeps
uncancellable embedding reads from acquiring a new ownership requirement merely
because reporting uses an internal cancellation context.

Selected reporting decisions and trade-offs:

- Always announce preparation immediately, rather than use a delayed spinner.
  A fixed two-second heartbeat establishes that work is still pending without
  claiming a percentage or ETA. Plain lines work across terminals and redirected
  logs; quiet supports automation that only wants program/diagnostic output.
- Serialize preparation/artifact/Go-diagnostic writers. Stop and join reporting
  before execution, then pass the original runtime IO objects. Wrapping runtime
  `*os.File` streams would make os/exec substitute pipes and lose terminal identity;
  a parent heartbeat could also write while its child owns terminal foreground.
  The final `running script` phase is immediate; script runtime has no Gorn
  heartbeat and retains ordinary interactive behavior.
- Buffer a diagnostic only until its newline, then sanitize URL userinfo/query
  values before output. Do not stream raw chunks: a credential can span writes.
  At EOF redact a URL in an unterminated fragment in full, since cancellation may
  have cut it off before its credential delimiter. The same EOF policy applies
  to non-reporting library errors. Omit an entire line over 64 KiB, with a fixed
  notice, rather than buffer without limit or expose an unsafe partial line.
- Preserve Go diagnostics even in quiet mode, including any informational Go
  messages. Filtering by Go's human-message wording could hide useful warnings or
  failures as compiler versions change. Toolchain discovery still withholds raw
  private configuration diagnostics under the selected profile policy.
- A reporting/output failure cancels pending Go work and retains its underlying
  IO error with runner status 1. Caller-owned blocking IO still follows the
  lifecycle contract. Existing background stdin reads without an interruption
  callback retain their declared blocking mode; reporting cannot invent authority
  to close or interrupt that reader.

Emit phase information immediately before source/lock waits, discovery, downloads,
and compilation,
and periodic stderr status while slow work is pending. Stream useful Go errors.
Quiet suppresses Gorn progress, not compiler errors or script stderr. Synchronize
progress and subprocess writes, then stop and join the heartbeat before return.
Keep stdout for script/artifact/structured results. Missing Go, unsupported
inputs, dependency, compiler, and script failures identify their phase without
replaying already streamed output or dumping generated source by default.

Implement and verify in bounded slices: dependency validation/bundling (07, 10),
profile identity (06), lifecycle/progress (08, 11), repair/leases (05), export/cache
commands (12–13), and helper execution/shell behavior (14–15). Keep dependent
integration coverage in each slice and complete release-wide coverage under 18.

Required behavioral evidence includes offline file-proxy graphs with lazy
transitives/MVS upgrades/cycles, missing declarations and graph imports, bundled
and external helper conflicts, pseudo-versions/major paths, private-module
credential redaction, changed effective flags/toolchain/generator, rejected
unsupported inputs, and warm runs without downloads. Cancellation tests cover
all phases, inherited pipes, nested helpers, real PTYs, Windows trees, repeated
Wait/Kill, and canceled publication. Cache tests cover concurrent repair/cleanup,
active run/export leases, age/size pruning, explicit deletion/overwrite rules,
wrong schemas/profiles, unknown paths, and failure preservation. Progress tests prove immediate/periodic status and quiet behavior.

The independent architecture review verified a local file-proxy lazy A→B→C graph
requiring iterative root expansion before readonly build, and confirmed that Go's
race build requires cgo. These probes guide tests; they do not establish untested
Windows or terminal behavior. Full behavioral and cross-platform evidence is
required before the corresponding backlog item is complete.

References: [Go modules](https://go.dev/ref/mod),
[module validation](https://pkg.go.dev/golang.org/x/mod/module), and
[process waiting/cancellation](https://pkg.go.dev/os/exec#Cmd).
