# Optional local daemon

For current scoped execution evidence, see [release readiness](releases.md).
The original source-only statements below describe the authored runbook, not
the later Mac validation. Background operation remains explicitly opt-in.

The session daemon, capture adapters and IPC client are authored source. Compilation,
formatting, adapter compatibility, peer authentication, crash recovery and runtime
acceptance are **NOT RUN**. Do not collect real histories before owner acceptance.
The integrated source includes event-aware correction invalidation; independent
source review and owner execution remain pending. See [session status](session-status.md).

All controls start off. A source registration is disabled; capture, publication,
local derivation, raw-session access and cross-vault promotion are separate choices.
A dedicated private session profile and relay are recommended. Existing memory and
skill profiles retain their manual commands. There is no network MCP listener.

## Owner setup sequence

These are commands for later owner execution, not executed evidence. They do not
install a service or authorize this shared host to run workloads.

1. Back up an existing profile through the documented owner procedure. Stop its
   daemon and direct clients before `capsule-memory --home /PRIVATE/profile profile migrate`.
   Migration takes an exclusive lock and creates a consistent, private SQLite
   snapshot before its v2 transaction. The snapshot contains plaintext; protect it
   like the profile. Old binaries cannot open a migrated DB.
2. For a new dedicated profile, use `init --relay URL --role session`. The role is
   local scope metadata; `device.json` has no added fields. Approve project paths
   with `project map --path /ABS/project --name Project`. The longest approved
   path prefix selects the encrypted project scope; unresolved paths are omitted.
3. Use `daemon init --profiles sessions=/PRIVATE/session,memory=/PRIVATE/memory`
   with absolute paths. This authors a private `daemon.json` and admin token.
   Review the generated configuration before an owner starts `daemon run`.
4. Register a source with `source add --harness claude|codex|t3|import --config JSON`.
   Read its preview, explicitly pin native versions, and approve capture separately
   with `policy set --json '{"capture":true}'`. `source enable --source ID` baselines
   existing streams at their current end. It does not request historical capture.
5. Historical capture requires `source backfill --source ID --since RFC3339
   --backfill-max-bytes N --streams key1,key2`. Use explicit positive byte and stream
   bounds. Publication additionally needs policy `publish:true` and explicit
   `publish_scopes`. Derivation needs `derive:true` and a reviewed loopback model
   configuration. None of these settings installs or starts a model.

The daemon owns each configured profile with an exclusive advisory lock. A second
user daemon cannot remove its socket: it must acquire `daemon.lock` first. A busy
profile reports `profile_busy` and is retried every 30 seconds; other profiles keep
running. Linux and macOS are the daemon targets. Windows reports unsupported and
keeps legacy direct operation. Linux checks the connecting UID with SO_PEERCRED;
macOS relies on the private directory/socket permissions because stdlib Go has no
matching peer-UID API.

## Sources and consent

Claude reads version-pinned JSONL under the approved projects root, including
subagents when enabled. Codex reads pinned rollout files under the sessions root;
an archived root is explicit, and no App Server process is spawned. Neither
adapter opens a harness database, installs a hook, executes an SDK, follows a
spilled tool result, or executes exported content. Unknown versions and malformed
known shapes stop capture with visible quarantine. Partial final lines are held;
a stable tail requires 60 seconds without growth. Rotation and unresolved rewrites
stop the stream rather than overwrite captured evidence.

T3 uses only the documented read-only GET environment, shell, bounded-thread and
history endpoints with protocol 2. An owner supplies a token file explicitly,
with mode 0600 and matching ownership. Capsule never discovers, mints or copies a
T3 credential. The token is **broad**: upstream orchestration:read can also read
server-readable files. Capsule's endpoint allowlist does not narrow a leaked
credential. Responses are bounded; redirects and proxy use are disabled. HTTP
capture and authentication remain unverified. No WebSocket is used.

Imports use [capsule-transcript/v1](../contracts/adapters/capsule-transcript-v1.md).
They preserve the declared source origin, native event IDs and redacted event
metadata. Reimporting a suppressed origin is blocked; a matching harness/session
with another store identity is quarantined as `suppressed_match` until explicit
session restoration. Exports are raw JSONL; `session export --session ID --out FILE`
creates a new private file. Without `--out`, plaintext JSONL goes to stdout.

