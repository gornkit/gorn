---
title: Gorn v1 Release Delivery
status: authoritative
type: spec
updated: 2026-10-17
summary: Candidate-first versioned delivery, binary matrix, checksums, published module verification, and release recovery.
---

# Release delivery

Root tags version the CLI and published `github.com/gornkit/gorn/sh` together.
The first candidate is `v1.0.0-rc.1`; the first stable release is `v1.0.0`.
Tags are canonical v1 semantic versions, with no build metadata. Tags and
published assets are immutable: never move a tag, replace an asset, or reuse a
failed version after a source change. Issue the next candidate/patch instead.

## Supported distribution

| Target | Archive | Native release smoke |
| --- | --- | --- |
| linux/amd64 | tar.gz | Ubuntu runner |
| linux/arm64 | tar.gz | Cross-build only |
| darwin/amd64 | tar.gz | Cross-build only |
| darwin/arm64 | tar.gz | macOS runner |
| windows/amd64 | zip | Windows runner |

Archives contain a flat `gorn` or `gorn.exe`, LICENSE and INSTALL.txt. Unix binary
mode is 0755; data files are 0644. SHA256SUMS.txt covers every other asset,
including BUILD_INFO.json and RELEASE_NOTES.md. Build information records source
revision, Go version, targets and executable/archive digests. Archives use fixed
entry order and commit timestamps. This controls incidental packaging variance;
bit-identical compilation across arbitrary hosts/toolchain environments is not
promised. Checksums detect corruption; they are not an independent signature or
proof against a compromised GitHub account. Signing/package managers/self-update
are deferred beyond v1.

The runner and source install require installed Go **1.26.4 or newer** on PATH,
including warm runs. Exported programs require neither Gorn nor Go. Published
helper scripts explicitly select `//gorn:go 1.26.4` (or higher), because the parent
module floor exceeds the default script language floor of 1.26.

Explicit linker version stamps take precedence; otherwise source installs report
Go's module version. Unversioned local builds report `dev`. This preserves the
local development contract and makes `go install ...@v1.0.0` identify its version.
The CLI binary digest remains part of build identity regardless of the label.

## Mechanical procedure and gates

Use GPT-6.1 Sol, high reasoning, for implementation; independently review changes
to version/build identity and publishing gates with GPT-6 Astra, high reasoning.
Deliver tooling, workflow, notes, tests and this spec together through a short
PR. Local preparation may use `-preview`; publishing must use an exact clean tag. Preview provenance is marked and cannot
satisfy published-release verification.

```sh
mise exec -- go run ./internal/release build -version v1.0.0-rc.1 -dir /tmp/gorn-preview -preview
mise exec -- go run ./internal/release verify -version v1.0.0-rc.1 -dir /tmp/gorn-preview
```

The output directory must not already exist. After review and all gates, create
and push the annotated tag on the verified main commit; the tag push triggers
`.github/workflows/release.yml`. For an existing published release, checkout its
tag and rerun verification with:

```sh
gh workflow run release.yml -f tag=v1.0.0-rc.1
```

Dispatch verifies existing bytes and cannot upload replacements.

1. Run focused version/archive verification, `mise run test`, cross-target lint,
   and dependency-boundary checks. Cross-build all five archives. Recheck the
   candidate's stamped/stripped CLI against the reference-host startup budget.
2. Merge green native CI. Create an annotated candidate tag on that verified
   main commit. Release workflow reuses three-platform CI at the tag's commit.
3. Build five binaries with the pinned Go toolchain, cgo disabled, trimpath and
   stripped version stamps. Create the GitHub release as a prerelease, with all
   archives, checksums, build information and release notes attached.
4. Native jobs download the actual published archive/checksums/build information.
   Verify digests, Unix permissions, version output, first-run arguments and
   exported execution without Go on PATH. In fresh Go/Gorn caches, install the
   actual public module tag with normal Go checksum verification (inherited private
   or insecure-module patterns are cleared), then compile
   and run a script selecting that exact published parent helper. No replacement,
   synthetic proxy or bundled-helper substitution can satisfy this gate. The smoke
   program checks its compiled module metadata for the exact parent tag without
   a replacement, and rejects the bundled child module. Downloaded provenance
   must match the checked-out tag commit. Dispatched retries run CI at that tag
   as well; they cannot promote through a skipped validation gate.
5. Candidate remains a prerelease. After candidate evidence passes, record it
   durably before tagging the stable version. Repeat all gates for the stable
   tag. Only successful native smoke jobs permit stable promotion to Latest.

The public Go module tag necessarily exists before public-proxy/source-install
verification. Even a failed final smoke may leave that final module resolvable;
a GitHub prerelease flag cannot hide it from Go. Candidate verification reduces
this risk without eliminating it. A failed stable tag stays unpromoted; diagnose
and publish a new patch rather than retagging. The workflow never uses admin
bypasses, force pushes or asset clobbering.

If an upload is interrupted, inspect the draft and workflow logs. An unpublished
draft can be removed and recreated for the same unchanged tag; a published release
must retain its bytes. Verification can be rerun through workflow dispatch using
an existing tag, without rebuilding or re-uploading assets. Never mark Latest
manually to bypass a failed gate. Git/PR/release history holds approvals and
publication state; this specification defines the durable procedure.

## Alternatives and rationale

Publishing stable first would make a failed source-install path the default
install experience. A candidate exercises the real module proxy and packaging
before stable selection. A local module replacement tests generation but cannot
prove tag availability, module checksums or published-helper selection, so release
verification uses the public tag. Running only a locally built executable cannot
catch archive permissions/contents or upload mistakes, so native jobs download
published bytes. Cross-builds cover two additional architectures affordably but
are explicitly weaker than native evidence; they are not described as native tests.

A contributor Go command avoids new packaging dependencies and shell-specific
archive behavior. Reusing CI at the same commit avoids a second divergent test
policy. GitHub releases provide a direct install path before adding package-manager
maintenance. SHA-256 is portable and inspectable without claiming signing that we
do not provide. Version fallback uses Go module metadata rather than a VCS hash:
source installation already knows the selected version, while ordinary local
builds retain `dev`. Release flags change binary identity and are measured again;
startup work does not weaken integrity validation to meet its budget.

GitHub workflow reuse and release mechanics follow the
[reusable workflow documentation](https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows)
and [release CLI](https://cli.github.com/manual/gh_release_create).
