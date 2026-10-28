# Autonomous Mac / Cloudflare QA — 2026-10-28

Bounded synthetic QA of the 193-case inventory completed on one Mac Mini and the
existing dedicated Cloudflare QA Worker/DO/R2. Two defects were reproduced,
repaired, committed to local `main`, rebuilt and installed. No GitHub push,
release, production change, paid upgrade or new cloud resource was made.
This is scoped beta evidence, not complete security, platform or load acceptance.

## Cases and limits

[Case ledger](2026-10-28-cases.jsonl) records every original ID, feasibility,
status, source identities, evidence and remaining limits. The original
[failure inventory](../failure-cases.jsonl) remains its historical baseline.

| Initial feasibility | Cases |
| --- | ---: |
| Locally testable | 128 |
| Live-cloud testable | 10 |
| Partially testable | 48 |
| Blocked | 7 |
| Total | 193 |

| Final disposition | Cases |
| --- | ---: |
| Passed described bounded check | 91 |
| Partially verified; specified variants remain untested | 95 |
| Blocked / unexecuted | 7 |
| Unresolved reproduced product failures | 0 |

186 cases received a runtime probe or bounded evidence assessment; 91 passed
within the stated scope. The 95 partial cases must not be counted as fully
passed. Case-level NOT RUN is zero, but individual checks remain NOT RUN inside
partial cases. Overlapping cases share meaningful scenarios rather than one
test per inventory row. No claim covers every fault schedule or edge case.

The seven blocked cases are KEY-009 (erase prior copies), KEY-010 (automatic
owner-key rotation/distribution), SKL-011 (native executable skill installation),
CNS-011 (physical erasure of backups/recipients), DRV-008 (cryptographic proof of
human approval), CF-010 (account loss/malicious provider), and OPS-002 (host
compromise). These are unsupported guarantees or actions outside this setup.

## Source and installation

- Starting clean source: `0ef7afeb7f932633c04e329f7414885bfa611e84`.
- Product source, full quality gate and installed build:
  `72490890d5b7f0684a77e8a3b21788c4863ad7f9`.
- Worktree: `~/dev/worktrees/capsule-memory/session-history-source-repair`,
  local branch `main`. Initially fast-forwarded local main from `e27cd2e` to
  the already validated QA branch. The canonical checkout and other worktrees
  were preserved. The final documentation commit is newer than the product build.
- Command: `~/.local/bin/capsule-memory`; versioned build under
  `~/.local/share/capsule-memory/releases/72490890d5b7f0684a77e8a3b21788c4863ad7f9/`.
- Executable SHA-256:
  `9bc65bc80d49a27df67307c420c1b2802062faf955577992f4995ce67a017936`.

Only Codex Daily was used; no agents were launched. Review of the two repair
diffs was performed by the coordinator, not a new independent review agent.

## Repairs and regression evidence

1. `e182671` — T3 advertised older history but supplied an empty continuation
   cursor. Capture incorrectly advanced its checkpoint and reported caught-up.
   `ReadChanges` now returns `history_cursor_missing` quarantine with the prior
   checkpoint. One focused regression fails on the parent and passes on the fix;
   capture/session race tests pass. Synthetic HTTP checks also cover partial
   continuation, resume, stalled/cyclic cursors, lineage and inherited rows.
2. `7249089` — incremental `RefreshStale` scanned unrelated archive revisions
   on every append. The roots query now starts from affected records and uses a
   record index; existing v2 profiles receive the additive index on open.
   A query-plan/upgrade regression detects the old scan without a brittle timing
   threshold. It fails with the old join and passes on the fix. Existing wire
   bytes, schema version, consent holds and outbox identities are unchanged.

The slow scale run was stopped after about nine minutes at roughly 2,500 records;
its stack identified the roots query. The repaired run fully ingested 4,097 hidden
knowledge rows plus an independent visible insight and completed retrieval in
6.17 seconds. Retrieval returned the visible insight and reported incomplete
coverage for the capped knowledge stream. These are different completion states,
so they are not a controlled speedup ratio or production capacity estimate.

