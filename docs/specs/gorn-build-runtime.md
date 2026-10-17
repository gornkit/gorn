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
[bundle identity/materialization tests](../../internal/script/bundle_test.go),
[maintained-source embedding checks](../../helper_embed_test.go),
[real file-proxy graph tests](../../internal/cache/resolve_test.go), and
[installed-CLI offline runs](../../dependency_integration_test.go). These cover
lazy transitives, MVS upgrades, module cycles, missing direct/graph declarations,
published/bundled helper modes and conflicts, source Go floors, populated offline
rebuilds, and private-proxy transport diagnostic redaction. Completed builds save
the effective module/checksums and schema-1 `dependencies.json`. Generation key
version 4 and manifest schema 3 also bind the delivered profile below.

The full local gate passes in the documented private Git/TMPDIR environment.
Cancellation, progress, and cache ownership are delivered below.
Workspace/toolchain control, readonly builds, and generated VCS/PGO policy are
applied through the frozen profile.

## Delivered build profile

GORN-V1-06 discovers and validates one immutable profile before every lookup,
including warm runs. It fingerprints the installed Go executable, reported
version/GOROOT/architecture settings, normalized supported flags, source Go floor,
helper bytes, dependency policy, resolver configuration, and executing CLI bytes.
Provenance describes the actual compiler, with opaque digests for resolver
settings and linker values. Manifest schema 3 checks the complete profile
fingerprint and host target; generation version 4 excludes older identity.
Occupied-entry repair and leases are implemented under item 05.

[Profile tests](../../internal/buildprofile/profile_test.go),
[Go flag grammar tests](../../internal/buildprofile/flags_test.go), and
[installed-CLI behavior](../../profile_integration_test.go) cover frozen GOENV,
SDK/user/process precedence, normalized flag reuse, private linker values,
same-version generator changes, selected compiler-path changes, foreign targets,
unsupported inputs on warm runs, and real versus literal embed directives.
[Manifest regressions](../../internal/cache/cache_test.go) reject changed schemas,
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

- Require installed Go 1.26.4 or newer before profile/lookup, including warm runs.
  Use `GOTOOLCHAIN=local`; enforce the source and dependency Go
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

Evidence: [process and real PTY tests](../../internal/process/process_posix_test.go),
[concurrent waits and deadlines](../../internal/process/process_test.go),
[Windows tree/fallback tests](../../internal/process/process_windows_test.go),
[stalled launcher](../../internal/buildprofile/context_test.go),
[stalled dependency download/staging cleanup](../../internal/cache/cancellation_test.go),
[source-reader ownership](../../internal/cli/input_test.go),
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

**Implemented (05):** cache layout/manifest schema 3 uses
`apps/3/<full-64-lowercase-hex-key>`; descriptive source names remain metadata.
The former schema-2 namespace is left untouched and unmanaged. Omitting a slug
reduces Windows path length and removes the old prefix-collision failure.
Generation version 4 is unchanged; the CLI digest already identifies generator
behavior and the new layout prevents reuse of old publications.

Validity and ownership are independent. A matching manifest, complete frozen
profile, host target, regular binary and SHA-256 are required for a hit. An
`ownership.json` schema-1 record separately binds layout 3, full key, stage ID,
kind (`stage` or `entry`), creation time and, for completed entries, build time.
Source is descriptive and confers no authority. Corrupt/missing manifests remain
repairable only when independent ownership is intact. Missing, corrupt,
mismatched or unsupported ownership causes an explicit refusal to replace data;
a key-shaped directory or manifest alone is never deletion authority.

Stable OS-backed locks live outside removable data:
`locks/entries/<key>.mutation`, `locks/entries/<key>.activity`, and
`locks/stages/<stage-id>`. Lock files are never unlinked, including after a crash.
Linux/macOS use nonblocking `flock`; Windows uses nonblocking `LockFileEx` over
the same one-byte range, with separate noninheritable handles for each lease.
Only contention is retried, at 20ms intervals with context cancellation and the
existing progress heartbeat. Unsupported locking/filesystem errors are explicit;
there is no PID-file fallback.

