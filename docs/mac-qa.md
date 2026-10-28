# Mac beta QA — 2026-10-28

This records authorized local QA after the source-only beta delivery. Repairs
remain on a local branch; these results do not describe the published beta tag.
All transcripts, memories and device profiles used here were synthetic. No real
harness capture was enabled.

## Source and checks

The model/owner-proof acceptance run used
`2c547a2b12e59a5569a41b83749e656487fb98ca`, based on released source
`6797fbfb39a6ff942809ea07864ca96317a699db`. Go 1.24.0 and the pinned relay
dependencies were used. The full `scripts/check.sh --range BASE..HEAD` gate
passed: Go formatting, vet, tests and build; relay TypeScript and formatting;
Conventional Commit subjects. Range whitespace checks passed. Focused
race-enabled daemon/session/app and inference/derivation tests also passed.

The existing twelve-scenario local E2E journey passed after its ancestry retry
was corrected to require durable queue progress. That run preceded the later
daemon and inference repairs; it is not a fresh E2E result at the installed SHA.

## Observed runtime behavior

- A dedicated QA Worker, SQLite Durable Object and private R2 bucket accepted
  signed encrypted publication, enrollment and independent replica replay.
  Public bucket access was disabled; no custom domain or cron was configured.
- The daemon captured a post-consent import event and excluded older history.
  Legacy recall did not disclose session records. Withdrawal held unpublished
  records; re-enabling capture and rebuilding retained those holds.
- Private session export, encrypted schema-v2 session publication, scoped replica
  search, local deletion/rebuild and replica tombstone propagation passed.
- A scoped stdio MCP client exposed exactly its permitted tools. Session search
  worked; ungranted mutation and an out-of-scope request were rejected.
- The existing loopback Qwen3.5 4B model produced a locally validated, searchable
  summary and one reviewable candidate using explicit `prompt_json` mode and
  `digest/v2`. Review accepted the observed revision and rejected an obsolete
  revision. Deleting the session while keeping the assertion produced an owner
  proof with no evidence or generator. That assertion remained valid through
  rebuild and capture withdrawal, while its required private parent stayed held.

These cloud/session checks preceded the prompt repairs. They establish those
bounded paths, not exhaustive synchronization, crash or security acceptance.

The constrained-output default failed with this local model: the endpoint put
JSON in reasoning content and returned an empty final answer. An initial
compatibility run also returned invalid output. Both failed without a summary
commit and retained their budget reservations. Explicit compatibility mode plus
the versioned prompt correction passed; this is one synthetic acceptance run,
not a model-quality evaluation. Shared model settings were unchanged.

Follow-up product revision `1e73dfc32f90bb1bfc2db5700200727c12fb5e26`
preserves ancestry-wait and obsolete-review domain errors over IPC. Previously,
the operation was rejected correctly but the CLI reported daemon unavailability.
The private installation manifest records the final binary revision and digest;
later documentation-only commits do not change the product implementation.

## Performance samples and limits

Ten local legacy recall samples had median 8.893 ms and nearest-rank p95
18.279 ms, including CLI startup. The twelve-scenario local E2E journey took
7.02 seconds; its wrapper reported peak child RSS about 269 MiB. A relay deploy
reported 1 ms startup. These are small-run observations, not throughput or
service-level guarantees; provider CPU and billing usage were not measured.

The accepted local derivation completed in 37.288 seconds after scheduling,
including the daemon's job polling. One resource snapshot during the run showed
normal memory pressure, 13.55 GiB estimated headroom and no swap growth; it is not
a peak-memory measurement. File ingestion also has a one-minute stability delay,
separate from derivation latency and the capture flush interval.

The owner confirmed available free capacity and R2 allowances. Billing reads
were unavailable to the OAuth login. No paid upgrade was requested. This does
not establish that all future usage is free.

Linux/Windows runtime, native Claude/Codex/T3 version acceptance, long-running
load, destructive crash recovery, broad cross-vault promotion and independent
security acceptance remain unverified. Published-body cache eviction remains
unimplemented. The task installed no login service and changed no shared model
or harness configuration. Private evidence and rollback artifacts stay outside
Git; credentials, databases and keys must not accompany this report.
