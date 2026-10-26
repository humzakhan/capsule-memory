# Adapter examples — authored, NOT EXECUTED

These contain no real transcript, credential or accepted compatibility pin.
`fixture-only` is a synthetic marker, never a vendor release claim.

| Example | Intended observation, not a passing result |
| --- | --- |
| claude-synthetic.jsonl | Version gating; string/blocks; tool body omitted by default; parent/call linkage |
| codex-synthetic.jsonl | Exact prompt duplicate within a turn, including a poll boundary; both positions retained; one view has no text |
| t3-bounded-synthetic.json | Strong native binding; inherited row omitted; fixed protocol-2 bounded shape |
| import-origin-synthetic.jsonl | Arbitrary exporter origin retained; partial tail held; reimport suppression applies |

Owner acceptance must also cover unknown version/shape, duplicate JSON key,
partial newline, 8 MiB line, 4 GiB file, prefix rewrite/rotation, source loss,
redaction offset accuracy, excluded projects, cross-project sessions, source
withdrawal during upload, malformed token permissions, redirect refusal,
source suppression with a changed store ID, concurrent capture, all-manifest
session deletion, out-of-order corrections and rebuild. None has been executed.

The T3 fixture includes a `shell` companion snapshot for project fallback: an
empty worktreePath resolves through thread.projectId to projects[].workspaceRoot.
The primary source is t3code d8d037e, packages/contracts/src/orchestrationV2.ts
lines 367, 374 and 1936–1938, and orchestrationProject.ts lines 8–11. It is pinned
schema evidence, never a live-harness compatibility result. Event timestamps
support explicit historical bounds in these synthetic examples.
