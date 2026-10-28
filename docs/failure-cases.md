# Failure-case inventory

A queryable inventory of **193 failure cases across 18 areas**, mapped
to local source `2af1f9d8fb4ea12dd5a129bbeae6dd60dad41b14` on 2026-10-28.
This is a broad starting inventory, not a claim that every possible failure or
combination has been enumerated. Most entries are risks to verify, not confirmed
bugs. No additional product tests or QA workloads were run to create this list.

## Data and labels

[JSONL inventory](failure-cases.jsonl) is the canonical record for querying. Each
line is one object with `id`, `area`, `priority`, `kind`, `title`, `trigger`,
`consequence`, `required_result`, `coverage`, `evidence`, and `source_paths`.
The tables below are a readable index of that data. `source_paths` identify
ownership and investigation starting points, not confirmed defect locations.
Keep both files consistent
when changing cases; retain IDs and do not renumber existing cases.

- **Priority:** P0 means potential data loss, privacy/authority failure or invalid
  durable history; P1 means availability, correctness, recovery or operational
  failure. Priority indicates impact if a safeguard fails, not probability or
  confirmation that the current code is defective.
- **Kind:** `risk` is a failure condition to exercise; `repaired` is a previously
  observed defect with a local correction; `backend_failure` is an observed
  backend incompatibility; `limit` is a current architectural or product boundary.
- **Coverage:** `observed` means the described narrow scenario has executed
  evidence; `partial` means only a related path, smaller variant
  or focused regression is covered; `not_run` means this scenario has no recorded
  execution evidence. An expected rejection counts as an observation, not a bug.
- A passing row never means all fault schedules, platforms or provider variants
  passed. Local emulator skill tests do not establish live-cloud skill acceptance.
  The old E2E suite was not rerun after the final daemon/inference repairs.

Snapshot: **37 observed**, **63 partial**, and
**93 not run**. These counts describe rows in this inventory,
not numbers of tests, discovered defects or independent security findings.

## Evidence references

These IDs resolve the `evidence` arrays; private runtime artifacts stay outside
Git. [Mac QA](mac-qa.md) records the dated observations and their limits.

| ID | Evidence and scope |
| --- | --- |
| E2E | Twelve local-emulator scenarios passed at `5dbe6ac82dc4e2ce10d6a38a755421728cc320d6`; see [journey](../e2e/run.mjs) and [QA](mac-qa.md). Includes skill text lifecycle, not live-cloud skills. |
| LIVE_MEMORY | Signed bootstrap, encrypted memory publication, enrollment and independent replay on the dedicated Cloudflare QA relay; source `5dbe6ac82dc4e2ce10d6a38a755421728cc320d6`. |
| LIVE_SESSION | Synthetic daemon capture, future-only exclusion, holds, export, encrypted v2 replay and deletion/rebuild during the later Mac QA; multiple documented binary replacements, not a blanket final-SHA replay. |
| LIVE_MCP | Scoped stdio tool discovery/search and negative capability/scope checks during Mac QA. |
| MODEL_QA | Actual loopback Qwen summary/candidate and owner-proof keep/rebuild/withdrawal at `2c547a2b12e59a5569a41b83749e656487fb98ca`; prior failed attempts also recorded. |
| IPC_QA | Live ancestry and obsolete-revision error checks at `2af1f9d8fb4ea12dd5a129bbeae6dd60dad41b14`; healthy daemon and held bytes verified. |
| QA_INSTALL | Recorded synthetic install/configuration/authentication/export and cleanup checks; inspect [QA](mac-qa.md) for boundaries. |
| QUALITY | Full gate at `2af1f9d8fb4ea12dd5a129bbeae6dd60dad41b14`: formatting, vet, tests, build, relay checks and commit subjects; not broad runtime acceptance. |
| GO_FOCUSED | Relevant focused regression source ran in the final gate; see referenced source paths and [QA](mac-qa.md). Unit success alone does not establish provider, race or crash acceptance. |
| SOURCE_ONLY | Prior source inspection/repair evidence in [session status](session-status.md); no execution proof for this row. |
| LIMITS | Boundaries described in [threat model](threat-model.md), [session status](session-status.md), [architecture](architecture.md) and [QA](mac-qa.md); documentation is not a runtime test. |