Only these two focused regressions were added to the repository. Broader
exploratory probes live in a private temporary QA module outside Git and introduce
no product dependencies or permanent testing infrastructure.

## Executed evidence

Private artifacts are under
`~/.local/state/t3-orchestrate/runs/capsule-memory/20261028-overnight-qa/`.
They contain only synthetic runtime data and stay outside Git. `events.jsonl`
retains attempts; `cases.jsonl` applies conservative evidence limits;
`performance-summary.json` aggregates measurements. Scripts retain the actual
command arguments. Key commands and artifacts:

| Check | Command / scope | Result and artifact |
| --- | --- | --- |
| Full gate | `scripts/check.sh --phase all --range 6797fbfb39a6ff942809ea07864ca96317a699db..72490890d5b7f0684a77e8a3b21788c4863ad7f9` | Passed Go formatting/vet/test/build, relay TypeScript/formatting and commit subjects; `repaired-full-gate.log` |
| Focused race regressions | `go test -race -p=2 -count=1 ./internal/capture/t3 ./internal/session`; `go test -race -p=2 -count=1 ./internal/memory` | Passed; `t3-repaired.log`, `refresh-repaired.log` |
| Installed black-box journey | `node extended-e2e.mjs --cli ~/.local/bin/capsule-memory` | 13 local workerd scenarios passed; `repaired-e2e.log` |
| Private fault probes | `go test -race -mod=mod -p=2 -json -count=1 -timeout=90s -skip TestRankedHiddenKnowledgeRefillBound ./...` in private `harness/` | 37 top-level tests / 44 including subtests passed; one child-only helper skipped; `final-bounded-race.jsonl` |
| Scale/refill | Non-race `go test` with `-run 'TestRanked\|TestAmbiguous'` | Passed in 6.43s total; `probes-8.jsonl` |
| Real Cloudflare | `python3 live_skills.py`; `python3 live_concurrency.py` | Memory/skill lifecycle and eight concurrent records converged on two enrolled profiles; private scripts and timings |
| Platform builds | `GOOS=linux GOARCH=amd64 go build ... ./cmd/capsule-memory`; Windows amd64 equivalent | Cross-compilation passed; foreign-platform execution NOT RUN |

Go used the installed pinned 1.24.0 toolchain, `GOTOOLCHAIN=local`, cached
reviewed dependencies with `GOPROXY=off`, and bounded `GOMAXPROCS=2`/`-p=2`.
The installed build used `go build -trimpath -buildvcs=true -mod=readonly -p=2`.
Live memory/skill lifecycle ran before the two repairs; concurrent cloud writes
and the 13-scenario local journey ran against the repaired installed binary.

The private probes exercise commit rollback, WAL crash recovery, exact retry
bytes, lease replacement, budget rollover, source/config fencing, bounded capture,
corrections and uncited influence, candidate review, owner proofs, aggregation,
private IPC/MCP, strict/historical freshness, terminal withdrawal, ambiguous source
copies, and hostile signed input. Local signed-control/empty-tail projection
fixtures establish local transaction and policy behavior, not cloud bridge
archive durability or distributed atomicity.

A target-commit/source-receipt SQL fault was injected: target knowledge remained
durable, strict retrieval stayed hidden until source receipt retry, and recovery
did not duplicate knowledge. A POST barrier showed the asynchronous withdrawal
limit: an already submitted raw chunk can receive a receipt after withdrawal;
durable consent roots/holds remain. This is not instantaneous remote erasure.

## Performance

CLI timings include process startup, SQLite and network work; samples are small.

| Measurement | Samples | Median | p95 / completion |
| --- | ---: | ---: | ---: |
| Initial live store | 7 | 13.67 ms | 26.85 ms |
| Initial live sync | 23 | 1.061 s | 2.080 s |
| Initial live skill operations | 19 | 13.24 ms | 26.07 ms |
| Repaired concurrent store | 8 | 8.46 ms | 10.04 ms |
| Repaired concurrent sync | 4 | 4.368 s | 8.521 s |
| Daemon store | 1 | 7.86 ms | single sample |
| Shutdown with incomplete client | 1 | 1.52 ms | single sample |
| Repaired 4,097-row fixture + retrieval | 1 | — | 6.17 s |

