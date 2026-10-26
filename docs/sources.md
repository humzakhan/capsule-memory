# Sources and evidence

Primary documentation consulted for the design, checked **2026-10-26**. External capability claims are facts from these sources; architecture and scheduling choices are project decisions. Pricing and harness behavior can change, so recheck them before implementation or deployment.

## Cloudflare

| Source | What it supports |
| --- | --- |
| [R2 consistency](https://developers.cloudflare.com/r2/reference/consistency/) | Read visibility, atomic conditional writes, and the limitation of negative reads during an ambiguous in-flight write |
| [R2 Workers API](https://developers.cloudflare.com/r2/api/workers/workers-api-reference/) | Object operations, conditional request options, and version metadata |
| [R2 S3 compatibility](https://developers.cloudflare.com/r2/api/s3/api/) | Supported operations and differences; S3-shaped APIs are not complete S3 equivalence |
| [Workers pricing](https://developers.cloudflare.com/workers/platform/pricing/) | Free allowances and the Workers Paid monthly minimum plus metered usage |
| [Durable Objects pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/) | SQLite-backed availability and separate request, duration, and storage allowances |
| [DO WebSockets](https://developers.cloudflare.com/durable-objects/best-practices/websockets/) | Hibernating WebSockets and connection attachments |
| [R2 pricing](https://developers.cloudflare.com/r2/pricing/) | Standard storage/operation free allowances, rates, and storage-class differences |
| [Budget alerts](https://developers.cloudflare.com/billing/manage/budget-alerts/) | Alerts notify; they do not impose a universal hard spending ceiling |

Using free allowances is a deployment target, not a verified bill. The initial design avoids a dedicated VM and paid broker, but Cloudflare usage, account eligibility, quota failures, and real crypto/request CPU still need measurement.

## Local runtime and harnesses

| Source | What it supports |
| --- | --- |
| [SQLite FTS5](https://www.sqlite.org/fts5.html) | Embedded full-text indexing and querying |
| [SQLite WAL](https://www.sqlite.org/wal.html) and [synchronous](https://www.sqlite.org/pragma.html#pragma_synchronous) | WAL and durability trade-offs; configuration must match actual driver behavior |
| [SQLite backup](https://www.sqlite.org/backup.html) | Consistent backup API rather than copying a live main file alone |
| [Go review comments](https://go.dev/wiki/CodeReviewComments) and [Effective Go](https://go.dev/doc/effective_go) | Go coding practice; Effective Go explicitly notes its age and limited coverage of newer features |
| [MCP SDKs](https://modelcontextprotocol.io/docs/sdk) and [official Go SDK](https://github.com/modelcontextprotocol/go-sdk) | Supported SDK implementation; not universal harness hooks |
| [Codex project instructions](https://learn.chatgpt.com/docs/agent-configuration/agents-md) and [skills](https://learn.chatgpt.com/docs/build-skills) | `AGENTS.md`, project skill discovery under `.agents/skills`, and supported symlink behavior |
| [Claude instructions](https://code.claude.com/docs/en/memory), [skills](https://code.claude.com/docs/en/skills), and [subagents](https://code.claude.com/docs/en/sub-agents) | Imports, project instruction/skill locations, symlink discovery, and agent Markdown frontmatter |
| [GitHub webhook guidance](https://docs.github.com/en/webhooks/using-webhooks/best-practices-for-using-webhooks) | Delivery handling; webhook receipt does not replace reconciliation |

## Design decisions and unresolved assumptions

Accepted choices are recorded in [decisions](decisions/). Cursor polling cadence, maintenance windows, platform targets, and free-tier suitability are proposals to verify with actual usage.

Cryptographic primitive selection, byte framing, enrollment UX, SQLite driver, dependency versions, and exact supported harness versions are unresolved implementation choices. No source citation substitutes for project-specific integration, failure, or security tests.
