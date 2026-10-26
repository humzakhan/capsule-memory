# V1 delivery status

This page is the acceptance record for the implementation branch. **No runtime check has been executed.** Update each row from the actual source and job evidence before handoff; an authored test is not a passed test.

## Required implementation and evidence

| Requirement | Evidence needed | Current status |
| --- | --- | --- |
| Go CLI and official-SDK MCP | Actual command/tool entrypoints, shared scoped domain operations, machine-run instructions | Implementation in progress |
| SQLite FTS5 and provenance | Durable revision/index schema, scoped visible-query path, exact source references in results | Implementation in progress |
| Offline queue and retry identity | Atomic local revision/envelope/outbox commit, persisted exact retry bytes, pending status | Implementation in progress |
| Device-only E2EE | Local key permissions, AES-GCM framing, exact Ed25519 bytes, owner pinning, no remote keys/plaintext | Implementation in progress |
| Cloudflare archive path | Worker/DO/R2 source, head CAS publication boundary, durable replay and coordinator recovery | Implementation in progress |
| Conflicts and deletion | Retained concurrent heads, causal resolution, tombstones, explicit restore, no stale resurrection | Implementation in progress |
| Skill trust | Incoming staging, exact approved revision, approval separated from activation | Implementation in progress |
| Focused E2E suite | One runnable suite covering the five requested behavior groups | Being authored; NOT RUN |
| Focused inspection | Exact reviewed source identity and concrete findings/fix evidence | Pending integrated source |
| Delivery | One pushed branch, matching local/remote commit, concise run and cleanup commands | Pending integration |

## Runtime acceptance

| Behavior group | Result |
| --- | --- |
| MCP store/recall and provenance | NOT RUN |
| Offline convergence and duplicate delivery | NOT RUN |
| Concurrent edits and deletion without resurrection | NOT RUN |
| Interrupted synchronization and local rebuild | NOT RUN |
| Tampered/cross-scope data and unapproved skill rejection | NOT RUN |

The shared OVH host has no approved isolated execution route for this job. No installs, builds, typechecks, validators, tests, or runtimes are authorized here. The delivery must include exact commands for the owner's machine and retain this limitation until an actual authorized job provides evidence.

## Deliberate scope

Embeddings, automatic summaries, Git automation, broad hooks, destructive history pruning, and cloud deployment are deferred. Device key storage initially uses restrictive local files; native keystore integration and automated remote key distribution are not implied. Read the implementation runbooks for any further limitations established during integration.
