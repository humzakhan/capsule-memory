# Optional session protocol

This document specifies the authored schema-v2 extension. **Compile, formatting,
validation, cryptographic interoperability and runtime acceptance are NOT RUN.**
The feature is optional. Capture, publication, derivation and promotion controls
start disabled. Cache eviction is deferred and reported as `not_implemented`.

## Compatibility boundary

Outer envelopes remain `version:1`. The legacy `agent-memory/aead/v1`,
`agent-memory/envelope/v1`, `agent-memory/control/v1`,
`agent-memory/request/v1` and `agent-memory/skill-approval/v1` byte domains are
unchanged. Schema-1 memory and skill plaintext remains unframed JSON, with the
same field order and serialization. New schema-2 records are always framed.
Framed schema 1 and unframed schema 2 are rejected. Existing v1 readers reject
magic's leading invalid UTF-8 byte and stop replay without advancing the cursor.

Use a dedicated session profile/vault by default. Separate scopes in the same
vault are supported. Legacy memory clients must never be granted v2 scopes.
Legacy Get, Recall, Write, FetchSkill and ListSkills cannot expose v2 records.
The generic visibility function always hides knowledge; only the projection
lifecycle and per-request freshness path may expose it.

## Compression before encryption

A frame is one revision in one scope, encrypted under unchanged v1 AES-GCM AAD.
All frame integers use big-endian encoding:

| Offset | Bytes | Value |
| --- | --- | --- |
| 0 | 4 | `89 43 4d 46` |
| 4 | 1 | frame version `01` |
| 5 | 1 | codec: writer always `01` raw DEFLATE; reader also accepts `00` identity |
| 6 | 1 | content `01`: revision JSON |
| 7 | 1 | flags `00` |
| 8 | 4 | uncompressed length |
| 12 | 4 | compressed body length |
| 16 | body length | codec body |
| after body | 0–4095 | zero padding to the next 4096-byte boundary |

`MaxFrame` and `MaxDecoded` are both 1,048,576 bytes. The writer's maximum
**entire marshalled revision** is `ChunkHardMax = 917,504` bytes (896 KiB).
The envelope remains capped at 2 MiB. JSON escaping counts toward these bounds.
Chunking measures the final revision, not only event text.

Each encoding creates a fresh stdlib DEFLATE writer at default compression,
without a dictionary. Closing the writer emits the final stream block.
There is no identity fallback, shared compressor, batching across sessions or
shared compression window. Redaction precedes serialization and compression.

Readers check minimum and maximum size, 4096-byte alignment, version, codec,
content, flags, declared lengths, and minimal all-zero padding. The DEFLATE
reader receives a `*bytes.Reader` directly, preserving its byte-reader interface.
Output is limited to declared length plus one. A fixed declared-size buffer is
filled with `ReadFull`; one further byte read must return zero bytes and EOF,
and the compressed reader must have no remaining bytes. Truncation, excess
output, trailing compressed data and nonminimal padding fail closed.

Decoded content passes UTF-8, duplicate-key/depth checks, exact field names,
unknown-property rejection, required/non-null checks, logical schema validation,
and vault/scope/author checks. A frame alone is not a valid revision.

Compression, padding, encryption and signing happen during Prepare. Commit
persists the exact signed envelope in the durable outbox. Retries reuse those
bytes; they do not generate another nonce, transport identity or compressed body.
SQLite keeps the original uncompressed marshalled revision for local JSON queries
and idempotence comparison. SQLite and its backups contain plaintext.

## Logical record kinds

| Kind | Purpose |
| --- | --- |
| project | Encrypted project name and identity hints |
| session_manifest | Stable origin, policy, lineage, bounded branch high-water marks and omissions |
| session_chunk | Immutable redacted events, source generation and content digest |
| summary | Findings with exact source heads and routes to primary chunks |
| insight | Candidate or reviewed memory/decision with provenance and evidence |
| knowledge | Reviewed projected assertion, bound to a lifecycle link and source revision |
| projection | Source or target lifecycle state and complete source-scope attestation |
| bridge_grant | Owner-signed cross-vault bridge authority |
| aggregation_grant | Owner-signed aggregation source and destination authority |
| job | Authorized durable assignment request |
| suppression | Origin-preserving deletion/exclusion suppression |

