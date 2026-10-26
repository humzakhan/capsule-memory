# Run v1 on the owner's machine

**Source artifact; all runtime checks NOT RUN.** This sequence is for your own
trusted Linux/macOS machine with synthetic data. It is not authorization to run
workloads on the shared OVH host or provision Cloudflare resources. No production
deployment is included. Budget roughly 15–30 minutes for the first dependency
download/build and 2–5 minutes for the focused E2E if the authored source works.

## 1. Install and build locally

Use the Go version required by `go.mod`, Node 22+ supported by the pinned Wrangler,
and npm. Clone/check out the implementation branch through your normal Git
workflow, then run these commands from the repository root. Dependency resolution
requires internet access. There is no authored fake lockfile.

```sh
umask 077
mkdir -p "$HOME/.local/bin"
go mod tidy
go build -o "$HOME/.local/bin/agent-memory-v1" ./cmd/agent-memory
cd cloudflare
npm install --save-exact
./node_modules/.bin/wrangler types
cd ..
```

Wrangler is pinned in `cloudflare/package.json`; owner installation creates a
real `package-lock.json` for review. Its first installation does not have frozen
transitive versions. Ask the coordinator to review and track the generated
lockfile before relying on `npm ci`.
`wrangler types` generates runtime declarations; it does not itself typecheck
the source. Runtime API compatibility and TypeScript checking remain unverified.

The client stores plaintext SQLite, FTS, and restricted-file private keys locally.
Use disk encryption and private backups. Do not put any home directory below a
source checkout, shared directory, cloud sync folder, or container image.

## 2. Run the focused local E2E

```sh
node e2e/run.mjs --cli "$HOME/.local/bin/agent-memory-v1"
```

The script initializes disposable synthetic devices, chooses loopback ports,
starts Wrangler with local-only bindings, and stops its processes afterward.
It asserts MCP provenance, offline replay, duplicate/lost-ack recovery,
concurrent edits and tombstones, clean-device archive replay, index rebuild,
DO cache recovery, scope filtering, auth/crypto rejection, staged skills, and
historical-author validation after revocation. See [E2E details](../e2e/README.md).

Success prints one `PASS` per scenario and removes the private temporary state.
Failure exits nonzero and retains its synthetic artifacts for local inspection.
Never describe this authored suite as passing until you have executed it. Even
a pass establishes only local emulator evidence, not production guarantees.

## 3. Initialize a separate manual demo

This optional demo is independent of the E2E's temporary state. Keep the terminal
open so the private paths remain available. The initializer emits only public
deployment values and identity metadata; do not print `device.json` or `owner.json`.

```sh
AM_DEMO="$(mktemp -d "${TMPDIR:-/tmp}/agent-memory-demo.XXXXXX")"
AM_OWNER="$AM_DEMO/owner"
AM_CHILD="$AM_DEMO/child"
AM_BIN="$HOME/.local/bin/agent-memory-v1"
export AM_DEMO AM_OWNER AM_CHILD AM_BIN
"$AM_BIN" --home "$AM_OWNER" init \
  --relay http://127.0.0.1:8787 --allow-loopback-http > "$AM_DEMO/public-init.json"
node --input-type=module -e '
import {readFileSync,writeFileSync} from "node:fs";
const v=JSON.parse(readFileSync(process.env.AM_DEMO+"/public-init.json","utf8"));
writeFileSync("cloudflare/.dev.vars",`VAULT_ID=${v.VAULT_ID}\nOWNER_PUBLIC_KEY_B64=${v.OWNER_PUBLIC_KEY_B64}\nALLOW_LOOPBACK_HTTP=true\n`,{mode:0o600,flag:"wx"});
writeFileSync(process.env.AM_DEMO+"/scope",v.scope_id,{mode:0o600});
'
AM_SCOPE="$(cat "$AM_DEMO/scope")"
export AM_SCOPE
```

The `wx` write intentionally refuses to overwrite an existing `.dev.vars`.
Choose a separate checkout or preserve/review that file before using another
demo. The local values include no private keys; still keep them out of source.

Start Wrangler in a second terminal from the repository root, replacing the
example state path with the printed private demo path from `printf '%s\n' "$AM_DEMO"`
in the first terminal:

