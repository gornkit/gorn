---
title: Gorn v1 Next Steps and Delivery Workflow
status: draft
type: plan
updated: 2026-10-17
summary: Recommended implementation sequence, bounded Plan and Goal usage, and short-lived PR workflow for the v1 backlog.
---

# Gorn v1 next steps and delivery workflow

## Recommendation

Use the [v1 backlog](v1-backlog.md) as the roadmap. Plan and deliver one coherent
slice at a time, with short-lived branches and PRs into `main`. Start by settling
and implementing the ordinary-Go source format; do not repair the old wrapper
as a separate prerequisite when the approved format change will remove it.

This records the recommended workflow. It does not start implementation, create
a Goal, or authorize commits, pushes, or publishing. GORN-V1-25 has since been approved and implemented locally; its
[implementation record](v1-standard-main-plan.md) tracks validation and review.
The remaining backlog stays subject to its recorded status and approval boundaries.

## Sequence

1. **Record the planning work.** Review the backlog and this workflow as a small
   documentation change. A documentation-only PR into `main` is the recommended
   first delivery; it gives subsequent implementation PRs a stable reference.
2. **Plan the format slice.** Use Plan mode for GORN-V1-25. Resolve the package
   clause, entry point, metadata header, optional preamble, shebang processing,
   diagnostics, source mapping, and cache invalidation. Include the relevant
   source-preservation acceptance checks from 02–03 in that plan.
3. **Implement the approved format in one logical slice.** Keep parser,
   generation, authoritative specification, maintained examples, and behavioral
   tests together. Retain optional preamble/shebang support; no migration tooling,
   dedicated legacy errors, or permanent dual grammar is required.
4. **Complete execution correctness.** Follow with inspection safety (01), exit
   and IO behavior (04), path identity (09), and cache repair (05). Respect the
   dependency graph: settle relevant cache identity rules before finalizing repair.
   Inspection is independent and can be pulled forward if needed.
5. **Settle build/dependency/lifecycle contracts.** Complete 06–08 and the related
   validation/progress work before expanding build and cache commands. Establish
   the supported inputs and process behavior before stabilizing public APIs.
6. **Deliver the core workflows and release gates.** Implement build/cache and
   the small `sh` additions, then complete real workflow coverage, documentation,
   performance evidence, API commitments, and release packaging. Optional backlog
   items remain optional.

Before implementation, reproduce the validation baseline, including the observed
VCS-stamping test failure. Diagnose and resolve that failure or report its actual
environmental blocker; do not use a global `GOFLAGS` override to claim a clean
gate. If a product/test repair is needed, keep it as a small independently
reviewable change. Every slice should report baseline issues separately from
new regressions and finish with the required checks passing.

## Plan mode and Goal usage

**Use Plan mode next.** It provides the implementation approach for review
before code changes. After the plan is accepted, return to normal execution.
For sustained implementation, use a Goal scoped to that approved slice with a
verifiable finish line. Plan-only work does not trigger Goal continuation.
See the official [Plan and Goal workflow](https://developers.openai.com/blog/mastering-codex-remote-for-engineering),
[commands](https://learn.chatgpt.com/docs/developer-commands), and
[Goal lifecycle](https://developers.openai.com/cookbook/examples/codex/using_goals_in_codex).

Use **GPT-6.1 Sol (`gpt-6.1-sol`), high reasoning**, for the format plan and its
implementation. Use independent high-reasoning review of source preservation,
preamble insertion, and source mapping. Use **GPT-6 Astra (`gpt-6-astra`), high
reasoning**, for the later build/dependency/lifecycle design work.

Suggested next planning prompt:

```text
/plan Plan GORN-V1-25 in docs/planning/v1-backlog.md using
docs/planning/v1-next-steps.md. Target package main and an explicit func main(),
retain optional preamble and shebang support, and remove section splitting and
the generated main wrapper. No migration tooling or dedicated legacy errors.
Include Go-aware metadata handling, literal preservation, original-source
diagnostics, cache invalidation, and the relevant 02–03 acceptance checks.
Check the validation baseline. Propose a complete, reviewable implementation
plan and verification strategy; do not implement yet.
```

After approving that plan, leave Plan mode. A suggested bounded Goal is:

```text
/goal Implement only the approved GORN-V1-25 plan. Record the approved plan and
verification evidence in the repo. Update the format specification, maintained
examples, tests, and relevant backlog status. Preserve the repository dependency
invariants. Pass mise run test and the approved behavioral checks, and address
actionable review findings within this slice. Finish with a validated,
uncommitted diff ready for user review. Do not commit, push, open a PR, merge,
or begin another backlog item as part of this Goal.
```

The implementation Goal is complete when its scoped diff is validated and ready
for review. Delivery is a subsequent step. This avoids an active Goal waiting
indefinitely for commit, push, or merge approval. Do not make the entire v1
backlog one Goal. A normal implementation request is also sufficient when the
slice is short; a Goal is useful when repeated investigation and validation are
expected. The Goal wording above is a suggestion, not an activated objective.

## Worktrees, branches, and PRs

Keep the existing worktree for this work. At preparation time it was detached at
`d045beb`, with only the planning documentation changed; the primary checkout
had `main` checked out at the same commit. These are local observations, not a
fresh remote-state assertion.

Create a named `codex/` branch before committing. Suggested first branches are
`codex/v1-planning` for the documentation and `codex/standard-main` for the
format slice. These names are recommendations; neither branch has been created
by this document.

For each slice:

1. Start from the latest verified `origin/main`, preserving unrelated local
   changes. Keep the branch scoped to one behavior or inseparable format change.
2. Implement, validate, and review locally. Report the exact diff and evidence.
3. Obtain user review approval before committing. Obtain approval before pushing.
   Open the PR only within the approved delivery scope.
4. Verify hosted checks and address review comments on the exact PR head. Merge
   after the required landing approval and protections are satisfied.
5. Base the next branch on the updated `main`. Reuse the clean worktree for
   serial work; create a separate worktree only for independent parallel work.

Use small PRs to `main` throughout v1. A long-lived v1 integration branch would
delay feedback and accumulate integration work. The source format, specification,
and tests belong in one atomic PR so `main` remains buildable. Related follow-up
fixes can use separate PRs after that change lands. Keep backlog status aligned
with verified delivery; distinguish locally validated, PR open, and merged work.