## Query examples

With `jq` available, list skills cases:

```sh
jq -r 'select(.area == "Skills") | [.id,.priority,.coverage,.title] | @tsv' docs/failure-cases.jsonl
```

List P0 risks without a recorded execution of the full listed scenario:

```sh
jq -r 'select(.priority == "P0" and .kind == "risk" and .coverage != "observed") | [.id,.area,.coverage,.title] | @tsv' docs/failure-cases.jsonl
```

Inspect one case, including trigger, consequence, expected result and code paths:

```sh
jq 'select(.id == "SKL-009")' docs/failure-cases.jsonl
```

Natural-language follow-ups can use the same IDs: “Which memory and skills cases
still need live Cloudflare tests?” or “Prioritize untested privacy/data-loss
cases.” Do not treat this inventory as approval to run tests, enable real capture,
spend beyond free allowances, delete data or publish changes. Choose the fewest
meaningful checks for concrete invariants and reuse existing coverage first.

## Case index

### Installation and configuration

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| SET-001 | P1 | Wrong binary or wrong profile selected | partial | risk |
| SET-002 | P0 | Upgrade interrupted during replacement | not_run | risk |
| SET-003 | P0 | Older binary opens a migrated database | not_run | risk |
| SET-004 | P0 | Migration snapshot incomplete or public | partial | risk |
| SET-005 | P1 | Invalid or ambiguous configuration | partial | risk |
| SET-006 | P1 | Policy patch fails on internal null slices | observed | repaired |
| SET-007 | P1 | Platform-specific assumptions fail | not_run | risk |
| SET-008 | P1 | Sleep, restart or clock jump interrupts work | not_run | risk |
| SET-009 | P1 | Service ownership or startup is assumed | observed | limit |
| SET-010 | P0 | Dependency or toolchain drift | partial | risk |

### Local durability and recovery

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| DB-001 | P0 | Acknowledgement before durable commit | not_run | risk |
| DB-002 | P0 | Disk full, read-only volume or fsync failure | partial | risk |
| DB-003 | P0 | Live database copied without WAL state | not_run | risk |
| DB-004 | P0 | Corrupt database, WAL or index | partial | risk |
| DB-005 | P1 | Concurrent writers contend or deadlock | partial | risk |
| DB-006 | P0 | Rebuild clears outbox or consent holds | observed | risk |
| DB-007 | P0 | Failed SQL statement leaves partial apply | not_run | risk |
| DB-008 | P0 | Key or grant files finalize separately from SQL | not_run | risk |
| DB-009 | P0 | Device loss before publication | not_run | limit |
| DB-010 | P0 | All recovery material is lost | not_run | limit |

### Memory semantics

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| MEM-001 | P1 | Published memory cannot be recalled on a replica | observed | risk |
| MEM-002 | P0 | Concurrent edits overwrite one another | observed | risk |
| MEM-003 | P0 | Deleted memory returns after stale replay | observed | risk |
| MEM-004 | P1 | Explicit restore uses the wrong ancestry | partial | risk |
| MEM-005 | P0 | Reused operation ID changes meaning | partial | risk |
| MEM-006 | P0 | Out-of-order ancestry applies incorrectly | not_run | risk |
| MEM-007 | P1 | Record-kind confusion crosses interfaces | partial | risk |
| MEM-008 | P0 | A valid signature is mistaken for safe instructions | not_run | limit |

