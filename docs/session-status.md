# Session module source status

**Integrated source authored; independent source review and runtime acceptance pending.** This record describes work on `feat/session-history` from base `a08ffd8fb7a2fcd3d0f4b848fb919295d6dbf758`, 2026-10-26. It does not establish a compiled binary or working harness/model integration. The initial proposal is preserved in [session history](session-history.md).

| Area | Source location | Execution evidence |
| --- | --- | --- |
| Schema-v2 records, DEFLATE frame, revision/outbox transactions, migrations, scope grants and stale closure | `internal/records`, `internal/frame`, `internal/memory` | NOT RUN; integrated source requires independent review |
| Capture, redaction, suppression, durable checkpoints, daemon/IPC/MCP/CLI | `internal/capture`, `internal/session`, `internal/daemon`, `internal/app` | NOT RUN; adapters have no accepted runtime version |
| Local loopback-only OpenAI-compatible inference | `internal/inference/inference.go` | Authored; NOT RUN |
| Session/window summaries, daily project and opted-in weekly personal rollups, candidate review, budget/lease checks | `internal/derive` | Authored; NOT RUN |
| Owner-signed aggregation grants, v2 knowledge promotion, lifecycle and live freshness checks | `internal/projection` | Authored; NOT RUN |
| Hand-authored protocol/adapter examples | `contracts/` | Authored data; not validated or executed evidence |

All installation, dependency resolution, formatting (`gofmt`), compilation, vet, tests, validators, runtime, daemon, inference, real-format capture, synchronization and production provider checks are **NOT RUN**. No new automated tests were authored for this task. Source review and `git diff --check` are the only permitted checks; the implementation handoff records their actual result.

## Defaults and practical limits

Capture, publication, derivation, automatic acceptance and cross-vault promotion start off. A source must be previewed and enabled explicitly; backfill needs separate bounds. Cross-vault setup requires explicit `enabled_unaccepted`, which is not runtime acceptance.

The daemon targets Linux and macOS only. macOS IPC relies on private directory/socket permissions; Linux also checks peer credentials. Windows keeps direct legacy mode. No service/hook is installed by source authoring. Version-pinned Claude/Codex files and the pinned T3 HTTP history API can change; unknown versions/shapes fail closed. Codex App Server is not spawned. A T3 owner-supplied read token is broader than history access upstream even though the adapter uses only allowlisted GET paths.

Inference uses HTTP to literal loopback or localhost, checks the resolved dial address, disables proxies/redirects and sends no tools. Token counts are byte estimates unless the server supplies larger usage. Daily budgets reserve the configured maximum per call before inference; failed/crashed attempts keep the reservation. This is deliberately conservative and can exhaust a day early. CPU/RAM hints do not enforce model-server limits. Long inputs are windowed and reduced through at most eight intermediate levels, bounded to 64 MiB of revision bodies, 4096 input references and 4096 initial windows; oversized jobs block or fail visibly instead of truncating evidence.

Strict projected knowledge is hidden without a live source replay completed during the same request. Busy/offline scopes, revoked access, timeout and incomplete replay fail closed. Historical results may be stale and carry `current:false` and last verified heads/time. Twenty-four-hour re-attestation is a maintenance cadence. Cross-vault deletion is asynchronous; prior disclosure is irreversible. Legacy memory/skill paths reject all v2 kinds.

Plaintext body cache eviction is **not implemented**, including for published revisions. Status reports `cache_eviction: not_implemented`. SQLite, indexes, summaries and private snapshots remain plaintext on the trusted device. Redaction is best effort; compression/padding do not hide all traffic or ratio metadata. No embeddings, cloud inference, binary attachments, destructive retention or universal harness resumption is provided.

## Next evidence

The coordinator must checkpoint the integrated source and complete independent source review. The owner can then use the [runbook](run-session.md) on an explicitly permitted machine/job, with disposable synthetic profiles and a local model. Preserve the exact commit, tool versions, commands, failures and retained artifacts. A skipped or unavailable check remains NOT RUN.

## Integration representation

Corrections retain raw events and reconstruct scoped `retracted_events` from
immutable correction records. Current-head dependency edges preserve exact event
ranges and distinguish evidence from coverage. Retraction hides cited evidence;
coverage still requires exact current revisions, visibility and authorization.
New summary revisions can use surviving events from the same immutable chunk.
Deletion and suppression remain separate, group-wide visibility conditions.

T3 project attribution uses worktreePath, then thread.projectId against
shell.projects[].workspaceRoot. The pinned primary schema is upstream t3code
`d8d037eae1b77a77f372fd1afd2662a31ebe37c8`, not a live-harness observation.
Generic import origins preserve bounded arbitrary harness labels. Capture uses
stable per-device/source/stream branches, bounded buffers, durable gap audits and
snapshot-pinned export. Codex turn deduplication survives poll boundaries using
only hashes and event references in checkpoint state.

Live projected retrieval binds the proof to the exact target knowledge revision,
source revision/vault/scope and link. A concurrent target replacement is hidden
if its row differs from the resolver proof. Final retrieval rechecks current
heads, lifecycle and grants in one local snapshot. Historical output never gains
`current:true` from a mismatched or unavailable proof.

A live proof also carries a request-local source fence over the immutable revision
high-water mark, signed control and key-map identity. Resolve and retrieval check
that fence after releasing target snapshots. Source correction, grant withdrawal
or key replacement during that interval invalidates current:true; strict hides,
and historical output can remain visible only as current:false. There is no
cached current proof or cross-profile nested read transaction. The last guard
check is the request's observation boundary; later writes require a new request.
