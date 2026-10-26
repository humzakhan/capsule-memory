# Threat model

**Decision:** only enrolled trusted devices hold content-decryption keys or plaintext search indexes. **Status:** intended controls, not an implemented or audited security system.

## Assets and trust

Protect memories, summaries, skill source and executable artifacts, provenance, search terms, private keys, and recovery material. Integrity and availability matter as well as confidentiality: an intact ciphertext blob is insufficient if its publication or deletion state is lost.

Trust the owner's enrollment/recovery authority and each enrolled device within its granted scopes. Trust the local OS to isolate the user process and keystore. Treat harness input, model output, imported Git artifacts, relay responses, archive bytes, and skill instructions as untrusted input until checked.

Cloudflare and GitHub are not trusted with content plaintext. They still observe account/network identity and some metadata: device public keys, opaque scope relationships, object sizes, timing, publication counts, and access patterns. Version one does not hide that metadata with padding, private information retrieval, or traffic cover.

## Identity, enrollment, and recovery

The first device creates an owner recovery authority and separate device signing and encryption identities using reviewed libraries. Back up the owner recovery material independently of the encrypted archive, with an owner-understood offline recovery procedure. Losing every device and the recovery material makes the archive unrecoverable by design.

Enrollment uses a short-lived invitation plus an authenticated comparison or transfer of the new device's public-key fingerprint. The owner signs a membership grant naming device identity, permitted opaque scopes, control version, and epoch. A bearer invitation alone must not authorize a substituted key.

Use separate per-scope content-key epochs and authenticated device-specific wrapping. The relay stores only public verification material and opaque wrapped key packages. Exact algorithms, wrapping format, fingerprint UX, and recovery format remain pre-implementation review items.

The Worker checks device proof and current grants, not merely a copied device ID. Use short-lived relay credentials scoped to a device; keep object-storage credentials server-side. Local IPC restricts the OS user and validates each harness grant. A project/harness identity cannot invent another scope in a tool argument.

## Revocation

An owner-signed revocation is an ordered control operation. Stop accepting that device's publication and archive requests at the control barrier, rotate affected content-key epochs, and distribute future keys only to remaining authorized devices. Advance and verify control state before publishing queued work after reconnection.

An authorized remaining device may re-encrypt queued drafts under the new epoch without changing their logical operation identity. A revoked device cannot. Preserve rejected drafts locally for owner-directed recovery rather than pretending they synchronized.

Revocation cannot erase plaintext, old ciphertext, or keys already copied by a device. Re-encrypting retained history may reduce future server-mediated access with old keys but cannot revoke an adversary's existing copies. Do not promise retrospective secrecy after device compromise.

## Content and executable skills

An authenticated memory is still data, not an instruction with higher authority. Recall results include source/trust labels; adapters must not automatically execute snippets, follow embedded URLs, or turn recalled text into shell commands.

Incoming skill artifacts remain staged until an owner-authorized signed release approves the exact digest, scope, compatible runtime, and declared capabilities. Signatures establish origin, not harmless behavior. Review diffs locally, reject path traversal and unsafe extraction, and do not execute imported installation scripts during synchronization.

Keep skill acquisition, approval, and activation separate. Retain a known approved release when an unresolved update arrives. A revocation or explicit disable may require removing an active release; do not silently fall back to an unapproved older artifact.

## Plaintext handling

Local SQLite, FTS tables, payload caches, temporary files, crash dumps, and swap may contain plaintext. E2EE of remote objects does not encrypt those files. Require restrictive file permissions, OS disk encryption where available, and controlled backup handling. Application-level database encryption is a separate future decision.

Logs use operation categories, bounded error codes, and opaque identifiers only where necessary. Never log text, search queries, decrypted paths, secrets, or complete envelopes containing sensitive metadata. Telemetry is minimal and opt-in; no central semantic analytics.

Summarization and embedding generation run on a trusted device. Do not send content to a cloud model by default. Recalled text passed to a cloud-hosted agent still leaves the device through that agent's inference path; storage encryption cannot prevent that. Strict device-only plaintext requires local inference or an explicit owner-authorized exception for that content.

## Hostile infrastructure and failure

Authenticate ciphertext, signatures, and vault/scope/epoch context; reject cross-scope substitution and malformed or oversized content. Persist exact retry bytes to avoid nonce/identity mistakes. Select primitives and framing only after protocol review; do not invent cryptography.

A malicious sole provider can withhold or replay old valid state. Devices can compare remembered heads and exchange known checkpoints to detect some inconsistencies, but a fresh device lacks an independent freshness witness. Availability, global fork prevention, traffic secrecy, and protection from a compromised enrolled endpoint are outside version one's guarantees.

Keep local offline work durable and maintain a separately authorized encrypted backup strategy. An archive in one account is not an independent backup against account loss. This repository does not authorize backup services or storage spending.

## Optional session-history extension (source only)

Session frames redact before compression and encryption, use a fresh DEFLATE
writer without dictionaries for each revision, and pad to 4 KiB. A frame never
mixes scopes or sessions. Compressed length still leaks a residual ratio signal
when attacker-influenced text shares a chunk with sensitive text. Padding does
not eliminate this signal. Default tool metadata filtering and redaction reduce
exposure; plaintext bodies, FTS and migration snapshots remain private local
assets. None of these controls has runtime or side-channel acceptance evidence.

The optional T3 adapter's owner-configured token is broad: upstream
`orchestration:read` can also read server-visible files. Capsule's history endpoint
allowlist does not narrow the token itself. The owner supplies a future token file;
the implementation must never mint or discover credentials, copy harness tokens,
or expose token contents through status. Claude/Codex disk formats are version
pinned and untrusted; unknown shapes fail closed.

Projected assertions are v2 knowledge with lifecycle/grant data in the same target
scope. Legacy memory readers cannot expose them. Historical visibility does not
prove current source state; strict visibility needs a live bounded source replay
and exact dependency-head check for that request. Cross-vault deletion is not
atomic, and previously disclosed text cannot be recalled. Consent withdrawal
holds unsent outbox revisions durably; a relay request already in flight may
already have published before withdrawal.

The G5 independent assertion requires an owner signature over its exact revision,
operation, ordered ancestry, author and proof-free body digest. Reviewer equals
the envelope author; ordinary review remains writer-attested and provides no
cryptographic human-review authority. Invalid signed records block replay/rebuild;
revocation does not remove poison already journaled. Owner-key rotation remains
out of scope. Cross-vault publication cannot verify source consent without an
open source profile and retains those bytes locally. Already-submitted bytes can
race withdrawal, and previously published data requires explicit deletion.

Derived content is untrusted. A retained independent owner insight explicitly
severs deleted evidence instead of claiming an inaccessible source. Local model
inputs use escaped JSON data under fixed prompts, with no tools; endpoints enforce
loopback dialing, no proxies and no redirects. Review these inference and IPC
boundaries independently before operational use.
