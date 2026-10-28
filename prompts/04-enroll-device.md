# Join another machine

Use this for a new ordinary device in an existing vault. Owner recovery or
transferring primary owner authority is a separate operation.

~~~text
Enroll this trusted machine into my existing Capsule Memory vault.

Read AGENTS.md, prompts/README.md, docs/install.md, docs/device-enrollment.md
and the matching enrollment protocol. Build/install v0.1.0-beta.5 or the selected
later compatible release on this machine before joining. The normal macOS/Linux
target is /usr/local/bin/capsule-memory, verified on PATH and recorded by exact
revision; use normal-user execution and approved privileges only for installing
the executable. Respect host restrictions, including OVH's workload policy.
Verify the client and relay match; beta.4 lacks these pairing endpoints.

1. Preserve the existing vault/owner identity. Never run init on the new device,
   clone another active device identity, or give this machine owner.json.
2. On the existing owner machine, use the devices console or device invite with
   explicit collection IDs. Transfer only the short-lived private invitation
   through an authenticated encrypted channel; never put it or keys in chat,
   Git, diagnostics or command arguments. If owner interaction is unavailable,
   prepare safely and report enrollment pending without granting access.
3. On this machine, run device join --invite-file PRIVATE_FILE in an unused
   private home. Keep its generated state and compare its complete fingerprint
   with the owner's approval screen. Select only authorized collections, knowing
   they grant read and write, including existing records under their keys.
   Use the installed absolute executable path and --home PRIVATE_DEVICE_HOME;
   do not leave the CLI usable only from a source checkout.
4. Resume the exact saved request after interruption; do not reset or regenerate
   an identity to escape an uncertain outcome. Distinguish pending, published
   membership and confirmed activation. Never bypass a denial or revoked grant.
5. Once active, sync granted collections and use synthetic round-trip checks.
   Configure scoped MCP/background sync only as authorized. Keep capture and
   inference off. Skill text stays staged for local review, not execution.

For older releases, stop and identify the capability gap. The legacy
owner-generated child-profile transfer in docs/go-client.md remains available
only as an explicitly chosen alternative, not a silent downgrade.

Return installed revision, public device identity, granted scopes, activation
and replay results, and exact blockers/NOT RUN checks. Expose no secrets.
~~~
