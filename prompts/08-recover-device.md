# Repair search or rebuild a replica

Use this after a search/index problem, lost local database, or long offline absence.

~~~text
Recover the specified Capsule Memory device state without losing unsynced work.

Inputs: CHECKOUT/SOURCE_REVISION, BINARY, existing DEVICE_HOME, intended scope,
observed failure, availability of protected profiles/backups, proposed NEW_HOME,
EXECUTION_ROUTE and ALLOWED_ACTIONS. Provide custody/location, never key contents.

Read AGENTS.md, prompts/README.md, docs/run-v1.md section "Recovery and cleanup",
docs/go-client.md section "Manual synchronization and recovery", and docs/renaming.md.

1. Classify the problem: rename/path mismatch, derived search damage, offline lag,
   missing database, missing keys, or relay cache failure. Preserve the original home.
   Do not initialize a new vault, delete SQLite/WAL/outbox, or reset owner pins.
2. For pre-rename profiles, use explicit --home with the actual previous path.
   There is no automatic migration. Inspect no unrelated private directories.
3. For search repair, use the documented rebuild command only when authorized.
   It rebuilds derived indexes and must preserve revisions, cursors and pending writes.
4. For offline lag, use bounded manual sync and record coverage/pending progress.
   Reuse exact operation identities; do not treat missed notifications as data loss.
5. For an authorized clean replica, stop processes using the source identity and
   follow the runbook's private copy of device.json to a new empty home. Replay
   retained authorized history; do not discard the original pending outbox.
   Never operate both copies concurrently as independent devices. Normal additional
   machines require independent enrollment instead.
6. Distinguish device rebuild from relay cache rebuild; never delete archive objects.
   A revoked identity cannot perform current archive reads. Losing all content keys
   and recovery copies is unrecoverable; report this rather than invent a reset.

Return the selected recovery path, preserved original/pending state, actual index
or replay evidence, and unverified steps. Retained local data and backups remain
private; do not attach them to an issue or send them to a cloud test runner.
~~~
