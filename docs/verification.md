# Verification and acceptance

**All runtime scenarios below are UNVERIFIED.** The repository now includes [one executable black-box E2E journey](../e2e/README.md) alongside synthetic contract examples; it has not been run. Source inspection or an authored test cannot establish these outcomes. The broader scenarios below remain acceptance targets beyond the limited v1 journey where noted in its runbook.

Use these scenarios to design the first meaningful suite. Execute only through an authorized isolated route. Record commit identity, tool versions, exact command/job, exit status, observations, and missing coverage. A disconnected, blocked, skipped, or timed-out job is not a pass.

## V1 acceptance scenarios

| Scenario | Setup and fault | Observable acceptance condition | Why the design should support it |
| --- | --- | --- | --- |
| Offline convergence | A and B start at one checkpoint, disconnect, create different records, then reconnect; drop all hints | Both retain their local writes and eventually show the same published revisions with empty outboxes | Atomic local queue plus head reconciliation and idempotent replay |
| Conflict preservation | A and B edit the same parent offline; vary upload and replay order | Both heads and their provenance survive on both machines; neither timestamp wins; explicit resolution links both | Immutable revision DAG and causal conflict rules |
| Delete versus stale edit | A deletes while B edits offline; reconnect B after a checkpoint | Normal recall hides the record; B's edit remains inspectable as a suppressed conflict; only explicit restore can reveal it | Tombstones and ancestry survive checkpoints; restore is a distinct operation |
| Duplicate delivery | Lose a publication response, retry the exact transport envelope, and deliver the resulting revision repeatedly | One logical mutation; stable revision identity; no repeated skill activation or index duplication | Persisted retry bytes, transport checks, logical deduplication, transactional application |
| Interrupted synchronization | Kill client/coordinator before and after upload, journal write, head CAS, acknowledgement, and local cursor commit | Every acknowledged write remains reachable; unpublished objects stay invisible; retry and replay recover without loss | Immutable object staging and a single durable head commit point |
| Long absence and revocation | Return a machine older than the checkpoint window with queued drafts; repeat after its grant is revoked | Authorized drafts reconcile with current heads/epochs; deleted records stay hidden; revoked machine cannot publish or obtain future keys | Control-first reconnect, retained operation identity, tombstones, and re-enrollment rules |
| Local rebuild | Destroy only a clean replica's DB/index, retaining recovery authority; separately repair an index with an unsent outbox | Clean replica restores authorized published data and coverage; index repair preserves pending writes; irrecoverable local-only loss is reported | Archive checkpoint/tail plus separate durable/derived roles |
| Traceable recall and summary | Retrieve a record and summary, edit a cited source, then delete it | Results name exact revisions and coverage; stale summary is marked/hidden; deleted content does not appear through summary or FTS | Provenance, source dependency invalidation, and visibility filtering |

The table specifies expected behavior, not a passing report. Split scenarios into focused tests and bounded integration jobs as implementation arrives.

## Boundary and security scenarios

| Boundary | Required checks |
| --- | --- |
| E2EE and context | Cloud requests/logs contain no plaintext or content keys; modified ciphertext, signatures, scope/epoch substitution, replayed control state, and unsupported versions are rejected |
| Enrollment and recovery | Substituted fingerprints cannot enroll; recovery succeeds without a surviving original device; missing recovery material is a documented hard failure |
| Skills | Unsigned, unapproved, conflicting, revoked, or path-traversing artifacts cannot activate; the exact approved release remains identifiable |
| Search | Queries stay local; scope filters apply before return; partial archive coverage and offline missing bodies are visible; tombstones remove stale hits |
| Parser and resource limits | Malformed JSON, duplicate keys, huge decimal cursors, oversized bodies, compression bombs, deep ancestry, slow providers, and cancellation remain bounded |

Cross-language vectors must include exact signing input bytes, hashes, authenticated context, keys/nonces from synthetic test material, valid ciphertext, and negative cases. The current logical fixtures are not cryptographic vectors and cannot satisfy this gate.

## Provider conformance

Race two create-once writers; at most one may create a different immutable object. Race two head updates from the same version; only one may commit. Read after each acknowledged write and verify exact bytes without CDN caching.

