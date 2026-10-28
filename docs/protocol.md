# Synchronization protocol

**Status: full protocol target.** The record schema illustrates the inner logical model; [the v1 contract](v1-contract.md) specializes exact implemented framing and the narrower manual-sync/text-skill scope. The unreleased [enrollment contract](device-enrollment-protocol.md) adds owner-reviewed initial device key delivery without changing that framing. Automated epoch rotation/distribution and full security acceptance remain separate work; use the current release evidence rather than historical source-only notes for executed checks.

## Identity and records

| Field | Meaning |
| --- | --- |
| `vault_id`, `scope_id` | Stable identity and authorization boundary; opaque outside trusted devices |
| `record_id`, `kind` | Stable logical record and memory, skill, or summary type |
| `revision_id`, `parents` | Immutable revision identity and same-record ancestry; multiple parents express explicit resolution |
| `operation_id` | Stable logical mutation identity across retries and authorized re-encryption |
| `operation`, `restores` | Put, delete, restore, or resolve; restore explicitly identifies a deletion revision |
| `author_device_id`, `created_at` | Provenance and display time; time never settles conflicts |
| `provenance`, `tags`, `payload` | Origin, classification, and content inside encryption; summary payloads name source revisions |

These fields belong inside the encrypted record. The outer envelope contains only the routing/authentication fields needed by the relay: protocol version, opaque vault and scope, device verification identity, membership/key epoch, transport identity, ciphertext reference/digest/size, and signature material. Outer fields are authenticated and bound to the encrypted inner context.

The signature framing is deliberately not specified by a JSON example. Define exact domain-separated bytes, encoding, limits, rejection rules, and algorithm/version negotiation before coding it. No implementation may sign an ad hoc serialization and assume Go and TypeScript produce identical bytes.

Publication cursors and epochs use canonical nonnegative decimal strings across Go and JavaScript; compare numerically, not lexicographically. IDs are opaque, bounded values. Reject unsupported protocol versions and malformed, oversized, or scope-inconsistent records before applying them.

## Local write and outbound queue

1. Validate caller scope and operation semantics. Create the immutable revision and its signed encrypted envelope on the device. Store the revision, exact envelope bytes, transport identity, and pending outbox state in one SQLite transaction before reporting local success.
2. When online, refresh membership and current epoch before sending the queue. A revoked device stops publishing. If still authorized after a key rotation, rewrap or re-encrypt pending work under the allowed epoch using a new transport identity while retaining its logical operation identity.
3. Retry persisted bytes with capped exponential backoff and jitter. Distinguish transient transport failures, throttling, permanent validation failures, and revoked credentials. Preserve failed operations for inspection; do not silently drop them.
4. Mark an operation remotely published only after the relay returns or replay proves its durable archive publication. Keep publication receipts and the applied local cursor crash-safe.

Same logical operation ID plus same decrypted revision is idempotent. The same identity with different content is an integrity conflict and must be quarantined. Transport deduplication and logical deduplication are distinct because rekeying changes ciphertext.

## Remote publication

The per-vault coordinator serializes publication, including owner-signed control operations. It uses the storage head's conditional version token as a fence; process-local exclusivity alone is insufficient.

1. Authenticate the submitting device against current owner-authorized membership, opaque scopes, and epoch. Verify envelope signature and size limits without decrypting. The relay cannot validate private logical content.
2. Create immutable ciphertext objects. A retry that finds an existing object verifies the expected digest and length. An upload's missing response is an ambiguous outcome, not proof of failure.
3. Write an immutable journal segment referencing the objects, preceding head, transport identities, and new cursor. Persist any deduplication data needed to recover from a coordinator reset.
4. Conditionally replace the durable head only after all referenced objects are committed. On a precondition failure, reload the head and reconcile the competing publication. Do not blindly replace it.
5. Acknowledge only a publication reachable from the committed head. Update rebuildable DO caches and send WebSocket hints afterwards. Crash before a hint is repaired by replay.

The head and journal are content-blind manifests: they expose routing metadata, not memory text. An authorized device verifies the signed content envelopes, membership transitions, and available chain/checkpoint evidence before application. Sequence order is publication order, not record conflict resolution.

Do not keep the only deduplication entry or membership state in DO memory or a disposable database. After a reset, rebuild from durable checkpoint/journal state before admitting writes. A device can safely replay a duplicate revision even if a repeated transport entry reaches the journal; it must never create a second logical mutation.

Version one uses bounded single-object uploads. Multipart upload is optional later. A partially uploaded or fully uploaded but unpublished object is an orphan, never visible committed state. Garbage collection must retain a grace period and prove non-reachability, including in-flight publication, before deleting orphans.

## Receiving changes and discovery

Clients reconcile on service startup, network reconnection, an authenticated hint, and a configurable periodic interval. An initial five-minute interval with jitter is a scheduling proposal, not a propagation guarantee; quota pressure and battery policy can lengthen it. Hints contain only opaque vault/cursor information.

