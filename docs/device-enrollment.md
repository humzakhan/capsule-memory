# Approve a new device from your terminal

Available in **v0.1.0-beta.5**, this extends the existing Go CLI on Linux/macOS and the Unix
platforms with supported exclusive profile locks. Windows retains its existing
direct legacy mode. Build the client and upgrade
your existing Worker from the **same revision** before using it. An older relay,
including the beta.4 deployment, does not provide these enrollment endpoints.
Keep the existing vault ID, owner public key, Durable Object and private R2
binding. Do not create a replacement vault or change its signing identity.

Start with [machine-wide CLI installation](install.md). If your MacBook will be
the primary approver for OVH and the Mac Mini, use the
[MacBook controller prompt](../prompts/12-macbook-controller.md). Installing the
CLI does not transfer owner authority or enroll the computer.

The owner runs `capsule-memory devices`. A new computer submits a pairing request;
the owner compares its fingerprint, chooses collections, and approves or denies
it. Private device keys are generated on the new computer and stay there.
The invitation permits one request; it grants no memory access by itself.

## Quick start

Use an existing private **owner/admin profile**, separate from daemon-owned
ordinary device profiles. Examples use `/private/owner`; substitute your own
path. A profile in use is refused rather than silently routed through MCP.

1. **On the owner computer**, open the terminal console:

   ```sh
   capsule-memory --home /private/owner devices
   ```

   Choose **Invite**, select collection numbers or IDs, and securely transfer the
   resulting private invitation file to the new computer. Its default lifetime
   is ten minutes. Treat it as a secret: no Git, chat, public link or unencrypted
   email. The console never prints its bearer signature or any content keys.

2. **On the new computer**, use an unused private profile directory:

   ```sh
   capsule-memory --home /private/new-device device join --invite-file /private/invitation.json
   ```

   The command saves a fresh signing identity and a separate encryption identity
   before network submission. It displays a fingerprint and waits when run in a
   terminal. Keep the command open while approving. Do **not** run `init`, copy
   another active device profile, or transfer `owner.json` to this computer.

3. **Back in the owner console**, refresh, choose **Approve**, and enter the
   complete fingerprint shown on the new computer. Choose the collections and
   confirm. Collections grant **read and write**, including existing records
   encrypted with their keys. No selection is automatically expanded to all
   collections. Existing aggregation/private-ancestry grant restrictions apply.

4. The new computer receives an encrypted access package, verifies its owner
   signature and exact request/collection binding, commits its profile and
   acknowledges activation using a random challenge that only package decryption
   reveals. A signing key alone cannot falsely acknowledge activation. Then
   synchronize each granted collection:

   ```sh
   capsule-memory --home /private/new-device sync --scope GRANTED_SCOPE_ID
   ```

   Activation is not complete archive replay. Configure harness MCP clients and
   optional background publication separately through the existing setup guides.
   Capture, inference and automatic acceptance stay disabled. Synced skill text
   still requires local approval; pairing does not install or execute skills.

Only invited devices can enter this inbox. This is explicit internet pairing
through your relay, not automatic discovery of every computer on your LAN.

## Commands and automation

The interactive console accepts displayed numbers or exact IDs. Its normal
commands use exact IDs and return JSON without invitation signatures, private
profiles or encrypted key packages:

```sh
capsule-memory --home /private/owner devices --json
capsule-memory --home /private/owner device requests
capsule-memory --home /private/owner device invite --scopes SCOPE_ID --out /private/invitation.json --expires 10m
capsule-memory --home /private/owner device approve --id REQUEST_ID --fingerprint FULL_DEVICE_FINGERPRINT --scopes SCOPE_ID
capsule-memory --home /private/owner device deny --id REQUEST_ID
capsule-memory --home /private/owner device resume
capsule-memory --home /private/owner device cancel --confirm
capsule-memory --home /private/owner device revoke --id DEVICE_ID --confirm
capsule-memory --home /private/new-device device join --once
```

`join --once` performs one exchange. Nonterminal invocations also perform one
exchange instead of silently waiting. Interactive `--wait` defaults to ten
minutes and is capped at one hour. An interrupted join resumes from the same home
without `--invite-file`. Never delete its pending state to retry a lost response.

An invitation permits one immutable request and can last 1 minute to 1 hour. The
inbox retains at most 32 invitations/requests until 24 hours after invitation
expiry. A full inbox refuses new requests rather than discarding an outstanding
one. Expired entries are pruned on subsequent writes; an expired invitation cannot
recreate them. Immutable encrypted delivery objects may remain in R2 history;
expiry is not physical deletion. The key package has a 1 MiB plaintext bound;
the transport body is bounded at 2 MiB. Excessive control history fails explicitly.

## Local owner credentials

