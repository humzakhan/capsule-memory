# Documentation

Start with [Run v1](run-v1.md) for a synthetic local evaluation. The source is experimental: installs, builds, formatting, typechecks, tests, and E2E are **NOT RUN**. [Delivery status](v1-status.md) is the evidence record; architectural targets are not promises of implemented behavior.

## Use and evaluate

| Guide | Purpose |
| --- | --- |
| [Run v1](run-v1.md) | Dependency resolution, build, local E2E, private demo, recovery, cleanup |
| [Go client](go-client.md) | CLI flags/results, fixed-scope MCP, local skill approval and text activation |
| [Relay setup](relay-setup.md) | Authentication, archive publication, limits, provider assumptions |
| [E2E journey](../e2e/README.md) | Authored synthetic local assertions and failure artifacts |
| [Support](../SUPPORT.md) | Troubleshooting navigation and safe bug reports |

## Understand the design

| Guide | Purpose |
| --- | --- |
| [Architecture](architecture.md) | Full system target; explicitly includes deferred components |
| [V1 contract](v1-contract.md) | Implemented slice's specialized wire and behavior contract |
| [Protocol](protocol.md) · [Draft contracts](../contracts/README.md) | Broader protocol design and synthetic schemas/examples |
| [Threat model](threat-model.md) · [Security](../SECURITY.md) | Trust boundaries, known limits, private reporting |
| [Decisions](decisions/) · [Sources](sources.md) | Accepted choices, trade-offs, and provider references |

## Contribute and assess evidence

| Guide | Purpose |
| --- | --- |
| [Contributing](../CONTRIBUTING.md) · [Development](development.md) | Contributor flow, dependencies, execution boundaries |
| [V1 status](v1-status.md) · [Source inspection](v1-inspection.md) | Authored inventory, historical findings, missing runtime evidence |
| [Verification](verification.md) | Durability, privacy, protocol, and failure scenarios |
| [Roadmap](roadmap.md) | Remaining implementation and acceptance work |
| [Agent guidance](../AGENTS.md) | Repository rules for coding agents |
