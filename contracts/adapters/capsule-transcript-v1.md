# capsule-transcript/v1

**Authored contract and synthetic examples, NOT EXECUTED.** This is an owner export
format, not a claim about any vendor's native transcript schema.

A UTF-8 file contains one complete JSON object and newline per record. The first
record is a session header; subsequent records are events for that same session.
Files are explicit owner paths; no URL, command, database, attachment or tool
reference is fetched or executed. An incomplete final line is held. Files over
4 GiB and lines over 8 MiB are refused/omitted with visible coverage.

```jsonl
{"capsule_import":1,"kind":"session","harness":"import","source_store_id":"synthetic-store","session_id":"synthetic-session","cwd":"/synthetic/project","title":"Synthetic example","generation":"g1"}
{"capsule_import":1,"kind":"event","session_id":"synthetic-session","event_id":"synthetic-event-1","seq":0,"role":"user","type":"message","text":"Keep the synthetic parser strict.","status":"final","native":{}}
```

Header identity is `{harness, source_store_id, session_id, environment?}`; imports
preserve it and compute the same session key as capture. `capsule_import` must be
1. `kind`, `harness`, `source_store_id`, and `session_id` are mandatory on a header.
Optional metadata includes harness_version, parent_session_id, lineage, cwd,
title, captured_at_basis and generation. No timestamp decides identity or conflict.

Events require session_id, seq, role, type and `status: final|partial`. `partial`
is held without checkpoint advancement. event_id is preserved; otherwise the
source generation and byte position determine identity. Optional fields are
parent_event_id, cwd, time, text, tool and native. `event` can carry the complete
redacted Capsule Event, preserving parts, duplicate views, tool linkage,
corrections and recall-origin metadata on a Capsule reimport. Unknown envelope
fields, malformed values, duplicate JSON keys and unknown versions quarantine.
Native/tool extras are dropped unless tool detail is explicitly full.

Origin harness labels preserve the exporter's namespace. They contain 1–128 ASCII
characters matching `[A-Za-z0-9][A-Za-z0-9_.:/-]{0,127}`. The adapter registry remains
allowlisted to claude, codex, t3 and import; an arbitrary origin does not install
or select an adapter. Reimports preserve suppression identity without relabelling.
Events admitted through historical import need a valid RFC3339 time at or after
the explicitly supplied `since`; missing or invalid times never bypass the bound.

Source configuration is either `{"files":["/absolute/export.jsonl"]}` or
`{"dir":"/absolute/exports","glob":"*.jsonl"}`. A directory glob cannot include
path separators. Capture policy, approved project attribution, consent, redaction,
withdrawn outbox holds and suppression checks apply equally to imports.

The sibling fixtures are hand-authored documentation. No fixture was parsed,
validated, compiled or run. Native examples use synthetic version labels, which
must never be advertised as compatible vendor releases.
