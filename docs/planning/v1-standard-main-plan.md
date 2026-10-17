---
title: Standard Go Source Format Implementation Record
status: authoritative
type: plan
updated: 2026-10-17
summary: Implemented GORN-V1-25 source-format contract, scope boundaries, and verification evidence.
---

# GORN-V1-25: standard Go source format

## Outcome

GORN-V1-25 implements a complete Go source file with a source-authored
`package main` and `func main()`. Generation preserves source formatting and
literal contents while retaining optional shebangs, header metadata, and preamble
imports. The authoritative contract is the
[format specification](../specs/gorn-script-format.md).

This record captures the implemented scope and verification evidence for 25 and
the related fixes in 02–03. The [backlog](v1-backlog.md) tracks remaining v1 work;
the [delivery workflow](v1-next-steps.md) describes how to prepare each slice.

## Implemented contract

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
separate backlog items.

## Verification

Verification recorded on 2026-10-17 used Linux and mise-pinned Go 1.26.4.
Focused checks covered the parser, generator, key, and CLI packages:

```sh
mise exec -- go test -count=1 ./pkg/script ./pkg/app .
```

Coverage includes valid/invalid entry points, lexical directive handling,
cardinality, physical source lines, shebang/CRLF/EOF, raw-string equivalence,
comments attached to imports/helpers, compact forms, preamble compilation,
compiler line/column diagnostics, documentation-example compilation, generation
version invalidation, and real cold/warm/bypass runs with arguments and stdin.

The validation host had invalid `.git` markers at its default temporary-directory
roots. Go discovered those markers, while Git rejected them; generated-module
builds failed to obtain VCS status. This was reproduced on the unchanged
baseline. Tests passed with TMPDIR pointing inside a private valid Git fixture
without disabling VCS stamping. The fixture is created outside the repository
and removed afterward; no global Go or Git setting changes are needed.

The full `mise run test` gate passed with that isolated TMPDIR: lint reported
zero issues, the stamped CLI built, and all package tests passed. Dependency-graph
checks using `go list -deps -json` verified stdlib-only `sh` and no root/`pkg/`
dependency on `sh`. Documentation links/frontmatter and `git diff --check` passed.
The `testdata/require.gorn` example also built and ran with the local module
replacement used for preamble verification, printing `hello, Gorn`.

Independent review found one issue: preamble insertion could promote a trailing
package-clause `//line` comment into an active directive. Insertion now keeps the
package terminator and trailing comments together, with compiler regressions for
both implicit and explicit semicolons. The reviewer verified the correction
across 18 separator/comment variants, including CRLF and multiline comments, and
reported no further confirmed source-preservation or mapping issues.

This evidence covers Linux validation. Cross-platform installed-CLI coverage and
published dependency resolution remain release requirements under 18 and 21.
