# Relay setup and publication boundary

For current executed checks and provider limits, see [release evidence](releases.md).
Use [run-v1.md](run-v1.md) with synthetic data before a separately authorized
deployment. This guide does not itself authorize cloud changes.

## Upgrade an existing relay for beta.5 pairing

An authorized operator builds/deploys the existing Worker from the same beta.5
tag as the CLI, using the pinned lockfile and reviewed configuration. Preserve
the existing Worker identity, vault ID, owner public pin, private R2 binding and
Durable Object namespace/migration history. Do not rerun `init`, replace the
archive or bootstrap another owner. Beta.4 lacks the enrollment endpoints.
Install on the new computer through [the CLI guide](install.md); a joining
device uses signed requests and needs no Cloudflare account token.

The bootstrap instructions below apply to a new vault. For an existing vault,
use [device pairing](device-enrollment.md) to add ordinary devices. The legacy
owner-created profile route remains an explicit alternative.

## Bindings and bootstrap

`cloudflare/wrangler.jsonc` declares the `VAULTS` Durable Object binding, the
SQLite migration `v1` for class `Vault`, and private R2 binding `ARCHIVE`.
`remote: false` keeps local development on emulated storage. The shipped Worker
has no public route and disables observability. Do not enable R2 public access,
CDN caching, object expiration, or destructive lifecycle rules.

One Worker configuration serves exactly one `VAULT_ID` and independently pinned
`OWNER_PUBLIC_KEY_B64`. These are public verification values from `init` JSON,
not recovery material. Put only these values in local `.dev.vars`, with
`ALLOW_LOOPBACK_HTTP=true` for local development. Never copy `device.json`,
`owner.json`, scope keys, private keys, or the local SQLite database into Worker
configuration or R2.

1. Initialize the owner locally with the CLI and retain its output privately.
2. Configure the two public values before starting the local Worker.
3. Run owner `sync --scope <scope_id>` to publish generation `1`. Alternatively,
   `PUT /v1/vaults/<vault_id>/manifest` accepts the exact owner-signed control
   envelope. `init` retains that pending signed control in the local database;
   do not invent another unsigned bootstrap format.
4. Create another device with `device create --out <new-home>` on the owner.
   Sync the owner control before the new device syncs. Transfer the new profile
   only through an authenticated private channel. It contains scope keys and a
   device signing identity, but no owner private key.

The relay accepts generation `1` only from its pinned owner. Every later control
increments the current generation by exactly one. Retrying the current identical
signed payload/signature returns its original cursor. A different signed payload
at the same generation returns `409`; an older generation is rejected. Control
publication shares the record journal and atomic head barrier.

Manual revocation/rotation requires an owner-signed next control, followed by
authenticated distribution of new scope keys only to remaining devices. Do not
edit the pinned owner key to enroll a device. Initial scoped key delivery is
provided by beta.5 pairing. Automatic epoch rotation/distribution, owner-key
rotation and recovery invitations remain unsupported. The relay understands
signed control updates; use only a reviewed owner-side authoring path available
in the CLI, or keep that operation manual.

## Authentication and resource limits

The exact wire format is [v1-contract.md](v1-contract.md). The relay verifies
Ed25519 with Web Crypto over exact payload strings, never reserialized payload
JSON. Record requests also bind method, exact pathname/query, vault, scope,
device, time, nonce, and SHA-256 of exact request bytes. Both envelope and HTTP
identity must agree. The relay checks ciphertext digest, 12-byte nonce, outer
shape, signature, current scope grant, and key epoch without decrypting AES-GCM.

Authenticated requests permit at most 300 seconds of clock skew. Nonces must be
32–128 lowercase hex characters. Nonces are not stored as a replay cache:
timestamp-bounded replay is safe for mutation because exact transports are
deduplicated durably. Revoked devices cannot publish or fetch changes. Historical
record authors are checked against the control active at original publication.

Limits are 2 MiB per record request, 128 KiB decoded control payload, 64 scopes,
256 devices, and 128 decimal digits per counter. IDs are 8–128 URL-safe ASCII
characters. JSON uses fatal UTF-8 decoding and rejects duplicate object keys,
including equivalent escaped key names, before archive publication. The root
value has depth 0; values beyond depth 32 are rejected, matching the Go parser.
Standard JSON parsing supplies value semantics; a bounded structural walk checks
decoded key uniqueness without changing the signed bytes. Unknown payload
fields and noncanonical base64 are rejected. Control
requests are at most 256 KiB. GET request bodies are forbidden. Body transfer is
limited to ten seconds; provider operations also have ten-second deadlines.
Provider deadlines do not cancel an in-flight write or prove it failed.
The queue permits
eight requests per vault, including the active one; excess requests return `429`.
These bounds do not guarantee a provider spending cap or prevent abusive traffic.