The lock protocol is part of the contract:

1. A warm lookup acquires shared activity, validates, and returns a leased
   `Entry`. A miss releases activity before waiting for mutation.
2. A cold/repair builder acquires exclusive mutation, then rechecks under shared
   activity. A valid peer wins without another build. A miss releases activity
   while mutation remains held during compilation in a private stage.
3. Every stage, including ordinary cached builds, acquires its exclusive stage
   lock before its ownership record becomes visible. A crash before that record
   leaves unknown data that automatic maintenance must skip. Completed ownership
   is written before the validity manifest, which is written last.
4. Publication acquires exclusive activity while mutation remains held. Existing
   run/export leases therefore delay replacement. Revalidate: a valid destination
   wins; an invalid independently owned destination is renamed into quarantine;
   unknown data is refused. Rename the completed candidate into place.
5. Release exclusive activity and reacquire shared activity while still holding
   mutation, then release mutation. Return the shared lease through execution or
   export. No lock upgrade is assumed. `Entry.Close` releases it.
6. Maintenance (13) must try mutation first, then exclusive activity without
   waiting, and skip active entries. A stage cleanup must try its stage lock.
   No mutator may invert this order or wait on mutation while holding activity.

Failed builds never mutate existing publications. If quarantining succeeds but
final publication fails, preserve the invalid quarantined data and report the
error; it remains recoverable, rather than being silently deleted. The final candidate rename is the publication commit point; cancellation is
checked after lock acquisition and before each mutation. Cancellation racing
after that rename may leave a valid publication even when acquiring the returned
lease is canceled. Publication
provides atomic visibility, not a power-loss/fsync durability guarantee.
Quarantine management/reporting remains item 13.

`BuildContext` and `Acquire` return leased capabilities. Bare `Lookup` remains a
read-only validity check and provides no lifetime protection. Copies of an Entry
share idempotent cleanup state, including the first cleanup error. Changing its
exported path fields cannot redirect cleanup. `Remove` is an alias for `Close`:
it removes only private bypass stages, never published entries. Bypass mode
acquires no entry/key locks and never serves a cache hit, but holds its stage
lock until execution and cleanup finish.

Stage cleanup checks a directory identity captured through an open handle and
the stage ID. Capturing through a handle matters on Windows, whose pathname Stat
can otherwise load identity lazily after a replacement has occurred. It removes
payload first, retaining ownership until all payload removal succeeds. Windows
sharing violations therefore leave independently owned residue for later
maintenance; the lock is released and the cleanup failure is reported. A prior
child failure keeps its exit status and receives an additional cleanup diagnostic;
cleanup failure after child success makes the runner fail.

All internal cache directories and ownership/manifest/binary/lock files reject
symlink substitutions for validation; quarantine moves owned directories without
traversing linked payloads. Deletion removes symlinks themselves, never targets.
The operator-selected root may be an alias, including normal platform aliases.
This protects cooperating Gorn processes and accidental corruption; pathname
preflights are not an adversarial same-UID sandbox. Stable lock metadata can grow
with identities/stages; retaining these small files avoids split lock namespaces.

Verification: [cache/ownership tests](../../internal/cache/ownership_test.go) cover
shared/exclusive leases, same-process and killed cross-process lock holders,
serialized real cold builders, cancellable repair while leased, bypass stage
protection, independent ownership failures, symlinks, quarantine retention,
private copied-entry cleanup and replacement identity. The
[Windows cleanup test](../../internal/cache/cleanup_windows_test.go) requires a real
sharing violation to preserve ownership and release the stage lock. These tests
run on all three supported CI hosts; cross-compilation alone is not lock proof.

### Cache inspection and maintenance

