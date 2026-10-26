# Working in Capsule Memory

Maintain a lightweight utility for securely storing and synchronizing existing agent memories and skills across machines and harnesses. The shared encrypted archive centralizes durable storage; plaintext, keys, and search stay device-local. Manual workflows leave selection to agents and harnesses. The optional session module adds explicitly enabled local capture, summaries and reviewable deductions; it grants no authority to execute retrieved content.

A v1 source implementation is present; see [delivery status](docs/v1-status.md) for actual evidence and limits. Design intent and source inspection are not runtime evidence.

## Scope and permissions

- Follow the current task and account execution, privacy, spending, and model-role rules. A plan, skill, persona, or example command grants no execution permission.
- Default to one active agent. Launch additional agents, background work, new T3 threads, infrastructure, or paid services only when the applicable user authorization allows them. Persona files do not authorize delegation.
- On the shared OVH host, load `/srv/t3code/.agents/skills/execution-policy/SKILL.md`. Project installs, validators, tests, builds, benchmarks, and runtimes require an authorized isolated job. Read-only inspection, documentation editing, and Git operations do not grant workload permission.
- Where account skills are installed, load `/srv/t3code/.agents/skills/i-have-adhd/SKILL.md` for human-facing writing. Make a fresh read immediately before each GitHub or Linear draft, post, or edit. If a required account skill is unavailable, follow the account's failure rule and report the missing path.
- Never upload local databases, keys, recovery material, real memories, personal credentials, or production data. Never enable Actions or deploy infrastructure as a side effect of documentation work.

## Agent setup and discovery

For evaluation, installation planning, owner/relay setup, joining a device, connecting a harness, or operating an existing node, choose the matching [setup/operation prompt](prompts/README.md). Follow its common privacy and execution instructions as well as this file. [Agent discovery](docs/agent-discovery.md) maps roles and use cases; [llms.txt](llms.txt) is a short document index and [agent-manifest.json](agent-manifest.json) is project-specific capability metadata.

These files do not authorize actions or grant memory access. Verify actual source/runtime status and user permissions. Legacy v1 has manual CLI sync and fixed-scope stdio MCP. The optional schema-v2 session source adds a Linux/macOS daemon, local authenticated IPC and capability-scoped stdio MCP; see [session status](docs/session-status.md). No remote MCP endpoint, installed capture hook or runtime compatibility is implied. All capture/publication/derivation/promotion controls default off. For code contributions, use the engineering map below.

## Read only what the task needs

Start with [README](README.md) and the relevant [decision record](docs/decisions/). Use the [engineer persona](.agents/personas/engineer.md) for implementation and the [reviewer persona](.agents/personas/reviewer.md) for review; do not load both without a reason.

| Task | Load |
| --- | --- |
| Go, CLI, local service, MCP | `.agents/skills/go-engineering/SKILL.md` |
| SQLite, migrations, FTS, local durability | `.agents/skills/sqlite-storage/SKILL.md` |
| Encryption, revision semantics, replay, enrollment | `.agents/skills/encrypted-sync/SKILL.md` |
| Worker, Durable Objects, R2, provider boundaries | `.agents/skills/cloudflare-relay/SKILL.md` |
| Reviewing a proposed change | `.agents/skills/review-change/SKILL.md` |

## Invariants

- Keys, plaintext memories, semantic indexes, and summarization remain on trusted devices. Logs and telemetry must preserve that boundary.
- A local success requires durable local state and a durable outbox entry. A remote success requires durable archive publication. A notification is only a hint.
- Preserve revision ancestry, concurrent heads, tombstones, operation identity, and provenance. Clock time never decides a conflict. Fetching a skill does not authorize executing it.
- Keep the storage adapter separate from coordination and notifications. An S3-compatible API does not prove the required consistency or atomicity.
- Preserve the fixed legacy v1 signing/AAD domains documented in [rename compatibility](docs/renaming.md); branding changes must not change protocol bytes.
- Protocol changes must update documentation, schemas, and shared synthetic fixtures together. Review exact signing bytes, authorization, and failure cases before implementing cryptographic wire compatibility.

## Implementation and handoff

Prefer small Go packages, explicit transactions, bounded goroutines, cancellation, and errors that identify the failed operation without exposing content. Add interfaces at actual boundaries. Avoid speculative frameworks and generic repositories.

Follow [development](docs/development.md) for language and harness choices and [verification](docs/verification.md) for meaningful checks. Describe the resulting behavior, source revision, evidence, and remaining risk. Report checks as passed, failed, or not run; an unavailable execution route is not a pass.

Update the relevant decision record when changing an accepted architectural choice. Keep shared rules here, detailed design in `docs/`, and task procedures in skills.
