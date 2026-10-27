# Draft contracts

Session repair recipes are hand-authored source-review scenarios, not executable
tests or computed crypto vectors. The derived fixture input fingerprint is an
explicit shape placeholder; owner-proof recipes contain no fabricated signatures.
The schema and Go source remain unvalidated. See
[session protocol](../docs/session-protocol.md) for the final owner-proof,
generation, influence, consent and indexed-storage amendments.

These files describe the **decrypted logical record model**, using synthetic content. They are planning artifacts, not a final wire protocol, generated client library, or approved cryptographic specification.

The current [v1 implementation contract](../docs/v1-contract.md) supports memory and skill text records. Summary examples here describe the wider target and are not accepted by the v1 Go service. Exact signed framing is defined in that contract; runtime interoperability remains unverified. The fixed legacy v1 domains remain unchanged by the Capsule Memory rename; see [rename compatibility](../docs/renaming.md).

## Included

- [Record revision schema](schemas/record-revision.schema.json): draft metadata, operations, and summary provenance.
- [Base revision](fixtures/memory-base.json), [offline edit A](fixtures/memory-offline-a.json), and [offline edit B](fixtures/memory-offline-b.json): concurrent heads from one parent.
- [Concurrent deletion](fixtures/memory-delete.json): hides the stale edits from normal recall while preserving them.
- [Summary](fixtures/summary.json): cites both edit revisions without silently choosing one.

The fixture IDs identify synthetic objects, not real hashes or signatures. Fixtures have been authored for review; no schema validator or runtime scenario has been run during setup.

## Expected interpretation

Apply the base and the two edits in either edit order: the logical record has two concurrent heads. The later display timestamp must not choose a winner. Apply the deletion in any arrival order: normal recall hides the memory, but retains the edits and deletion history.

The summary can be current relative to the two edits before deletion. Once the source deletion is learned, it must be invalidated and excluded from normal recall until safely regenerated. Source references must remain inspectable under the appropriate permissions.

Schema validity cannot prove parent existence, same-record ancestry, authorization, operation deduplication, valid restores, or summary freshness. Those are application semantics defined in [protocol](../docs/protocol.md) and [verification](../docs/verification.md). The schema is intentionally not an API endpoint contract.

## Required before interoperable implementation

Define exact authenticated envelope fields, domain-separated signing bytes, key wrapping, nonce rules, hashes, limits, version transitions, and canonical decimal handling. Add real shared Go/TypeScript positive and negative vectors from synthetic keys.

Do not treat placeholder ciphertext as a real vector, infer canonical JSON from these examples, or upload decrypted records using this schema. Changes to the logical model must update the design, schema, and examples together.

## Optional schema-v2 source contract

The [session protocol](../docs/session-protocol.md) is the source specification
for the optional extension. The revision schema now separates legacy memory/skill
schema 1 from the eleven schema-2 body kinds. The old `summary.json` fixture is a
preserved legacy planning example; the Go service accepts summaries only as v2.

[Session body schemas](schemas/session-records.schema.json), `session-*.json`
fixtures and [frame recipes](vectors/frame-v1.json) are authored review documents,
**not executed or validated evidence**. Fixture wrappers carry notices and
expectations; their `revisions` entries are the logical objects. Grant payloads
are shown before base64, with explicitly invalid placeholder signatures. Content
checksums in fixtures were authored with Python stdlib; they do not validate Go
serialization, signatures or interoperability. Frame positives describe byte
framing only; a minimally shaped JSON frame still fails complete revision checks.

No AES-GCM/Ed25519 interoperability vectors were generated. Before acceptance,
exercise all frame mutation recipes, duplicate/null/casefolded JSON properties,
size limits after JSON escaping, private migration/recovery, revision/head races,
leased-job cancellation, outbox holds and source invalidation in an authorized
isolated job. Schema validity cannot prove those semantics.