**Implemented (13):** `cache path`, `cache list`, `cache clean`, and `cache prune`
resolve the root without invoking Go, scripts, or shell helpers. Selection is
`GORN_CACHE`, otherwise the OS user-cache directory plus `gorn`, otherwise the
temporary directory only if the OS cannot determine a user-cache directory.
Relative overrides resolve from invocation cwd. Existing aliases resolve to their
physical root. A missing root is an empty inventory. Read-only commands and
`--dry-run` create no directories or lock files. Run/build create the same selected
root; creation/permission errors fail explicitly instead of redirecting elsewhere.

List reports full key, descriptive source, creation/build time, logical bytes,
state/reason and sanitized recorded build profile when its manifest header is
self-consistent. It does not discover the installed Go toolchain. Ownership and
validity are separate: a corrupt manifest or binary can remain independently
owned and eligible for cleanup; unsupported layouts, invalid/missing ownership,
and unrecognized data are preserved as unmanaged. Ownership binds layout, key,
kind and location. Stage locations bind the stage ID; quarantine names bind the
full entry key. Metadata paths and public inventory records confer no deletion
authority.

`--json` on each command emits one schema-1 object on stdout. Path has `schema`
and `root`; list adds `items` and `totals`. Each size is `{bytes, complete}`;
totals separate managed entries, quarantine, stages, unmanaged data and lock
metadata. Maintenance has `operation`, `dry_run`, per-object `actions`, `before`,
`after`, optional `predicted`, `unmet_budget`, and optional `error`. Action states
are `selected` (preview), `removed`, `skipped-active`, `skipped-changed`,
`skipped-missing`, `skipped-size-unknown`, or `failed`. Operational partial
failures still emit the maintenance report and return status 1; diagnostics and
progress go to stderr. Invalid flag/selection syntax returns usage status 2.

- `cache clean <key-prefix>` requires exactly one current owned entry matching
  1–64 lowercase hexadecimal characters. No match or ambiguity is an operational
  error. Mutually exclusive `--all` selects current owned entries plus owned
  abandoned stages and quarantine. Mutation requires `--yes`; `--dry-run`
  previews without confirmation.
- `cache prune --max-age <duration>` selects current entries built before one
  command-start timestamp minus the duration. `--max-bytes <integer>` then
  considers remaining entries oldest first, with full-key ordering for ties.
  Both bounds are nonnegative; zero is meaningful. Require at least one bound
  and `--yes` unless previewing. Age selection runs first when both are present.
  Stages, quarantine, unmanaged data and lock metadata cannot satisfy this budget.
- Size counts regular-file logical lengths; hard-linked names count separately,
  symlink targets and filesystem allocation overhead do not count. Sparse-file
  physical allocation is not measured. Errors and overflow mark totals incomplete.
  Unreadable namespaces or ownership records also make the affected membership
  total incomplete, rather than falsely reporting a known empty category.
  Incomplete managed totals prohibit size-driven deletion and leave a requested
  byte budget unmet/status 1. Independently justified age selection can proceed.
- Preview reports predicted selection/bytes and leaves actual `after` equal to
  `before`. It neither probes nor reserves activity locks, so it cannot promise
  the subsequent deletion outcome.
- Actual cleanup tries mutation then exclusive activity locks without waiting;
  stages use their exclusive stage lock. Busy objects are skipped, never
  interrupted. After locking, compare the entire ownership tuple and eagerly
  captured directory identity against the selection snapshot. Changed/missing
  objects are skipped. Stable lock files remain in place.
- Remove payload without following symlinks, retaining ownership until payload
  cleanup succeeds. Check cancellation between filesystem operations; this does
  not interrupt an arbitrary blocked OS syscall. Once final owner/directory
  removal starts, finish that pair. If final directory removal fails on the same
  directory identity, attempt to restore ownership and report the original error.
  Windows sharing violations can leave owned residue for later cleanup.
- Select once and take one final inventory. Continue past busy/failed oldest
  candidates, counting only successful removal toward the estimate. Concurrent
  publications, incomplete final inventory, or retained entries can leave a byte
  budget unmet; return status 1 with the observed result. Busy/changed skips alone
  are nonfatal for clean and age-only prune; deletion failures are not.

