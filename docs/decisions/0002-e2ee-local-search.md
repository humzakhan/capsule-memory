# 0002: End-to-end encryption and device-only search

**Status:** accepted requirement; cryptographic protocol pending review.

## Decision

Only trusted enrolled devices decrypt memory, skill, and summary content. Search indexes and semantic processing stay on those devices. Use local SQLite FTS5 before considering embeddings.

Cloudflare stores ciphertext and opaque routing metadata. GitHub stores the application's source and instructions; optional personal vault exports and skill artifacts are encrypted before upload. No Actions job receives content keys.

## Why

The owner explicitly requires end-to-end encryption and local search indexes. A central plaintext catalog, vector index, or summarizer would violate that requirement. A custom searchable-encryption system adds complexity without evidence it is needed.

## Consequences

A complete local search index requires reading and decrypting the authorized archive on each new device. Partial indexes must report their coverage. Full-text indexes contain sensitive text and still consume local space; hot/cold payload eviction does not remove that cost.

Embeddings, if later justified, are generated locally, treated as sensitive derived data, and versioned by model/chunking. They are not shared by default. Summaries cite exact source revisions and become stale when sources change.

OS disk encryption and permissions protect local plaintext; archive E2EE alone does not encrypt SQLite, swap, or backups. Text returned to a cloud-hosted agent still reaches its inference provider, so strict device-only plaintext also constrains harness/model selection.
