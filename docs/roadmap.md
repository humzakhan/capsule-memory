# Build plan

**Build the reliable local write path first.** Keep every slice reviewable and preserve the privacy boundary before adding cloud synchronization.

## 0. Repository foundation — current deliverable

Architecture, decisions, threat model, draft logical contracts, project skills, personas, and verification scenarios. No application code, installed dependencies, deployed infrastructure, CI workflows, real vault data, or approved cryptographic protocol.

## 1. Local durable core

Choose and pin a supported Go toolchain and SQLite driver after a bounded platform/FTS5 decision. Introduce the minimum CLI and application/store packages, schema migrations, record revisions, tombstones, visibility, durable outbox, and FTS5 recall.

Exit evidence: local writes and queued state survive the relevant crash boundaries; search respects scope and deletion; index repair preserves pending writes. Use synthetic records. A local-only slice must not pretend unencrypted data is safe for remote upload.

## 2. Reviewed encrypted protocol and device identity

Resolve cryptographic framing, key wrapping, nonce rules, owner recovery, enrollment, control epochs, signed skill approval, and rollback limits. Produce real shared Go/TypeScript vectors and implement authenticated envelopes and device keystore integration.

Exit evidence: context substitution fails, exact bytes interoperate, recovery works, revocation blocks future access, and retry/rekey preserves logical identity. Require explicit protocol/security review before real personal data is used.

## 3. Minimal archive synchronization

Implement the storage-provider contract and R2 adapter, Worker authentication, per-vault coordinator, immutable publication journal/head, replay, checkpoints, and reconnect. Establish correctness using reconciliation before adding WebSocket acceleration.

Exit evidence: offline convergence, conflicts, deletion, duplicates, interrupted publication, and coordinator rebuild scenarios pass. Provision and integrate only through separately authorized test infrastructure and spending limits.

## 4. First usable harness integration

Add the Go MCP stdio bridge, scoped client configuration, service status, and one explicit memory store/recall journey. Add hibernating WebSocket hints, background synchronization, and a visible pending/coverage state. Verify a concrete supported harness rather than promising universal hooks.

Exit evidence: a memory captured in one configured harness is recalled on a second machine with provenance after convergence; disconnects and missed hooks are repaired. Signed skill fetch stays staged until approved activation.

## 5. Useful maintenance

Add payload-cache budgets, pinned records, encrypted archive grouping, local opt-in summary generation, source invalidation, recovery UX, and reviewed manual Git import/export. Measure retrieval quality and ongoing provider usage.

Exit evidence: traceable summaries, complete versus partial index coverage, safe eviction, and restore behavior are demonstrated. No destructive history pruning by default.

## Defer until evidence requires them

- Embeddings, vector extensions, and local model packaging.
- Automatic Git webhooks/import/export and broad harness-hook support.
- Other storage adapters, mobile clients, multi-user sharing, and remote MCP.
- K2/Queues, distributed pub/sub, and advanced traffic/fork privacy.
- Destructive compaction, selective cryptographic erasure, and automatic skill merging.

Each implementation task needs a bounded slice, an authorized execution route, and meaningful evidence. The next implementation proposal should start with slice 1's database/driver and acknowledgement boundary; it should not deploy the whole diagram.
