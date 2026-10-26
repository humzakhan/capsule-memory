# Focused v1 source inspection

This is one coordinator inspection of the initial combined Go and Cloudflare source, followed only by checks of concrete fixes. It is **source evidence, not compiler, runtime, or security-audit evidence**. All workloads remain NOT RUN.

## Findings and required corrections

1. **Parser agreement:** `cloudflare/src/wire.ts` initially used plain `JSON.parse`, while Go rejects duplicate object keys. A device-signed outer payload with duplicate keys could be published by the relay and then rejected by every Go replica. Reject duplicate keys (including equivalent escaped key names), excessive nesting, and malformed UTF-8 before relay publication; add this negative case to the existing E2E journey.
2. **Deleted content:** `internal/memory/store.go` initially returned plaintext payloads in deleted record heads even when history was not requested. Normal `get`/MCP fetch must retain deletion metadata while withholding deleted payload text; explicit history remains available. Extend the existing deletion scenario to assert this distinction.
3. **Required skill transition:** the initial artifact unconditionally disabled activation. Implement the existing approval's narrow `text/markdown` / `read-as-text` activation: verify the exact owner-signed scope/revision/digest authorization, permit only the unique visible current skill revision, and make changed/deleted/conflicting skills inactive. This is a local active text selection exposed through CLI/MCP, not executable installation, scripts, or automatic harness registration. Keep incoming unapproved skills staged. Add one positive activation and invalidation segment to the existing E2E suite.

## Boundaries examined

Go write transactions contain immutable revisions, exact signed ciphertext, ancestry, visibility, and outbox. AES-256-GCM context and Ed25519 signatures use the contract's exact bytes. Go pins owner control, processes historical grants at publication, fixes scope at MCP launch, rejects redirects, and applies incoming revisions/cursors atomically.

The Worker validates current device/scope grants and exact request bytes. R2 immutable object and journal creation precede conditional durable head publication; acknowledgement is not based on DO state. Coordinator caches rebuild from retained head-reachable history, and duplicate receipts use durable publication evidence. Provider timeouts remain ambiguous and retries preserve transport identity.

These observations support the implementation approach but do not prove that builds, SQL, SDK handshakes, cross-language signatures, provider operations, or E2E scenarios work. Native keystores, background service/IPC, WebSockets, automated rotation/key distribution, summaries, and production resource/CPU/cost behavior remain outside this implemented slice.

## Fix ownership and evidence

The initial inspected source is `3b73b6e8ea28c833fb64cd2a6a75204b885b35ea`. The coordinator rechecked the concrete corrective diff against these findings: relay decoded-key/depth validation, deleted-fetch payload suppression, transactional exact-owner-release activation/current-state computation, and the corresponding existing E2E assertions. All three source corrections are present; runtime acceptance is still NOT RUN.

Two bounded GPT corrective tasks authored the changes with disjoint ownership and are settled. No additional broad review or unit-test suite was added. The run ledger records source commits, decisions, and verified worker closure; Git history records the final fix revision.
