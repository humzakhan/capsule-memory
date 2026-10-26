# 0001: Go locally, TypeScript at the Cloudflare boundary

**Status:** accepted architecture; dependency versions and platform packaging pending.

## Decision

Use Go for the local service, domain model, SQLite access, cryptography, sync client, CLI, and MCP bridge. Use TypeScript only for the Cloudflare Worker and Durable Object. Keep shared wire contracts and synthetic examples in `contracts/`.

Prefer Go standard packages, the official Go MCP SDK, explicit SQL, and direct Workers/DO APIs. Add a CLI library or router only when concrete command/routing complexity justifies it.

## Why

The local component benefits from a small process, explicit concurrency, durable embedded storage, and straightforward deployment. TypeScript fits Cloudflare's native runtime and tooling without forcing the desktop core into a JavaScript service or requiring a Go-to-Wasm layer in the relay.

This is a project trade-off, not a claim that two languages are always simpler. It creates a cross-language protocol boundary that requires exact byte framing and shared vectors. Keep business and conflict semantics on the Go side; the relay handles opaque coordination and authorization only.

## Consequences

Two dependency graphs and toolchains will need pinned versions. SQLite, OS key stores, and CGO may complicate cross-platform delivery. Do not advertise a universally static single binary until the actual driver and supported platforms are tested.

The repository begins without unused manifests or application directories. The first implementation introduces only the packages and pinned tools it needs. The initial desktop assumption is Linux/macOS; confirm the owner's required OS matrix before committing packaging choices.