### Synchronization and ordering

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| SYN-001 | P1 | Offline writes never leave the queue | observed | risk |
| SYN-002 | P0 | Lost upload acknowledgement creates duplicates | observed | risk |
| SYN-003 | P0 | Retry recompresses or re-signs ambiguous work | partial | risk |
| SYN-004 | P0 | Cursor advances before durable apply | not_run | risk |
| SYN-005 | P0 | Old control or epoch used for queued upload | partial | risk |
| SYN-006 | P1 | Pagination repeats or skips hidden rows | observed | risk |
| SYN-007 | P1 | Cursor is malformed, huge or noncanonical | observed | risk |
| SYN-008 | P1 | Rate limit or temporary outage causes retry storms | partial | risk |
| SYN-009 | P1 | Authorization failure blocks forever | not_run | risk |
| SYN-010 | P0 | Child publishes before a private parent | observed | risk |
| SYN-011 | P1 | Ancestry error is shown as daemon failure | observed | repaired |
| SYN-012 | P1 | One blocked session starves unrelated work | not_run | risk |

### Encryption, framing and parsers

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| ENC-001 | P0 | Modified ciphertext or signature accepted | observed | risk |
| ENC-002 | P0 | Cross-vault, scope or epoch substitution | partial | risk |
| ENC-003 | P0 | Legacy signing or AAD bytes drift | partial | risk |
| ENC-004 | P0 | Nonce reuse or weak randomness | not_run | risk |
| ENC-005 | P0 | Go and TypeScript disagree on JSON | observed | risk |
| ENC-006 | P0 | Oversized body bypasses an early limit | partial | risk |
| ENC-007 | P0 | Compression bomb, short frame or trailing data | not_run | risk |
| ENC-008 | P1 | Valid incompressible v2 record hits the writer bound | not_run | risk |
| ENC-009 | P0 | Schema or canonical digest mismatch | not_run | risk |
| ENC-010 | P0 | Valid but hostile record poisons replay | not_run | limit |

### Enrollment, grants and key authority

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| KEY-001 | P0 | Enrollment substitutes the new device key | not_run | risk |
| KEY-002 | P0 | Enrollment grants extra scopes | partial | risk |
| KEY-003 | P0 | Copied identity bypasses proof | partial | risk |
| KEY-004 | P0 | Revoked or expired credentials remain usable | observed | risk |
| KEY-005 | P0 | Replay omits the control barrier | partial | risk |
| KEY-006 | P0 | Target-reader widening reveals archived assertions | partial | risk |
| KEY-007 | P0 | Cross-scope aggregation widens its audience | not_run | risk |
| KEY-008 | P0 | Key epoch changes strand offline drafts | not_run | risk |
| KEY-009 | P0 | Revocation is promised to erase prior copies | not_run | limit |
| KEY-010 | P1 | Owner-key rotation or automatic distribution assumed | not_run | limit |
| KEY-011 | P1 | Device clock skew rejects otherwise valid requests | not_run | risk |

### Skills

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| SKL-001 | P1 | Skill text fails to sync or fetch | observed | risk |
| SKL-002 | P0 | Unapproved skill activates automatically | observed | risk |
| SKL-003 | P0 | Changed skill inherits old approval | observed | risk |
| SKL-004 | P0 | Conflicting skill heads retain active status | observed | risk |
| SKL-005 | P0 | Deleted skill remains available through fetch or MCP | observed | risk |
| SKL-006 | P0 | Approval identity or digest is substituted | partial | risk |
| SKL-007 | P1 | Deleted history consumes the skill-list limit | observed | risk |
| SKL-008 | P0 | Skill text executes during synchronization | partial | risk |
| SKL-009 | P1 | Skill assumptions differ between emulator and live cloud | not_run | risk |
| SKL-010 | P1 | Large, Unicode or long-offline skill history diverges | not_run | risk |
| SKL-011 | P1 | Synced skill is not discovered by the actual harness | partial | limit |

