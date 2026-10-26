# agent-memory

Portable, end-to-end encrypted memories and skills for agents across machines and harnesses. **Carry** is the temporary product name; `agent-memory` is the repository name.

**Status: documentation and agent setup only.** No application, daemon, deployed infrastructure, or executable verification suite exists yet. Runtime behavior described here is a design target, not a tested guarantee.

## Start here

1. Read [the architecture](docs/architecture.md) for ownership, retrieval, and system boundaries.
2. Read [the roadmap](docs/roadmap.md) for the first implementation slice and deferred work.
3. Read [development guidance](docs/development.md) before changing code or running tools.

## The system we are building

Each trusted machine runs a small Go service with a local SQLite database, device-only search, a durable offline queue, and an MCP interface. Cloudflare R2 stores encrypted records and history. A Worker and a SQLite-backed Durable Object coordinate publication and send change hints without decrypting content.

GitHub holds source, instructions, and reviewed changes. A separate private vault repository may hold encrypted skill sources and curated exports. Search indexes, private keys, plaintext memories, and working databases never belong in either repository.

| Area | Decision |
| --- | --- |
| Local runtime | Go; standard library first; official Go MCP SDK |
| Local storage | SQLite with FTS5; embeddings deferred |
| Relay | TypeScript Worker and one Durable Object per vault |
| Cold archive | R2 Standard initially; explicit provider capabilities |
| Privacy | Device-held keys; device-only plaintext search and summarization |
| Synchronization | Durable outbox, immutable revisions, replay, preserved conflicts |

## Repository map

```text
agent-memory/
|-- AGENTS.md                 shared agent rules and reading map
|-- CLAUDE.md                 thin import of AGENTS.md
|-- .agents/
|   |-- personas/             engineer and reviewer guidance
|   `-- skills/               five focused engineering skills
|-- .claude/
|   |-- agents/               adapters to the canonical personas
|   `-- skills -> ../.agents/skills
|-- docs/                     architecture, protocol, risks, verification
|   `-- decisions/            accepted choices and their trade-offs
|-- contracts/                draft schemas and synthetic examples
`-- .github/                  pull request template; no workflows yet
```

Application directories and dependency manifests will be added with their first implementation slice. There is no reason to install dependencies for this scaffold.

## Evidence and limits

[Protocol](docs/protocol.md), [threat model](docs/threat-model.md), and [verification scenarios](docs/verification.md) describe the invariants implementation must establish. [Sources](docs/sources.md) separates official capability documentation from project decisions.

The free Cloudflare tier is the initial deployment target. Workers Paid starts at a monthly minimum, not a spending ceiling. No infrastructure is provisioned by this repository setup.