Changes use canonical `after` cursors, `limit=1..100`, and return at most eight
visible entries and roughly 8 MiB per response. `limit` bounds scanned entries,
including unauthorized scopes; their bytes are withheld while the returned
watermark advances. Every control entry is visible to authorized devices so
historical envelope validation remains possible. The Go client requests one
scanned entry per page. There are no plaintext search or object-list endpoints.

All errors use bounded generic JSON codes. Source contains no content/auth
logging, and observability is disabled. Review account-level access logs and
external instrumentation separately before any production use.

## Archive, retries, and rebuild

Keys below private prefix `v1/<vault>/` are `objects/<sha256>` (exact opaque
signed-envelope bytes), `journals/<sha256>` (one linked publication), and mutable
`head`. A journal names its cursor, previous journal hash, type, and object hash.
Head names its cursor, last journal, and current signed control. Objects and
journals are written with `If-None-Match: *`; existing bytes must match their
content hash. Head is written with `If-Match: <etag>` or initial
`If-None-Match: *`. The conditional R2 success establishes the acknowledgement
boundary. SQLite state never establishes publication.

On a failed/ambiguous R2 call, the relay returns `503`. Retry the exact persisted
envelope with a fresh HTTP timestamp/signature. Do not change its transport ID,
nonce, ciphertext, or signature. Reconciliation reads the current head directly
through the binding and discovers successful ambiguous writes by following its
journal. Uploaded objects alone are never evidence of commitment. Orphans remain
harmless and are retained; no history GC is implemented.

SQLite caches archive references, transport identities, signed digests, control
receipts, and rebuild progress. It stores no decrypted content and no record
ciphertext copies. After cache loss, requests walk at most eight journal links
backward and verify at most eight entries forward per attempt. Forward replay
validates every object digest, historical owner control, envelope signature,
scope, author, and epoch. Partial recovery persists across requests and returns
`503 archive_rebuilding` with `Retry-After: 2`; use bounded, jittered retries.
An archive rollback or fork relative to retained cache fails closed. Rebuilding
an empty cache cannot independently prove provider freshness.

No WebSockets, background fanout, remote reset endpoint, retention deletion,
or automatic compaction is implemented. Manual CLI sync is the correctness path.
To demonstrate cache recovery locally, stop Wrangler and discard only the
disposable emulator DO directory while retaining its R2 directory, then restart;
the E2E harness describes and owns this isolated synthetic-state operation.

## Provider and dependency evidence

Checked primary documentation on 2026-10-26. Wrangler is pinned to
[`4.129.1`](https://github.com/cloudflare/workers-sdk/releases/tag/wrangler%404.129.1),
with compatibility date `2026-09-07`, matching that release's workerd generation.
No lockfile was fabricated; the owner-machine install must resolve transitive
dependencies and create a real lockfile for review.

- [R2 Workers API](https://developers.cloudflare.com/r2/api/workers/workers-api-reference/)
  documents conditional `put`, `null` on failed preconditions, direct reads,
  object size, and ETags. The adapter uses quoted ETags in `If-Match`.
- [R2 consistency](https://developers.cloudflare.com/r2/reference/consistency/)
  documents atomic conditional writes, read visibility, and ambiguous in-flight
  writes. A negative read does not cancel an earlier in-flight write.
- [SQLite DO storage](https://developers.cloudflare.com/durable-objects/api/sqlite-storage-api/)
  documents `sql.exec` and synchronous transactions; [Wrangler configuration](https://developers.cloudflare.com/workers/wrangler/configuration/)
  documents bindings and SQLite migrations.
- [Workers Web Crypto](https://developers.cloudflare.com/workers/runtime-apis/web-crypto/)
  documents Ed25519, SHA-256, raw verification-key import, and signature checking.
- [Local persistence](https://developers.cloudflare.com/workers/local-development/local-data/)
  describes emulated persisted storage. Emulator success would not establish
  production R2 durability, races across provider failures, quotas, or cost.

Another object store must independently provide create-once immutable writes,
atomic head CAS, direct fresh reads, and durable read-after-write visibility.
S3-shaped APIs alone do not satisfy this boundary. R2 account loss, provider
omission, rollback with an empty cache, and compromised trusted endpoints remain
outside these guarantees. Maintain separately authorized encrypted backups.
