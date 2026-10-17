---
title: Standard Go Source Format Implementation Record
status: authoritative
type: plan
updated: 2026-10-17
summary: Approved GORN-V1-25 contract, implementation boundaries, and local validation evidence for source-authored Go main files.
---

# GORN-V1-25: standard Go source format

## Approval and delivery status

The user approved this bounded implementation on 2026-10-17 after planning.
Source formatting is preserved, and no migration tooling or dedicated legacy
errors are required. Work is on `codex/standard-main`, based on planning commit
`796fda2`. The user approved the implementation and a local commit, including
the added `testdata/require.gorn` example. Push and PR delivery remain pending.
The [backlog](v1-backlog.md) and [delivery workflow](v1-next-steps.md)
remain the roadmap and delivery process.

Implementation model recommendation: GPT-6.1 Sol (`gpt-6.1-sol`), high reasoning,
with independent review focused on source preservation and diagnostic mapping.

## Approved implementation contract

- Require `package main` and exactly one package-level `func main()` with a body,
  no parameters, no results, and no type parameters. Empty main and helpers on
  either side are valid. Remaining Go semantics belong to the compiler.
- Parse one Go file with comments enabled. Blank an optional first-line shebang
  without shifting offsets or line endings. Accept only standalone header metadata
  before the package keyword. Literal/block-comment contents and trailing comments
  are ordinary Go. Preserve current directive semantics and enforce one preamble.
- Remove section splitting, generated main wrappers, and source-wide formatting.
  Preserve user source text. Inject preamble imports in the same file after the
  package clause and append keep-alive declarations after source. Keep import-path
  conflicts and the existing ambient package set.
- Restore original file, line, and column locations around insertions; use absolute
  file-backed source anchors. Keep Go parser error information and metadata error
  wrapping. Raw generated output is reserved for internal generation failures.
- Retain Parse/Generate entry points; replace section fields with complete normalized
  Content and private syntax/positions. Rename MainFileFormatted to MainGenerated.
- Bump cache-key generation version from 1 to 2; leave manifest schema unchanged.
  Update specification, maintained examples, tests, and backlog evidence together.

## Boundaries

The CLI commands, `.gorn` extension, dependencies, and public `sh` library are
unchanged. The CLI still does not import `sh`, and `sh` remains stdlib-only.
Archived documentation stays frozen. Warm-cache inspection, exit status,
cancellation, corruption repair, helper pinning, and broader build identity remain
separate backlog items. Push requires separate user approval.

## Verification

Focused local checks pass for the parser, generator, key, and CLI packages:

```sh
mise exec -- go test -count=1 ./pkg/script ./pkg/app .
```

Coverage includes valid/invalid entry points, lexical directive handling,
cardinality, physical source lines, shebang/CRLF/EOF, raw-string equivalence,
comments attached to imports/helpers, compact forms, preamble compilation,
compiler line/column diagnostics, documentation-example compilation, generation
version invalidation, and real cold/warm/bypass runs with arguments and stdin.

The default temporary-directory environment has invalid `.git` markers at its
roots. Go discovers those markers, while Git rejects them; generated-module
builds therefore fail to obtain VCS status. This was reproduced on the unchanged
baseline. Tests run with TMPDIR pointing inside a private valid Git fixture pass
without disabling VCS stamping. The fixture is created outside the repository
and removed afterward; no global Go or Git setting changes are needed.

The full `mise run test` gate passed with that isolated TMPDIR: lint reported
zero issues, the stamped CLI built, and all package tests passed. Dependency-graph
checks using `go list -deps -json` verified stdlib-only `sh` and no root/`pkg/`
dependency on `sh`. Documentation links/frontmatter and `git diff --check` passed.
The added `testdata/require.gorn` example also built and ran with the local module
replacement used for preamble verification, printing `hello, Gorn`.

Independent review found one issue: preamble insertion could promote a trailing
package-clause `//line` comment into an active directive. Insertion now keeps the
package terminator and trailing comments together, with compiler regressions for
both implicit and explicit semicolons. The reviewer verified the correction
across 18 separator/comment variants, including CRLF and multiline comments, and
reported no further confirmed source-preservation or mapping issues.

These are local Linux results. Hosted cross-platform CI has not been run; the
implementation has user review approval for a local commit. Push and PR delivery
remain pending.
