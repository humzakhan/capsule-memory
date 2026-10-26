# Security

The initial implementation has received focused source inspection, but compilation, E2E behavior, and deployment security remain unverified. Use synthetic data while establishing runtime acceptance. Executable skill installation is not implemented.

The intended boundary is end-to-end encryption with keys and search indexes on trusted devices. Cloudflare and GitHub must not receive plaintext memories, content keys, or decrypted skill bundles. See the [threat model](docs/threat-model.md) for assumptions and limits.

## Reporting a concern

Report concerns privately to the repository owner through an established private channel. Include the affected file or commit, the failure condition, and a minimal synthetic example. Do not put real memories, key material, credentials, or exploitable private details into an issue or pull request.

If a credential is exposed, revoke it through its provider and coordinate recovery with the owner. Deleting a file or rewriting a branch does not revoke a secret or erase previously copied data.

## Before a release

Cryptographic framing, enrollment and recovery, revocation, hostile input handling, update integrity, and executable skill activation need explicit review and the corresponding [acceptance evidence](docs/verification.md). No security claim follows from a schema, documentation review, or successful happy-path demo alone.
