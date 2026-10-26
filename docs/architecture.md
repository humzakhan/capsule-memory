# Architecture

**Decision:** build a local Go service on each trusted machine, with device-only SQLite search and an encrypted archive coordinated by a small Cloudflare relay. **Status:** this is the full architectural target; [v1 delivery status](v1-status.md) describes the source implemented so far. All runtime guarantees remain unverified.

V1 uses explicit CLI sync and fixed-scope MCP processes sharing SQLite. The daemon/IPC, wake hints, automated key distribution, maintenance, and Git bridge shown below are future work. Read the [v1 contract](v1-contract.md) and [runbook](run-v1.md) for current behavior.

## System shape

```text
                         GitHub
              source, docs, reviewed changes
              optional encrypted vault exports
                            |
                local reviewed import/export
                            v
  EACH TRUSTED MACHINE                         CLOUDFLARE
  +--------------------------+          +-----------------------+
  | Harness                  |          | Worker                |
  | MCP client / optional    |          | device auth, limits,   |
  | harness-specific hooks   |          | opaque scope routing  |
  +------------+-------------+          +-----------+-----------+
               | stdio MCP                         |
  +------------v-------------+          +-----------v-----------+
  | Go MCP bridge + service  |<-------->| Durable Object/vault  |
  | domain, keys, sync       | HTTPS /  | publication ordering  |
  | local IPC, maintenance   | WS hints | hibernating WebSocket |
  +------------+-------------+          +-----------+-----------+
               |                                   |
  +------------v-------------+          +-----------v-----------+
  | SQLite                   |          | R2 Standard           |
  | revisions, durable queue |          | encrypted payloads,   |
  | FTS5, cache, sync cursors |          | journal, checkpoints, |
  +--------------------------+          | durable head          |
                                        +-----------------------+
```

The machine service owns synchronization independently of harness lifetime. A stdio MCP process bridges to that service using local user-restricted IPC; it does not open another SQLite writer per harness. The CLI calls the same application operations. Linux and macOS are the initial desktop assumption; platform support must be confirmed before implementation.

The central deployment contains a Worker, a SQLite-backed Durable Object per vault, and a private R2 bucket. It has public device verification keys and opaque authorization metadata, but no content-decryption keys. A custom domain is optional. There is no VM, central plaintext index, or cloud summarizer.

## Ownership

| Data | Authoritative home | Derived or secondary copy |
| --- | --- | --- |
| Program source, project skills, architectural decisions | This Git repository | Local checkouts |
| Pending personal memory or skill revision | Creating device's committed SQLite state and outbox | None until published |
| Published personal revisions and control history | Durable archive journal reachable from the R2 head | SQLite replicas; DO cache |
| Approved personal skill source | Owner-reviewed encrypted artifact and signed release digest | Local decrypted staging; optional private vault Git export |
| Summaries | Published immutable summary revisions with source references | Rendered previews and optional encrypted Git exports |
| Search index, embeddings, snippets, payload cache | Rebuildable device state | Never authoritative archive records |
| Private keys and recovery material | Trusted device keystore and independent owner backup | Never cloud-held plaintext |

The project skills under `.agents/skills` are development instructions for this codebase. They are not the user's runtime memory vault. This setup creates only the source repository, not a second vault repository.

For personal content, GitHub is a reviewed import/export surface, not a second multi-master database. Only ciphertext belongs in an optional private vault repo. A trusted device turns a reviewed Git change into the same signed revision operation used by local edits. R2 determines what has been published to devices; a Git commit alone does not deploy or activate a skill.

## Local store and retrieval

Use SQLite with FTS5 first. It provides one embedded transactional store for durable revisions, outbox rows, cursors, visibility, summary dependencies, and search. Choose the actual Go driver after verifying FTS5, backup support, migration behavior, CGO implications, and the target platforms. Keep that decision explicit.

Use lexical search plus scope, kind, tags, source, and recency filters. Rank relevance first; freshness and pinned status can contribute. Do not hide alternative conflict heads or stale-summary warnings. Embeddings wait until a measured retrieval benchmark shows a need; if added, generate them on devices and version them by model and chunking scheme.

For full archive search, the device must first download and decrypt its authorized archive, incrementally, to construct its local index. R2 cannot identify semantic matches without seeing content or receiving a specialized searchable-encryption design. That complexity is out of scope.

The full local index contains sensitive lexical content and can be comparable in size to the archived text. Hot/cold separation therefore mainly saves payload and attachment space, not all local text. A constrained device may index selected scopes or windows, but must report incomplete coverage. Never silently represent a partial index as a complete archive answer.

Retrieval follows this path:

