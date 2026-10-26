# Support

Start with the [runbook](docs/run-v1.md) and [current status](docs/v1-status.md). agent-memory is experimental source; no runtime checks have been completed, and no support SLA is promised.

## Find the relevant guide

| Symptom or question | Where to look |
| --- | --- |
| Dependency setup, local build, or synthetic E2E | [Run v1](docs/run-v1.md) · [E2E guide](e2e/README.md) |
| CLI flags, scope IDs, MCP launch, or staged skills | [Go client](docs/go-client.md) |
| Sync pending, incomplete coverage, 429/503 responses | [Manual sync](docs/go-client.md#manual-synchronization-and-recovery) · [Relay setup](docs/relay-setup.md) |
| Index rebuild, device recovery, or cleanup | [Recovery and cleanup](docs/run-v1.md#5-recovery-and-cleanup) |
| Design limits or missing features | [Docs index](docs/README.md) · [Roadmap](docs/roadmap.md) |

Synchronization is manual. A local write can succeed while publication is pending. On transient failures, retry the same sync with bounded backoff; preserve the device home and queued bytes. Do not reset an identity, delete a database, or regenerate ciphertext to make an error disappear.

## Ask for help

For non-sensitive bugs or usage questions, open a GitHub issue if available, or use your established maintainer contact. Include the source commit, OS, Go/Node versions, exact command with sensitive values replaced, and a minimal synthetic reproduction. State which checks actually ran and what failed.

Review diagnostics before sharing. Never upload SQLite/WAL files, device homes, private keys, recovery material, real memories, or retained E2E directories. For suspected vulnerabilities, use [private security reporting](SECURITY.md).

Feature requests should describe the workflow and desired outcome. The [contributor guide](CONTRIBUTING.md) explains how to propose a focused change.
