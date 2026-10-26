# Revocation, key loss, and backup assessment

Use this to plan sensitive owner operations; v1 has no turnkey rotation/revocation/backup CLI.

~~~text
Assess the requested Capsule Memory security operation without inventing tooling.

Inputs: CHECKOUT/SOURCE_REVISION, affected public device/vault identity, requested
containment or recovery outcome, known custody of owner/content keys, approved
private communication route, EXECUTION_ROUTE and ALLOWED_ACTIONS.
Never paste keys, profiles, real memories or credentials into this prompt.

Read AGENTS.md, prompts/README.md, SECURITY.md, docs/threat-model.md,
docs/relay-setup.md and docs/v1-contract.md.

1. Determine whether the issue is device loss, endpoint compromise, exposed provider
   credentials, unavailable archive, missing recovery keys, or backup planning.
   Preserve evidence without publishing exploitable details or secret content.
2. Report actual implementation limits: relay revocation checks exist, but the CLI
   exposes device create, not a turnkey revoke/rotate/backup command. Do not fabricate
   flags or hand-edit signed profiles/owner pins to simulate revocation.
3. Prepare a concrete owner-reviewed control/key-distribution plan if needed.
   Revocation blocks future authorized access, not already copied plaintext/keys.
   Automated key rotation and secure remote key distribution are deferred.
   Do not implement or execute a new cryptographic procedure as incidental setup.
4. For backups, distinguish an encrypted archive in one provider account from an
   independent protected backup and device-key recovery. The user must choose custody,
   destination, retention and spending before any copies/services are created.
5. Route suspected vulnerabilities privately under SECURITY.md. Report unsupported
   or unrecoverable situations directly. Never wipe a home, purge archive history,
   rotate credentials or upload private evidence without the necessary authority.

Return containment options, the existing capabilities that can be used safely,
missing tooling and approval boundaries, a recovery/backup plan, and actual NOT RUN
checks. Do not claim a signed-control design is an exercised recovery procedure.
~~~