Read the durable head and obtain missing authorized journal segments or a checkpoint plus tail. The relay filters payload access by opaque scope; a client's global cursor may advance across entries outside its grant without downloading their payloads. Requests use explicit pagination and continuation state, never an unbounded archive scan.

Validate object hashes, signatures, authorization history, version, and encrypted context. Apply a batch's revisions, visibility/conflicts, summary invalidations, and applied cursor atomically in SQLite. If any required item is unavailable or invalid, preserve the last safe cursor and surface a repair state. Do not skip data merely to advance progress.

A reconnecting client first brings control state current, then reconciles its existing outbox, then publishes still-authorized work. It can catch up through checkpoints without replaying every historical journal file. Its local operation IDs and revision parents remain unchanged.

## Conflict rules

| Situation | Required result |
| --- | --- |
| A and B edit the same parent while offline | Keep both concurrent heads. Present both with provenance; no last-writer-wins |
| Explicit resolution | New revision names all resolved heads as parents; keep original revisions addressable |
| Different records with similar text | Keep both. Suggest semantic deduplication locally; do not infer identity from text |
| Delete concurrent with, or followed by, a stale edit | Tombstone hides the record from normal recall; preserve the edit as suppressed conflict history |
| Restore | Explicit authorized operation names the relevant deletion and current heads; no implicit resurrection |

A device applies causality, not arrival time. An edit based on an old parent does not cancel a known deletion even if delivered later. An unresolved deletion branch keeps the record hidden until an explicit resolution/restore accounts for it. Multi-delete restore must account for every unsuperseded deletion head.

Skills use the same preservation rules but stricter activation. An incoming skill is staged. Activate only an owner-authorized signed release that identifies the exact artifact digest, scope, compatibility, and required capabilities. Concurrent skill releases retain the last approved local release or disable a revoked one; do not automatically merge executable files or choose a release by timestamp. Activation and approval are separate from sync publication.

## GitHub and local edits

Local MCP/CLI edits enter the ordinary application write path. An optional Git bridge runs on a trusted device and observes an explicitly configured private vault repository, never arbitrary working files.

A Git import decrypts and validates a reviewed artifact, checks its signed release and base revision, and maps its stable commit/path/artifact identity to one logical import operation. Persist the mapping with the new revision. Repeated webhooks, polling, and exports of already-seen operations are no-ops.

An export records the source operation/revision IDs and is created only after the relevant publication is known. The importer recognizes that provenance rather than creating another revision. Divergent Git and local edits therefore enter the same revision DAG as conflicts instead of bouncing between systems. No cloud CI job receives decryption keys.

Webhooks accelerate discovery; local Git reconciliation repairs missed events. Manual reviewed import/export remains a future option, with automation deferred. No Git bridge is implemented in the initial source slice.

## Deletion, compaction, and long absences

A tombstone is a durable operation. Keep enough record identity, ancestry, logical operation identity, and deletion state in every checkpoint to reject stale resurrection. Devices older than the retained incremental window must rebase from an authenticated checkpoint before sending queued work; keep their pending edits during that process.

Version one does not prune the operation/deletion history. Future destructive compaction needs an explicit protocol for retired device generations and retained anti-replay metadata; a time-to-live alone is insufficient. A returning retired device must enroll again and reconcile drafts against current deletion state.

Logical deletion removes normal recall and invalidates dependent summaries/index entries. Ciphertext may remain in immutable history, independent backups, or Git history. Physical erasure is a separate retention policy with different limits; scope-key destruction cannot selectively erase one record while preserving other records encrypted under that key.

## Guarantees and limits

Target: once connectivity, credentials, storage availability, and service execution are stable, authorized replicas eventually converge to the same revision/conflict state through replay. Connected devices will usually receive a wake hint promptly; there is no promise of immediate propagation or a fixed delivery deadline.

Sleeping devices, suspended processes, quota exhaustion, outages, missing hooks, and rejected credentials delay synchronization. Show pending count, last successful reconciliation, coverage, and blocked reasons. Correctness relies on durable replay and idempotent application, not notification delivery.

The relay may deny service or omit history. Signed envelopes protect origin/integrity; remembered checkpoints can reveal rollback below known state. A fresh recovery device cannot independently prove it has the newest history from a malicious sole provider. Stronger fork/freshness detection requires an independent witness or device comparison and is deferred.

## Optional schema-v2 session extension

The [session protocol](session-protocol.md) specifies the optional authored
extension: strict schema-2 bodies, mandatory independent DEFLATE compression
before encryption, 4 KiB frame padding, durable exact retry bytes, derived source
invalidation, owner grants and projection lifecycle. Outer v1 envelopes and all
legacy signing/AAD domains remain unchanged. Memory and skill schema-1 records
remain unframed. Compilation, runtime and cryptographic interoperability for the
extension are **NOT RUN**; schema examples alone do not establish acceptance.
