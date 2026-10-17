---
title: Gorn Startup Measurements and Budget
status: authoritative
type: spec
updated: 2026-10-17
summary: Reproducible cold, populated-cache, warm and bypass distributions and a measured reference-host warm budget.
---

# Startup measurements and budget

## Decision

Use **100 ms p95 full quiet-CLI warm wall time per fixture on the reference host**
as the v1 startup budget. The observed worst p95 is 54.21 ms, leaving about 85%
headroom. This is a release comparison on the documented host/conditions, not a
portable latency guarantee or a timing assertion on shared CI runners. Recheck
warm distributions when release build flags, toolchain or runner behavior change;
record the new environment when the reference hardware changes.

Keep full toolchain/CLI/binary integrity validation. The current implementation
meets this budget, so no runner optimization is selected. Direct cached programs
have median wall times around 0.8–1.0 ms; subtracting their medians gives roughly
47–49 ms of runner/profile/cache/process overhead. This is an approximate
comparison of separate distributions, not per-phase profiling or a claim about
which validation step dominates.

## Reference environment and data

[Raw samples and metadata](../benchmarks/2026-10-17-startup-linux-amd64.json)
record the complete schema-1 result:

- Fedora Linux 44 (COSMIC), linux/amd64; Intel Core Ultra 5 228V, eight logical
  CPUs/GOMAXPROCS, 32,356,336 kB reported host memory.
- Installed Go 1.26.4; quiet CLI `gorn 2f9713a-dirty`, SHA-256
  `4b6fc50040a5520b0efbdfc4a900e981498bcc1fc1eeb0f81b22964f80d782ed`.
- Source checkout base `2f9713ad20af7bbc85d25a42e9e8dc3229b261ec`, dirty only
  for the benchmark tool/task at measurement time; runner runtime code is the
  verified compatibility baseline. Embedded CLI VCS settings were unavailable,
  so the exact binary digest, version and separate checkout state are retained.
- Three empty-Go-cache samples, ten populated-Go-cache cold samples, fifty warm
  samples, ten bypass samples, and fifty direct-binary samples per fixture.

All times below are milliseconds, shown as **median / p95**:

| Fixture | Empty Go caches | Populated Go caches, cold Gorn | Warm Gorn | Bypass | Direct binary |
| --- | --- | --- | --- | --- | --- |
| Tiny main | 1696.57 / 1702.24 | 263.22 / 271.72 | 48.17 / 53.71 | 259.23 / 282.41 | 0.78 / 1.08 |
| Preamble/bundled helper | 2598.17 / 2606.09 | 356.61 / 381.98 | 49.65 / 54.21 | 361.33 / 382.17 | 0.96 / 1.21 |
| Declared dependency | 1728.52 / 1734.22 | 285.71 / 292.29 | 48.65 / 53.56 | 284.72 / 303.58 | 0.82 / 1.08 |

P95 uses the nearest-rank sample (ceil(0.95 × count)); at three cold samples it
is simply the largest observation, not a reliable population-tail estimate.
Raw data also includes min, mean, max and every observation. These are wall-clock
measurements on a developer laptop, without isolated scheduling/power controls.
The baseline measures a development CLI build; candidate verification must
record and check its own release flags/bytes before promotion.

## Reproduce

With the mise-pinned toolchain and a normal contributor checkout:

```sh
mise run bench -- -output /tmp/gorn-startup.json
```

The inline task builds the stamped CLI, then runs
[the internal benchmark command](../../internal/startupbench/main.go). It is a
contributor tool, not a public Gorn subcommand. Sample progress goes to stderr;
JSON goes to stdout unless `-output` is supplied. Full defaults match the counts
above. A shorter functional smoke is:

```sh
mise run bench -- -cold 0 -reuse 1 -warm 2 -bypass 1 -output /tmp/gorn-startup-smoke.json
```

On Windows choose an existing writable output path, for example a file under
`$env:TEMP`, instead of the Unix `/tmp` spelling. A separately built CLI can be
selected with `go run ./internal/startupbench -gorn <path> ...`. Extensionless
Windows CLI inputs are copied to a private `.exe` when needed; bytes/digest remain
unchanged. Native measurement evidence here is Linux only. Target lint/build
checks do not establish equivalent timing on macOS or Windows.

Each fixed fixture does minimal runtime work. Preamble imports the actual bundled
helper; dependency mode imports a concrete module from a local HTTP Go proxy.
The harness uses private Gorn, Go build and Go module cache directories, clearing
only its own temporary trees. It does not reset user caches. Source and selected
CLI bytes are hashed into the report. Environment is controlled for the supported
host-only pure-Go profile, using local installed Go and no user GOENV/GOFLAGS.
Checksum-database lookup is disabled only for this synthetic local-proxy fixture;
normal product dependency verification is unchanged.

Empty-cache samples each get new logical Go/Gorn caches. This does not flush OS
page caches or cold-boot the machine. A separate unmeasured primer populates Go
and Gorn caches for each fixture. Populated-Go-cache cold samples use fresh Gorn
roots; warm samples reuse one valid publication; bypass uses fresh private stages
with populated Go caches. Direct samples execute that cached program without the
runner. The tool asserts the warm publication's build time stays unchanged.

Only measured-mode requests are counted; primer downloads are excluded. Dependency
empty-cache samples made nine local requests total (three per sample); every
other measured mode made zero requests. No public internet latency is represented.
The [installed dependency test](../../dependency_workflow_test.go) independently
makes the populated proxy unavailable and proves warm runs do not request it on
native CI hosts. [Release smoke](../planning/v1-backlog.md#gorn-v1-21--deliver-and-smoke-test-versioned-releases)
separately verifies actual published module resolution.

## Alternatives and rationale

A public-network cold benchmark mixes runner/build work with variable transport
and upstream availability. A controlled proxy isolates the declared-dependency
cost; published-path verification remains a separate release requirement. Shared
user caches hide cold state and resetting them would be disruptive, so the harness
owns private caches. Mean-only results hide latency tails, so raw distributions
and p95 accompany the median. Equivalent direct binaries help separate trivial
script/process work from runner cost.

Skipping profile discovery on warm hits or replacing checksums with file-stat
trust would weaken build identity/integrity. The measured 100 ms budget does not
require that trade-off. Shared CI timings are noisy and differ in hardware; a
reference comparison plus deterministic no-download tests gives actionable
regression evidence without inventing a universal threshold.

## Published candidate qualification

The actual `v1.0.0-rc.1` Linux/amd64 archive, downloaded from GitHub Releases,
passes the same reference-host warm check: fifty samples per fixture, warm p95
52.96 ms tiny, 52.79 ms preamble and 52.17 ms dependency. All remain below the
100 ms reference budget. [Raw candidate data](../benchmarks/2026-10-17-rc1-startup-linux-amd64.json)
records the exact binary SHA-256
`14584524bca18ffc0282db5582a90784614cc33d6b0915785d8df1462373ee26`,
source/embedded VCS revision `dbcb08603646f17d2738f81f8d2fef54e831b905`,
pinned Go, hardware and distributions. All seven candidate asset checksums match;
local published-archive/source-install/helper verification and native three-platform
release smoke pass. This measures delivered release flags/bytes rather than
extrapolating from an unstripped development executable. It does not create a
portable CI timing promise.
