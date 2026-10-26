# Operate an existing node

Use this for capture, local retrieval, manual publication, and reconnect catch-up.

~~~text
Use an existing Capsule Memory node for the user-authorized memory workflow.

Inputs: trusted CHECKOUT/SOURCE_REVISION, BINARY, private DEVICE_HOME,
granted SCOPE_ID, requested capture/query, synthetic OR real-data permission,
EXECUTION_ROUTE and ALLOWED_ACTIONS. Keep real content out of logs and this prompt
unless the user's existing data policy explicitly permits it.

Read AGENTS.md, prompts/README.md, docs/go-client.md, and docs/v1-status.md.
Use only real documented CLI/MCP operations.

1. Confirm existing identity, relay and scope. Do not initialize a replacement vault
   or widen access. Check status/coverage only if execution is permitted.
2. Capture only the user-approved content with store --scope ... --text ...,
   or fixed-scope memory_store. Preserve the returned record/revision identity
   and provenance. Local durable success and published:false are expected before sync.
3. Retrieve using recall's local FTS5 query or explicit record get. Report coverage,
   provenance and conflicts; partial local coverage is not an exhaustive archive answer.
   Do not claim cold search without replaying the authorized archive/index locally.
4. On an allowed network connection, run sync --scope SCOPE_ID. Preserve exact queued
   envelopes across retries. Use bounded backoff for transient 429/503/network failure;
   do not regenerate operations/ciphertext, delete queues, or retry indefinitely.
5. Record pending count and caught-up/coverage evidence. Large archives may require
   repeated bounded sync invocations; distinguish failure, partial progress and completion.
   For another device's visibility, check that independently authorized replica.

If asked for a daemon, timer, instant push/pull or universal hooks, explain that v1
implements manual sync only. Provide the manual workflow and a separately scoped
future integration plan; do not install a scheduler/service or claim one is running.
Do not silently import every session, transcript, skill, or repository file.

Return record IDs/provenance without unnecessary content, local versus remote state,
actual observed evidence and NOT RUN checks. The agent, not Capsule Memory, decides
what to remember and when to call the utility.
~~~
