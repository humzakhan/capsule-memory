# Session module source status

**Source repairs reviewed; runtime acceptance pending.** The original
`feat/session-history` delivery used base
`a08ffd8fb7a2fcd3d0f4b848fb919295d6dbf758`, 2026-10-26. The follow-up
`fix/session-history-source-repair` starts from current-main base
`e27cd2ee12bae4fce683a0af74f959cf5710d0f5`, after PR #1 merged. Neither
source review establishes a compiled binary or working harness/model integration.
The initial proposal is preserved in [session history](session-history.md).

## Follow-up source evidence — 2026-10-27

The combined source review examined `578421bc468fba98a55eabebd72d94f6c29bfaba`:
capture/consent snapshots and durable buffering, adapter attribution and rewind,
revision-filtered retrieval, exact bridge publication/freshness context, ordered
local commit exclusion, lease-bound budget blocking, ranked refill, daemon/IPC
ownership and CLI export finalization. It found one high-severity gap: target-only
key issuance could disclose already archived same-vault assertions. The core now
checks retained immutable source/target reader bindings before grant/enrollment
output, independently of projection hooks or lifecycle deletion. Focused review
at `cf221adba7b2f555252f1bf8d05cfa286dfebb22` closed that finding by source
inspection and established no additional concrete consequential finding.

Permitted `git diff --check` checks passed for the combined and correction ranges.
Nine regression files contain focused synthetic cases for concrete uncovered
behavior; all are **NOT RUN**, as are the authored ordering recipes. Source review
covered the combined diff and relevant surrounding operations, not exhaustive
analysis or runtime/security acceptance. Formatting, compilation/type correctness,
SQL/FTS/driver behavior, races/crash recovery, platform/harness compatibility,
cryptographic interoperability and provider durability remain unverified.

Ranked knowledge scanning is capped at 4096 candidates per scope/kind stream;
exhausting that bound reports incomplete coverage. Repeating the same query does
not advance a continuation cursor. Grant checks protect locally retained history,
not undiscovered remote history or previously disclosed keys. Writer exclusion is
local; private files and SQLite commits are separate durability boundaries.
Shutdown joins owned work, but mutex/driver/filesystem delays have no proven
latency bound. Published-body cache eviction remains explicitly unimplemented.


| Area | Source location | Execution evidence |
| --- | --- | --- |
| Schema-v2 records, DEFLATE frame, revision/outbox transactions, migrations, scope grants and stale closure | `internal/records`, `internal/frame`, `internal/memory` | NOT RUN; source inspection only |
| Capture, redaction, suppression, durable checkpoints, daemon/IPC/MCP/CLI | `internal/capture`, `internal/session`, `internal/daemon`, `internal/app` | NOT RUN; adapters have no accepted runtime version |
| Local loopback-only OpenAI-compatible inference | `internal/inference/inference.go` | Authored; NOT RUN |
| Session/window summaries, daily project and opted-in weekly personal rollups, candidate review, budget/lease checks | `internal/derive` | Authored; NOT RUN |
| Owner-signed aggregation grants, v2 knowledge promotion, lifecycle and live freshness checks | `internal/projection` | Authored; NOT RUN |
| Hand-authored protocol/adapter examples | `contracts/` | Authored data; not validated or executed evidence |

