---
title: Gorn v1 Compatibility Surface
status: authoritative
type: spec
updated: 2026-10-17
summary: Supported script, CLI, JSON and sh surfaces, private implementation/cache boundaries, and upgrade policy.
---

# Compatibility surface

Gorn v1 supports single-file Go apps, its CLI workflows, and the
`github.com/gornkit/gorn/sh` library. The root module installs an executable; it
is not a library import. Implementation packages live under `internal/` and are
not an SDK. The former pre-v1 `pkg/` paths are removed without forwarders before
v1, while `.gorn` source and helper import paths remain unchanged.

## Supported contracts

| Surface | v1 promise |
| --- | --- |
| Script source | Complete `package main`/`func main`, optional first-line shebang, header directive grammar/cardinality, ordinary Go semantics, preserved user text/literals and original-source diagnostic mapping |
| Metadata | `go`, `module`, `require`, and optional `preamble`; documented defaults, concrete requirements and explicit source Go floor |
| Preamble | Fixed names `fmt`, `os`, `filepath`, `strconv`, `strings`, `time`, `sh`; explicit-import conflict rules; bundled helper by default |
| CLI | Documented run/shorthand/build/cache commands and flags, `--` argument boundary, stdin-source consumption, invocation cwd/stdio and exit classifications |
| Automation | Documented schema-1 cache JSON fields/types and snapshot/partial-result meaning; valid JSON stays on stdout, diagnostics/progress on stderr |
| Go library | Exported `sh` declarations and documented copying, IO, context, execution, shell and result semantics |
| Export | Standalone host binary and documented output naming, refusal, permissions and atomic-publication behavior |

[Format](gorn-script-format.md), [workflows](gorn-workflows.md),
[helper](gorn-sh.md) and [build/runtime](gorn-build-runtime.md) specifications
define the detailed behavior and platform limits. Ordinary Go compiler/platform
behavior remains relevant; shell syntax is not made portable by Gorn.

`sh` supports Cmd; Host/Bash/Zsh/Sh/CmdExe/Pwsh/NewShell; ShellSpec Setup/Strict/
Shell/Exec; Command Dir/Env/Stdin/Stdout/Stderr/Context/Start/Exec; Process Wait/
Kill; Result OK/Code/Error/OrExit/OrExitf; package OrExit/OrExitf and the exported
PreambleSentinel constant. Struct internals are private. Result is status-only:
its Error method returns an error rather than implementing Go's error interface.
A zero Result is success; zero Command/ShellSpec cannot select an executable,
and zero/nil Process Wait/Kill report invalid use. Readers/writers are references,
not automatically synchronized or closed. No capture, pipeline or globbing API
is promised.

## Versioning and upgrades

Root module tags govern both installed CLI and published parent-module `sh`.
Bundled helper behavior belongs to the CLI release and its recorded source digest.
After v1, source/CLI/library removals or incompatible changes require a major
release; additive opt-in commands, flags, directives and APIs may arrive in minor
releases. Corrections can restore documented behavior in patch releases. A
deprecated public surface remains available through the current major line with
an identified replacement and release-note notice. Pre-v1 implementation paths
were never selected as supported v1 APIs.

JSON consumers must check the schema, ignore unknown fields and handle unknown
kind/state values conservatively. Schema 1 can add fields or new object kinds;
existing field meanings/types remain stable. A breaking representation uses a new
schema with an explicit upgrade notice. Inventory is a bounded observation, not
an atomic global snapshot or deletion capability. Dry-run predicts selection and
does not reserve future activity; mutation reports observed outcomes, including
incomplete sizes and unmet budgets.

Human-readable tables, help layout, progress phase wording, diagnostic prose and
exact compiler error strings are not machine APIs. Their meaning, stream and
status contracts remain stable. Printed generated Go retains user source and
mapping guarantees; generated additions and module-plan formatting can evolve
with build policy. Do not parse private cache files to automate workflows; use
cache JSON. Child output remains the script's own output contract.

## Toolchain and platform policy

The CLI, including its installed compiler for run/build, and the published parent
module require Go 1.26.4. Discovery rejects older installed compilers before
identity/cache lookup, including warm runs. Inspection/cache management remain
Go-independent. Contributor CI uses the
mise lock's pinned toolchain. The script default and bundled child language
minimum remain Go 1.26, which is distinct from the CLI's patch prerequisite.
Opting into a published parent helper may require an explicit
`//gorn:go 1.26.4` or higher. A newer installed compiler does not silently raise
a source minimum to satisfy a dependency. Gorn selects installed local Go and
does not auto-download toolchains for script builds.

Go prerequisites may advance in a Gorn minor release with an explicit release
note and install/upgrade guidance; a patch-floor correction may be required for
security/correctness. This prerequisite policy avoids committing v1 to an obsolete
Go toolchain indefinitely. It does not permit silent source-language upgrades.
Pin a Gorn version when a fixed toolchain prerequisite matters. Supported process
backends remain Linux/macOS amd64/arm64 and Windows, with the documented Windows
best-effort tree boundary. Release notes identify distributed architectures and
which targets have native runtime evidence; cross-compilation alone is not proof
of native behavior.

## Private implementation and cache transitions

`internal/app`, `buildprofile`, `cache`, `cli`, `process`, `progress` and `script`
may change together without a Go library compatibility promise. Their exported
names serve repository boundaries only. Parser AST representation, generated-file
structs, lock names, raw manifests/ownership records, cache layouts/hashes and
provenance sidecar details are private implementation data unless exposed by a
versioned command schema.

Generator/preamble/helper/dependency/profile changes affecting builds must change
build identity. The executing CLI binary digest and embedded helper digest already
separate source revisions; explicit generation/profile/layout versions separate
policy/format changes. A stale entry cannot bypass the current format/profile.
Unsupported old layouts remain preserved; they are not silently adopted or deleted.
Cache invalidation may rebuild a program and need its normal dependency sources;
source compatibility does not promise perpetual reuse of compiled cache data.

Two dependency invariants are enforced by the real Go graph:

- `sh` depends only on stdlib and itself and remains a single self-contained
  package that can be materialized into a private generated module.
- The root executable and `internal/` graph cannot import `sh`; they reference its
  import path as text and embed production source as data.

The [graph test](../../dependency_boundary_test.go), embedded-source consistency
test, private-process mirror test, generated-module compilation, and existing
installed/native workflow tests enforce these boundaries.

## Decisions and alternatives

Supporting the former `pkg/` API would freeze parser syntax objects, cache
capabilities and evolving build/profile types as an accidental SDK. Calling them
internal only in prose leaves Go consumers able to rely on them. Moving behind
Go's enforced `internal/` boundary before v1 makes the intended surface concrete;
forwarders would perpetuate the accidental commitment. Source/CLI/helper consumers
retain their selected entry points.

Promising cache filenames and manifests would couple user automation to ownership
and repair internals. Versioned cache JSON provides an inspection boundary while
allowing safe private format transitions. Promising byte-identical generated
additions would prevent ordinary generator/preamble improvements; user-text and
source-location preservation are the behavior that matters. Keeping Go prerequisites
forever fixed would retain unsupported toolchains; documented prerequisite changes
with explicit source minima separate tool maintenance from script semantics. Allowing older incidental
compilers would leave a support promise outside the validated module/install
floor, so discovery enforces that floor independently of a lower source minimum.
