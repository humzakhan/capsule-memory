---
name: go-engineering
description: Implement or review capsule-memory Go packages, CLI commands, and local MCP integration using the project's durability and privacy boundaries. Does not authorize installs or execution.
---

# Go engineering

Apply the shared [maintainability contract](../../../docs/development.md#maintainability-contract) and [delivery/check gate](../../../docs/development.md#explicit-check-gate) within the current task and host permissions.

Read `AGENTS.md` and [development](../../../docs/development.md). For boundary changes, read [architecture](../../../docs/architecture.md); for wire changes, also load the encrypted-sync skill.

1. Identify the domain operation and its acknowledgement boundary. Keep CLI, MCP, and harness adapters thin; they call the same application service.
2. Use standard packages first, small consumer-owned interfaces at real boundaries, and explicit dependency construction. Avoid generic repository frameworks, global mutable clients, and one interface per struct.
3. Pass `context.Context` through cancellable work. Give every goroutine an owner, shutdown path, and bound. Bound queues and retry work; keep a slow provider from blocking local reads.
4. Wrap errors with operation context and preserve error inspection. Do not expose memory content, tokens, keys, search queries, or raw database errors containing values. Treat partial writes as explicit states.
5. Cover observable invariants with the relevant [verification scenarios](../../../docs/verification.md). Run checks only in the authorized route and report skipped work honestly.

Prefer explicit SQL and transactions for SQLite. Use the official Go MCP SDK after pinning a supported version. Select the Go toolchain and SQLite driver together with the supported-platform and FTS5 decision; do not assume a Go binary is CGO-free.

Use ordinary expanded critical flow, cohesive responsibilities and meaningful boundary names. Make cleanup, errors, lock/transaction ownership, cancellation and durable success explicit. Inspect Go formatting through the gate; rewrite with `gofmt` only when source formatting is in scope and authorized. Follow [Go review guidance](https://go.dev/wiki/CodeReviewComments); use [Effective Go](https://go.dev/doc/effective_go) for core idioms while checking current language and library documentation for newer features.

For a new dependency, explain the concrete need, license, update burden, platform cost, and simpler alternative. Profile measured bottlenecks before introducing pools, parallel pipelines, embeddings, or caches.
