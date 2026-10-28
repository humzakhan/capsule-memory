# Changelog

## v0.1.0-beta.4 — refreshed README artwork

Documentation-only update; product code and dependencies are unchanged.

- Replace the three original README SVG diagrams with the supplied FROST Light
  PNG illustrations for encrypted archives, memories/skills and session history.
- Preserve descriptive alternative text and the existing opt-in/privacy guidance.
- Remove the superseded SVG assets; previous release tags retain their originals.

## v0.1.0-beta.3 — everyday workflows and visual guides

Documentation-only source update; product code and dependencies are unchanged.

- Rewrite the README for non-technical readers: everyday benefits, guided setup,
  encrypted background sync, optional conversation capture and privacy controls.
- Add accessible SVG diagrams explaining how devices sync, why skill approval
  stays local and how conversations can become reviewable memories.
- Link the companion Agent Workspace with an explicit access-required note;
  clarify that saved content sync does not install skills or import every app.
- Keep public-beta wording, build-from-source availability, prior validation and
  known limits. No binaries, services or infrastructure are introduced.

## v0.1.0-beta.2 — validated experimental source update

Source archives only; synthetic evaluation recommended. No prebuilt executables
or hosted service. See [release evidence](docs/releases.md) for exact scope.

- Update Go to 1.26.10, MCP SDK to 1.4.1, segmentio/encoding to 0.5.4, x/sys to
  0.44.0, Wrangler to 4.154.0 and compatible pinned Worker types. Dependency and
  standard-library vulnerability scans are clean for the documented targets.
- Repair policy patches and successful IPC acknowledgements; preserve ancestry
  and stale-review errors over IPC; keep local-backend schema compatibility
  explicit and validate memory/decision candidate states.
- Quarantine T3 responses missing advertised history continuations and bound
  incremental stale-state refresh with affected-record indexed lookups.
- Include scoped Mac/Cloudflare QA evidence, failure-case ledger, rollback
  guidance, current setup/discovery metadata and private vulnerability reporting.
- Preserve default-off session controls, exact encrypted retries, v1 wire bytes,
  experimental status and all documented incomplete/unsupported behavior.

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