### Search and retrieval

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| SRH-001 | P0 | Search returns content outside the caller's scopes | partial | risk |
| SRH-002 | P0 | Stale or deleted FTS rows remain visible | partial | risk |
| SRH-003 | P1 | A limit hides one record kind or scope | partial | risk |
| SRH-004 | P1 | Malformed query or Unicode breaks search | not_run | risk |
| SRH-005 | P0 | Candidate changes during a search snapshot | not_run | risk |
| SRH-006 | P0 | Strict retrieval trusts old or offline source state | not_run | risk |
| SRH-007 | P0 | One source copy overrides a withdrawn copy | not_run | risk |
| SRH-008 | P1 | Missing chunks are described as complete coverage | partial | risk |
| SRH-009 | P1 | Repeating a bounded search never finds later candidates | not_run | limit |
| SRH-010 | P0 | Evidence links leak inaccessible source content | not_run | risk |
| SRH-011 | P0 | Query input becomes SQL or command syntax | not_run | risk |

### Capture and buffering

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| CAP-001 | P0 | Capture reads before consent or backfills implicitly | observed | risk |
| CAP-002 | P0 | Buffered work is acknowledged before persistence | partial | risk |
| CAP-003 | P0 | Configuration or consent changes during adapter I/O | partial | risk |
| CAP-004 | P0 | Backpressure skips buffered pages | partial | risk |
| CAP-005 | P1 | A file never becomes stable while active | partial | risk |
| CAP-006 | P1 | Partial or oversized events are silently truncated | not_run | risk |
| CAP-007 | P0 | Wrong project inherits a child's event | partial | risk |
| CAP-008 | P0 | Tool bodies are captured without their separate opt-in | not_run | risk |
| CAP-009 | P0 | Redaction leaks or corrupts evidence | not_run | risk |
| CAP-010 | P1 | Exclusions, disk floors or worker bounds silently drop work | not_run | risk |
| CAP-011 | P0 | Native checkpoint metadata stores prompt plaintext | not_run | risk |
| CAP-012 | P1 | Manual import races background capture | partial | risk |

### Harness and import adapters

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| ADP-001 | P1 | Native harness version or shape changes | not_run | risk |
| ADP-002 | P0 | Append is mistaken for rewrite | not_run | risk |
| ADP-003 | P0 | Rotation, truncation or same-size rewrite is missed | partial | risk |
| ADP-004 | P0 | Partial-line rewind loses fingerprint protection | partial | risk |
| ADP-005 | P0 | Native and T3 copies double-count evidence | not_run | risk |
| ADP-006 | P0 | Fork identity or event ordering changes | not_run | risk |
| ADP-007 | P0 | T3 endpoint or environment is substituted | partial | risk |
| ADP-008 | P1 | T3 transport failure is treated as permanent corruption | observed | risk |
| ADP-009 | P0 | Paginated history falsely reports caught up | partial | risk |
| ADP-010 | P0 | Adapter follows paths or symlinks outside approval | not_run | risk |
| ADP-011 | P1 | Codex turn context changes CWD attribution | observed | risk |
| ADP-012 | P0 | Explicit import spoofs origin, time or generation | partial | risk |

### Consent, withdrawal and deletion

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| CNS-001 | P0 | One opt-in implicitly enables another | partial | risk |
| CNS-002 | P0 | Withdrawal misses unpublished descendants | partial | risk |
| CNS-003 | P0 | Withdrawal races a lease, commit or source receipt | not_run | risk |
| CNS-004 | P0 | Re-enabling capture releases old held content | observed | risk |
| CNS-005 | P0 | Temporary unavailability becomes permanent withdrawal | not_run | risk |
| CNS-006 | P0 | Missing source router authorizes publication | not_run | risk |
| CNS-007 | P0 | Manual link withdrawal is resurrected | not_run | risk |
| CNS-008 | P0 | Delete misses alternate views or re-import | partial | risk |
| CNS-009 | P0 | Kept assertion fabricates independence | observed | risk |
| CNS-010 | P0 | Kept assertion bypasses private ancestry | observed | risk |
| CNS-011 | P0 | Deletion is advertised as physical erasure | not_run | limit |
| CNS-012 | P0 | Withdrawal occurs after a request is submitted | not_run | limit |

