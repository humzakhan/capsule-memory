# V1 delivery status

This page records the v1 source delivery, now integrated into `main`. **No runtime check has been executed.** Source is authored and inspected; usable runtime behavior is still unproven. An authored test is not a passed test.

## Required implementation and evidence

| Requirement | Evidence needed | Current status |
| --- | --- | --- |
| Go CLI and official-SDK MCP | [CLI](../cmd/agent-memory/main.go), [MCP](../internal/memory/mcp.go), [usage](go-client.md) | Source authored; runtime unverified |
| SQLite FTS5 and provenance | [Store](../internal/memory/store.go): durable revision/index schema, scoped visibility and source references | Source inspected; SQL/runtime NOT RUN |
| Offline queue and retry identity | [Store](../internal/memory/store.go) and [sync](../internal/memory/sync.go): atomic revision/exact-envelope/outbox commit, retained retry bytes | Source inspected; crash behavior NOT RUN |
| Device-only E2EE | [Crypto](../internal/memory/crypto.go), [profile](../internal/memory/profile.go), [wire contract](v1-contract.md) | Source inspected; cryptographic interoperability NOT RUN |
| Cloudflare archive path | [Archive](../cloudflare/src/archive.ts), [coordinator](../cloudflare/src/index.ts), [setup](relay-setup.md) | Source authored; Wrangler/provider behavior NOT RUN |
| Conflicts and deletion | [Store](../internal/memory/store.go), [deleted-fetch handling](../internal/memory/skills.go), existing E2E assertions | Source inspected; convergence/runtime NOT RUN |
| Skill trust | [Approvals and activation](../internal/memory/skills.go): owner signature, scope/revision/digest, current-head eligibility | Local text activation authored; runtime NOT RUN |
| Focused E2E suite | [One black-box journey](../e2e/run.mjs) with twelve scenarios covering the five behavior groups | Authored; NOT RUN |
| Focused inspection | [Findings and fixes](v1-inspection.md); initial source `3b73b6e8ea28c833fb64cd2a6a75204b885b35ea` | One source inspection and concrete-fix rechecks; no broad repeat review |
| Delivery | [Implementation source](https://github.com/humzakhan/agent-memory/tree/18f2d323dd56d2bb03e0088d0ff49822e61955ea), [run/cleanup commands](run-v1.md) | Original handoff recorded the verified remote implementation commit; documentation consolidation preserves its ancestry on `main`. No deployment or runtime acceptance. |

## Runtime acceptance

| Behavior group | Result |
| --- | --- |
| MCP store/recall and provenance | NOT RUN |
| Offline convergence and duplicate delivery | NOT RUN |
| Concurrent edits and deletion without resurrection | NOT RUN |
| Interrupted synchronization and local rebuild | NOT RUN |
| Tampered/cross-scope data and unapproved skill rejection | NOT RUN |

The shared OVH host has no approved isolated execution route for this job. No installs, builds, typechecks, validators, tests, or runtimes were executed. [The runbook](run-v1.md) contains exact commands for the owner's machine. Keep this limitation until an authorized execution provides evidence; do not infer runtime acceptance from the pushed branch or Git diff checks.

## Source inspection evidence

One combined inspection examined encryption/signature framing, authorization, local transactions, archive acknowledgement, replay, and tombstones. It found three concrete issues/gaps: duplicate-key parser disagreement, deleted normal fetch exposing head payloads, and missing approved text activation. Corrective source and additions to the same E2E journey address these findings. The four GPT worker turns used medium effort and priority service, with at most two active workers and disjoint ownership; all worker threads are settled.

Read-only file/Git inspection and `git diff --check` establish the authored inventory and whitespace state only. Dependency checksums, npm lockfile, formatting, compiler/SDK behavior, E2E results, SQLite crash safety, real R2 durability, quotas, and cost remain unverified.

## Deliberate scope

Embeddings, automatic summaries, Git automation, broad hooks, destructive history pruning, and cloud deployment are deferred. There is no background synchronization daemon/IPC or notification bus yet; run `sync` explicitly. Device key storage uses restrictive local files. Native keystores, automated rotation/key distribution, cross-device approval releases, executable skill installation, production deployment, and independent backup automation are not implemented. Local activation means selecting approved Markdown text for retrieval. Read the implementation runbooks for exact limits.
