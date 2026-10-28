# Changelog

## v0.1.0-beta.1 — Initial source beta

This prerelease makes the authored implementation available for inspection and
synthetic evaluation. It contains source archives, not tested executables or a
hosted service. Runtime and security acceptance remain pending.

### Included in the source

- Local memory and skill storage, SQLite FTS5 recall, exact provenance, immutable
  revision history, concurrent heads, tombstones, restore and index rebuild.
- Device-local AES-256-GCM encryption and Ed25519 signatures, a durable encrypted
  outbox, explicit synchronization, and a Cloudflare Worker/DO/R2 relay adapter.
- A Go CLI, scoped stdio MCP, exact owner-approved local Markdown skill
  activation, and agent-oriented setup/operation documentation.
- Optional Linux/macOS daemon and private IPC; version-pinned Claude, Codex, T3
  and explicit-import capture; independently compressed schema-v2 histories;
  local summaries, authorized rollups and reviewable memory/decision candidates.
- Controlled knowledge promotion with exact source/target provenance,
  withdrawal handling, strict live freshness and honest historical coverage.
  Capture, publication, derivation, automatic acceptance and cross-vault
  promotion are independent controls and default off.

### Source repairs included

PR [#2](https://github.com/humzakhan/capsule-memory/pull/2) repairs capture
configuration/consent fencing and buffered durability, adapter attribution and
rewind, publication/lease authority, ambiguous source routing, retrieval limits,
archived same-vault reader constraints, authorization retries, daemon/IPC
ownership and export finalization. Combined and focused independent source
reviews closed the established material finding; they do not prove execution.

### Verification and limits

Git/whitespace and focused source inspection passed. Current-source formatting,
builds, static/type checks, tests, E2E, database/concurrency behavior, adapter
compatibility, cryptographic interoperability and provider acceptance are
**NOT RUN**. Historical setup recorded formatting and relay typecheck failures;
this beta does not claim those gates pass.

There are no tested binary assets, package-manager installs, hosted relay or
cloud inference service. Published-body cache eviction, native keystores,
automatic key rotation, embeddings, binary attachments, destructive retention,
universal session resumption and executable skill installation are not provided.
Local plaintext and keys require trusted devices. Tombstones retain history;
previously disclosed content or keys cannot be recalled.

Read [release readiness](docs/releases.md), [session status](docs/session-status.md)
and the [security policy](SECURITY.md) before evaluation. Use synthetic data.
