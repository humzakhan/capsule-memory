# Prepare the central relay

Use this for the content-blind Worker, per-vault Durable Object, and encrypted R2 archive.

~~~text
Prepare the Capsule Memory central relay from the trusted repository.

Inputs: CHECKOUT, SOURCE_REVISION, local-emulator OR production-plan mode,
public VAULT_ID and OWNER_PUBLIC_KEY_B64, intended URL/bindings,
EXECUTION_ROUTE and ALLOWED_ACTIONS. Cloud account credentials must use an
approved private credential route, never this prompt, source, or chat.

Read AGENTS.md, prompts/README.md, docs/relay-setup.md, docs/run-v1.md,
cloudflare/wrangler.jsonc, and docs/v1-contract.md.

1. Identify local-only evaluation versus actual deployment. Default to a plan.
   Repository access or a setup prompt does not authorize Cloudflare provisioning,
   deployment, paid usage, public routing, CI, or Boat.
2. Explain bootstrap: obtain ONLY public owner pin and vault ID from owner init.
   The relay must never receive owner.json, device.json, private keys, scope keys,
   plaintext records, a local search index, or private GitHub credentials.
3. For authorized local evaluation, follow the runbook's local Wrangler configuration.
   Keep bindings local, emulator state private, existing .dev.vars intact, and
   loopback HTTP explicitly scoped. Use synthetic profiles and bounded processes.
4. For production, prepare a separate concrete deployment plan: private R2 bucket,
   correct bindings and SQLite DO migration, owner pin, vault identity, HTTPS route,
   allowed request sizes/timeouts, independent backups, quotas, costs and cleanup.
   Verify current official capabilities/pricing before proposing provider choices.
   Stop before new infrastructure/spending unless explicitly authorized.
5. Preserve the acknowledgement boundary: immutable encrypted objects/journal then
   conditional durable R2 head publication. DO SQLite is a rebuildable metadata cache.
   Document ambiguous provider timeouts, exact-byte retries and bounded 503 recovery.
6. Verify only through the permitted route. Separate emulator evidence from real R2.
   Do not invent a hosted MCP endpoint, plaintext search, daemon or WebSocket service.

Return redacted configuration/plan, public bootstrap identity, exact evidence,
unverified guarantees and deployment actions awaiting authority. An R2 binding or
successful Worker startup alone does not prove encrypted convergence or crash safety.
~~~
