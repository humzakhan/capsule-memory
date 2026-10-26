# Capsule Memory V1 implementation contract

Branding does not change the fixed legacy v1 cryptographic domains below. See [rename compatibility](renaming.md) before changing any signing or AAD bytes.

Implementation target for the two-hour build. This specializes the architecture; runtime behavior is unverified until executed. Implementers must preserve these exact cross-language bytes and report unsupported behavior honestly.

## Boundaries and v1 limits

Go owns local records, SQLite FTS5, outbox, cryptography, CLI, MCP, and sync client. TypeScript owns `cloudflare/`; black-box tooling lives under `e2e/`. One private Worker deployment is pinned to one opaque vault and one owner verification key through configuration. A vault may have multiple opaque scopes and enrolled devices.

Keys stay on devices in restrictive permission files for v1; native OS keystore integration is deferred. Enrollment uses an owner-created signed membership plus an explicitly transferred device configuration over an authenticated private channel. Never upload content/private keys in Worker configuration, source, or diagnostics. Automatic remote key distribution and unattended recovery are not part of v1. Unsupported key epochs fail closed.

## Exact encoding

All IDs match `[A-Za-z0-9_-]{8,128}`. Base64 uses the standard padded alphabet. Epochs, generations, and cursors are canonical decimal strings (`0` or a nonzero digit followed by digits), numerically compared without JavaScript Number rounding. Maximum signed envelope body is 2 MiB; decoded record text is bounded to 1 MiB. All delimiters below are literal LF, including the final LF.

A signed envelope is JSON `{ "payload": "<base64 exact UTF-8 JSON bytes>", "signature": "<base64 Ed25519 signature>" }`. Envelope signing input is UTF-8 `agent-memory/envelope/v1\n` followed by the exact ASCII `payload` value and final `\n`. Do not parse/re-serialize the payload to verify it.

Decoded envelope payload fields: numeric `version: 1`, `vault_id`, `scope_id`, `device_id`, `key_epoch`, `transport_id`, `nonce_b64`, `ciphertext_b64`, `ciphertext_sha256` (lowercase hexadecimal SHA-256 of ciphertext bytes). Reject unknown versions, malformed encodings, inconsistent identities, excessive size, and invalid digests/signatures. `transport_id` is random and stable for exact retries.

Encrypt the JSON logical revision with AES-256-GCM using its scope's 32-byte key and a fresh cryptographically random 12-byte nonce. Ciphertext includes the GCM tag. AAD is exact UTF-8:

```text
agent-memory/aead/v1\n<vault_id>\n<scope_id>\n<device_id>\n<key_epoch>\n<transport_id>\n
```

The decrypted revision follows the logical record schema. Verify inner vault/scope/author against the outer context, operation/revision identity, parent causality, and authorization before applying. Persist the exact envelope in the local transaction with revision/outbox before local success. Signed bytes and transport identity remain unchanged across ambiguous retries.

## Owner control

Signed control uses the same `{payload,signature}` shape, with prefix `agent-memory/control/v1\n`, payload ASCII, final LF. Decode to `{version:1,vault_id,generation,scopes:[{scope_id,key_epoch}],devices:[{device_id,public_key_b64,scope_ids,revoked}]}`. Owner public key is raw 32-byte Ed25519; relay pins it via `OWNER_PUBLIC_KEY_B64` and vault via `VAULT_ID`. Control generation increases exactly by one from initial `1`.

The public signed manifest exposes only opaque metadata/public keys. Clients pin the owner key independently, verify the control signature, and remember the highest generation. Do not accept an older control generation. Device key packages include only granted scope keys and that device's private identity, never the owner private key. Automated rotation is optional only if correctly implemented; otherwise describe the manual key-distribution limit and reject stale-epoch writes.

An exact retry of the currently committed signed control must return its existing publication receipt, not create another generation. The same generation with different signed bytes is a conflict. Never infer publication merely because an immutable object upload exists; only head-reachable journal state establishes commitment.

The owner retains each generated signed control locally until publication is confirmed. On an empty relay, sync publishes initial generation `1` and any queued successors in order; creating device B before owner A's first sync must not skip generation `1`. A device profile contains the public control history needed for verification, not an invented trust-on-first-use owner key. Bootstrap `GET /manifest` may return `404` before the first owner publication; this is not permission to reset an established local owner pin or generation.

Control changes publish through the same durable archive head as records. Current authorization uses current control; validating old committed envelopes uses the control state at their publication. A presently revoked author does not invalidate legitimate historical records. A revoked device cannot append or fetch future changes.

