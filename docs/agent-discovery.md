# Agent discovery and setup

Start at [the prompt index](../prompts/README.md). Capsule Memory is a utility for securely storing and synchronizing existing agent memories and skills. Agents choose the content and when to use it. Discovery metadata and prompts do not authorize actions or grant memory access.

**Status:** the repository is public, with readable Markdown and agent entry points. These are authored documentation, not proof of agent adoption, search indexing, runtime compatibility, or a successful install. Public access does not grant access to anyone's private memories, profiles or keys.

## Discovery layers

| Entry point | Purpose | Limit |
| --- | --- | --- |
| [README](../README.md) and repository description/topics | Describe the utility and link to supported workflows | Public discovery requires public access |
| [llms.txt](../llms.txt) | Small curated index with direct Markdown links | Proposed convention, not guaranteed agent ingestion |
| [agent-manifest.json](../agent-manifest.json) | Project-specific capability and task map | Not an MCP registry manifest, installer, or standard discovery API |
| [AGENTS.md](../AGENTS.md) and [CLAUDE.md](../CLAUDE.md) | Instructions for agents with a checkout | Harness loading rules vary; never assume all clients auto-load them |
| [Setup prompts](../prompts/README.md) and [docs index](README.md) | Role-based task instructions and source of detailed commands | Templates need actual values, authority and runtime evidence |

The normal reading path is README → prompt index → selected task/runbook. An agent with a checkout reads AGENTS.md first. A fetch-only agent can use the raw Markdown links in llms.txt. A user can also hand an agent one selected prompt; every prompt points back to the common rules and exact guides.

## Supported roles and use cases

| Situation | Existing path | Important boundary |
| --- | --- | --- |
| First-time source evaluation | [First run](../prompts/01-first-run.md) | Synthetic local E2E; runtime still unverified |
| New vault / owner bootstrap | [Owner setup](../prompts/02-owner-setup.md) | Only public owner pin/vault ID go to relay |
| Central relay preparation | [Relay setup](../prompts/03-relay-setup.md) | Worker + DO + R2, not a plaintext memory server |
| New workstation, server or publishing node | [Enroll device](../prompts/04-enroll-device.md) | Distinct device identity; no owner key transfer |
| Harness switch or additional MCP client | [Connect MCP](../prompts/05-connect-mcp.md) | Local stdio, fixed launch scope; write/delete tools included |
| Capture, search, offline work, reconnect | [Existing node](../prompts/06-capture-and-sync.md) | Local FTS and explicit manual sync; coverage is visible |
| Concurrent edits, delete, restore | [Record lifecycle](../prompts/07-conflicts-and-deletion.md) | Preserve branches/tombstones; explicit restore |
| Index repair, long absence, clean replica | [Recovery](../prompts/08-recover-device.md) | Preserve pending writes, profile custody, original state |
| Incoming skill review and local activation | [Skill review](../prompts/09-review-skill.md) | Exact owner approval; read-as-text only, no installer |
| Lost device, revocation, key/backup planning | [Security operations](../prompts/10-security-operations.md) | Some relay checks exist; turnkey management commands do not |

The current device enrollment CLI grants all scopes for which the owner holds current keys. It does not implement a narrow-scope or read-only enrollment UI. MCP fixes a chosen granted scope, but does not offer a read-only tool profile. Document this before connecting a client.

## What discovery does not create

There is no daemon, background publication service, systemd unit, launch agent, scheduler, hosted/remote MCP endpoint, public agent-to-agent service, or automatic capture hook in this version. A daemon request uses the existing-node prompt to explain manual sync and scope future work.

There are no independent push and pull CLI commands; sync performs both publication and replay. Cold data becomes searchable by authorized local replay/indexing, not a central plaintext search endpoint. Automatic summarization, embeddings, key distribution, executable skill installation, other storage adapters, and Git import/export remain deferred.

## Public discoverability checklist

1. Keep the repository title, utility description and topics accurate. Suggested topics include agent-memory, memory-sync, mcp, ai-agents, end-to-end-encryption, offline-first, sqlite, golang and cloudflare-r2. Preserve the distinction between source capabilities and tested behavior.
2. Before an owner-authorized public release, review repository history/content, examples, licensing, generated dependency locks and runtime/security evidence. Never publish profiles, keys, real memories or retained E2E artifacts. This checklist does not itself authorize a visibility change.
3. Keep README, llms.txt, the capability manifest and prompt links aligned with the actual command/tool surface. A local file is useful after clone; it is not a hosted docs site or automatically indexed content.
4. If a docs site is separately authorized, publish readable Markdown and an llms.txt index at that site's actual path. Add discovery links using that site's facilities. No website, DNS, headers, crawler enrollment or hosting was provisioned here.
5. Consider MCP registry submission only after verified packaging/distribution exists, with exact version, transport and private-path configuration reviewed. Do not publish fabricated package names, releases, URLs or server.json records.

No registry submission or package release has been performed. The Cloudflare TypeScript package is the relay tooling, not a distributable MCP server package. The actual MCP server is the Go executable on the user's device; publishing metadata does not host it or prove its security.

## Sources and evidence

- [GitHub topic documentation](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/classifying-your-repository-with-topics): topics support discovery; private repositories remain visible only to authorized users.
- [llms.txt proposal](https://llmstxt.org/): curated Markdown overview and document links. This project uses the convention without promising universal support.
- [Official MCP registry quickstart](https://modelcontextprotocol.io/registry/quickstart): registry metadata is separate from package artifacts and requires its publication workflow.

External capability references were checked on 2026-10-26. Project decisions are the layered entry points and role-specific prompts above. Actual crawl ingestion, agent-driven installation, harness sessions, build/E2E results and production behavior remain unverified.