Enrollment administration uses the existing owner's signing key, not a Cloudflare
OAuth session or API token. Ordinary devices receive only their own signing key
and their approved collection keys. No owner signing key goes to the relay or a
joining computer.

On macOS, a native cgo build can explicitly protect an existing owner signing key
with Keychain:

```sh
capsule-memory --home /private/owner owner protect
```

This uses the Security/CoreFoundation APIs directly, not a shell command carrying
the secret. It writes a nonsynchronizing Keychain item, verifies a readback,
durably writes `owner-key.json`, then removes the old `owner.json`. A crash leaving
both files is resumable with the same command. A selected but locked/unavailable
Keychain fails closed; it never silently falls back to the old private file.
OS Keychain access may prompt depending on its lock state and the binary's access
permissions; a prompt or Touch ID for every action is **not** guaranteed.

The owner profile's collection keys, local database and ordinary device identity
remain private files. Keychain protection here covers the **owner signing key**,
not the whole profile, disk or SQLite. Standard OS disk protection still matters.
There is no hardware-backed/nonexportable Ed25519 claim. Builds without macOS cgo
retain the existing private-file owner mode and explicitly reject `owner protect`.

Use one primary owner controller initially. Moving that authority to a laptop is
a separate, deliberate transfer of your existing recovery authority, current
collection keys and a consistent profile snapshot over an authenticated encrypted
channel. Do not copy a live SQLite file or run two active clones. A Keychain
reference file alone cannot transfer its secret. Verify the vault/owner identity
and recovery custody before moving authority; never initialize another owner as
an enrollment substitute. Ordinary MCP clients should use separate non-owner
profiles and narrowly scoped local grants.

## Interrupted operations and revocation

Approval first catches up every current collection at one archive head, checks
existing reader restrictions, and persists the exact owner-signed decision and
encrypted bytes. Pending approval blocks profile writes, owner mutations, replay
and rebuild until its outcome is reconciled. No write transaction is held during
network I/O. Other ordinary device profiles can keep working.

The relay durably records the prepared decision before publishing membership
through the existing immutable journal/head-CAS protocol. It releases encrypted
keys only after that membership is confirmed in committed history and the current
device grant/epochs still match. The interface distinguishes prepared, published,
and activated; an owner receipt does not establish that the new device is ready.

- **Lost approval reply:** run `device resume`. It submits the exact saved
  decision/ciphertext; it does not generate another identity or re-encrypt keys.
- **Stale review:** refresh/replay and review again. If a local intent already
  exists, reconcile it with `resume`, deny the request, or explicitly cancel before making
  other changes. Never delete the reservation from SQLite to unblock writes.
- **Cancel pending:** `device cancel --confirm` fences the old grant generation
  with a no-op successor or revokes the exact device if already granted. It
  works even after the inbox receipt expires. Exact cancellation bytes are
  persisted; `device resume` continues an interrupted cancellation. The primary
  console marks the request cancelled and refuses to reapprove it. It attempts
  a terminal relay denial when possible; `request_closed:false` means the
  membership is safely cancelled but inbox notification is unconfirmed or a
  prepared receipt remains. Use a fresh invitation for future onboarding.
- **Deny:** terminal for that request. If membership publication is already
  prepared, denial is refused until its outcome is resolved; revoke an already
  granted device instead. A denied/expired request needs a fresh invitation.
- **Revoke:** the owner publishes an ordered membership change. Once committed,
  it blocks future archive requests and delayed key delivery. Old local copies
  and keys remain. Rotation/distribution of new collection-key epochs remains a
  separate manual operation; this feature does not promise future secrecy from
  a stolen old key if new ciphertext is otherwise obtained.
- **Relay unavailable:** preserve local state and retry the same operation. A
  timeout is not denial, successful publication, or proof that nothing happened.

The relay can withhold or replay valid older state; this feature adds no global
freshness witness or protection from a compromised owner/device. A fingerprint
comparison and a signed membership establish identity/authorization, not proof
that a human approved content. See the [threat model](threat-model.md).

## Verification

`go test ./internal/enrollment ./internal/memory ./cmd/capsule-memory` covers
recipient/context binding, altered ciphertext, scope bounds, custody migration
failures and restart fencing. `node e2e/run.mjs --cli /ABSOLUTE/BINARY` runs the
real CLI and local Worker, including approval, exact-byte lost-response recovery,
denial, selected-scope sync and revocation. All data is synthetic.

The implementation run recorded local-emulator results, not live Cloudflare
durability, billing, a physical second laptop, or OS Keychain runtime acceptance.
The custody test substitutes a failing native-store boundary; the macOS native
adapter is compiled but does not access personal Keychain items during tests.

See [the enrollment contract](device-enrollment-protocol.md) and
[decision 0005](decisions/0005-device-enrollment.md) for the exact boundaries.