The concrete structures and bounds live in `internal/records`, with authored
schemas and synthetic examples in `contracts`. Timestamps are metadata, never
conflict ordering. Concurrent heads and tombstones retain v1 ancestry semantics.

Derived records cite exact source revisions. Dependency visibility is transitive
and considers only dependencies of current heads. Deleted, retracted, stale,
missing or superseded sources hide dependents. An invalidation closure over
100,000 rows aborts the entire transaction. Rebuild recomputes dependencies and
staleness after deterministic applier order, while retaining durable local state.

An evidence-free insight is permitted only for an independent owner assertion:
no generator, explicit basis, accepted/edited review, empty evidence, contradictions
and influence, and a verified `owner_proof` bound to this exact revision.
`Reviewer` must equal the revision author. A device signature alone cannot claim
this exception. Ordinary evidence-bearing review remains writer-attested; it is
not cryptographic proof of human review or owner authority.
Keeping an insight during session deletion writes that independent form; it does
not retain a false evidence claim. This exception does not authorize automatic
model acceptance or projection of auto-accepted material.

## Projection and grants

Knowledge, its target projection and its bridge grant share one v2 target scope.
Knowledge is never cloned into a v1 memory record. Target readers verify the
unique current lifecycle revision, exact knowledge/source revision and text
checksum, attestation, author authority, and an unrevoked owner grant when the
source is another vault. Historical disclosure carries `current:false` unless a
live proof was made in that request. Strict disclosure requires a bounded live
source replay and exact dependency-head check; missing bridge, offline source,
busy scope, incomplete replay or any failure hides the result. A 24-hour
attestation interval is a maintenance cadence, not a freshness guarantee.

New owner signing bytes use these exact domains:

```text
capsule-memory/bridge-grant/v1\n<base64 JSON payload>\n
capsule-memory/aggregation-grant/v1\n<base64 JSON payload>\n
```

Each domain signs its UTF-8 bytes, with actual newline separators, using owner
Ed25519 authority. `bridge_grant` has exactly one target scope, equal to its
ledger scope. An aggregation grant binds the vault, name, source scopes,
destination and revocation. Every destination reader must also hold every source
scope. Grant-change hooks check proposed control before signing or writing a key
package; derivation checks again before leasing and inside its result transaction.

The local job identity is SHA-256 over UTF-8:

```text
capsule-memory/job-key/v1\n<kind>\n<scope>\n<subject>\n<sorted input revision ids joined by comma>\n<versions>\n<destination>\n
```

Local bridge jobs use the source scope as their queue destination. Their actual
cross-vault destination lives in durable projection state and is checked by the
bridge. Queue scope is not authority to access another vault.

## Local transactions and scheduling

A query-only pool has at most four reader connections. `Read` uses a snapshot.
One `wmu` serializes write transactions using `BeginTx`, equivalent to the
contract's single-writer choice; no network or model call is made in a transaction.
Prepare checks current device grant and key epoch before encryption. Commit
rechecks authorization and implicit parent heads, inserts revisions and exact
outbox bytes, runs appliers and caller extras, and commits atomically.

Replay still requests `limit=1`. A page's revision, derived rows, invalidations,
receipt and cursor commit together. Unknown frame/schema/kind, missing applier or
stale overflow blocks the scope, persists `replay_block`, and retains the safe
cursor. Successful replay clears that scope's block. Default ticks process up to
100 pages, so large backfills have an explicit throughput limit.

Each scope has a capacity-one sync semaphore. Waiting observes the caller's
context; a bounded strict read receives `ErrSyncBusy` rather than waiting without
a deadline behind a publisher. Durable outbox holds survive rebuild. Manual sync
also skips held revisions and continues to other queued records. A request already
submitted to the relay cannot be recalled by a later consent withdrawal.

Durable jobs have deterministic keys, capped leases and random attempt tokens.
Completion requires the current unexpired token. Cancellation includes pending,
leased and blocked jobs and clears leases/tokens, rejecting late model results.
`CancelJobs` with an empty scope covers all scopes; its kind filter always applies.

## Profile migration and owner administration

New profiles initialize local schema 2. Existing schema-1 profiles remain usable
for legacy operations. Session features require explicit `profile migrate`.
Migration takes an exclusive nonblocking profile lock, creates a unique empty
`memory.db.pre-v2.<timestamp>` file with `O_EXCL` at 0600, takes `VACUUM INTO`
that private target, fsyncs file and directory, then applies additive DDL in one
transaction. A failed migration retains the snapshot and reports its path.