```sh
cd cloudflare
./node_modules/.bin/wrangler dev --local --ip 127.0.0.1 --port 8787 \
  --persist-to /absolute/private/agent-memory-demo.XXXXXX/relay-state
```

No Cloudflare login or token is needed for local-only emulation. Do not add
`--remote`, enable remote bindings, or run `wrangler deploy` for this sequence.

## 4. Publish control, capture, enroll, and recall

Back in the first terminal:

```sh
"$AM_BIN" --home "$AM_OWNER" sync --scope "$AM_SCOPE"
"$AM_BIN" --home "$AM_OWNER" store --scope "$AM_SCOPE" \
  --text 'Synthetic demo: orchard meeting notes.'
"$AM_BIN" --home "$AM_OWNER" status
"$AM_BIN" --home "$AM_OWNER" sync --scope "$AM_SCOPE"
"$AM_BIN" --home "$AM_OWNER" device create --out "$AM_CHILD"
"$AM_BIN" --home "$AM_OWNER" sync --scope "$AM_SCOPE"
"$AM_BIN" --home "$AM_CHILD" sync --scope "$AM_SCOPE"
"$AM_BIN" --home "$AM_CHILD" recall --scope "$AM_SCOPE" --query orchard
```

First owner sync publishes generation `1`; enrolling a child prepares generation
`2`, which the owner must publish before child sync. On `HTTP 503` during a cache
rebuild, wait two seconds and retry the same `sync`, up to a bounded retry budget.
Local writes remain queued on failure. Do not regenerate ciphertext to retry.

The child profile has independent device identity and granted scope keys. It
has no owner authority. Transferring that profile to a real second machine needs
an authenticated private channel and protected local storage, not Git or R2.

Configure a compatible MCP client to execute this absolute command with these
fixed arguments. Keep diagnostics separate from stdio JSON-RPC:

```sh
"$HOME/.local/bin/agent-memory-v1" --home /absolute/private/device-home \
  mcp --scope YOUR_OPAQUE_SCOPE_ID
```

Tools include `memory_store`, `memory_recall`, `memory_get`, `memory_delete`,
`sync_status`, `skill_list`, and `skill_fetch`. Sync remains an explicit CLI
operation. A tool argument cannot grant another scope. Fetched skills remain
staged text; executable activation is disabled. Owner `skill approve` records
local exact-revision text review, not a deployed skill release.

## 5. Recovery and cleanup

`rebuild` repairs the local derived search index and preserves the outbox:

```sh
"$AM_BIN" --home "$AM_OWNER" rebuild
"$AM_BIN" --home "$AM_OWNER" status
```

For a clean authorized replica, stop all CLI/MCP processes using the source
profile. Create a private empty directory and copy only its `device.json` with
mode `0600`; keep the original profile intact. Run `sync --scope <id>` in the new
home. Its empty database replays all retained authorized history, including
controls/tombstones. This uses an existing enrolled device identity; do not use
both copies concurrently as separate devices. Enroll an independent device for
normal two-machine operation. Losing every device key and owner recovery copy
is unrecoverable. The v1 CLI has no unattended recovery/key-distribution service.

Stop Wrangler with Ctrl-C and close any MCP processes before cleanup. The
following removes only the synthetic demo just created and its local public
binding file; inspect both paths first. It does not touch cloud resources:

```sh
printf '%s\n' "$AM_DEMO"
cat cloudflare/.dev.vars
# After confirming this is the synthetic demo's directory and public config:
rm cloudflare/.dev.vars
rm -r -- "$AM_DEMO"
unset AM_DEMO AM_OWNER AM_CHILD AM_BIN AM_SCOPE
```

If E2E retained a separate `agent-memory-e2e-*` directory, inspect its printed
path and remove that directory manually after stopping any remaining processes.
Do not delete an actual device home or upload retained artifacts. Dependency
directories and the demo binary can be removed separately when no longer needed.

Read [relay setup](relay-setup.md) for exact auth/storage limits and provider
assumptions. Production deployment, provider credentials, spending controls,
independent backups, and security acceptance require separate owner decisions.