1. Resolve the caller's explicitly granted vault and scopes; an agent-supplied path cannot expand them.
2. Query local visibility-filtered FTS and metadata, returning record/revision IDs, provenance, conflict state, and coverage cursor.
3. Return cached content where available. Otherwise fetch only the indexed encrypted payload objects, verify and decrypt locally, then cache them.
4. If offline, return available verified content and mark unavailable bodies and stale coverage. A new, unindexed device cannot search unseen archive text offline.
5. Reconcile remote heads and incrementally extend the local index in the background. Remote semantic search is never a fallback.

## MCP and harness boundaries

Planned MCP operations are scoped memory store, recall, resolve/delete/restore, skill list/fetch, and sync status. Destructive actions and executable activation require stronger caller permissions than recall. Return machine-readable source references and distinguish local durability from remote publication.

MCP provides a tool/resource interface to compatible clients. It does not force the model to recall, guarantee a lifecycle callback, intercept every write, or make all harnesses behave alike. A harness may fetch a skill as text without exposing a native skill-installation API. Activation adapters must handle that difference explicitly.

Hooks are optional conveniences: a supported event can request capture or wake sync. Their event names, trust model, payloads, and blocking behavior are harness-specific. Where a harness lacks hooks, explicit MCP calls and the independent machine service still work. Polling, import reconciliation, and startup sync repair missed hooks.

Local IPC must restrict access to the OS user and scoped client grants. Stdio MCP must keep protocol output separate from diagnostic output. Remote MCP exposure is not part of version one.

## Storage-provider boundary

The provider stores bytes. Coordination assigns publication order; notifications announce that work may exist. Keep these as separate interfaces.

| Storage operation | Required contract |
| --- | --- |
| `PutImmutable(key, bytes, digest)` | Create once or prove existing bytes match; ambiguous retries cannot overwrite different content |
| `Get(key)` | Return committed bytes and an opaque version token; content hash checked independently |
| `GetHead(vault)` | Read the current publication head without stale intermediary caching |
| `CompareAndSwapHead(expectedToken, newHead)` | Atomic create/update against the committed head; reject a competing writer |
| `Delete(key)` | Idempotent removal, allowed only under explicit reachability/retention policy |

`ListPage`, range reads, multipart uploads, lifecycle cleanup, and notification integrations are optional capabilities. Normal change discovery follows the head and journal, not bucket-wide listing. Version tokens are concurrency tokens, not content hashes.

R2 maps immutable writes to conditional object creation, head replacement to conditional writes, and reads to direct bucket APIs. Its documented consistency and atomic conditional writes support the intended publication boundary. The provider adapter must still demonstrate these semantics through conformance scenarios. See [R2 consistency](https://developers.cloudflare.com/r2/reference/consistency/) and [Workers API](https://developers.cloudflare.com/r2/api/workers/workers-api-reference/).

R2 is not the notification bus, identity authority, or plaintext search service. The Worker and Durable Object provide the first two functions; devices provide search. There is no multi-object storage transaction. Immutable-object publication followed by a fenced head update is the commit protocol.

A provider without the required read visibility or atomic head update needs a separate strongly consistent coordination mechanism with fencing and recoverable publication state. An adapter cannot replace those semantics with a list call or an S3-shaped client. Such a provider is unsupported until that mechanism and its failure tests exist.

## Maintenance and recovery

Pin active skills, unresolved conflicts, recent working memories, and user-pinned records in the payload cache. Evict least-recently-used bodies after remote publication and integrity confirmation. Never evict the only durable copy of a pending write or unresolved local operation.

Rollover groups inactive records by scope and provenance into immutable encrypted archive segments and optionally a new local summary revision. Summarization is opt-in and runs only on an authorized trusted device. Proposed initial scheduling is a daily maintenance check with a 30-day inactivity candidate window; make the window and byte budget configurable after workload measurements.

A summary names exact source revisions, coverage, generator/version, and generation time. New source revisions produce a replacement summary; older summaries become superseded or stale rather than silently rewritten. Contradictions, decisions, unresolved questions, and exceptions remain traceable. Source deletion also invalidates dependent summaries and removes their content from normal recall until regenerated.

Version one has no automatic destructive history retention. Checkpoints compact replay and carry the full state needed for heads, tombstones, operation deduplication, ancestry, membership, and summary dependencies. They do not authorize erasing source history. Later garbage collection requires explicit retention policy and proof that old devices cannot resurrect retired state.

Rebuild a device from owner recovery material, authenticated membership/control state, an encrypted checkpoint, and subsequent journal entries. Keep pending local work when repairing a damaged index. A machine lost before its queued operations reach another durable store can lose those operations; local acknowledgement is not a remote backup guarantee.
