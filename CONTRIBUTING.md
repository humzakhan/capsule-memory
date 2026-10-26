# Contributing

Start with [AGENTS.md](AGENTS.md), [development](docs/development.md), and the [roadmap](docs/roadmap.md). This is a private project scaffold; implementation has not begun.

## Make a change reviewable

1. Describe the user-visible outcome and the invariant affected.
2. Change the smallest coherent boundary. Update protocol examples and decisions when behavior changes.
3. Inspect the diff for secrets, accidental data files, and unrelated changes.
4. Run appropriate checks only through an execution route already authorized for this job. Record exact source and results; otherwise state what was not run and why.
5. Provide a brief handoff with evidence and material limitations.

Use the [pull request template](.github/pull_request_template.md) when a PR is requested. A contribution does not authorize publishing, enabling CI, paid usage, deployment, or additional workers.

## Data and dependencies

Use synthetic fixtures only. Keep SQLite files, plaintext exports, keys, and credentials outside the working tree. Do not introduce a dependency without a concrete requirement, maintenance rationale, license review, and supported-platform implications.

The exact Go toolchain, SQLite driver, JavaScript package manager, and dependency versions remain implementation decisions. Pin them with the first corresponding code change rather than adding unused manifests now.
