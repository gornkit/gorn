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
verification evidence. **Inspection safety (01) is the next implementation slice.**

## Remaining sequence

1. **Make inspection safe (01).** Check print flags before execution and cache
   reuse. Prove inspection cannot run a script across cold, warm, bypass, and
   corrupt caches, and report output-write failures.
2. **Complete execution correctness (04, 09).** Preserve child exit status and
   define arguments, IO, cwd, and source-path behavior. Extend real CLI coverage
   as each contract is implemented.
3. **Settle build, dependency, and lifecycle contracts (06–08).** Define effective
   build inputs, dependency resolution, helper versioning, cancellation, and
   process ownership before expanding build/cache workflows. Use those decisions
   to complete cache repair (05), validation (10), and progress reporting (11).
4. **Deliver core workflows (12–15).** Implement binary export, cache management,
   exact-argument execution, and supported shell behavior. Keep the `sh` package
   stdlib-only and independent of the CLI dependency graph.
5. **Complete release gates (18–22).** Extend installed-CLI and cross-platform
   tests, finish onboarding, review public APIs, measure startup, and verify
   installation and published dependency resolution. Optional items 16–17 and
   23–24 do not hold v1 open.

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
dependency invariants: `sh` is stdlib-only, and the root/`pkg/` graph excludes
`sh`. Report environmental failures separately from regressions.

The [format implementation record](v1-standard-main-plan.md) documents a Linux
validation-host issue: invalid `.git` markers under temporary-directory roots
prevented generated-module builds from obtaining VCS status. A private valid Git
fixture used as TMPDIR allowed the full gate to pass with normal VCS stamping.
Use that isolation if the same condition recurs; preserve evidence of the default
environment failure and avoid global `GOFLAGS` overrides.

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
