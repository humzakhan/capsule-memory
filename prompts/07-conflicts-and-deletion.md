# Conflicts, deletion, and explicit restoration

Use this when operating on particular records, not as a general cleanup prompt.

~~~text
Handle the specified Capsule Memory record lifecycle operation.

Inputs: CHECKOUT/SOURCE_REVISION, BINARY, DEVICE_HOME, SCOPE_ID, RECORD_ID,
requested resolution/delete/restore outcome, permission to inspect preserved text,
EXECUTION_ROUTE and ALLOWED_ACTIONS. Never infer destructive authority from a conflict.

Read AGENTS.md, prompts/README.md, docs/go-client.md section "Offline writes,
search, and conflicts", and docs/v1-contract.md.

1. Inspect normal get and relevant sync coverage in the allowed environment.
   Fetch --history only when the user authorized preserved/deleted text inspection;
   keep that text private. Identify concurrent heads and tombstone ancestry.
2. Never select a winner by wall clock, delivery order, or plausibility.
   Preserve all unresolved branches and cite their revision IDs/provenance.
3. For an authorized merge, use store with the existing record ID and explicit
   reviewed comma-separated parents plus agreed replacement text. Re-read current
   heads if new revisions arrived. Do not approve an ambiguous executable skill merge.
4. Delete only the explicitly requested record(s). A tombstone suppresses ordinary
   retrieval, including stale concurrent edits. Deletion retains history and is
   not physical erasure or revocation of already learned content.
5. Restore only as an explicit user-requested operation. Use restore --text for
   an ambiguous replacement as documented; never revive stale text automatically.
6. If sync is allowed, publish/replay and check normal retrieval versus explicit
   history. Do not compact/prune ancestry, drop tombstones or reset a replica.

Return the reviewed operation and resulting head/deletion state, preserved branch
references, local versus remote evidence, and any NOT RUN checks. Unresolved intent
must remain unresolved rather than being silently merged.
~~~