### Jobs, budgets and concurrency

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| JOB-001 | P0 | Expired or replaced lease commits late | not_run | risk |
| JOB-002 | P0 | Retry mutates the job's intended inputs | not_run | risk |
| JOB-003 | P1 | Budget block parks a replacement attempt | not_run | risk |
| JOB-004 | P0 | Failed inference resets its budget reservation | partial | risk |
| JOB-005 | P1 | Budget boundary or clock change never resumes work | not_run | risk |
| JOB-006 | P0 | Shutdown closes profiles before workers finish | partial | risk |
| JOB-007 | P1 | A cancellation waits forever on a mutex or driver | not_run | risk |
| JOB-008 | P0 | Remote assignment exceeds the writer's grants | not_run | risk |
| JOB-009 | P1 | Same history schedules repeated derivations | not_run | risk |
| JOB-010 | P1 | Job bounds permanently starve unrelated sessions | not_run | risk |

### Local inference boundary

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| LLM-001 | P0 | Endpoint, proxy or redirect sends transcripts remotely | not_run | risk |
| LLM-002 | P1 | Server or configured model is unavailable | partial | risk |
| LLM-003 | P1 | Constrained output lands only in reasoning content | observed | backend_failure |
| LLM-004 | P0 | Model calls tools or returns executable authority | not_run | risk |
| LLM-005 | P0 | Transcript injection alters system authority | not_run | risk |
| LLM-006 | P0 | Prompt-only compatibility bypasses validation | partial | risk |
| LLM-007 | P1 | Request, output or response exceeds its budget | partial | risk |
| LLM-008 | P1 | Model emits plausible but semantically invalid JSON | observed | repaired |
| LLM-009 | P1 | Model returns code fences, partial JSON or extra choices | partial | risk |
| LLM-010 | P1 | Model quality is mistaken for correctness | partial | limit |

### Derivation, provenance and review

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| DRV-001 | P0 | Consumed but uncited input is corrected | not_run | risk |
| DRV-002 | P0 | Correction arrival order changes the result | not_run | risk |
| DRV-003 | P0 | Window reduction silently truncates source evidence | not_run | risk |
| DRV-004 | P0 | Claimed success is reported as verified success | not_run | risk |
| DRV-005 | P0 | Reviewer approves an obsolete revision | observed | risk |
| DRV-006 | P0 | Rejection or explicit edits disappear on rerun | not_run | risk |
| DRV-007 | P0 | Owner proof is copied or incompletely bound | partial | risk |
| DRV-008 | P0 | Ordinary review is called cryptographic human approval | not_run | limit |
| DRV-009 | P0 | Project or personal rollup violates aggregation policy | not_run | risk |
| DRV-010 | P1 | Candidate normalization collides or misses equivalents | not_run | limit |

### Promotion and cross-vault lifecycle

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| PRJ-001 | P0 | Target commit succeeds before source receipt then crashes | not_run | risk |
| PRJ-002 | P0 | Source changes between validation and target commit | not_run | risk |
| PRJ-003 | P0 | Same-vault promotion skips source consent checks | not_run | risk |
| PRJ-004 | P0 | Publication checks the wrong link or target revision | not_run | risk |
| PRJ-005 | P0 | Reader grants widen after projection commit | not_run | risk |
| PRJ-006 | P0 | Opposite-direction bridges deadlock | not_run | risk |
| PRJ-007 | P0 | Terminal withdrawal is defeated by concurrent heads | not_run | risk |
| PRJ-008 | P0 | Minimal withdrawal marker hides unrelated content | not_run | risk |
| PRJ-009 | P0 | Target-only replica returns projected content as current | not_run | risk |
| PRJ-010 | P0 | Local exclusion is mistaken for distributed atomicity | not_run | limit |
| PRJ-011 | P0 | Deleted source is restored as a fresh promotion implicitly | not_run | risk |
| PRJ-012 | P1 | Promotion is silently enabled or called accepted | partial | risk |

