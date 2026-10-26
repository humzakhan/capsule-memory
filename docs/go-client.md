# Capsule Memory Go client v1

Source is implemented; runtime behavior is **unverified**. No install, Go command,
formatter, build, test, checker, MCP session, or relay request was run for this
implementation. Use an authorized isolated environment or the owner's machine
for every command below. This document does not authorize execution on OVH.

## Dependencies and source decision

The module targets Go 1.24.0. It pins the official MCP SDK v1.4.0 and
`modernc.org/sqlite` v1.39.1, with that driver's exact `modernc.org/libc`
v1.66.10 requirement. No `go.sum` is fabricated: the authorized dependency
resolution step must create and review it before acceptance.

The [official MCP v1.4.0 module](https://github.com/modelcontextprotocol/go-sdk/blob/v1.4.0/go.mod)
requires Go 1.24. Its [stdio example](https://github.com/modelcontextprotocol/go-sdk/blob/v1.4.0/README.md)
and [typed tool API](https://github.com/modelcontextprotocol/go-sdk/blob/v1.4.0/mcp/server.go)
match the adapter's `NewServer`, `AddTool`, and `StdioTransport` calls. The
SDK's license includes MIT and Apache-2.0 terms; review its license on packaging.

SQLite provides durable transactions and FTS5 without an ORM or a separate search
service. The [pinned driver source](https://github.com/modernc-org/sqlite/blob/v1.39.1/sqlite.go)
implements `_pragma` DSN settings on each opened connection. Its
[Linux amd64 generated source](https://github.com/modernc-org/sqlite/blob/v1.39.1/lib/sqlite_linux_amd64.go)
includes `SQLITE_ENABLE_FTS5` and SQLite 3.50.4. The driver is BSD-3-Clause,
uses a CGo-free SQLite translation, and carries transitive maintenance and binary
size costs. The [exact dependency manifest](https://proxy.golang.org/modernc.org/sqlite/@v/v1.39.1.mod)
pins libc v1.66.10. The simpler alternative, a CGO SQLite driver, adds a native
build dependency. Linux/macOS are intended targets; no platform build or static
binary claim has been verified. Dependency vulnerability review remains pending.

Resolve modules and build only in the approved environment:

```sh
go mod tidy
gofmt -w cmd/capsule-memory internal/memory
go build -o ./bin/capsule-memory ./cmd/capsule-memory
go vet ./...
```

These commands are **NOT RUN**. Review dependency changes and generated sums;
minimal source `go.mod` may gain transitive requirements during `go mod tidy`.
The focused black-box journey under `e2e/` owns integration verification; no
parallel unit suite was added.

## Profile paths and rename compatibility

New CLI profiles default to `capsule-memory` under the OS user-config directory. To reuse an existing profile, always pass `--home /previous/private/path`; the CLI does not discover or move pre-rename profiles. See [rename compatibility](renaming.md) for the previous `agent-memory` defaults and fixed v1 protocol labels.

## Create two device profiles

Commands return one JSON value on stdout and errors on stderr with a nonzero
exit code. MCP uses stdout exclusively for SDK JSON-RPC. `--home` works before
or after the command. Flag values must precede any positional arguments; the
CLI does not accept extra positional arguments. Examples use synthetic content.

```sh
./bin/capsule-memory --home /private/device-a init --relay https://relay.example
```

The result contains `vault_id`, `scope_id`, `device_id`, `VAULT_ID`, and
`OWNER_PUBLIC_KEY_B64`, plus `control_pending:true`. These deployment values
are public. Configure the relay using its separate runbook. Never use
`device.json` or `owner.json` as relay configuration.

For an explicitly local development relay only:

```sh
./bin/capsule-memory --home /private/device-a init \
  --relay http://127.0.0.1:8787 --allow-loopback-http
```

HTTP is otherwise rejected. Redirects are rejected, including redirects to
another relay. Device requests sign the exact method, escaped pathname/query,
vault, scope, identity, timestamp, nonce, and body digest from the contract.

Create device B, including before A's initial sync:

```sh
./bin/capsule-memory --home /private/device-a device create --out /private/device-b
./bin/capsule-memory --home /private/device-a sync --scope SCOPE_ID
```

Replace `SCOPE_ID` with the returned opaque ID. Owner-created control generations
are queued durably and published in numerical order, starting at generation 1.
Ambiguous retries reuse the exact owner-signed control bytes. B must wait until
A publishes B's membership. Each generated profile includes signed public
control history and an independently pinned owner public key. B's private
identity and granted scope keys are separate from A's; **B never receives the
owner private key**. Creation currently grants all scopes for which the owner
profile has the current key. Narrow-scope enrollment UI is deferred.

Transfer only B's profile through an authenticated private channel to its trusted
machine. The destination state directory must be mode 0700 and `device.json`
mode 0600. The source profile is also sensitive: remove or retain it according
to the owner's device-transfer procedure. No transfer, upload, or cleanup runs
automatically. Never run two machines concurrently with a copied identity.

A profile creation failure after control is queued can leave an unused membership;
create another profile in a new private directory. Existing files are never
overwritten. Initializing an already initialized directory fails. Interrupted
initialization may leave partial private files; preserve them for owner-directed
inspection rather than deleting or silently resetting the identity.

## Offline writes, search, and conflicts

```sh
./bin/capsule-memory --home /private/device-a store \
  --scope SCOPE_ID --kind memory --text 'Synthetic release notes'
./bin/capsule-memory --home /private/device-a recall --scope SCOPE_ID --query 'release'
./bin/capsule-memory --home /private/device-a get --scope SCOPE_ID --id RECORD_ID --history
./bin/capsule-memory --home /private/device-a status
```

`store` returns `record_id`, `revision_id`, `operation_id`, parents,
`local_durable:true`, and `published:false`. Local success follows one SQLite
commit containing the immutable revision, ancestry, updated FTS5 visibility,
and exact signed ciphertext outbox bytes. `published:false` describes this local
acknowledgement; another process may synchronize immediately afterward.

`recall` accepts SQLite FTS5 query syntax and returns at most 100 visible revision
hits with author/provenance, revision identity, conflict state, and coverage.
Queries are bound SQL arguments. No cloud search or summarizer exists. Skill
results carry staged status derived from current local approval/activation.
The index contains every visible concurrent head.

Updating an existing `--id` without `--parents` names all locally known heads.
With multiple parents the operation is an explicit `resolve` revision. To create
an edit from a particular ancestor, pass `--parents REVISION_ID`; comma-separated
parents are supported. Concurrent offline edits retain both heads. Timestamps
are display provenance and never decide a winner. Independent concurrent root
writes to the same record remain separate heads. Operation/revision identity
reuse with different logical content stops replay.

```sh
./bin/capsule-memory --home /private/device-a delete --scope SCOPE_ID --id RECORD_ID
./bin/capsule-memory --home /private/device-a restore --scope SCOPE_ID --id RECORD_ID
```

A deletion hides the entire record from recall, including concurrent stale edits.
Normal `get` retains head/revision, deletion, and conflict metadata but omits
every head's `payload` while the record is deleted. History stays inspectable
through `get --history`, which includes preserved plaintext. Ordinary edits cannot undo a
tombstone. Restore names all current local heads and every known unresolved
deletion. If the nearest preserved content differs between branches, restore
fails until the owner supplies reviewed replacement content:

```sh
./bin/capsule-memory --home /private/device-a restore \
  --scope SCOPE_ID --id RECORD_ID --text 'Reviewed restored content'
```

No summary kind is accepted in v1; dependent-summary invalidation is therefore
not claimed. The supported inner payload is text for memory and skill records.

## Manual synchronization and recovery

```sh
./bin/capsule-memory --home /private/device-a sync --scope SCOPE_ID
./bin/capsule-memory --home /private/device-b sync --scope SCOPE_ID
./bin/capsule-memory --home /private/device-b recall --scope SCOPE_ID --query 'release'
./bin/capsule-memory --home /private/device-a rebuild
```

`sync` publishes queued owner controls, refreshes the signed owner manifest,
checks current device/scope/epoch authorization, catches up history, publishes
up to 100 queued records in original local commit order, and replays again.
Historical signatures are checked against the control state **at publication**;
revocation now does not invalidate an author's legitimate old records. Revoked
devices cannot fetch changes or publish. Signature verification uses the exact
base64 payload string; encrypted context includes all v1 AAD fields.

Each scope has its own replay cursor and historical control state. Every page's
validated revisions, derived visibility, publication evidence, control history,
and cursor commit atomically. Malformed/missing parents or authorization failures
leave that page unapplied. Pages already committed remain durable after later
failure. Replay of an existing logical revision is idempotent and can clear an
outbox entry whose upload acknowledgement was lost. Otherwise exact persisted
transport bytes are retried; no nonce or transport ID is regenerated.

One call uses a two-minute context, 30-second HTTP deadlines, one scanned journal
entry per page, and at most 100 pages before and 100 pages after publication.
The JSON result reports `published`, `caught_up`, `pending`, and `coverage`.
If `caught_up:false`, call sync again; pending writes remain queued while initial
catch-up is incomplete. There is no automatic retry loop: a transient relay
503/429 or network failure returns an error and preserves progress. Retry manually
with backoff. Relay cache reconstruction may require repeated calls.

Coverage means completion relative to the **last scanned relay head**; it does
not prove online freshness or detect every malicious-provider omission. New
profiles start with incomplete coverage. Scope filtering advances cursors over
other scopes without receiving their envelopes. Pinned owner generations and
remembered cursor/head values reject observed rollback; an independent freshness
witness and full archive fork proof are deferred.

`rebuild` only rebuilds FTS5 from durable local revisions. It never clears pending
writes, outbox envelopes, receipts, keys, history, or cursors. For clean-device
archive recovery, create a newly enrolled profile (or restore an independently
backed-up profile) into a new private directory and repeatedly sync each granted
scope until coverage catches up. Unsynchronized writes lost with their local DB
cannot be reconstructed from the remote archive.

Do not copy a live SQLite main file alone: WAL may contain committed writes.
Use a consistent SQLite backup process or quiesce every process and preserve the
complete device state. No backup/restore CLI or native keystore is implemented.
SQLite, FTS5, and local cached revision bodies are plaintext; disk encryption,
private backups, and trusted local inference remain owner responsibilities.

## MCP and staged skills

A harness launches:

```sh
./bin/capsule-memory --home /private/device-b mcp --scope SCOPE_ID
```

The official Go SDK serves stdio tools: `memory_store`, `memory_recall`,
`memory_get`, `memory_delete`, `sync_status`, `skill_list`, and `skill_fetch`.
Inputs use `text`, optional `kind`, optional `id`, and optional `parents` for
store; `query` for recall; `id` and optional `history` for get/fetch; and `id` for
delete. Status/list take an empty object. Results are JSON text content. All calls
use the same domain methods as the CLI. Tool arguments cannot widen the launch
scope. No MCP sync, owner enrollment, approval, or activation tool is exposed.
A small official-SDK client example is under `examples/go-client/`.

```sh
./bin/capsule-memory --home /private/device-a store \
  --scope SCOPE_ID --kind skill --text '# Synthetic reviewed skill'
./bin/capsule-memory --home /private/device-a skill list --scope SCOPE_ID
./bin/capsule-memory --home /private/device-a skill fetch --scope SCOPE_ID --id RECORD_ID
./bin/capsule-memory --home /private/device-a skill approve \
  --scope SCOPE_ID --id RECORD_ID --revision REVISION_ID
./bin/capsule-memory --home /private/device-a skill activate \
  --scope SCOPE_ID --id RECORD_ID --revision REVISION_ID
```

Approval requires the owner private key and the unique visible current skill
revision. It signs a **local** exact revision/scope/text digest, runtime
`text/markdown`, and capability `read-as-text`, under the distinct
`agent-memory/skill-approval/v1` prefix. That local review record is not a
cross-device executable release and is not uploaded. Approval returns
`approved:true, staged:true, active:false`.

Activation verifies the stored exact owner signature and selects only the unique
visible current skill revision for local text consumption. It requires no owner
private key once the approval exists. The signature frames the exact base64
payload as `agent-memory/skill-approval/v1` + LF + payload + LF. The signed JSON
requires exactly `version`, `vault_id`, `scope_id`, `record_id`, `revision_id`,
`sha256`, `runtime`, and `capabilities`; duplicate, unknown, missing, null, or
incorrectly cased fields are rejected. Version must be 1; vault, scope, record,
revision, and lowercase SHA-256 of the exact UTF-8 text must match. Runtime must
be exactly `text/markdown` and the capability list exactly `["read-as-text"]`.

`skill activate` returns one JSON object with this shape (key order is unspecified):

```json
{"record_id":"RECORD_ID","revision_id":"REVISION_ID","approved":true,"active":true,"staged":false,"runtime":"text/markdown","capabilities":["read-as-text"]}
```

Approval remains owner-only and always resets that revision to inactive/staged,
including re-approval. Activation commits `approvals.active=1` only after exact
verification in the same SQLite transaction as the current-head check. Missing
or tampered approvals and noncurrent, deleted, conflicting, or wrong-scope records
cannot activate. Failed activation leaves the durable active flag unchanged;
reads independently verify eligibility and never treat that flag as authority.

CLI `skill fetch` and MCP `skill_fetch` return
`{skill:<record-view>,approved:<bool>,active:<bool>,staged:!active}`. Skill record
views from `get`, fetch, and list also contain those three status fields. A new
revision, deletion, or conflict makes the current record unapproved, inactive,
and staged even when a historical approval has `active=1`. Tampering likewise
produces unapproved/inactive/staged status. An exact valid inactive approval is
approved/staged. CLI `skill fetch --history` and MCP `skill_fetch` with
`history:true` explicitly expose preserved history; normal deleted fetch returns
metadata with every head's payload omitted. Non-skill fetch is rejected.

CLI/MCP skill list returns
`{skills:[<record-view>,...],activation_supported:true,runtime:"text/markdown",capabilities:["read-as-text"]}`.
It excludes deleted records and reports per-record verified state. Recall still
filters tombstones and reports skill `staged:!active` for each returned hit.
MCP scope remains fixed at launch, and MCP exposes no approval/activation tool.

Approvals and active selections are **local only**: they are neither uploaded
nor synchronized to another device. Incoming skills without a local signed
approval stay staged. Fetch/list/recall execute no code and install no scripts,
files, or harness registration. Executable releases and trusted installers remain
outside this slice.

## Source limits and acceptance checks

The SQLite driver has one open/idle connection per process, per-connection WAL,
FULL synchronous, foreign-key and five-second busy-timeout settings. SQLite
serializes concurrent direct processes. For the legacy manual path described in
this section, MCP processes share the durable DB through a bounded connection; the optional daemon path below uses exclusive profile ownership.
An agent calling MCP has that process's fixed-scope write/delete authority.
Launch read-only integrations only when such authority is appropriate; there is
a separate capability grant for the optional IPC MCP path described below.

V1 bounds include a 2 MiB signed envelope, 1 MiB UTF-8 text, at most 128 parents,
64 scopes, 256 devices, and 256 bootstrap control generations for profile
creation, with a 2 MiB total profile limit. JSON escaping and envelope overhead
may reduce accepted text size.
Each record's history working set is capped at 4,096 revisions or 32 MiB;
exceeding it fails closed without dropping history. Recall/list are limited to
100 results. Control body and archive bounds may be narrower at the relay.

Manual key rotation/distribution only: each scope profile holds one epoch key.
Unknown/stale epochs and old history requiring an unavailable epoch key fail
closed; automatic rotation, key wrapping, automated recovery, notifications,
polling, background sync, native keystores, summaries, embeddings, Git import,
physical erasure, and destructive history GC are not implemented.

Manual source inspection covered transaction boundaries, signature/AAD framing,
identity checks, generation order, scoped replay state, DAG/tombstone behavior,
MCP scope binding, exact retries, deleted payload suppression, and local signed
text activation. This is not
compiler or runtime evidence. **Dependency resolution, gofmt, compilation,
vet, race checks, tests, cross-language crypto execution, SQLite crash/WAL
behavior, MCP handshakes, relay/cache-reset integration, and real R2 durability
checks are all NOT RUN.** The coordinator owns acceptance, integration,
inspection, and Git publication.

## Optional session commands and daemon clients

The optional module adds authored source for `profile migrate`, `scope add`, scoped
`device create|grant|revoke-scope|import-keys`, `daemon init|run|client add`,
`source add|configure|list|enable|disable|pause|resume|backfill`,
`project map|unmap|exclude|include|suggest|list`, `policy get|set`,
`session search|read|delete|restore|export|import`, `summary search`,
`candidate list|accept|reject`, `derive assign|retry|run`, `promote request|list|withdraw`,
`bridge grant`, `aggregation grant`, and `hint`. Global `--daemon-home` selects the
private daemon directory; `--home` still selects the profile.

Read [daemon setup](daemon.md) before any owner execution. All session runtime,
formatting, compilation and harness compatibility checks are **NOT RUN**. The integrated source includes event-aware correction regeneration and arbitrary
origin labels; independent source review and owner execution remain pending.

Existing legacy flags and JSON results remain the manual interface. V2 direct
sync/rebuild installs every module applier, while daemon-owned legacy commands
route to the same memory/skill services through local authorized IPC. New-client
knowledge is never returned by legacy `get`, `recall`, or skill tools.

For an IPC MCP client, use `mcp --client NAME --token-file /PRIVATE/NAME.token`.
Its grant determines the tool set. `knowledge_search` accepts `mode:historical`
(default) or `mode:strict`; strict calls the live freshness resolver and fails
closed. Historical output includes `current`, `last_verified_at`,
`last_verified_heads`, coverage and untrusted labels. `session_search` and
`session_read` require explicit session.read. Grant status.read so the MCP process
can obtain its own grant and status; admin/hint grants are refused by MCP.

Raw session search uses `--query`; bounded reads use `--session`, `--branch`,
`--from-seq`, and `--max-bytes`. `session export --session ID [--out NEW_FILE]`
writes redacted capsule-transcript/v1 JSONL. IPC export pages remain bounded;
the exported origin and native IDs are preserved on reimport. `session import
--source ID --file /ABS/export.jsonl` uses the source's existing consent and
approved paths; it does not grant capture or implicit historical backfill.

`candidate accept --id ID --revision OBSERVED_REVISION [--text 'reviewed assertion']` preserves explicit human
edits. `session delete --session ID --keep-insight ID1,ID2` keeps selected assertions
only as independent owner-key-authorized statements with a proof for the exact
new revision, no generator and no source evidence. Ordinary evidence-bearing
review is writer-attested; it does not cryptographically prove human review.
Candidate reject also requires --revision. Rejecting an independent assertion
returns an instruction to use separately authorized explicit deletion; edits or
accepts require a fresh owner proof. Other dependent accepted insights are withdrawn. Deletes preserve
immutable history and suppression; they are not physical erasure.

Aggregation policy contains signed grant record IDs, not unsigned scope bundles.
`aggregation grant --name N --sources S1,S2 --destination PERSONAL` calls the
owner-signed grant operation. For bridge grants, `--bridge-device` identifies the
target signing device; source vault/device identity is recorded separately.
The bridge queue's local destination is the source scope; the actual cross-vault
target is authorized from the durable projection. Promotion is off by default.

Session integration details: origin harness labels are bounded namespaces rather
than adapter names. `project exclude/include --path` manages explicit local path
denials. Backfill requires event times and explicit byte/stream bounds. Snapshot
export paging adds `snapshot` to request/response; callers must return the token
unchanged after page one. `session.read` reports `Branch` and `Branches`; concurrent
capture branches remain separate. `knowledge.search` proof results name the exact
target revision, and the returned row must match that revision after the resolver.