Linux/macOS daemon profiles take exclusive locks; direct clients take shared
locks. A busy profile returns `ErrProfileBusy` for IPC routing. The daemon is
unsupported on Windows. Locks live in private `profile.lock` files.

`device.json` keeps its original Config fields. Scope labels and roles live in
`scope_labels` and private key packages. Adding a scope journals its identity,
key and base generation in `pending-scope.json`, atomically replaces private
`device.json`, commits signed control and label together, then removes the journal
and fsyncs the directory. Open and admin operations recover a matching pending
scope, or report `scope_add_incomplete` without inventing another identity.

GrantDevice checks hooks, writes a private `O_EXCL` key package, then commits the
signed control. A crash after package creation leaves unauthorized key material;
retrying with a new output path completes the control. Scoped child profiles
contain exactly the selected keys. Import merges only matching vault/device keys,
rejects older epochs or same-epoch key substitution, and journals the private
filesystem/SQLite update for recovery. Scope grant revocation stops future relay
fetches; old copied keys remain until owner-directed rotation.

Rebuild preserves outbox, holds, source association, jobs, checkpoints, event
identity, policy/consent, source definitions, pauses, project paths, projection
coordination, budgets and scope labels. Derived tables are reconstructed in order:
project, manifest, chunk, suppression, summary, insight, knowledge, projection,
bridge grant, aggregation grant, job, then transitive stale state.

## Remaining acceptance

The source repair contract below supersedes earlier implementation handoff
wording about append reconciliation, citation-only influence and shape-only G5.

### Exact owner assertions

`OwnerProof{payload,signature}` is appended after Generator in Insight and omitted
from ordinary bodies. The payload is strict standard-base64 JSON of
`OwnerAssertionBody`, in this field order: version, vault_id, scope_id, record_id,
revision_id, operation_id, operation, parents, restores, author_device_id,
content_sha256. Version is 1; operation is put, resolve or restore. Ordered parents
and restores bind the revision exactly; empty ancestry compares consistently with
nil and creation emits empty arrays. IDs use the ordinary ID bounds; each ancestry
list has at most 128 entries. The hash is lowercase SHA-256 of
`json.Marshal(records.Insight)` with OwnerProof nil. Future insight fields must
be omitted when empty to preserve old proof-free serialization.

`PrepareOwnerInsight` requires the local owner key and seals after revision IDs,
operation, author and parents exist. Existing `sign` / `OwnerSign` /
`VerifyOwner` helpers sign the new domain using the existing pinned OwnerPublic.
Decrypt checks local commits, replay and pending retries. The insight applier
independently checks shape, missing proof, signature and every binding on rebuild.
Retries retain the exact prepared envelope and proof. Invalid signed records
fail closed without advancing replay; revoking their writer does not repair
poison already journaled. Owner-key rotation is outside this contract.

Owner edits or accepts of an independent assertion require a fresh proof.
Candidate reject returns an actionable instruction to use a separately authorized
explicit delete; it creates no new delete grant. Tombstones keep existing writer
semantics. A future independent-insight restore must bind fresh IDs and restores;
session restore restores manifests, not withdrawn insights.

### Repair wire and local storage amendments

Summary now requires `influence` and `input_fingerprint`; Insight adds optional
`influence`. Influence contains every consumed prompt event, merged only across
contiguous surviving array positions, independently of selected citations.
Intermediate-summary influence propagates transitively. Coverage-only inputs
never claim influence. Input fingerprints include rendered lines, heads, gaps and
omissions; final/window commits and cached windows compare exact snapshots.
Reference arrays allow 16,384 refs; job inputs remain 4,096. Dependency unions
are bounded to 32,768 before coverage edges; overflow rejects, never truncates.

Projection requires positive integer `generation` and adds optional
`owner_withdrawn`, valid only with state withdrawn. OwnerWithdrawn is terminal for
the exact logical link/source/target vault, scope and record binding across
concurrent heads, later heads, replay and rebuild. It authenticates author intent
through the envelope, separately from the G5 owner-key proof. Source coordination
retains monotonic generation and terminal intent; jobs carry decimal generation
in Versions, attempt tokens and observed source parents. Old work cannot finalize
a new generation or write over authenticated target withdrawal. New promotion
after withdrawal needs a new explicit request/link.

