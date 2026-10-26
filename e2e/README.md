# Owner-machine black-box journey

**NOT RUN.** No runtime, dependency install, typecheck, build, or E2E was executed
while authoring this suite. Follow [the runbook](../docs/run-v1.md) on a trusted
owner-operated Linux/macOS machine. This suite never selects a production relay.

```sh
node e2e/run.mjs --cli "$HOME/.local/bin/agent-memory-v1"
```

The script requires Node 22+ and the pinned locally installed Wrangler from
`cloudflare/node_modules`. It starts a real local Worker with a SQLite-backed
Durable Object and emulated R2. It uses a built CLI as a subprocess and speaks
JSON-RPC to that CLI's official-SDK MCP stdio server. No application internals,
SQL driver, unit framework, fake archive, or alternate relay implementation is
imported. A loopback HTTP forwarding proxy injects offline and lost-response
failures while the real Worker handles each delivered request.

The journey asserts:

1. Public bootstrap values, pre-sync enrollment, ordered initial controls,
   exact current signed-control retry, and MCP
   provenance survive encrypted publication and replica replay.
2. Offline outbox persistence, concurrent heads, tombstone suppression, lost POST
   acknowledgement, and exact transport duplicate receipts preserve history.
3. Index rebuild preserves pending writes; an empty authorized profile recovers
   from archive; discarding the DO cache preserves durable deduplication.
4. Tampered envelopes, substituted/unauthorized scopes, expired auth, malformed
   cursors, non-owner approval, and unapproved skill activation are rejected.
5. Scope-filtered pages advance across invisible entries; revocation blocks
   current access while old publications validate during a fresh recovery.

Only generated synthetic records/identities are used. A separate HTTP-only
synthetic scope uses freshly generated cryptographic bytes to check filtering;
it is deliberately never granted to a CLI replica. It is a relay-envelope
boundary scenario, not evidence of logical-record application in that scope.

All state lives in one private temporary directory. Success removes it;
`--keep` or failure retains it and prints only its path. Logs are bounded and
kept in the private directory; CLI output, keys, config, and complete envelopes
are never printed. Retained files include synthetic device secrets. Do not
attach the directory to an issue or upload it. Stop/cleanup is owned by the
script; SIGKILL or machine failure may require owner cleanup.

The cache-loss scenario stops the private Wrangler process group, checks for
separate `relay-state/v3/do` and `relay-state/v3/r2` directories, deletes only
`do`, and restarts with the same R2 state. Unknown layouts fail explicitly with
`DO reset scenario NOT COVERED`; they never silently pass. That path is an
emulator detail, not a production reset API. No HTTP cache-reset endpoint exists.

Emulator success would **not** prove production R2 durability, account quotas,
cost, failure races within Cloudflare, crash timing during each individual
provider write, or independent freshness/fork resistance. The suite injects a
lost acknowledgement after the real Worker returns success; it does not claim
to simulate an R2 server committing after a network timeout. CPU suitability
and long-history rebuild need separate authorized production-like evidence.

Supported negative activation evidence is rejection: full executable skill
activation/installation is unavailable in v1. Automatic epoch rotation, remote
key distribution, multi-head restore UX, background daemon/hooks, notifications,
and a Git bridge are outside this journey. No mock converts them into passes.
