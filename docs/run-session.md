# Evaluate the optional session module

Use disposable synthetic data on an explicitly authorized Linux/macOS development machine or isolated job. **Every command below is prepared for later owner execution; none was run during implementation.** Read [session status](session-status.md) before setup. Allow roughly 30–60 minutes for dependency/build review and a synthetic first pass, excluding model installation and debugging.

The coordinator must first integrate the parallel source deliveries and complete independent review. New source is not a working release until compiled and exercised. These instructions grant no service installation, real-history access, credential discovery, inference call, infrastructure deployment or paid resource use.

## 1. Review and build the exact source

Record `git rev-parse HEAD` and inspect the complete diff. On the authorized machine, resolve/review the existing pinned dependencies and any missing lock/checksum files using [Run v1](run-v1.md). Keep that legacy runbook and its fixtures unchanged. Then prepare a private output directory outside a real profile and run:

```sh
umask 077
go build -o /absolute/private/bin/capsule-memory ./cmd/capsule-memory
go vet ./...
go test ./...
```

These are pending checks, not successful evidence. The session task adds no automated tests; existing tests alone do not establish the session invariants. Run `gofmt` on the reviewed new/changed Go source and inspect its diff before acceptance. Formatting was deliberately NOT RUN on the shared host. Resolve build failures before loading real data.

## 2. Prepare private profiles and migration

Prefer a dedicated session vault/profile; the present relay remains one vault per Worker deployment, so a second vault needs separately authorized relay setup. Do not deploy as a side effect of evaluation. Keep profile directories `0700`, files `0600`, outside the checkout, with disk encryption and independent recovery backups.

For an existing synthetic profile, stop its daemon/direct clients before the explicit `profile migrate` operation. Migration needs an exclusive profile lock, creates a private consistent `memory.db.pre-v2.*` snapshot, fsyncs it and applies the additive migration. Preserve the reported snapshot path even after failure. A legacy binary cannot open a v2 DB. Do not restore a backup over a live SQLite profile.

Use the authored CLI command reference in [daemon operation](daemon.md) and [Go client](go-client.md) after checking the actual binary flags. A session profile must have owner-approved project/personal scopes; the legacy default scope is not a knowledge destination. New devices receive only explicitly granted scope keys. Revoking a grant stops new relay access but does not erase old key copies; rotation remains manual.

## 3. Preview a synthetic source with controls off

Start with [synthetic adapter examples](../contracts/adapters/) and the documented import format. Keep source configuration private. Add/preview a source while disabled, approve an explicit project map and version pin, and inspect omissions before enabling capture. No harness version is runtime-accepted merely because its prefix matches. Unknown shapes/versions must quarantine visibly.

Enabling capture establishes a future-only baseline. Historical backfill is a separate bounded request with explicit streams, time and byte bounds. Capture, publication, derivation, auto acceptance and cross-vault promotion are independent opt-ins. Pause requires a later explicit include/skip resolution. Disabling with withdrawal holds unpublished source revisions durably; manual sync must honor those holds.

T3 setup uses an **owner-configured future token file**, never an agent-discovered credential. The token is broad upstream: `orchestration:read` also reads server-readable files. The Capsule adapter uses only pinned allowlisted GET history routes with protocol 2, expected environment and owner version pins. Missing credentials report unavailable. Do not paste the token into prompts, fixtures, logs or source. Claude/Codex routes read version-pinned files; no SDK or Codex App Server is started.

## 4. Configure local inference explicitly

The owner supplies an already authorized local OpenAI-compatible server. Capsule does not install or start it. A daemon inference object can use this synthetic configuration (merge into the complete private daemon config, preserving its other fields):

```json
{
  "enabled": false,
  "endpoint": "http://127.0.0.1:1234/v1/chat/completions",
  "model": "OWNER_LOCAL_MODEL_ID",
  "max_input_tokens": 8192,
  "max_output_tokens": 2048,
  "daily_input_budget": 100000,
  "daily_output_budget": 20000,
  "timeout": 120000000000,
  "max_response_bytes": 1048576,
  "cpu_hint": "owner configured",
  "ram_hint": "owner configured"
}
```

`timeout` is a JSON duration in nanoseconds (the example is two minutes), not a string. CPU/RAM hints are labels, not enforcement. Configure the model server's own limits separately. Keep inference disabled until its explicit synthetic call is authorized. Endpoint host must be localhost or a literal loopback IP with a port; dial-time checks reject non-loopback resolved addresses. Proxies and redirects are disabled; no tools/functions are sent.

Transcript events are escaped JSON lines inside a fixed transcript wrapper; the system prompt is versioned and separate. Validate unknown fields, missing fields, enums, size bounds and exact citations. A model claiming user acceptance without user evidence is downgraded; evidenced verification needs a tool-result citation. All output stays untrusted. Budget reservations count the configured maximum per call, including failed/crashed calls. Completed windows checkpoint separately and are reused after a new lease; daily budget blocks resume after UTC midnight.

## 5. Verify review, grants and lifecycle with synthetic evidence

Use [verification scenarios](verification.md#optional-session-module-acceptance-not-run). Exercise candidate accept/edit/reject and rerun suppression. Auto acceptance is separately off by default and limited to explicit project facts with user evidence, no contradictions and no promotion. Candidate normalization lowercases and collapses ASCII whitespace; it does not provide Unicode NFC or semantic paraphrase deduplication.

Personal weekly aggregation needs an owner-signed named grant. Every destination reader must also hold every source scope; attempt a later incompatible device grant/revocation and require rejection. Grant revocation updates the same grant record. Remote assignments need current author access to all inputs and destination, exact same-scope rules for session/project jobs and a valid aggregation grant for personal jobs. Derivation withdrawal cancels leased tokens so late results cannot commit.

Cross-vault promotion remains `off`. An explicit future setup can enable `enabled_unaccepted`, which is not runtime acceptance. The target owner's grant identifies the **target profile signing device** and one project/personal scope. The source profile retains separate source identity. Knowledge, lifecycle and grant must share the target scope; legacy memory APIs must never expose v2 bodies. Queue destination stays the source scope; actual targets come from the durable projection row and are independently authorized.

Read historical knowledge with its `current:false` and last verified time/heads. Strict reads must reconcile source scopes during that request and hide on offline/busy/incomplete/timeout conditions. Re-attestation after 24 hours is maintenance, not proof of currentness. Delete/correct an input and verify immediate local transitive hiding, eventual target withdrawal and no resurrection after late chunks, reimport or rebuild. Retained insights must become explicit independent owner assertions with empty evidence and no generator.

Record saved-here, published and summarized separately. Inspect source gaps, quarantine, pending bytes, failed/blocked jobs, stale dependencies, replay coverage and `cache_eviction: not_implemented`. Replay remains one record per page and about 100 pages per tick; backfills can take many ticks. Stop the foreground daemon and owner-run local model when done. Do not install packaging templates or hooks without a separate request.