`source pause --source ID` blocks reads. Resume requires explicit `--resolution
skip|include`; skip moves the baseline and include retains it. `source disable`
withdraws consent and atomically holds every unpublished automatic descendant
of that source, including summaries, insights and source projections. Cross-vault
target publication rechecks source consent before sending; without an open source
profile it retains automatic descendants locally. A manual sync also honors holds. Re-enabling a source does not release
old held revisions. Physical erasure and destructive retention are not implemented.

Legacy schema-1 manual sync keeps its existing behavior. An explicit schema-v2
manual sync may publish while background publication is off, but automatic
cross-vault exports require an open source profile and an exact source-consent
check immediately before POST. A direct target-only sync retains those bytes
when the source/router is unavailable; an error is temporary, confirmed withdrawal
creates a durable hold. Holds are never silently released. Delete, hidden and
withdrawn lifecycle cleanup remains publishable after source loss.
Independent opaque withdrawal markers can publish without private assertion
parents. A dependent tombstone still waits if its parent is held or publication
is ambiguous; private bytes are never released to unblock it. Per-scope
publication status distinguishes Held/CleanupWaitingAncestry from
PendingMarkers/ReadyMarkers and records temporary publication failures.
An independently owner-proofed kept assertion can also report waiting_ancestry
when its same-record parent is private/held. It remains valid locally: the proof
severs evidence dependencies, not revision ancestry. No parent release, implicit
clone or new owner re-authoring CLI is provided.

Default tool capture keeps names, status and linkage, omitting tool bodies.
Redaction runs before chunk serialization, search, model input, compression or
outbox creation. Extra owner regexes append to redact/v1. Session exclusion rules
match native IDs or title regexes. Unknown or unmapped project text is not archived.
Original omitted content can only be recovered while the source harness retains it.

`source configure --source ID --config JSON` changes a disabled source's version
pins without changing its identity or checkpoints. For T3, changing the environment
requires a new source identity. Native compatibility pins are empty by default.

## IPC and clients

Successful commands without a return value acknowledge with `result:null`.
Clients accept that operation payload while strictly validating the response
envelope and rejecting duplicate keys. An invalid or interrupted reply remains
an ambiguous outcome; it must not trigger an automatic mutation retry.

The `waiting_ancestry` error preserves a retained revision's unpublished-parent
condition across IPC. `heads_changed` reports a review of an obsolete revision.
The CLI returns these domain errors without describing the daemon as unavailable;
neither error authorizes releasing held parents or retrying a review against a
different revision.

The Unix socket is `<daemon-home>/run/daemon.sock`, mode 0600 inside a 0700 directory.
Paths must fit the platform Unix socket limit. Requests are versioned JSONL,
limited to 1 MiB; replies are limited to 8 MiB. The listener has at most 32 clients,
60-second idle deadlines and one request in flight per connection. Tokens are
32 random bytes encoded as base64; only their SHA-256 hashes live in client grants.
Grant files and token files are mode 0600. Returned content is untrusted data.

Add a non-admin client with `daemon client add --name reader --profile sessions
--scopes SCOPE --caps status.read,knowledge.search`. Client registration validates
local scopes. `session.read` is a distinct capability and is never implied by
knowledge search. MCP refuses admin and capture-hint grants; keep the CLI admin
token out of every agent MCP configuration.

Daemon-owned legacy CLI commands route over authorized local IPC and retain their
legacy JSON output. `mcp --scope S` on such a profile fails with a migration message.
Replace that configuration with:

```json
{"command":"/ABS/capsule-memory","args":["--daemon-home","/PRIVATE/daemon","mcp","--client","reader","--token-file","/PRIVATE/daemon/clients/reader.token"]}
```

Unlisted profiles continue using direct shared locks. `hint --source ID` is a
bounded 500 ms wake hint and exits zero even when the daemon is unavailable.
Hints never carry source content and never establish durable capture.

## Knowledge, aggregation and status

