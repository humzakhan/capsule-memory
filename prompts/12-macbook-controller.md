# Set up a MacBook as the primary device controller

Give the following prompt to an agent **on the MacBook**. It installs the CLI,
preserves your existing vault, and prepares the MacBook to approve OVH and the
Mac Mini. It does not transfer secrets through chat or remotely install the
other machines. Existing owner authority needs a private, deliberate handoff.

~~~text
Install and configure Capsule Memory v0.1.0-beta.5 on this MacBook as my primary
device controller, using https://github.com/humzakhan/capsule-memory.

Read AGENTS.md, prompts/README.md, docs/install.md, docs/device-enrollment.md
and docs/releases.md from the release tag. Follow this Mac's execution policy.
Use Codex Daily if this is a T3 session; do not substitute another provider.

1. Inspect the host, existing installation and profiles without displaying
   credentials or memory contents. Use a fresh tagged checkout; preserve other
   work and running services. Build with Go 1.26.10 and native macOS cgo, then
   install /usr/local/bin/capsule-memory through the approved local privilege
   mechanism. Run builds and the CLI as my normal user. Verify a new terminal
   resolves that command, record its exact source revision/checksum, and retain
   rollback material. If privilege or prerequisites are unavailable, finish the
   permitted preparation and report the precise blocker; do not claim installed.
2. Reuse my EXISTING vault, relay URL and owner identity. Never run init as a
   workaround for missing credentials. If owner authority is on the Mac Mini,
   arrange an authenticated encrypted handoff of the existing authority,
   collection keys and a consistent private profile. Quiesce that owner profile
   before taking a snapshot; preserve unsynced state and stop using the old owner
   clone after handoff. Do not copy a live SQLite file, expose keys in chat/logs,
   or expect a Keychain reference alone to transfer its secret. Use the private
   credential channel available on this host; otherwise report owner setup pending.
3. Check the existing Cloudflare relay matches beta.5. An older beta.4 Worker
   lacks pairing endpoints. Prepare the matching existing-Worker upgrade while
   preserving vault/owner pins and DO/R2 bindings; deploy only with the user's
   existing infrastructure authority and private Cloudflare authentication.
   Create no replacement vault/resources, paid capacity or public bucket.
   Complete the local installation even if relay access is pending.
4. Keep the owner/admin profile separate from ordinary assistant profiles.
   Open /usr/local/bin/capsule-memory --home OWNER_HOME devices. Verify its
   public vault identity and available collections. Explain i/a/d/v/u/c/r/q
   for invite/approve/deny/revoke/resume/cancel/refresh/quit. Optional owner
   protect moves only the signing key to Keychain and may prompt locally;
   do it only when requested, without promising per-action Touch ID.
5. Prepare OVH and the Mac Mini as independently enrolled ordinary devices via
   prompts/04-enroll-device.md, installing the same CLI release on each through
   its authorized host route. Send only short-lived invitation files privately.
   Compare each full fingerprint and grant only the collections I explicitly
   choose; these allow read and write, including existing records. Do not copy
   owner credentials or another device identity to those machines. Respect OVH's
   execution restrictions; do not bypass them with a direct build or service.

For this MacBook's assistants, enroll a separate ordinary local profile, configure
only the requested harnesses via prompts/05-connect-mcp.md, and verify scoped
sync and synthetic recall across authorized profiles. Installing the CLI alone
does not configure MCP or turn on background sync. Do not enable conversation
capture, inference or automatic acceptance. Incoming skills require local review.
Do not claim physical cross-machine checks before those machines are enrolled.

Return installed path/revision, public vault/device IDs, private file LOCATIONS
only, relay compatibility, owner-console status, actual smoke-test results,
rollback instructions, and any pending handoff/privilege/authorization steps.
Never put secrets or private profiles in chat, Git or diagnostics.
~~~