Alternatives and rationale: a permission-driven fallback could make inspection
and cleanup operate on a different cache than run/build, so all commands share
one deterministic selection. Recording last-use timestamps would mutate every
warm run; build age keeps reads cheap. Counting unmanaged/quarantine bytes toward
an entry budget would imply removable capacity without authority, so totals stay
separate. Deleting key-shaped paths or relying on validity manifests would grant
unsafe authority after corruption; independent ownership plus location and
generation checks protect replacement publications. Blocking cleanup would stall
behind long-running scripts; nonblocking skips preserve active work. Explicit
`--yes` suits pipelines; preview remains truly read-only instead of creating lock
metadata. A snapshot cannot reserve future activity, so predicted and observed
results are distinct. Repeated eviction until a live budget is satisfied could
run indefinitely with concurrent writers; bounded selection plus a final unmet
report gives a finite command. Automatic background eviction, old-layout
adoption and usage-recency tracking remain outside v1.

Verification: [inventory tests](../../internal/cache/inventory_test.go),
[maintenance tests](../../internal/cache/maintain_test.go),
[Windows sharing-failure tests](../../internal/cache/maintain_windows_test.go),
[CLI contract tests](../../internal/cli/cachecmd_test.go), and
[installed no-Go/active-run tests](../../cache_integration_test.go) cover these
contracts. Native Windows CI is required evidence for sharing violations and
ownership restoration; cross-compilation alone is insufficient.

Delete only owned entries under the resolved Gorn root and leave Go caches
untouched. Binary export uses the same validated pipeline and never executes the
script. Require `build <script> -o <path>`; refuse an existing destination by
default. `--force` can replace a regular file, while symlinks, directories, and the
source file itself are always refused. Preserve explicit output paths, adding
`.exe` on Windows only when no extension was supplied. Publish a completed
same-directory temporary file using the platform's atomic publication primitive;
failed builds/publication preserve an existing destination.

Alternatives considered: bare rename handles first publication but not corrupt
replacement or cleanup races; PID/mkdir locks need abandonment and identifier
reuse protocols. A single exclusive lock would serialize independent warm runs.
Shared activity retained while waiting for mutation can deadlock repair, and
building under shared activity alone permits replacement while another reader is
active. Separate mutation/activity locks with an explicit leased Entry keep warm
runs concurrent and make repair wait for readers. Ownership embedded only in the
validity manifest would make corrupt-manifest repair unsafe. Full keys avoid
prefix collisions; retaining old layouts avoids an implicit migration/deletion
policy. OS-backed lifetime locks plus private staging give publication, repair,
execution, and cleanup one ownership model. Build-time age
keeps warm cache reads free of recency writes. Explicit `--yes` supports
noninteractive automation without a blocking confirmation prompt. Refusing
overwrite by default protects existing output; `--force` provides a deliberate
regular-file replacement policy.

## Standalone binary export

**Implemented (12):** `gorn build <script> -o <path>` uses the same source parser,
generator, frozen build profile, dependency resolver, ownership checks and leased
cache entry as run. It never starts the script: neither `init` nor `main` runs.
The core path is shared in [preparation](../../internal/cli/prepare.go), with inspection
still returning before any toolchain/cache work. `--no-cache` builds and removes a
private leased stage; ordinary cold/warm exports share the same cache as run.

Export targets the invocation host with the existing pure-Go/CGO-disabled profile.
No new cross-compilation flag or dependency policy is introduced. The copied
binary is standalone: neither Gorn nor Go is needed at runtime, including for a
bundled `sh` import. Runtime programs may still need their own external commands
or data. Source `-` uses the same cancellable source-stdin contract as run.

Require `-o`/`--output`; its parent directory must already exist. Resolve relative
paths from the invocation directory. Refuse requested directories and symlinks
before platform naming, including dangling symlinks. On Windows, append `.exe`
only if no extension was supplied; preserve explicit extensions on every host.
Refuse source replacement through lexical identity, case-equivalent Windows
paths, or existing file identity, including hard-link and symlink-source aliases.
An existing regular output requires `--force`; force also permits an absent output.

