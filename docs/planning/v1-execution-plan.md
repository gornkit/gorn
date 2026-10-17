---
title: Execution and Source Identity Contract
status: authoritative
type: design
updated: 2026-10-17
summary: Selected execution, argument, source-path, and cache-location decisions for GORN-V1-04 and GORN-V1-09.
---

# Execution and source identity

Implementation recommendation: GPT-6.1 Sol, high reasoning, with independent
high-reasoning review of exit propagation and cross-platform path behavior.

## Decisions and rationale

| Concern | Selected behavior | Alternatives and trade-offs |
| --- | --- | --- |
| Exit status | 0 success/help/version; 2 CLI usage; 1 source/build/start/IO failures; ordinary script exits preserved | Collapsing every error to 1 loses script control flow. A separate build code would conflict with ordinary script codes without adding a reliable category channel. Runner stderr identifies the phase. |
| Signal termination | POSIX child signals map to 128+signal; Windows preserves reported process exit codes, with 1 for unavailable status | Re-raising the signal reproduces wait status but can bypass cleanup. Numeric mapping is predictable to shells and allows cleanup. Cancellation/process-tree ownership is specified separately under 08. |
| Failure output | Usage goes to stderr for CLI parse errors. Valid invocations never dump usage. Child nonzero exits preserve child output without a duplicate runner error line | Printing child status again adds noise and changes stderr for commands whose exit status is the intended result. Build/start errors still receive runner diagnostics. |
| Arguments | Keep current flag parsing before `--`; use `--` to forward flag-like, empty, and literal separator arguments. Explicit and shorthand run share behavior | Forwarding every token after the script would be familiar but break supported post-script runner flags, including stdin inspection. The separator provides an unambiguous boundary without a command-shape break. |
| File identity | Absolute, cleaned lexical source path; preserve symlink spelling. Runtime cwd remains invocation cwd | Resolving symlinks coalesces aliases but changes diagnostic names and can hide caller-selected paths. Preserving lexical identity permits separate cache entries for aliases and stable original-source diagnostics. |
| stdin source | Synthetic `<invocation cwd>/stdin.gorn` diagnostic path with a distinct stdin identity; include invocation cwd in cache identity; source input consumes stdin before runtime begins | `/dev/stdin` is platform-specific. An absolute synthetic path keeps generated-source diagnostics out of temporary build directories. A separate stdin discriminator prevents collision with a real `stdin.gorn` file. A cwd-independent key can reuse diagnostics or build assumptions across directories. Reopening a terminal would make pipeline semantics surprising; runtime stdin is already consumed. Separate source/runtime readers can be considered for an embedding API later. |
| Cache override | Normalize a relative GORN_CACHE against invocation cwd; explicit unusable roots fail with their path and cause | Rejecting relative overrides is stricter but less useful in CI. Silent fallback can make cache management affect an unexpected root. Cache management (13) selects the same root read-only; default fallback occurs only when the OS cannot determine a user-cache directory. |
| Source bytes | Own a copy of input bytes and return copies to callers | Mutable source bytes can diverge from a previously calculated cache key. Copying costs source-size memory but keeps identity valid and scripts are small. |

## Verification

A built CLI verifies to verify child 0/7/2 exit status, usage versus runtime errors,
stdout/stderr preservation, explicit and shorthand flags, empty arguments,
separators, cwd, and stdin. POSIX child signal mapping is exercised directly; the hosted
Windows job verifies native compilation and execution. Source tests cover
absolute/relative source equivalence, symlink identity, distinct same-named
scripts, Unicode/spaces, CRLF, portable stdin diagnostics and cwd-sensitive
identity, relative cache normalization, and refusal of an unusable explicit root.
Focused checks and the full mise gate passed on Linux with normal VCS stamping
in a private valid Git fixture used as TMPDIR. The default-host VCS limitation
remains as recorded in the format implementation record. Independent P0 review
found a shorthand `--version`/`--help` parity gap; both parsers now handle those
flags consistently, with installed-CLI regressions.

Evidence: [installed-CLI tests](../../execution_integration_test.go),
[source identity tests](../../internal/app/source_test.go), and
[cache-location tests](../../internal/cache/cache_test.go). Cancellation and process
ownership remain the separate contract in 08; this slice defines child status
propagation rather than the runner's response to receiving a signal.
