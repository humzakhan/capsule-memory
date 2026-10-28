# Device enrollment protocol v1

This is an additive protocol under `/v1/vaults/VAULT/enrollment`. Legacy control,
record signing and AAD domains do not change. All messages use HTTPS except an
explicit loopback-only development configuration. Unknown/duplicate fields,
noncanonical base64, malformed identities and unsupported versions fail closed.

## Signed messages and identities

`Signed` is `{payload,signature}`: standard canonical padded base64 of exact JSON
bytes and a 64-byte Ed25519 signature. Sign exact UTF-8
`DOMAIN + "\n" + payload + "\n"`; never verify a reserialized payload. Its digest
is lowercase SHA-256 of `payload + "\n" + signature + "\n"`. Domain strings:

| Message | Domain | Signer |
| --- | --- | --- |
| Invitation | `capsule-memory/enrollment-invite/v1` | Pinned existing owner |
| Request | `capsule-memory/enrollment-request/v1` | Joining device |
| Decision | `capsule-memory/enrollment-decision/v1` | Pinned existing owner |

Invitation JSON: `version:1`, `vault_id`, random `invite_id`, `relay`,
`owner_public_key_b64`, `scope_ids`, canonical decimal Unix seconds `issued_at`
and `expires_at`, boolean `allow_loopback_http`. Its signed bytes are a private
bearer capability for **one request**, with an explicit scope ceiling. Lifetime
is at most 3600 seconds; future issue times over 300 seconds are rejected.

Request JSON: `version:1`, `vault_id`, `invite_id`, `invitation_sha256`, random
`device_id`, 32-byte Ed25519 `public_key_b64`, and separate 32-byte X25519
`encryption_key_b64`. It binds the exact signed invitation. The request signature
proves control of the signing identity and binds the encryption key. Successful
encrypted delivery/acknowledgement completes possession of the encryption key.
The UI compares all 128 displayed bits of the signed-request digest, grouped as
eight four-hex-digit groups. Names or the relay's labels never establish identity.

Decision JSON always has `version:1`, `vault_id`, `invite_id`, `request_sha256`,
`action`, `scope_ids`, `parent_sha256`, `archive_cursor`, `activation_sha256`, `key_package_b64`.
An approval commits a random 32-byte activation challenge with `activation_sha256`
and additionally has `control`, the exact signed successor legacy
membership. Selected scopes must be a nonempty subset of the invitation ceiling.
It adds exactly the bound device/public key to the current control, preserving
existing scope epochs/devices. `parent_sha256` names the reviewed signed current
control; `archive_cursor` names the fully scanned current archive head. Both must
match at initial preparation. A denial has empty scope list, parent/cursor/key
package/activation digest strings and no `control` field.

## Encrypted key package

Only Go peers encrypt/decrypt the package, using standard `crypto/hpke` RFC 9180
base mode: DHKEM(X25519,HKDF-SHA256), HKDF-SHA256, AES-256-GCM. `hpke.Seal` returns
its encapsulation concatenated with ciphertext; `key_package_b64` encodes those
exact bytes. Info is exact UTF-8:

```text
capsule-memory/enrollment-key/v1\nVAULT\nINVITE_ID\nSIGNED_REQUEST_DIGEST\n
```

The complete decision signature authenticates the sender, ciphertext, request,
scope selection and signed control. HPKE base mode alone is not owner identity.
No recipient key is converted from Ed25519 or reused as an encryption key.

Plaintext contains `version:1`, `vault_id`, `device_id`, `invite_id`,
`request_sha256`, `activation_secret_b64`, `scope_keys` and ordered `control_history`.
The fresh activation secret is unrelated to content keys and appears only inside
the HPKE package until the authenticated acknowledgement. Each scope key has
`epoch`, `key_b64`, `label`, `role`. The history begins at generation 1 and ends
at the exact approval control; it is bounded to 256 entries. The receiver checks
every owner signature, scope/key/epoch and its own public identity before writing
a profile. No owner or signing private key appears in the package. Plaintext is
bounded to 1 MiB; the signed transport wrapper is bounded to 2 MiB. Request bodies
are independently bounded; error/diagnostic output never includes these messages.

## HTTP authentication and routes

Admin read/decision routes require the pinned owner. Requester read/ack routes
require the exact submitted device signing key, even before membership exists.
Headers are `X-CM-Subject` (`owner` or device ID), `X-CM-Time` (canonical Unix
seconds), `X-CM-Nonce` (48 lowercase hex digits), `X-CM-Signature` (Ed25519 base64).
Sign exact UTF-8:

```text
capsule-memory/enrollment-http/v1\nMETHOD\nPATH_AND_QUERY\nVAULT\nSUBJECT\nTIME\nNONCE\nBODY_SHA256\n
```

Allow at most 300 seconds clock skew. The activation acknowledgement must open the signed challenge commitment;
a signing key without package decryption cannot mark the device active.
Queries are rejected on enrollment routes;
the body digest covers exact bytes. Replays are bounded and logical mutation is
idempotent. No redirects, Cloudflare OAuth tokens or R2 credentials are used.

| Route | Authority / behavior |
| --- | --- |
| `POST /invites` | Owner-signed invitation; exact retry registers once |
| `POST /requests` | `{invitation,request}`; both signatures validated; may register the same invitation after a lost owner reply |
| `GET /requests` | Owner HTTP proof; at most 32 request metadata entries |
| `GET /requests/ID` | Owner or exact requester HTTP proof; reconcile prepared publication and return current receipt |
| `POST /requests/ID/decision` | Owner HTTP proof plus signed decision; exact decision only after durable preparation |
| `POST /requests/ID/ack` | Exact requester proof and `{decision_sha256,activation_secret_b64}` after durable local activation |

A receipt always has `state` and `cursor`; it may contain the exact signed
`decision` and current signed `manifest`. States distinguish `invited`, `pending`,
`expired`, `denied`, `approval_prepared`, `approved`, `active`, `revoked` and
`conflicted`. List metadata can show prepared while the membership is already
committed; request-specific receipt reconciliation determines that outcome.
Revoked requesters receive no key package. An owner can reconcile its own
already-signed delivery after revocation without authorizing it again.

## Durable state and crash ordering

The R2 index `v1/VAULT/enrollment/state-v1` is CAS-updated and bounded to 256 KiB
and 32 entries. Entries contain signed invitation/request, immutable decision
reference/digest, retention deadline and activation acknowledgement. R2 decision
objects use the existing immutable content-addressed provider boundary. Every
acknowledged index mutation survives loss of DO SQLite; no enrollment truth lives
only in DO memory. Invites/requests expire, but accepted prepared approval is an
already authorized operation: exact retries can finish membership after its
invitation's original expiry, within receipt retention. Replaying an expired
invitation cannot start another request.

Owner order: complete all-scope replay -> check grant authority -> commit local
intent and write reservation -> persist immutable relay decision and index ->
commit ordered membership through the legacy archive head -> recheck current
grant/epochs -> deliver ciphertext -> activate profile locally -> authenticate ack.
Transactions never span network operations. While a local intent is unresolved,
normal writes/replay/rebuild/owner changes fail closed. Read-only inspection and
exact resume/denial remain available.

The relay serializes each complete operation, uses exact immutable bytes and
conditional R2 writes, and recovers membership receipts from head-reachable
history. Lost responses are reconciled before retrying a new logical action.
Once an approval is durably prepared, a conflicting denial is refused: resolve
its membership outcome and revoke instead. A denial committed first is terminal.
An occupied conflicting control generation cannot be overwritten by a stale
approval; the owner records the conflict and refreshes authority.

Retain entries until 24 hours after invitation expiry; expired slots are removed
on writes. A full inbox reports 429. Do not reset or silently evict live requests.
Late recovery after receipt retention uses `device cancel --confirm`, not deletion
of local intent. The owner persists a minimal legacy successor control in a
separate local cancellation record: if the exact target is active, revoke it;
otherwise advance membership generation without granting anything. The durable
head/generation barrier fences a late old approval CAS. Exact cancellation bytes
are retried; a provably advanced generation can require a new minimal revocation,
with bounded attempts. Only after a committed fence does the owner clear its
reservation. A terminal inbox denial is attempted separately; the result reports
`request_closed` rather than claiming an unavailable/expired receipt was deleted.
The primary console retains a local cancelled-request marker and rejects reuse;
this does not invent a globally signed denial when the relay did not acknowledge
one. A signed newer owner control can intentionally change authorization.
Lost cancellation responses resume through `device resume`.

No global freshness witness, automatic key rotation, physical ciphertext erasure,
anonymous discovery, or human-review signature is claimed. The provider can deny
service or replay older valid state within the existing threat model.