Copy the validated leased binary into a private same-directory temporary file.
Check cancellation while copying and before publication; set executable mode
0755 (Windows uses inherited directory ACLs), sync/close the completed file, and
revalidate destination/source guards. Keep the entry lease through copy and
publication. Stdout stays empty; progress, verbose cache/output information and
errors use stderr, with the existing quiet and redaction policies.

Publication uses the standard library's directory-handle operations:

- Default: `os.Root.Link` atomically creates a new destination name without
  replacing an incumbent, including one created by a racing export. It requires
  filesystem hard-link support. Unsupported filesystems report an error; there
  is no racy rename or partially visible copy fallback. An operator can choose
  `--force` to request native rename/replacement semantics instead.
- Force: `os.Root.Rename` performs a single same-directory namespace operation.
  The pinned Go runtime uses Unix `renameat` and Windows native
  `NtSetInformationFile` rename, including its single-operation legacy fallback.
  It does not copy/delete an incumbent or use the multi-step ReplaceFile API.
  Recheck this implementation boundary when upgrading the pinned toolchain.

The successful link/rename is the publication commit point. Pre-publication
build, copy, validation, cancellation, permission or sharing failures preserve an
existing destination. Cancellation racing after commit may leave a valid output;
post-commit handle/staging cleanup failures are reported and do not roll it back.
A completed staged file is synced, but parent-directory fsync and power-loss
transactional durability are outside the atomic-visibility contract. Filesystem
semantics still apply; automatic destructive fallback is excluded.

Alternatives: writing directly to output exposes partial binaries and truncates
existing files on failure. Checking absence followed by ordinary rename loses an
output race. Universal no-replace rename lacks a portable standard-library API;
atomic hard-link creation provides that policy without another dependency.
A handcrafted Windows FFI rename would duplicate the pinned runtime's tested
ABI/path handling. Windows ReplaceFile has documented multi-step failure states
that can move/remove the incumbent, so the selected native rename operation is
preferable. Explicit output naming and overwrite intent make shell automation
predictable; automatically creating parent trees would hide path mistakes.

Evidence: [export policy/concurrency tests](../../internal/cli/export_test.go) and
[installed build tests](../../build_integration_test.go) cover cold/warm/bypass,
stdin, compiler and overwrite failures, source aliases, native naming/modes,
concurrent no-replace publication, cancellation, no execution at build time, and
stdlib/bundled-helper binaries running from another directory with an empty PATH.
The full inspection/execution suite also verifies the shared preparation path.

Native references: [Go os.Root](https://pkg.go.dev/os#Root),
[Windows FileRenameInformation](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-fsa/87f86c9b-6c2a-4803-84b7-131a74a434fa),
[ReplaceFile failure states](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-replacefilew).

## Progress and verification

GORN-V1-11 implements immediate preparation phases and a two-second stderr
heartbeat for pending source collection, discovery, cache access, dependency
resolution/downloads, and compilation. `-q`/`--quiet` suppresses Gorn's activity
messages, including during inspection. It preserves sanitized Go diagnostics,
script stderr, and explicitly requested verbose diagnostics. Help/version do not
start reporting. Missing Go, source, dependency, compiler, and script failures keep
their phase/cause; internal generated-source dumps require `--verbose`, while
`--print-main` remains the explicit inspect-only artifact path.

[Reporter tests](../../internal/progress/progress_test.go) cover synchronous initial
output, heartbeat/quiet behavior, joined shutdown, synchronized writers, IO
failure cancellation, complete-line framing, EOF, and oversized diagnostics.
[Resolver streaming tests](../../internal/cache/progress_test.go) prove redacted
diagnostics appear before a still-running child exits and are not replayed in
its final error. [Installed CLI tests](../../progress_integration_test.go) cover
cold/warm/bypass phases, quiet aliases, compiler failures, blocked-source
heartbeats, slow dependency failures/redaction, stdout integrity, and runtime
handoff. [Background stdin compatibility](../../internal/cli/runcmd_test.go) keeps
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
