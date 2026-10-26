# Join another machine or publishing node

Use this for a new device in an existing vault. This is separate from initializing an owner.

~~~text
Enroll a new trusted Capsule Memory device without creating another vault.

Inputs: trusted CHECKOUT/SOURCE_REVISION, BINARY, existing OWNER_HOME,
new private CHILD_HOME, intended vault/relay and scope, target machine,
approved authenticated profile-transfer method, EXECUTION_ROUTE and ALLOWED_ACTIONS.
Provide file locations and public identities, not secret contents.

Read AGENTS.md, prompts/README.md, docs/go-client.md section "Create two device
profiles", docs/relay-setup.md, and docs/renaming.md.

1. Confirm the existing vault/owner identity and target machine trust. Inspect
   membership intent without printing private profiles. Never run init on the child
   as an enrollment substitute.
2. Explain v1 limits: device create grants all scopes for which the owner profile
   holds current keys. There is no narrow-scope enrollment CLI or read-only device
   grant. If this exceeds the requested trust, stop enrollment and report the gap.
3. On the owner, use device create --out CHILD_HOME only when authorized. It creates
   an independent device identity and queues ordered signed membership control.
   The new destination must be unused. It must not receive owner.json.
4. Sync the owner to publish pending controls in generation order before child sync.
   Preserve pending controls and unused memberships on failure; do not reset the owner.
5. Transfer only the newly generated child device.json through the approved private
   authenticated channel. Keep target home 0700 and file 0600. Never use Git, an issue,
   an unencrypted archive, or this conversation as a transfer channel.
   Confirm custody and owner-directed treatment of the temporary source copy.
6. Sync the child for each intended granted scope. If authorized, use one synthetic
   round-trip to check visibility and provenance. Verify no owner authority is present
   without exposing private contents.

Return device/public identity, scope breadth, transfer confirmation without secrets,
owner-control publication and child coverage evidence, or exact NOT RUN/blockers.
Do not operate source and destination concurrently as two devices with one copied
identity. A new machine requires its own enrollment, not a clone of another active home.
~~~