### Daemon, CLI and MCP

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| IPC-001 | P0 | Another user or ungranted client reaches private operations | partial | risk |
| IPC-002 | P1 | Generated admin configuration cannot authenticate | observed | repaired |
| IPC-003 | P1 | Successful void operation returns an error | observed | repaired |
| IPC-004 | P0 | Interrupted reply is confused with the next request | partial | risk |
| IPC-005 | P1 | Stale socket or long path blocks startup | not_run | risk |
| IPC-006 | P0 | MCP receives admin, hint or implicit raw-session authority | partial | risk |
| IPC-007 | P0 | Capability exists but scope arguments bypass it | observed | risk |
| IPC-008 | P1 | MCP protocol output is polluted or framing differs | partial | risk |
| IPC-009 | P0 | Export reports success before file finalization | partial | risk |
| IPC-010 | P1 | CLI and direct/daemon routes disagree | partial | risk |

### Cloudflare and archive provider

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| CF-001 | P1 | OAuth login or permissions are insufficient | observed | risk |
| CF-002 | P0 | Wrong account, binding or bucket is deployed | partial | risk |
| CF-003 | P0 | Bucket public access or debug logs expose data | partial | risk |
| CF-004 | P0 | Immutable object collision overwrites bytes | not_run | risk |
| CF-005 | P0 | Concurrent head updates both appear successful | not_run | risk |
| CF-006 | P0 | Crash between payload, journal, head and acknowledgement | partial | risk |
| CF-007 | P0 | DO cache loss cannot rebuild archive state | observed | risk |
| CF-008 | P1 | Provider quota or CPU/memory limit is exhausted | not_run | risk |
| CF-009 | P1 | Newly deployed Worker is not immediately reachable | observed | risk |
| CF-010 | P0 | Account loss, malicious rollback or withholding | not_run | limit |
| CF-011 | P0 | Relay bootstrap accepts another owner or vault | not_run | risk |

### Privacy, scale and operations

| ID | Priority | Failure case | Coverage | Kind |
| --- | --- | --- | --- | --- |
| OPS-001 | P0 | Logs, errors, temp files or reports disclose plaintext | not_run | risk |
| OPS-002 | P0 | Trusted device or OS account is compromised | not_run | limit |
| OPS-003 | P0 | Recalled text reaches a remote inference provider | not_run | limit |
| OPS-004 | P1 | Published-body cache grows without reclamation | not_run | limit |
| OPS-005 | P1 | Large archives make routine work unbounded | not_run | risk |
| OPS-006 | P1 | Sustained load overheats or exhausts the Mac | partial | risk |
| OPS-007 | P1 | Status labels imply unsupported completeness | partial | risk |
| OPS-008 | P0 | Cleanup removes the only copy or another task's files | partial | risk |
| OPS-009 | P1 | Public beta differs from the locally tested build | observed | risk |
| OPS-010 | P0 | Traffic metadata is mistaken for encrypted secrecy | not_run | limit |
| OPS-011 | P0 | Private profile files become accessible through permissions or links | partial | risk |

## How to extend and verify

Add a distinct trigger and observable failure instead of speculative duplicate
rows. Query the JSONL for full details before scheduling work. For each new
execution, record exact source, environment, fault injection, expected outcome,
actual outcome and private artifact reference. A test that merely exists, a
source-level fix or a passed build is not a scenario pass.

Apply cross-cutting variations where meaningful: offline/online, first run and
restart, crash immediately before/after commit or acknowledgement, concurrent
heads, lease/policy/grant changes, same-vault/cross-vault, current/old/unsupported
formats, tiny/at-bound/over-bound inputs and replay/rebuild order. Do not multiply
all combinations into a speculative test matrix. No finite list proves absence
of security flaws, provider outages or implementation defects.
