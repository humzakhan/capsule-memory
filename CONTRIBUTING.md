# Contributing to agent-memory

Make a small, reviewable change with synthetic data and honest evidence. The v1 source is experimental: installs, builds, formatters, typechecks, tests, and E2E remain **NOT RUN**. Read [delivery status](docs/v1-status.md) before relying on behavior.

## Start a contribution

1. Read [AGENTS.md](AGENTS.md), [development guidance](docs/development.md), and the relevant [design decision](docs/decisions/). For larger behavior changes, discuss the outcome and affected boundary with the maintainer first.
2. Create a focused branch from `main` in your checkout (or a fork when available). Follow the [owner-machine runbook](docs/run-v1.md) for dependency setup and the synthetic local journey.
3. Update source, relevant docs, and synthetic fixtures together. Explain changes to protocol bytes, migrations, durability, scope authorization, or skill trust.
4. Inspect the diff for secrets and unrelated changes. Run appropriate checks in your permitted environment; record the source revision, commands, and actual results. Mark skipped checks **NOT RUN** with a reason.
5. Submit a pull request using the [template](.github/pull_request_template.md), with the concrete outcome first, evidence, and material limits. Link longer design context rather than pasting logs.

## Data and privacy

Use synthetic records, identities, and disposable device profiles only. Keep SQLite/WAL files, private keys, recovery material, decrypted exports, and real memories outside the checkout. Never attach device homes or retained E2E state to an issue or PR.

Share only manually reviewed, redacted diagnostics. Public IDs, paths, and metadata can still reveal sensitive context. Report vulnerabilities through the [security policy](SECURITY.md), not a public issue.

## Dependencies and verification

The source pins Go 1.24.0, the official MCP SDK v1.4.0, `modernc.org/sqlite` v1.39.1 (with libc v1.66.10), and Wrangler v4.129.1; the relay requires Node 22+. See [go.mod](go.mod) and [cloudflare/package.json](cloudflare/package.json).

There is no `go.sum` or npm lockfile yet. The owner's authorized dependency resolution must generate real files for review. Do not fabricate checksums or describe the initial npm install as frozen. New dependencies need a concrete requirement, maintenance and license rationale, and platform implications.

Use [verification scenarios](docs/verification.md) to select meaningful checks. The [E2E journey](e2e/README.md) is authored, not passed. Source reading and `git diff --check` establish only inspection and whitespace evidence; neither proves runtime behavior or security.

## Execution environments

Contributors can use their own trusted development machines under their own permissions. Local-only emulation in the runbook does not require a cloud deployment. Provisioning, spending, CI changes, or production use require separate authorization.

Agents on the shared OVH account must follow [AGENTS.md](AGENTS.md) and the account execution policy: project workloads remain blocked without an approved exception or authorized per-job isolation. A branch, directory, or thread does not provide isolation. Documentation work does not authorize running validators or tests there.

## License

Contributions are made under the project's [MIT license](LICENSE). Preserve third-party notices and identify any newly introduced license obligations.