Knowledge search defaults to historical mode. `summary search --mode strict`
requires live reconciliation and exact dependency-head checks during that request;
source outage, contention, incomplete catch-up or missing source keys hides a
projected result. Historical results carry current:false unless this request
verified them, plus last verification heads/time and coverage. Legacy memory/get
paths never expose schema-v2 knowledge or raw sessions.

`aggregation grant --name NAME --sources A,B --destination PERSONAL` calls the
owner-signed aggregation grant operation. Enable its returned record ID in policy
`aggregations`; the reader subset is checked on control changes and rollup work.
`bridge grant --source-vault SOURCE --bridge-device TARGET_SIGNER --target-scopes T
--ledger-scope T` is run against the target profile. TARGET_SIGNER is the **target
profile's signing-device ID**, because target Prepare authors the projected
knowledge and lifecycle. Source identity stays in the link/source references.
The target scope and grant/lifecycle scope are the same v2 scope. Cross-vault
policy remains off unless explicitly acknowledged as enabled_unaccepted. Local
bridge job destinations remain source scopes; the authorized target is read from
the durable projection row.

Status separates locally saved chunks, acknowledged publication, summarized
records, pending encrypted bytes, source gaps/quarantines and blocked reasons.
Published-body cache eviction is `not_implemented`; no disk-space reclamation is
claimed. Capture stops at the configured disk floor/watermark without advancing
its checkpoint. Replay remains limit=1 per page, about 100 records/scope/tick.
Capture, publication, derivation and bridge loops are independently cancellable.

The systemd and launchd files in `packaging/` are **templates only**. Review paths
and platform behavior before any owner installation. Nothing installs, enables
or starts those services automatically.

## Capture and export bounds

An unfinished-stream buffer flushes at 256 KiB, the configured flush interval,
caught-up state or a closed-session observation. Each stream holds at most 8 MiB
and 8192 events/observations/omissions in memory. Failed commits retry before more
adapter reads; durable checkpoints never acknowledge this buffer. File adapters
keep incomplete raw lines only in a separate bounded memory cache. Disk pressure
stays blocked until the low watermark and free-space floor are both satisfied.

`project exclude --path /ABS/path` adds a local explicit denial; `project include`
removes it. The longest approved mapping applies unless an equal or more specific
denial exists. Child inheritance re-evaluates the observed parent path under the
current exclusions. Unmapped or excluded text never enters a chunk. Coverage gaps
are durable even when no mapped manifest exists.

Historical enable requires a valid since, positive byte budget and 1–64 explicit
streams. Individual event times, native file end positions and the remaining byte
budget are enforced; missing times are excluded with an audit. A native rewrite
uses stable native IDs and explicit correction events. Ambiguous positional
rewrites quarantine without overwriting saved provenance.

CLI export carries a snapshot token between bounded IPC pages. New inserts cannot
shift the pinned order; deletion/suppression is rechecked on every page. Raw export
retains corrected originals and correction records. A conflicting branch sequence
fails bounded session.read explicitly; raw snapshot export preserves all rows.

Ordinary appends resume at the acknowledged byte offset. Identity changes,
truncation, bounded prefix/tail mismatch or a same-size rewrite start a bounded
reconciliation generation. The acknowledged tail digest is separate from adapter
state; it never hashes the entire file. Native IDs preserve unchanged content, changed
native content emits a replacement plus correction, and missing native content
is retracted only at a complete generation end. Unchanged positional content is
reused only at the exact old position; changed or missing positional evidence
quarantines. Re-reading a large stable generation can take many polls. Backfill
byte budgets include these reads and never reset automatically.

Codex checkpoint adapter state is version 1 and strictly decoded with a 64 KiB
limit, a 256-entry bound and validated hash/reference fields. Local checkpoint
metadata also records observed file size/modification, acknowledged tail digest
and caught-up state. T3 checkpoints persist a completed history-walk watermark;
unchanged sequence polls skip old history. Changed sequences reconcile under page,
event and byte bounds; oversized pages or stalled cursors quarantine visibly.
Capture status reports pending streams and buffered totals; aggregate queues
include capture-derived jobs across all open profiles.
