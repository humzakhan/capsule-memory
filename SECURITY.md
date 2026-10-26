# Security policy

agent-memory is experimental and has no runtime-verified or security-audited release. Focused source inspection has occurred; dependency resolution, builds, typechecks, tests, E2E, and deployment security remain unverified. Use synthetic data while establishing acceptance for your intended environment.

## Report a vulnerability privately

If GitHub private vulnerability reporting is enabled for this repository, use **Security → Report a vulnerability**. Its availability has not been confirmed. Otherwise, contact the repository owner, Humza Khan, through an established private channel. No dedicated reporting email or response SLA is published.

Include the affected commit/file, expected and observed behavior, impact, and a minimal synthetic reproduction. Do not include real memories, keys, credentials, device profiles, or exploitable private details in a public issue or pull request. If you have no established private channel and the reporting button is absent, request a private contact route without disclosing vulnerability details.

For exposed credentials, revoke them through the provider and coordinate recovery privately. Deleting a file or rewriting Git history does not revoke a secret or remove copies already obtained.

## Trust boundaries

- Content keys, plaintext search, and decrypted content belong on trusted devices. Local SQLite/FTS data is plaintext; private keys currently use restricted files, not native keystores.
- The relay receives encrypted records plus authorization and traffic metadata. Provider omission, freshness after empty-cache recovery, and account loss are known limits.
- Retrieved memories and skills are untrusted text. Approved activation is local Markdown/read-as-text only; executable installation and synced approvals are deferred.
- Tombstones preserve history and do not provide physical erasure. Device compromise and loss of all recovery keys are outside the encryption guarantee.

Read the [threat model](docs/threat-model.md), [v1 contract](docs/v1-contract.md), and [delivery status](docs/v1-status.md) for exact assumptions and remaining evidence.

## Release acceptance

Cryptographic framing/interoperability, enrollment and recovery, revocation, hostile inputs, update integrity, and skill trust need explicit review and the relevant [verification evidence](docs/verification.md). A documentation review, source inspection, or passing happy-path demo alone is not a security audit. Automated rotation, native keystores, and executable skill installation remain future work.
