# Build plan

**Build the reliable local write path first.** The initial source now covers the local core, encrypted manual relay path, CLI/MCP, and approved text activation. [Delivery status](v1-status.md) distinguishes authored code from runtime acceptance. Keep remaining slices reviewable and preserve the privacy boundary.

## 0. Repository foundation — authored

Architecture, decisions, threat model, draft logical contracts, project skills, personas, and verification scenarios are present. V1 source and its specialized contract have since been added. No dependencies were installed, no infrastructure/CI was provisioned, and no real vault data was used. The implementation has not received runtime or full security acceptance.

## 1. Local durable core

Go 1.24.0 and `modernc.org/sqlite` v1.39.1 are pinned. The minimum CLI and application/store packages, schema migrations, record revisions, tombstones, visibility, durable outbox, and FTS5 recall are authored. Platform/FTS5 runtime acceptance remains pending.

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

Each implementation task needs a bounded slice, an authorized execution route, and meaningful evidence. The next acceptance step is the [owner-machine build and focused E2E](run-v1.md). Background synchronization, notifications, key distribution, maintenance, and wider harness support remain separate implementation work; do not deploy the whole diagram as an incidental check.

## Optional session module — source delivery, acceptance pending

The accepted optional extension now has source for capture/daemon/IPC, mandatory compression before encrypted upload, local summary/candidate derivation, signed aggregation and lifecycle-checked v2 knowledge. See [session status](session-status.md) for the parallel-source integration boundary; no build, formatter, test, inference, adapter or runtime acceptance has run. Earlier roadmap entries describing these as future work are superseded for source availability only.

Next gates are integrated source review, permitted owner compilation/formatting, synthetic durability/privacy journeys and version-specific adapter/model acceptance. Keep every control off until explicitly configured. Cross-vault stays off or explicitly `enabled_unaccepted`. Cache eviction is deferred and visible as not implemented. Native keystores, automatic rotation, embeddings, executable skills, Git automation, WebSockets and destructive retention remain future work.
