# 0004: Optional session archive and local derivation

**Status:** accepted design, source implementation in progress, 2026-10-26. Compilation, formatting, tests, adapter compatibility, inference and runtime acceptance are **NOT RUN**. See [session status](../session-status.md).

The accepted implementation compresses every schema-v2 revision with isolated DEFLATE before encryption/upload, preserving legacy v1 protocol bytes. Selected assertions use v2 knowledge in the same scope as their lifecycle and owner grant. Strict retrieval requires live source reconciliation during the request; historical retrieval reports its verification coverage. Personal aggregation requires owner-signed authorization with reader-subset checks at control changes and rollup run/commit. [Implementation amendments](../session-history.md#accepted-implementation-amendments) supersede conflicting wording below while preserving the original decision detail.

## Proposed decision

Extend Capsule Memory with an opt-in session module. An enabled local Go daemon imports supported harness histories, publishes encrypted immutable session chunks, and runs permitted local jobs to generate summaries and memory/decision candidates. Capture, publication, derivation, and promotion have independent controls.

Use a dedicated session vault with independent keys and enrollment by default. Retain project scopes and a locally approved project identity map inside it. Keep selected knowledge in the existing memory vault; controlled projection connects the two. The [detailed design](../session-history.md) defines capture, durability, source dependencies, grants, and phased acceptance.

## Scope change

The existing architecture keeps memory selection with harnesses and agents. This proposal adds optional automatic deduction without changing manual workflows or making inferred text authoritative. Summaries and candidates can be produced automatically; broader sharing and acceptance require explicit policy.

The cloud remains content-blind. [Decision 0002](0002-e2ee-local-search.md) continues to apply to session content, indexes, and inference. [Decision 0003](0003-cloudflare-relay.md) continues to govern durable publication. No new central plaintext index, summarizer, or broker is required.

## Alternatives

| Option | Assessment |
| --- | --- |
| Dedicated session vault | Recommended: isolates sensitive raw history from ordinary memory access and enrollment. Adds a profile and current relay deployment configuration. |
| Separate scopes in the existing vault | Supported design alternative after selective scope grants are implemented; lower operational setup, more coupled enrollment. |
| Plaintext conversation Git repository or hosted semantic store | Does not meet the established device-only plaintext requirement. |
| Harness MCP calls alone | Useful for intentional memory capture, but cannot establish complete transcript ingestion or durable background work. |

## Consequences and gates

- Add typed session, summary, insight, and projection contracts. V1 accepts only memory/skill text, so schema negotiation, migrations, fixtures, and exact-byte interoperability review must precede implementation. Preserve legacy v1 protocol domains.
- Use versioned read-only adapters, incremental durable checkpoints, immutable bounded chunks, and one write owner per profile. Treat hooks as wake hints and report capture gaps.
- Preserve exact evidence and decision state. Source edits, deletion, and access changes invalidate dependent outputs. Cross-vault projection requires a durable lifecycle bridge before promotion can ship.
- Keep all controls disabled initially. A capture grant does not grant inference, historical backfill, raw access to every harness, or cross-project export.
- Establish the existing synthetic foundation and each new slice through authorized isolated execution. This record authorizes no installs, services, infrastructure, real-history collection, or model spending.

## Integration amendment — 2026-10-26

The sole integration pass supersedes whole-chunk retraction roots with scoped,
derivable event retractions and exact event-range dependencies. Evidence and
coverage are distinct edges; both retain exact revision freshness and grants.
Same-record regenerated summaries can cite valid events from a corrected chunk.
Session deletion/suppression remains independent and applies across strong views
and continuations. Projected retrieval proofs name the exact returned target
revision and retain exact source/link/vault/scope bindings.

Local capture adds durable byte/time/end bounds, gap audits and checkpoint state
for native Codex turn hashes/references. Raw prompt plaintext is never checkpoint
state. Generic exporter harness labels are bounded namespaces; adapter selection
remains allowlisted. Export pages carry an immutable snapshot token. These are
source decisions awaiting independent review; all execution acceptance is NOT RUN.
Only published-body cache eviction remains the accepted feature deferral.
