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
version 3 excludes output from the previous dependency policy.

The full local gate passes in the documented private Git/TMPDIR environment.
Actual toolchain/profile identity, cancellation, progress, and cache ownership
remain separate implementation items. Until item 06 supplies the supported flag
allowlist, nonempty effective `GOFLAGS` fails before dependency resolution on
cache misses/bypass builds. Warm-hit effective-profile validation remains item 06.
Allowing arbitrary flags
could redirect module resolution through a modfile or overlay; silently discarding
them would change a requested build. Temporary rejection keeps this dependency
guarantee executable while profile work follows. Go workspace/toolchain control,
readonly final builds, and generated-build VCS/PGO policy are already applied.

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

Carry context through source collection, discovery, lock/lease acquisition,
downloads, builds, execution, and `sh` commands. Source stdin collection must be
interruptible; the root CLI owns its process input and may close that input on
cancellation. Embedding APIs require an explicit interruptible-reader/ownership
contract and must not silently close caller-owned IO.
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

Bundled `sh` checks a reserved inherited ownership marker against its actual
process group. Inside a Gorn-owned group it keeps commands in that group, so
nested helpers remain subject to whole-invocation cancellation. Per-command
Kill/context cancellation in this mode targets the direct command and bounds
inherited-pipe waiting; whole-invocation cleanup belongs to Gorn. Standalone `sh`
owns a group per command and cancels its members. The marker is coordination,
not isolation. An explicitly selected external helper may create groups outside
the runner's ownership and retains its documented lifecycle behavior.

When stdin is the controlling terminal and the caller owns its foreground group,
transfer terminal foreground to the child's owned group and restore it after
completion. Use thread-scoped signal masking for restoration; do not replace the
embedding application's global signal handlers. Verify real terminal input,
Ctrl-C, and restoration with PTYs on both supported POSIX hosts.

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

Emit phase information immediately before source/lock waits, discovery, downloads,
and compilation,
and periodic stderr status while slow work is pending. Stream useful Go errors.
Quiet suppresses progress, not compiler errors or script stderr. Synchronize
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
