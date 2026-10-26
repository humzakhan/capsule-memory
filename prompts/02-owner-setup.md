# Initialize an owner device

Use this for a new vault only. An additional machine uses [device enrollment](04-enroll-device.md).

~~~text
Set up the owner device for a NEW Capsule Memory vault.
Use the trusted https://github.com/humzakhan/capsule-memory checkout.

Inputs: CHECKOUT, SOURCE_REVISION, BINARY, private OWNER_HOME, intended RELAY_URL,
EXECUTION_ROUTE, ALLOWED_ACTIONS, and whether this is a synthetic demo or real setup.
Do not put secrets into these inputs.

Read AGENTS.md, prompts/README.md, docs/go-client.md, docs/run-v1.md,
docs/relay-setup.md, and docs/renaming.md as relevant.

1. Confirm this is a new vault. If an existing profile/vault is intended, stop
   initialization and choose enrollment or recovery. Never overwrite existing files.
2. Use an explicit private home outside the checkout, shared folders and cloud sync.
   Explain local plaintext SQLite/FTS and restricted-file keys; recommend the user's
   existing disk encryption/private backup practice without provisioning services.
3. If execution is authorized, initialize with the actual CLI:
   BINARY --home OWNER_HOME init --relay RELAY_URL
   Allow HTTP only for an explicit loopback development relay with the documented flag.
   Record public vault/scope/device identifiers; never print owner.json or device.json.
4. The relay operator needs only VAULT_ID and OWNER_PUBLIC_KEY_B64 from init's public
   output. Never send owner/device private keys or scope keys to Cloudflare.
   Coordinate relay readiness with prompts/03-relay-setup.md.
5. Publish queued owner control with sync --scope SCOPE_ID after the relay is ready.
   Control generations start at 1 and publish in order. Device creation before initial
   sync is supported; publish membership before allowing a child to sync.
6. If a synthetic write/read/sync was authorized, check provenance and coverage.
   Distinguish locally queued data from archived publication. Document protected owner
   recovery custody; there is no turnkey backup/export/recovery-key service.

Return public identifiers, private file LOCATIONS only, relay/bootstrap status,
actual evidence and NOT RUN items. Keep owner authority on the owner device.
Do not grant this setup agent ongoing memory access, enroll extra devices, deploy,
enable schedulers, or activate skills as a setup side effect.
~~~
