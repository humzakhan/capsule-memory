---
name: cloudflare-relay
description: Design or change agent-memory Cloudflare Workers, Durable Objects, R2 publication, notifications, provider capabilities, and cost controls. Does not authorize provisioning or deployment.
---

# Cloudflare relay

Read [architecture](../../../docs/architecture.md), [the relay decision](../../../docs/decisions/0003-cloudflare-relay.md), and relevant [official sources](../../../docs/sources.md). Recheck current pricing and capabilities before changing a cost or platform claim.

1. Keep the Worker and per-vault Durable Object content-blind. Authenticate devices, enforce opaque scope and epoch grants, bound requests, and avoid content-bearing telemetry.
2. Publish immutable payloads and journal segments before conditionally advancing the durable archive head. Treat Durable Object state as coordination and a rebuildable cache, not the only copy of an acknowledged operation.
3. Use the hibernating WebSocket API for wake hints. Restore connection metadata from durable attachments. Never depend on an in-memory cursor, timer, or connected socket for correctness.
4. Repair missed hints through reconnect and periodic head reconciliation. Bound fan-out, retry storms, reads, object sizes, and per-vault admission. Separate caller rate limits from provider billing guarantees.
5. Keep object storage, coordination, and notification interfaces separate. Document missing atomic compare-and-swap or visibility guarantees instead of claiming portability from S3 API compatibility.

Use native Worker and Durable Object APIs before adding a routing framework. Target SQLite-backed Durable Objects and R2 Standard on free allowances first. Workers Paid has a minimum charge and metered overages; alerts are not a hard cap. K2, Queues, and KV are not dependencies of the first version.

Before any execution, apply the current task's host, sandbox, credential, and spending rules. Do not run Wrangler, create resources, add deployment workflows, or contact production as an incidental validation step.
