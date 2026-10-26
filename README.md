# agent-memory

Portable, end-to-end encrypted memories and skills for agents across machines and harnesses. **Carry** is the temporary product name; `agent-memory` is the repository name.

**Status: v1 source implementation, runtime unverified.** Go CLI/MCP, local SQLite storage, encrypted manual synchronization, a Cloudflare relay, and one focused E2E journey are authored. Dependency resolution, compilation, formatting, typechecks, and E2E execution are **NOT RUN**. No infrastructure is deployed.

## Start here

1. Follow [the owner-machine runbook](docs/run-v1.md) to build and run the synthetic local E2E journey.
2. Read [delivery status](docs/v1-status.md) for evidence and limits, then [Go usage](docs/go-client.md) for CLI/MCP configuration.
3. Read [the architecture](docs/architecture.md), [v1 contract](docs/v1-contract.md), and [development guidance](docs/development.md) before contributing.

## The system we are building

The v1 CLI and MCP processes share a device-local SQLite database with FTS5, immutable revisions, and a durable outbox. Explicit `sync` sends signed encrypted records to a TypeScript Worker, a SQLite-backed Durable Object, and R2. Publication follows an atomic durable archive-head update. Search and keys stay on devices.

Incoming skills stay staged. The owner can approve and activate an exact local revision for reading as Markdown text. Executable installation and cross-device approval distribution are deferred. Conflicts, tombstones, and source provenance remain explicit.

Background service/IPC, WebSocket hints, automatic rotation/key distribution, summaries, embeddings, Git automation, and broad hooks remain roadmap work. Device secrets currently use restrictive local files; there is no native keystore integration.

GitHub holds source, instructions, and reviewed changes. A separate private vault repository may hold encrypted skill sources and curated exports. Search indexes, private keys, plaintext memories, and working databases never belong in either repository.

| Area | Decision |
| --- | --- |
| Local runtime | Go; standard library first; official Go MCP SDK |
| Local storage | SQLite with FTS5; embeddings deferred |
| Relay | TypeScript Worker and one Durable Object per vault |
| Cold archive | R2 Standard initially; explicit provider capabilities |
| Privacy | AES-256-GCM content encryption; Ed25519 signatures; device-only plaintext search |
| Synchronization | Manual durable outbox/replay; preserved conflicts and deletion history |

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
|-- cmd/agent-memory/         Go CLI and MCP entrypoint
|-- internal/memory/          local domain, SQLite, crypto, and sync
|-- cloudflare/               Worker, Durable Object, and R2 adapter
|-- e2e/                      one real local Wrangler/CLI/MCP journey
|-- docs/                     architecture, protocol, risks, verification
|   `-- decisions/            accepted choices and their trade-offs
|-- contracts/                draft schemas and synthetic examples
`-- .github/                  pull request template; no workflows yet
```

The Go module and Wrangler version are pinned. Authorized dependency resolution must generate real checksums/lockfiles; none were fabricated. Run the installation/build commands on your own trusted machine or an explicitly authorized isolated runner, following the host policy.

## Evidence and limits

[The focused inspection](docs/v1-inspection.md) records the source findings and corrections. [The E2E journey](e2e/README.md) covers the requested behavior groups with a real local Worker/DO/R2 emulator and built CLI; it has not been executed. Emulator success would not establish production durability or cost.

[Protocol](docs/protocol.md), [threat model](docs/threat-model.md), and [verification scenarios](docs/verification.md) describe the wider invariants. [Sources](docs/sources.md) separates official capabilities from project decisions. Use synthetic data until the implementation has been built, exercised, and reviewed for your intended deployment.

The free Cloudflare tier is the initial deployment target. Workers Paid starts at a monthly minimum, not a spending ceiling. No infrastructure is provisioned by this repository setup.
