# 0005: Owner-reviewed device enrollment in the existing Go CLI

Status: implemented for v0.1.0-beta.5; bounded local validation and independent
source review passed. See [release evidence](../releases.md) for limits.

## Decision

Extend the current Go binary with an interactive `devices` console and ordinary
`device` commands sharing memory/enrollment services. Use an invitation-gated R2
inbox on the existing relay, locally generated device signing/encryption keys,
explicit fingerprint/collection approval, and owner-authenticated HPKE delivery.
Preserve legacy membership/signing/AAD bytes and existing reader-subset checks.

Use Go 1.26's standard `crypto/hpke`: RFC 9180 DHKEM(X25519,HKDF-SHA256),
HKDF-SHA256 and AES-256-GCM, with a separately owner-signed complete decision.
No crypto dependency or custom primitive is required. The console is a bounded
line-oriented terminal menu using the already pinned terminal-detection package;
it adds no terminal framework or local admin HTTP server. This keeps the first
workflow small while providing the same JSON-capable service operations.

macOS native builds can explicitly migrate the owner signing key to Keychain
through a small Security/CoreFoundation cgo boundary. It adds no external Go
module. Other builds retain private-file custody and reject that migration.
No silent downgrade from a selected native store is allowed. Scope keys remain
private local profile files. Native APIs can require OS interaction; credentials
are not passed through subprocess arguments or environment variables.

## Durability and authority

The private joining identity is durable before submission. The owner's prepared
decision reserves writes to that admin profile. Approval requires complete
current replay across all scopes and the same relay archive cursor at acceptance,
so a stale local view cannot disclose keys for newly protected history. The
owner's existing private-ancestry/aggregation checks run before key disclosure.

The relay's small CAS-updated R2 index and immutable decision object persist
preparation. Membership commits through the existing head-reachable archive.
Delivery always rechecks exact current membership and key epochs. A single-use
invitation admits a request, not archive authentication. Denial is terminal;
already prepared publication is reconciled before revocation. Explicit
cancellation can publish a minimal generation fence or exact-target revocation
even after inbox receipt retention, without deleting local uncertain state. Duplicate/lost
responses reuse exact signed/encrypted bytes and stable operation identities.

The owner-signed invitation pins the joiner's relay URL and owner/vault identity.
The relay validates that signed URL but does not equate it to the incoming Host
header: aliases/proxies for the same pinned cryptographic vault are supported.
New HTTP authentication binds method, path/query, vault, actor, time, nonce and
body digest in a separate domain. It cannot authenticate a different vault or
owner. The legacy transport is unchanged.

## Limits and alternatives

This replaces private child-profile handoff for ordinary onboarding, not owner
recovery or authority transfer. Initially use one primary owner controller;
ordered-generation conflicts fail instead of silently merging authority. No
anonymous public discovery inbox, LAN scan, automatic trust, cloud private keys,
automatic rekey, universal keystore, or human-approval cryptographic proof is
introduced. Previously disclosed data cannot be recalled.

See the [operator guide](../device-enrollment.md) for rollout, limits, known
verification boundaries and failure recovery. Tests use synthetic profiles and a
local Worker; live deployment and owner Keychain migration are separate actions.
