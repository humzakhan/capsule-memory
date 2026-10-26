---
name: encrypted-sync
description: Design or review agent-memory E2EE, revisions, durable synchronization, conflicts, deletion, enrollment, rekeying, and signed skill releases. Use for protocol invariants, not generic networking.
---

# Encrypted synchronization

Read [protocol](../../../docs/protocol.md), [threat model](../../../docs/threat-model.md), and [contracts](../../../contracts/README.md). The current contract is a draft, not an approved cryptographic protocol.

1. Name the logical operation, transport envelope, key epoch, and publication cursor separately. Persist encrypted bytes for retries. Bind authenticated context to vault, scope, device, epoch, version, object identity, and payload digest using a reviewed byte format.
2. Trace crashes before and after local commit, upload, journal publication, acknowledgement, and cursor advance. Remote acknowledgement follows durable publication; a WebSocket message or orphan blob is not a commit.
3. Preserve the revision DAG and all concurrent heads. A tombstone suppresses stale edits; restore is explicit and names the deletion. Never use timestamp order or arrival order to discard a branch.
4. Treat revocation as an ordered control operation. Refresh authorization before publishing an offline queue. Re-encrypt under the permitted epoch only when still authorized, with a new transport identity and the same logical operation identity. Revocation cannot erase previously learned plaintext.
5. Add shared synthetic examples and adversarial acceptance cases for changed semantics. Require reviewed signing bytes and real cross-language vectors before claiming interoperability.

Do not invent cryptographic primitives, nonce rules, or canonical JSON. Resolve framing and key handling explicitly before implementation. Never put content keys on the relay, in object metadata, GitHub Actions, logs, fixtures, or review prompts.

Summary revisions must cite exact source revisions and coverage. Source edits or deletion invalidate dependent summaries and local snippets. Receiving a signed skill revision proves origin, not safe execution; keep it staged until the trusted release policy authorizes the exact digest and scope.
