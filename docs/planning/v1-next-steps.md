---
title: Gorn v1 Next Steps and Delivery Workflow
status: draft
type: plan
updated: 2026-10-17
summary: Remaining v1 implementation sequence, bounded planning and verification, and short-lived PR workflow.
---

# Gorn v1 next steps and delivery workflow

## Recommendation

Use the [v1 backlog](v1-backlog.md) as the roadmap. Deliver one coherent slice at
a time through short-lived branches and PRs into `main`.

GORN-V1-25 establishes the standard Go source format and completes the related
source-preservation and directive fixes in 02–03. The
[implementation record](v1-standard-main-plan.md) captures the contract and
verification evidence. GORN-V1-01 makes inspection independent of execution and
cache access, with write-error propagation. GORN-V1-04/09 define and implement
[execution and source identity](v1-execution-plan.md). **Build, dependency, and
lifecycle contracts (06–08) are selected in the
[build/runtime design](../specs/gorn-build-runtime.md). GORN-V1-10 implements
metadata syntax/cardinality validation. GORN-V1-07 delivers bundled/published
helper modes, pinned-graph resolution, and saved module/checksum provenance.
GORN-V1-06 binds warm lookup, execution, and provenance to the installed
toolchain/profile. GORN-V1-08 delivers lifecycle ownership and the core exact-argv
helper API from 14. GORN-V1-11 delivers immediate phases, bounded diagnostic
streaming, heartbeats, and quiet behavior. GORN-V1-05 delivers cache repair, independent ownership and cancellable leases.
GORN-V1-12 delivers standalone binary export. GORN-V1-13 delivers read-only
inventory and bounded, ownership-aware cache maintenance. GORN-V1-14/15
stabilize exact arguments and supported shells. GORN-V1-18/19 deliver installed
workflow coverage and executable onboarding. GORN-V1-20 defines compatibility
and enforces the private implementation boundary. GORN-V1-22 records startup
distributions and a measured reference-host budget. GORN-V1-21 delivers release
tooling and a native/public-module-qualified `v1.0.0-rc.1`; stable publication
remains.**

## Remaining sequence

1. **Publish the stable release (21).** The [release contract](../specs/gorn-release.md)
   records selected targets, alternatives, trade-offs, candidate/final ordering
   and recovery. The published candidate passes all three native public-module/
   archive smokes and the reference startup budget. Record candidate evidence
   and stable install guidance, then repeat the pipeline for `v1.0.0`; Latest
   promotion requires successful final native checks. Installed workflows/onboarding
   (18–19), compatibility (20) and startup measurement (22) are delivered.
   Optional 16–17 and 23–24 are deliberately deferred and do not hold v1 open.

Follow item dependencies when refining this sequence. Each slice includes its
behavioral tests and affected documentation. Preserve archived documents.

## Planning and implementation

Use Plan mode when a slice requires contract decisions or spans several
packages. Resolve scope, acceptance criteria, implementation approach, and
verification before implementation. A settled, localized fix can use a normal
implementation request.

Use **GPT-6.1 Sol (`gpt-6.1-sol`), high reasoning**, for source semantics,
runner/cache workflows, and integration coverage. Medium reasoning is sufficient
for a localized inspection fix once its contract is settled. Use **GPT-6 Astra
(`gpt-6-astra`), high reasoning**, for cross-cutting build, dependency, and process
lifecycle design. Independently review P0 changes, focusing on their behavioral
invariants.

For sustained work, scope a Goal to one accepted plan with a verifiable finish
line: implementation, relevant tests, required checks, documentation, and review
corrections. Finish with a validated diff ready for user review. A planning
request alone does not start implementation or activate a Goal. Keep delivery
approvals separate from implementation completion.

## Verification

Establish the baseline before changing code. Run focused behavioral checks first,
then `mise run test` for lint, stamped build, and uncached tests. Verify both
dependency invariants: `sh` is stdlib-only, and the root/`internal/` graph excludes
`sh`. Report environmental failures separately from regressions.

The [format implementation record](v1-standard-main-plan.md) documents a Linux
validation-host issue: invalid ancestor `.git` markers under temporary roots
prevented ordinary-Go compiler fixtures from obtaining VCS status. GORN-V1-18
now initializes private valid Git roots inside those fixtures, retaining normal
VCS stamping and allowing the default gate without external TMPDIR/GOFLAGS
overrides. Preserve that isolation in compiler-oracle tests.

## Worktrees, branches, and PRs

For each slice:

1. Start from the latest verified `origin/main`, preserving unrelated local
   changes. Use a short-lived `codex/` branch scoped to one behavior or inseparable
   format change.
2. Implement, validate, and review locally. Report the diff and verification
   evidence. Reuse a clean worktree for serial work; use a separate worktree for
   independent parallel work.
3. Obtain user review approval before committing and separate approval before
   pushing. Open a PR only within the authorized delivery scope.
4. Verify hosted checks and address actionable review comments on the exact PR
   head. Merge after the required landing approval and protections are satisfied.
5. Base the next slice on the updated `main`.

Keep the source format, specification, examples, and tests together when they
form one contract change. Keep each branch buildable and each PR reviewable.

Planning documents record decisions, implemented scope, verification evidence,
and remaining acceptance criteria. Git and PRs carry branch, commit, publication,
and merge state; approval status belongs in the delivery conversation.