Inject ambiguous upload outcomes: a negative read while the original request is still in flight must not be treated as a definitive failure. Retry safely, compare existing object digest, and verify that a mismatched immutable key is rejected.

Destroy the coordinator's cache after publication and rebuild it from durable history. Interrupt notification delivery, hibernate/recreate the Durable Object, and replay from a client cursor. Verify bounded pagination and scope filtering during catch-up.

Test rate-limited and free-quota-exhausted operation: the device keeps local pending writes, reports a blocked remote state, and avoids a retry storm. Billing behavior and platform limits must be observed in an explicitly authorized integration environment before making a cost claim.

## Documentation setup evidence

Repository setup may use manual file/diff inspection, relative-path inspection, `git diff --check`, and GitHub repository/commit reads. These checks can establish what was authored and published. They do not establish harness discovery, schema validity, protocol interoperability, crash safety, E2EE, or platform cost.

Do not add a passing badge, fabricated CI result, or pre-completed acceptance checklist. Add a dated result linked to the exact source only after a real authorized job runs.

## Optional session module acceptance (NOT RUN)

The following are authored synthetic scenarios, not test results. The session task intentionally adds no automated test files. Run only through an owner-authorized execution route; record the exact integrated commit and all tool/model/adapter versions. See [session status](session-status.md) and [owner runbook](run-session.md).

| Scenario | Required observable result |
| --- | --- |
| Every schema-v2 kind, incompressible input, escaped control characters, retry after ambiguous upload | DEFLATE before encryption; full revision ≤896 KiB; exact persisted retry bytes; legacy v1 unchanged |
| Identity-reader fixture, nonzero padding, trailing DEFLATE data, short EOF, over-expansion | Only conforming bounded frames accepted; failing replay never advances its cursor |
| Partial line, rewritten/truncated log, unknown version, duplicate Codex prompt, strong T3 binding | Bounded re-read, visible quarantine/gaps, stable event identity and one evidence count |
| Private migration snapshot and scope/key-package crash points | Exclusive-lock refusal or private recoverable completion; no partial grants/key widening |
| Concurrent daemon/direct clients and stale socket | One daemon; profile_busy retry; legacy commands route or give explicit MCP migration error |
| Capture disabled, future baseline, bounded backfill, unresolved pause and withdrawn source | No unauthorized read/backfill/publication; checkpoint/outbox/hold transactions survive restart |
| Source delete, correction, superseded exact revision, out-of-order replay and rebuild | Transitive visibility hides stale descendants, overflow aborts fully, no arrival-order replay block |
| Local inference endpoint points off loopback, DNS resolves off loopback, proxy/redirect/tool response | Refused; no tool execution; bounded request/output/response and no prompt/output logging |
| Transcript includes `</transcript>` and role/tool instructions | Escaped JSON remains data; fixed system prompt unaffected |
| Model emits unknown citation, accepted decision without user evidence or evidenced outcome without tool result | Invalid IDs rejected; unsupported acceptance/verification downgraded with visible notes |
| Windowed job crashes, lease expires, policy changes while inference runs | Persisted windows reused; lease/policy mismatch rolls back late final or window commit; budget reservation survives |
| Candidate reviewed then regenerated or assigned remotely | Explicit edit/reject/accept persists; no implicit acceptance; unauthorized remote jobs ignored and counted |
| Weekly aggregation followed by reader grant widening/revocation | Owner signature required; reader subset checked at grant change, run and commit |
| Promotion crash after target commit, same-vault scopes, target-only replica | Idempotent exact v2 knowledge/lifecycle; strict target-only hidden; legacy APIs reject every v2 kind |
| Live strict read during publish contention, source outage, incomplete catch-up or dependency edit | Ten-second request bound and fail closed; historical labels never imply currentness |
| Source delete/keep-insight/reimport across two capture devices | Every matching manifest suppressed; retained independent assertion has no generator/evidence; no stale resurrection |

A manual walkthrough should inspect both primary and intermediate evidence, contradictions, completeness and untrusted labels without granting `session.read` implicitly. Search and returned snippets must respect scopes. Cross-vault withdrawal is asynchronous, and already disclosed plaintext is not recoverable. Cache eviction, binary attachments, cloud inference, embeddings, destructive retention and universal resumption remain outside acceptance.
