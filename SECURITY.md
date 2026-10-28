# Security policy

Capsule Memory is an experimental utility for securely storing and synchronizing existing agent memories and skills. Bounded synthetic runtime checks exist; no independent security audit or sensitive-use acceptance is claimed.

Focused source inspection, locked builds, tests and bounded E2E have run; [release evidence](docs/releases.md) identifies the exact source and remaining limits. Use synthetic data while establishing acceptance for your intended environment.

## Report a vulnerability privately

Use [GitHub private vulnerability reporting](https://github.com/humzakhan/capsule-memory/security/advisories/new), also available under **Security → Report a vulnerability**. Reports stay private during coordination. No response SLA is promised.

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