## HTTP interface

All routes are under `/v1/vaults/<vault_id>`.

| Method/path | Request | Response |
| --- | --- | --- |
| `GET /manifest` | No private content; signed public control is readable | Signed control envelope |
| `PUT /manifest` | Owner-signed control envelope | `{cursor, generation}` after durable publication |
| `POST /records` | Device-authenticated signed record envelope | `{cursor, duplicate}` after durable publication |
| `GET /changes?after=<decimal>&limit=<1..100>` | Device-authenticated, scope-bound | `{cursor, head, entries, has_more}` |
| `GET /events` | Optional authenticated WebSocket upgrade | Opaque `{cursor}` wake hints only |

Journal entries are `{cursor,type:"record",envelope}` or `{cursor,type:"control",manifest}`. Scope-filtered replay still advances `cursor` across unauthorized entries without returning their envelopes. Return entries in publication order. `head` is the latest durable head at the read; `cursor` is the scanned watermark. Cursor advancement and applied local changes are atomic.

Device-authenticated requests carry `X-AM-Device`, `X-AM-Scope`, `X-AM-Time` (Unix seconds decimal), `X-AM-Nonce` (random lowercase hex), and `X-AM-Signature` (standard base64). Signature input is UTF-8:

```text
agent-memory/request/v1\n<METHOD>\n<pathname-and-query>\n<vault_id>\n<scope_id>\n<device_id>\n<unix-seconds>\n<nonce>\n<lowercase-SHA256-hex-of-exact-body-bytes>\n
```

Allow at most five minutes of clock skew. Use HTTPS except explicit loopback development mode. Current membership must authorize the signing device and requested scope. Verify record envelope device/scope equals request context. Timestamp-bounded replay cannot cause a second logical mutation because publication/application are idempotent. No content or auth header logging.

Persist immutable encrypted objects and immutable linked journal segments before R2 head compare-and-swap; do not acknowledge a DO-only transaction. Head contains cursor, last journal reference, and current signed manifest. Durable transport deduplication and any index must rebuild from the head-reachable history; DO state is disposable cache. Use direct R2 reads and atomic conditional writes, never CDN-cached head reads. No destructive history GC in v1.

## CLI contract for black-box tests

Commands output one JSON result on stdout; errors/diagnostics go to stderr with nonzero exit. Global `--home <directory>` selects device state. Never print private keys or raw config on stdout.

- `init --relay <url>`: create owner/device identity, vault/scope keys and local DB. Result includes `vault_id`, `scope_id`, `device_id`, and public deployment values only.
- `device create --out <new-home>`: owner generates a separate device profile and updated signed control; new profile has no owner private key. Result includes new public device identity. Owner must sync control before the new device publishes.
- `store --scope <id> --kind memory|skill --text <text> [--id <record-id>] [--parents <comma-separated-revision-ids>]`: return logical revision metadata including `record_id`, `revision_id`.
- `recall --scope <id> --query <FTS-query>`: return `{results:[...],coverage:...}`; results carry revision IDs, provenance, and conflict state.
- `get --scope <id> --id <record-id> [--history]`, `delete --scope <id> --id <record-id>`, `restore --scope <id> --id <record-id>`: preserve causal heads/tombstones. History remains inspectable, never automatically recalled after deletion.
- `sync --scope <id>`: refresh owner control, publish authorized queued operations, replay atomically. `status`: show cursor/pending/coverage without secrets. `rebuild`: rebuild derived indexes without discarding outbox; document clean-device archive recovery separately.
- `skill approve --scope <id> --id <record-id> --revision <id>` records an owner-signed local exact scope/revision/text-digest release for runtime `text/markdown` and capability `read-as-text`. `skill activate` with the same arguments verifies that release and selects the unique visible current revision for local active text retrieval. Results identify `approved`, `active`, and `staged`; changed/deleted/conflicting revisions are inactive. Incoming unapproved text remains staged. This local text activation executes no code and installs no harness files. Signed-release propagation and executable installation are deferred.
- `mcp --scope <id>`: official SDK stdio server; tools call the same domain operations. Caller scope is fixed by launch, not widened by tool arguments. Implement `memory_store`, `memory_recall`, `memory_get`, `memory_delete`, `sync_status`, and skill staging/list/fetch as feasible.

Polling/background service may be added only after the durable manual `sync` path is complete. No promise of universal hooks or immediate delivery. E2E tooling must test the real implemented CLI/MCP/relay journey, not a mock that claims R2 guarantees.
