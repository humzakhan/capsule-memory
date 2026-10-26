---
name: sqlite-storage
description: Design or change agent-memory SQLite transactions, migrations, outbox persistence, local search, backup, and rebuild behavior. Use for local storage changes rather than Cloudflare database work.
---

# SQLite storage

Read [architecture](../../../docs/architecture.md), [protocol](../../../docs/protocol.md), and the affected [verification scenario](../../../docs/verification.md). SQLite is both a durable local write store and a host for rebuildable indexes; those roles must remain distinct.

1. Identify durable rows, derived rows, and the transaction boundary. Local revision plus exact outbound payload must commit atomically before local success. Incoming validated revisions plus applied cursor must also commit together.
2. Plan WAL mode, `synchronous=FULL`, foreign keys on every connection, busy handling, and a bounded write owner. Verify driver behavior rather than assuming PRAGMAs are global. Do not make a network call while holding a write transaction.
3. Keep FTS5 scoped at query time and consistent with visibility and tombstones. Index updates may lag only if stale or deleted hits are filtered before returning them. Treat snippets, tokens, and FTS side tables as sensitive plaintext.
4. Make migrations versioned and crash-safe. Require a supported backup and recovery path for destructive changes. An index rebuild must not delete the outbox or pending local revisions.
5. Exercise crash and duplicate-delivery boundaries through authorized verification. Record the source revision, commands, and actual results.

Use the SQLite backup API or another documented consistent snapshot method; copying a live main database file can omit WAL state. OS disk encryption protects local files at rest; application E2EE alone does not encrypt SQLite or swap.

A new device needs to decrypt the authorized archive to build full local search. Keep that progress visible. Do not present an incomplete local index as a complete cold-archive answer.

Sources: [WAL](https://www.sqlite.org/wal.html), [synchronous](https://www.sqlite.org/pragma.html#pragma_synchronous), [FTS5](https://www.sqlite.org/fts5.html), [backup](https://www.sqlite.org/backup.html).