All installation, dependency resolution, formatting (`gofmt`), compilation, vet, tests, validators, runtime, daemon, inference, real-format capture, synchronization and production provider checks for this source delivery are **NOT RUN**. The original task authored no automated tests. Later repairs may add only focused regression source for concrete uncovered defects; that source remains unexecuted. Source review and `git diff --check` are the only permitted checks in the repair task; its handoff records their actual result. Historical setup checks in [verification](verification.md#setup-gate-baseline-2026-10-27) retain their original source identity and do not verify these repairs.

## Defaults and practical limits

Capture, publication, derivation, automatic acceptance and cross-vault promotion start off. A source must be previewed and enabled explicitly; backfill needs separate bounds. Cross-vault setup requires explicit `enabled_unaccepted`, which is not runtime acceptance.

The daemon targets Linux and macOS only. macOS IPC relies on private directory/socket permissions; Linux also checks peer credentials. Windows keeps direct legacy mode. No service/hook is installed by source authoring. Version-pinned Claude/Codex files and the pinned T3 HTTP history API can change; unknown versions/shapes fail closed. Codex App Server is not spawned. A T3 owner-supplied read token is broader than history access upstream even though the adapter uses only allowlisted GET paths.

Inference uses HTTP to literal loopback or localhost, checks the resolved dial address, disables proxies/redirects and sends no tools. Token counts are byte estimates unless the server supplies larger usage. Daily budgets reserve the configured maximum per call before inference; failed/crashed attempts keep the reservation. This is deliberately conservative and can exhaust a day early. CPU/RAM hints do not enforce model-server limits. Long inputs are windowed and reduced through at most eight intermediate levels, bounded to 64 MiB of revision bodies, 4096 input references and 4096 initial windows; oversized jobs block or fail visibly instead of truncating evidence.

Strict projected knowledge is hidden without a live source replay completed during the same request. Busy/offline scopes, revoked access, timeout and incomplete replay fail closed. Historical results may be stale and carry `current:false` and last verified heads/time. Twenty-four-hour re-attestation is a maintenance cadence. Cross-vault deletion is asynchronous; prior disclosure is irreversible. Legacy memory/skill paths reject all v2 kinds.

Plaintext body cache eviction is **not implemented**, including for published revisions. Status reports `cache_eviction: not_implemented`. SQLite, indexes, summaries and private snapshots remain plaintext on the trusted device. Redaction is best effort; compression/padding do not hide all traffic or ratio metadata. No embeddings, cloud inference, binary attachments, destructive retention or universal harness resumption is provided.

## Next evidence

The combined repair pass adds terminal link generations, exact consumed-event
influence and snapshot checks, transitive consent holds, indexed affected-group
refresh, append-preserving capture, durable T3 completion, fair session scheduling,
expected-revision candidate decisions and independently verified G5 owner proofs.
Contracts include hand-authored recipes only; no signatures or computed vectors
were fabricated. Same-vault and cross-vault target publication retain queued
automatic bytes until source consent can be checked.

Independent source reviews examined the initial implementation and repairs. The
focused final review at `d42c525` found two import errors and a same-vault consent
bypass. The coordinator corrected all three and reread the affected paths; no
compiler or runtime was used. That review traced owner proof, withdrawal markers,
publication consent, bridge generations, candidate decisions and lock ordering.
It did not deeply re-review capture, correction influence, scheduling, freshness,
compression, migrations, schemas or fixtures. This is source-only PR evidence,
not a blanket security approval or execution acceptance.

Cleanup publication preserves revision ancestry. A child tombstone or lifecycle
change waits when its parent is local-only, held or lacks a confirmed receipt;
replayed and acknowledged parents can publish normally. Minimal independent
withdrawal markers can publish without private assertion revisions, using the
existing authenticated scope/bridge-grant binding. Status separates held bytes,
`waiting_ancestry`, cleanup waiting on ancestry and pending/ready markers.

DeleteSession keep retains the same insight record. Its verified owner proof
severs evidence and consent dependencies, not revision ancestry. A kept assertion
whose parent remains private/held is valid retained local work with
`waiting_ancestry`, not proof failure. No implicit clone, new ID, wider grant,
ancestry skip or held-parent upload occurs. This pass adds no fresh-root owner
CLI; empty-record PrepareOwnerInsight remains an internal capability.

The next evidence is an authorized isolated build and behavior pass using the [runbook](run-session.md), with disposable synthetic profiles and a local model. Preserve the exact commit, tool versions, commands, failures and retained artifacts. A skipped or unavailable check remains NOT RUN.

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
hashes and event references plus bounded directory metadata in checkpoint state;
no prompt plaintext is retained there.

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
