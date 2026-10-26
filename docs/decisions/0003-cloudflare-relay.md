# 0003: Cloudflare relay, durable archive, optional wake hints

**Status:** accepted initial provider; pricing and runtime suitability unverified in this project.

## Decision

Use a TypeScript Worker for authenticated device requests, a SQLite-backed Durable Object per vault for coordination and hibernating WebSocket hints, and private R2 Standard storage for encrypted payloads, journal, checkpoints, and the durable publication head.

Acknowledgement follows conditional durable head publication. DO state is a rebuildable coordination cache. Devices reconcile after reconnect and periodically, so hints and warm processes are not correctness dependencies.

## Cost target

Start on eligible free allowances. At the documentation check date, Workers Free includes 100,000 requests per day; Workers Paid has a $5 monthly minimum with metered overages. SQLite-backed DOs are available with free allowances. See [Workers pricing](https://developers.cloudflare.com/workers/platform/pricing/) and [DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/).

R2 Standard includes 10 GB-month storage, one million Class A operations, and ten million Class B operations in its monthly free tier. Its Infrequent Access class does not inherit those free allowances. The term cold describes our archive role, not a requirement to use that class. See [R2 pricing](https://developers.cloudflare.com/r2/pricing/).

These allowances are separate, account-scoped, and subject to provider rules. The design cannot promise a $0 bill or a hard $5 cap. Use bounded payloads, hibernating connections, batched publication, admission limits, jittered reconciliation, and owner-visible usage. Budget alerts are informational, not a universal stop mechanism.

## Alternatives and portability

No dedicated central VM or general-purpose broker is required. K2, Queues, and KV are deferred: the first version needs durable archive replay and wake hints, not another log or a plaintext index.

Keep storage, coordinator, and notifier interfaces separate. R2's [consistency and conditional-write contract](https://developers.cloudflare.com/r2/reference/consistency/) fits fenced publication, but another provider must establish equivalent capabilities or add a suitable coordinator. S3 compatibility alone is insufficient.

## Consequences

The first relay is Cloudflare-specific, while record semantics and object-store contracts remain independent. Hibernation and free-tier CPU behavior need measured integration evidence. Provider outages and quotas delay remote propagation; local durable capture continues and exposes its pending state.