Derived `projection_records` preserves every record/scope/link/role/target variant;
`knowledge` keys link/scope/record. Historical terminal assertions remain indexed
through deletion. Retrieval uses these indexes plus exact heads and grants.
`manifest_origins`, `manifest_bindings` and `event_lookup` locate affected view
groups and exact events. Correction apply indexes only that revision; immutable
correction history is rebuilt only on rebuild. Stale refresh walks affected
descendants, preserves unchanged markers and fires hooks only on transitions.
Rebuild pages revisions and reconciles manifest groups separately. Closure
overflow rolls back the whole operation and replay cursor.

Local `projections` also pins source_vault, source_scope and target_vault, alongside
source/target records and scope. This binding lets terminal intent survive even a
later lifecycle tombstone without selecting an unrelated conflicting body.
Cleanup can finish in durable coordination without implicitly restoring that
deleted source lifecycle.

Durable `consent_roots(revision,source_id)` and `outbox_hold` survive rebuild and
re-enable. Withdrawal follows exact historical ancestry, holds unpublished
automatic descendants and cancels affected leases. Derive/bridge commits recheck
consent. Before upload, cross-vault target descendants require a source-profile
consent check; unavailable source retains bytes locally, withdrawn consent adds
a durable target hold. Already submitted bytes and published data cannot be
recalled by this mechanism. It does not replace explicit archive deletion.

The publish check runs immediately before POST, after closing target query rows,
without a writer lock, nested cross-profile transactions or network under a
writer lock. Router matching uses source vault/scope/record/revision and exact
dependency ancestry, never a profile or link name as authority. Missing ancestry,
nil router and router errors retain bytes temporarily; only confirmed withdrawal
creates a durable hold. Every matching local source copy must be checkable.
Cleanup tombstones and hidden/withdrawn lifecycle records are exempt from
automatic export holds so source loss cannot prevent cleanup publication.
Publication also requires every same-record parent to be known durable: either
replayed from the relay, or a local outbox revision with an acknowledged receipt.
A local-only or ambiguously submitted parent stays unconfirmed. Cleanup cannot
release held parents or POST a child that would poison replica replay.

Projection adds `withdrawal_marker:true` for minimal opaque cleanup. Source
manual terminal intent uses a separate source-withdrawal record; target
hidden/withdrawn intent uses a separate target-cleanup record. First marker
revisions are independent roots; later marker-only ancestry is checked normally.
They carry exact logical link/source/target vault, scope and record, generation,
state/terminal intent and bridge grant. Both private revision IDs are omitted,
dependencies are [], attestation absent and text_sha256 is the reserved all-zero
sentinel, not an assertion digest. Projected/pending markers are invalid.
Source markers require terminal owner intent; target markers require authenticated
target scope authority and either historical same-vault source scope authority
or the exact verified owner bridge grant for author/vaults/target scope.
The applier/rebuild checks this independently of private source/target presence;
pre-POST additionally requires the matching grant to be known published.
Normal projections gain no ancestry exemption. Invalid/unrelated markers cannot
hide a different logical link/binding. Terminal marker history survives later,
conflicting or deleted heads and rebuild. Recoverable hidden markers are retired
in the same target transaction as a newer valid projection; terminal history
still prevents an owner-withdrawn link from returning.

Cleanup with unpublished private parents remains queued while its independent
marker can publish safely. `publication_block` records informational temporary
reasons without releasing holds. Capture status exposes per-scope Held,
PublicationBlocked, WaitingAncestry, CleanupWaitingAncestry, PendingMarkers and ReadyMarkers.
Ready marker counts check parent publication and authenticated published grants
in the local snapshot; MarkerStatusBounded labels a count limited to1000 markers.
An owner-proofed kept insight still uses the same logical record and revision
ancestry. OwnerProof severs evidence/consent dependencies, not the revision DAG.
If a required parent is private/held, the verified independent assertion remains
valid local work and reports `waiting_ancestry` / WaitingAncestry; this is not
proof failure. Published/replayed ancestry can publish normally. No held parent
is released, skipped or implicitly cloned. PrepareOwnerInsight with an empty
record can internally create a new root, but no fresh-root owner CLI operation
is supplied by this repair.
Schema-1 legacy manual sync is unchanged. Direct target-only schema-v2 manual
sync cannot publish automatic cross-vault exports without the source consent
router. Re-enabling capture or rebuilding never releases an existing hold.

