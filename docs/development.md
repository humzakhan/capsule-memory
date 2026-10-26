# Development

**Current next step:** follow the [owner-machine runbook](run-v1.md) to resolve pinned dependencies, build, and execute the focused E2E journey. Go and Cloudflare source/manifests exist; runtime checks and generated dependency lockfiles remain pending. See [delivery status](v1-status.md) before claiming a feature works.

## Execution boundaries

On the shared OVH host, read `/srv/t3code/.agents/skills/execution-policy/SKILL.md` before preparing any project job. Installs, project validators, tests, typechecks, builds, benchmarks, and runtimes remain blocked until an execution route and per-job isolation are authorized.

Do not substitute direct host execution, another account, an old runner, or unapproved CI. A separate directory, worktree, normal T3 thread, or systemd service is not isolation. This setup does not authorize Boat, cloud provisioning, paid usage, or background services.

Documentation authoring and read-only file/Git inspection are permitted within the current task. Record missing checks clearly. Do not run a skill validator or schema checker on this host merely because it is small.

## Language and library choices

V1 targets Go 1.24.0 and pins the official Go MCP SDK v1.4.0, modernc SQLite v1.39.1, and Wrangler v4.129.1 via npm with Node 22+. See the implementation runbooks for source evidence and platform limits. The table below separates source choices from pending acceptance; installed compatibility remains unverified.

| Boundary | Current source choice | Pending evidence or deferred work |
| --- | --- | --- |
| Local CLI | Go 1.24.0, standard packages, explicit construction | Platform builds; background service deferred |
| MCP | Official Go SDK v1.4.0; fixed-scope stdio process | SDK runtime behavior and specific harness compatibility |
| Local data | modernc SQLite v1.39.1 and FTS5, explicit SQL | Runtime FTS5, packaging, backups, migrations |
| Relay | TypeScript, native Workers/DO APIs, Wrangler v4.129.1, npm | Dependency lockfile, typechecks, provider acceptance |
| Cryptography | AES-256-GCM, Ed25519, explicit v1 framing | Runtime interoperability, security review; automated key distribution deferred |

No general web framework, ORM, vector database, distributed broker, or frontend is needed for the first slice. Dependencies must earn their maintenance cost. The [Go SDK list](https://modelcontextprotocol.io/docs/sdk) is the source for MCP SDK support, not a promise about every harness feature.

## Go practice

Keep domain behavior independent of transport and harness details. The current entrypoint is `cmd/agent-memory`, with domain code under `internal/memory`; extend cohesive packages rather than adding an empty clean-architecture tree. Consumer-owned interfaces belong at storage, crypto, and transport boundaries where testing or alternate implementations require them.

Use explicit errors and transactions. Keep context cancellation and shutdown observable. Bound goroutines, buffers, retries, body sizes, and archive batches. Avoid global state, hidden work in constructors, reflection-heavy plumbing, speculative caches, and optimization without measurements.

Use standard formatting, descriptive operation errors, and focused documentation for exported contracts. Follow [Go review comments](https://go.dev/wiki/CodeReviewComments); [Effective Go](https://go.dev/doc/effective_go) remains useful for core idioms but does not cover all modern Go features.

Pin dependency versions and document platform implications when modules are introduced. Verify FTS5 at the driver/build boundary, not just through an SQL example. Never claim portable single-binary delivery before checking native-library and keystore dependencies.

## Testing practice

Design tests around durable outcomes: lost acknowledgements, crashes between writes, replay, concurrent offline changes, deletion, rekeying, cross-scope input, and search provenance. Use synthetic content and deterministic clocks where time matters. Do not write tests that merely mirror a function's implementation or assert documentation wording.

With an authorized isolated route and actual modules, the expected Go checks include formatting inspection, `go vet ./...`, focused `go test`, and race testing where supported. TypeScript checks must include typechecking and Go/TypeScript contract parity. These are future expectations, not commands run during repository setup.

Provider conformance and quota/hibernation behavior need controlled integration jobs against explicitly authorized test resources. Emulator success alone cannot prove real R2 conditional-write or billing behavior. See [verification](verification.md) for acceptance scenarios and evidence format.

## Harness configuration

`AGENTS.md` is the shared project entrypoint. `CLAUDE.md` imports it, avoiding a second rulebook. Codex discovers project skills under `.agents/skills`; Claude uses the `.claude/skills` symlink. If a filesystem or harness cannot follow the symlink, use the canonical skill paths explicitly rather than maintaining divergent copies.

`.agents/personas` contains canonical engineer and reviewer guidance. Claude's `.claude/agents` files are thin native adapters that instruct a delegated worker to read that guidance. They do not select a model, bypass permissions, launch automatically by policy, or authorize additional concurrency. Codex can read the canonical persona directly; no unsupported native persona registration is implied.

The adapter format follows [Claude subagent documentation](https://code.claude.com/docs/en/sub-agents); shared instructions follow [Claude memory/import documentation](https://code.claude.com/docs/en/memory) and [Codex AGENTS.md](https://learn.chatgpt.com/docs/agent-configuration/agents-md). Discovery is documented by the providers but has not been exercised in a fresh project session. Do not restart active harness sessions to load these files.

Default to one agent. Do not create new top-level T3 threads, workers, hooks, MCP server registrations, or scheduled tasks without the relevant user authorization. The project defines future runtime hooks; it does not install account-policy enforcement hooks.

## Writing and delivery

Use main-point-first, short paragraphs, and a few useful bullets. On this account, freshly load the required writing skill before each GitHub or Linear write. Keep the actual public-facing text concise and preserve evidence and material risks.

Before publishing, inspect the staged diff for personal data and credentials. `.gitignore` is a convenience, not a security boundary. Use synthetic fixtures only. Never put decryption keys into GitHub Actions or a remote review prompt.
