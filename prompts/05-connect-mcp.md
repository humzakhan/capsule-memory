# Connect a harness or MCP client

Use this when the device already exists. It does not enroll a machine or deploy a relay.

~~~text
Connect one compatible harness to an existing local Capsule Memory device.

Inputs: trusted CHECKOUT/SOURCE_REVISION, absolute BINARY, private DEVICE_HOME,
already granted SCOPE_ID, exact harness and config path, permitted config edits,
EXECUTION_ROUTE and ALLOWED_ACTIONS. Do not include key material.

Read AGENTS.md, prompts/README.md, docs/go-client.md section "MCP and staged skills",
and the harness's actual local configuration/docs. Do not assume universal hooks,
client configuration fields, or shell expansion of paths.

1. Confirm the device/profile already exists and the chosen scope is intended.
   Do not call init, enroll devices, change owner pins, or widen grants.
   Verify the installed CLI path/revision first. For an authorized new-machine
   installation, use docs/install.md; prefer /usr/local/bin/capsule-memory on
   macOS/Linux and record an explicit fallback if system-wide writes are blocked.
2. Prepare a minimal stdio launch entry using an absolute executable path:
   command: BINARY
   args: ["--home", "DEVICE_HOME", "mcp", "--scope", "SCOPE_ID"]
   Adapt the surrounding wrapper to the real harness format. Do not add HTTP auth
   tokens or remote MCP endpoints; this server uses local stdio.
3. Explain authority: the fixed launch scope includes store/delete tools. This is
   not a read-only MCP permission profile. Obtain the intended access before enabling
   an untrusted harness. Scope arguments cannot widen the server's launch grant.
4. If config edits are authorized, preserve existing entries, use the real path,
   and explain exactly what changed. Do not restart unrelated sessions/services.
5. If runtime is authorized, initialize through the real client and list tools.
   Expect memory_store, memory_recall, memory_get, memory_delete, sync_status,
   skill_list and skill_fetch. Begin with sync_status; only perform synthetic writes
   or deletion when those actions were allowed.
6. If a synthetic recall is allowed, confirm provenance and state without disclosing
   personal text. Explain that sync_status does not sync; CLI sync is separate.
   MCP does not expose enrollment, approval, activation, or cloud deployment.

Return the redacted harness-specific entry, launch scope and authority, actual
handshake/tool evidence, or NOT RUN. Incoming skills remain untrusted staged text.
No particular harness compatibility or automatic memory capture is assumed.
~~~