CrossVault off cancels pending/leased exports and queues existing links for
withdrawal while invalidation remains enabled. Source-aware reads deny current
proof immediately; offline target-only propagation remains asynchronous.
Unknown/offline/busy proof retries without destructive lifecycle writes.
Historical resolver failures retain lifecycle-valid text with current:false and
visible reason/coverage issues; strict hides it. Search ranks bounded eligible
candidates across scopes and kinds, hides internal windows and marks partial
coverage. Per-link bounds replace the former whole-vault projection scan.

Scheduler-blocked aggregation reasons are local status. Session scheduling
advances a durable rowid cursor past terminal failures; explicit admin derive
retry resets only failed/cancelled derive jobs, never done or consent-held work.
Candidate accept/edit/reject requires the observed revision ID and explicit
parents; a concurrent unobserved head rolls the decision back. Edited text passes
current redaction policy before persistence. LastSummary orders local insertion.

Checkpoints add independent `tail_digest`; ordinary file appends retain offset.
T3 walk completion is durable. Lineage/metadata overflow quarantines visibly
without silently dropping audit data. Gap arrays aggregate by reason with counts;
local source_gaps retains detailed audit. An unmigrated profile reports
needs_migration while healthy profiles continue; migration remains explicit.

Authored frame vectors use hand-computable stored DEFLATE blocks. They are not
executed evidence. There are no new generated AES-GCM/Ed25519 vectors, test files,
relay changes or protocol interop claims. The owner must run formatting, compiler,
SQLite migration/recovery, cancellation/concurrency, adversarial frame cases,
synthetic adapter journeys and cross-language acceptance in an authorized isolated
job before treating this source as operational.

### Integrated correction and retrieval semantics

`deps` stores child revision, parent record/revision, event_from/event_to and an
`evidence` flag. Evidence ranges use source array order, not lexical event IDs.
Both endpoints must be present, ordered and belong to a session_chunk. Empty
evidence ranges conservatively cite the entire chunk. Coverage edges still
require exact unambiguous current heads but do not claim every event was cited.

`retracted_events(scope,session,event_id,correction_revision)` is reconstructed
from immutable correction events, including corrections received before the
source or summary. A native event-group retraction covers split parts. Raw chunks
remain visible as provenance; dependent current heads that consumed retracted events, including uncited input,
are hidden. Primary is rebuilt from surviving evidence before summary creation.
A new revision of the same summary record replaces its live dependency edges and
recomputes stale state in the same transaction. No permanent chunk retraction
root prevents regeneration. Session tombstones/suppression independently hide
all raw chunks and dependent records, including late native views and manifest
continuations. Explicit restore removes named deletion/suppression state while
immutable corrections continue to invalidate old evidence.

The closure limit remains 100,000 rows; overflow aborts the whole transaction.
Rebuild uses the same appliers and immutable correction index, then recomputes
stale state. It preserves durable capture metadata, source gaps, backfill budgets,
origin-path admission metadata, outbox holds, jobs and bridge coordination.

Projection source dependencies always include the exact source insight even for
an independent owner assertion. Strict proof traverses exact current revisions
and all dependency scopes after bounded live replay. Proofs include the exact
target knowledge revision, not only its record ID. Retrieval compares the proof
with the actual returned row and rechecks lifecycle/grants in its final snapshot.
Mismatch cannot be current; this implementation hides it. Historical reads with
no live proof remain current:false. Coverage never bypasses revision freshness
or grant authorization. Legacy memory paths continue to reject v2 knowledge.

A live proof also carries a request-local source fence over the immutable revision
high-water mark, signed control, consent-root watermark, policy and key-map identity. Resolve and retrieval check
that fence after releasing target snapshots. Source correction, grant withdrawal
or key replacement during that interval invalidates current:true; strict hides,
and historical output can remain visible only as current:false. There is no
cached current proof or cross-profile nested read transaction. The last guard
check is the request's observation boundary; later writes require a new request.

T3 rollback uses stable archived run IDs and preserves original project scopes
when a thread has moved projects. Session-generated empty-text correction copies
still pass normal scope grant checks. Large correction reference sets split
without dropping targets; raw export retains their part provenance.
