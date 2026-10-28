# Releases and readiness

## Initial beta

[v0.1.0-beta.1](https://github.com/humzakhan/capsule-memory/releases/tag/v0.1.0-beta.1)
is the first GitHub **prerelease**, distributed as source archives. The repository
is already public under MIT. A public repository and a published tag establish
availability, not a tested installation, deployment, or security guarantee.

The [changelog](../CHANGELOG.md) describes the included features and repairs.
There are no prebuilt executable assets, package-manager releases, installed
services or hosted relay. Windows retains legacy direct mode; the optional
daemon targets Linux/macOS. Platform and harness compatibility remain unverified.

Download the tagged source from the release or check out `v0.1.0-beta.1` through
Git. Use the pinned Go 1.24.0 toolchain and Node 22+ with the committed `go.sum`
and `cloudflare/package-lock.json`. Follow [Run v1](run-v1.md) for the locked
synthetic evaluation path and [Run sessions](run-session.md) for optional setup.
Do not connect real histories or enable capture as an installation side effect.

## Repository scan and evidence

The release preparation inspected merged PR #2 at
`88e0c0b042b57c89331b8cd6ad6e35c8211dcff9`, including public entry points,
manifests/locks, license, security/support/contribution guidance, packaging
templates, status/verification documents and source markers for missing features.
The release-documentation commit does not change product code or dependencies.
The tag and GitHub release identify the exact final source revision.

| Area | Source evidence | Remaining gate |
| --- | --- | --- |
| Public distribution | Public MIT repository; source tag and prerelease | No tested executable distribution or package registry entry |
| Dependencies and setup | Pinned manifests and committed locks; corrected stale install instructions | Current-source dependency/toolchain verification and license inventory before binary redistribution |
| Quality | Check script and focused regression source exist | Formatting, Go static/test/build and relay type/format checks on the exact release candidate |
| Behavior and trust | Combined review at `578421b`, correction review at `cf221ad`; Git whitespace checks passed | Synthetic durability/privacy journeys, SQL/FTS/concurrency, real cryptographic vectors and platform/adapter acceptance |
| Support and reporting | Security, support and contribution guidance exist | GitHub private vulnerability reporting was disabled at the scan; a reliable private reporting route is needed before sensitive-use invitations |

The [dated setup baseline](verification.md#setup-gate-baseline-2026-10-27)
recorded Go/relay formatting failures and a relay `TextDecoderOptions` typecheck
failure at `cloudflare/src/wire.ts:6`. That constructor remains unchanged in this
source. Historical Go vet/test/build success reported no test files and predates
the repairs. None of those results verifies this beta. No current execution was
performed during release preparation, and no CI workflow was enabled.

The scan included a narrow source-pattern check for recognizable credential/key
literals, with no matches. It was not a comprehensive history or secret audit and
does not certify an absence of secrets. Real profiles, databases, credentials and
histories were not accessed or uploaded.

## Gates for a tested binary beta

1. **Establish a clean quality gate.** In an explicitly authorized environment,
   use the locked dependencies and run the [explicit check gate](development.md#explicit-check-gate)
   over the exact candidate range. Resolve the known formatting/typecheck
   findings and any current failures; record source SHA, tools and exit results.
   A missing or skipped check is not a pass.
2. **Verify the affected behavior.** Run the existing focused regressions and
   synthetic E2E journey, then the relevant [session acceptance scenarios](verification.md#optional-session-module-acceptance-not-run):
   persistence/retry/crash recovery, capture consent/checkpoints, lease expiry,
   withdrawal/corrections, grant/enrollment disclosure and strict freshness.
   Add cases only for concrete uncovered defects; no broad coverage target.
3. **Establish compatibility and security evidence.** Check the intended OS/CPU
   matrix, pinned harness versions, IPC/daemon shutdown and bounded loopback
   inference. Verify exact Go/TypeScript cryptographic vectors and hostile-input
   rejection. Local relay emulation does not prove production provider durability
   or authorize a deployment; any provider job needs its own authority.
4. **Produce traceable artifacts.** Build executables from the accepted commit
   with an explicit version, checksums, dependency/license inventory and build
   provenance. Verify clean-machine install/uninstall and update/rollback paths
   for each advertised target. Document artifact verification before publishing
   binaries or package-manager metadata.
5. **Publish evidence and maintain support.** Record actual passed/failed/not-run
   gates, known issues and migration/recovery instructions. Establish a private
   vulnerability contact, review dependency advisories and resolve material
   findings before recommending sensitive use. Only promote beyond prerelease
   when the advertised behavior has evidence; source review alone is insufficient.

These are acceptance gates, not executed results or authorization for installs,
CI, services, infrastructure, spending or real-data access. The current release
task retained the source-only restriction. No runtime-ready release is claimed.

## Known source limits

Published-body cache eviction remains unimplemented. Ranked knowledge scans cap
each scope/kind stream at 4096 candidates and report incomplete coverage;
identical retries do not advance an external cursor. Grant checks cover locally
retained history, not undiscovered remote assertions or already copied keys.
Local writer exclusion is not a distributed transaction; private file output and
SQLite commits are separate durability boundaries. Shutdown owns and joins its
work but has no verified latency guarantee.

See [session status](session-status.md) and the [threat model](threat-model.md)
for the full evidence boundary. Cache eviction and the previously excluded
features are not silently reclassified as implemented by publishing this beta.