The large fixture's race-instrumented variant was stopped after 114.49 seconds;
that variant is NOT RUN to completion. It is not a race pass or a functional
failure. The same fixture passed without race instrumentation; the remaining
private suite passed under race instrumentation in 20.253 seconds.

Final resource snapshot: normal memory pressure, nominal thermal state,
18.98 GiB estimated headroom, 286.4 GiB free disk, 0.08 GiB swap and no measured
swap growth. Snapshot observations do not establish peak use or multi-hour load
capacity. No deliberate host/real-quota exhaustion was attempted.

## Remaining checks and accepted limits

Linux/Windows runtime, other-user IPC, real multi-device hardware, every crash and
revocation ordering, distributed provider CAS/failures, real quota exhaustion,
long-offline/large-source scheduling fairness, and multi-hour thermal/load tests
remain unverified. A knowledge stream still has a 4,096-candidate scan cap and no
continuation cursor; it reports incomplete coverage. Published-body cache eviction
remains explicitly unimplemented. Model extraction accuracy is not established.

The resident local model service was stopped when checked. Synthetic loopback
HTTP proved rejection of reasoning-only replies and semantic validation, but new
resident-model inference was NOT RUN. No shared model, service or harness settings
were changed. Existing historical model evidence was not relabelled as this run.

No unresolved reproduced product defect remains in the executed scope. Partial
and blocked cases retain their individual limits in the ledger. Authenticated
metadata still exposes routing identifiers, sizes and timing; padding is not
traffic-analysis anonymity. Revocation cannot erase prior recipient copies.

## Cleanup and rollback

No QA daemon, MCP child, private probe executable, local workerd or task socket
remains. Aborted owned scale fixtures were removed. Existing owner capture,
publication, derivation, automatic acceptance and cross-vault controls are off;
all sources are disabled. Eleven held revisions remain, none published.
The two newly enrolled devices' test-scope grants were revoked and owner sync
caught up. Synthetic immutable remote history, private evidence and the revoked
device's queued draft remain intentionally; unrelated data and resources were not
removed. No autostart, hook, service or continuous monitor was installed.

The existing dedicated Worker and private bucket were retained. Postflight
metadata reads succeeded and managed public access remained disabled. OAuth
expired during the first metadata recheck; the existing Wrangler login refreshed
successfully without a new login or authority change. No paid upgrade or quota
exhaustion occurred. The planning envelope was at most 1,500 requests / 10 MiB new
encrypted QA data; an exact global request/billing counter was not available, so
this is not a billing audit or zero-cost guarantee.

Previous binary retained:
`~/.local/share/capsule-memory/releases/2af1f9d8fb4ea12dd5a129bbeae6dd60dad41b14/capsule-memory`
with SHA-256
`6b3de6f4872610e18eb25c2d25bdbf89124e37da18f87b73f929eaa0882ded4b`.
If rollback is needed, verify no task daemon is running, verify that digest, and
atomically repoint only the command symlink:

```sh
python3 - <<'PY'
from pathlib import Path
import hashlib, os
previous = Path.home() / '.local/share/capsule-memory/releases/2af1f9d8fb4ea12dd5a129bbeae6dd60dad41b14/capsule-memory'
assert hashlib.sha256(previous.read_bytes()).hexdigest() == '6b3de6f4872610e18eb25c2d25bdbf89124e37da18f87b73f929eaa0882ded4b'
command = Path.home() / '.local/bin/capsule-memory'
assert command.is_symlink()
temporary = command.with_name('.capsule-memory.rollback')
os.symlink(previous, temporary)
os.replace(temporary, command)
PY
```

Then update the private installation manifest to reflect the rollback. Do not
replace databases, overwrite WAL state or point a legacy pre-v2 binary at migrated
profiles. Local main and the public release are separate; no push is implied.
