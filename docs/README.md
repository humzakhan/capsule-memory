# Capsule Memory documentation

Capsule Memory is a lightweight utility for securely storing and synchronizing existing agent memories and skills across machines and harnesses. The shared encrypted archive centralizes durable storage; plaintext, keys, and search remain device-local. Agents and harnesses decide what to remember and use.

Start with the [plain-language overview](../README.md) and [current release evidence](releases.md), then use [Run v1](run-v1.md) for a sample-data evaluation. Bounded Mac/Cloudflare QA and beta.2 fresh-checkout validation have executed; broader security, platform and compatibility acceptance remain incomplete. Dated [delivery status](v1-status.md) preserves the original source-only evidence; it is not the current release verdict.

## Set up with an agent

Use [the prompt index](../prompts/README.md) for evaluation, setup and operation tasks, including [optional session setup](../prompts/11-session-history.md). [Agent discovery](agent-discovery.md) maps roles, reading paths and public-release prerequisites. The root [llms.txt](../llms.txt) and [project capability manifest](../agent-manifest.json) are concise entry points, not automatic installation or access grants.

## Use and evaluate

| Guide | Purpose |
| --- | --- |
| [Releases](releases.md) · [Changelog](../CHANGELOG.md) | Initial source beta, known blockers and tested-release gates |
| [Run v1](run-v1.md) | Dependency resolution, build, local E2E, private demo, recovery, cleanup |
| [Run sessions](run-session.md) · [Session status](session-status.md) | Optional session source, owner setup and pending acceptance |
| [Rename compatibility](renaming.md) | Retained protocol bytes and explicit reuse of pre-rename profiles |
| [Go client](go-client.md) | CLI flags/results, fixed-scope MCP, local skill approval and text activation |
| [Relay setup](relay-setup.md) | Authentication, archive publication, limits, provider assumptions |
| [E2E journey](../e2e/README.md) | Authored synthetic local assertions and failure artifacts |
| [Support](../SUPPORT.md) | Troubleshooting navigation and safe bug reports |

## Understand the design

| Guide | Purpose |
| --- | --- |
| [Architecture](architecture.md) | Full system target; explicitly includes deferred components |
| [Session history](session-history.md) | Preserved proposal plus accepted implementation amendments; runtime NOT RUN |
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
| [Failure-case inventory](failure-cases.md) · [Queryable JSONL](failure-cases.jsonl) | Stable case IDs, triggers, consequences, priorities and recorded coverage |
| [Roadmap](roadmap.md) | Remaining implementation and acceptance work |
| [Agent guidance](../AGENTS.md) | Repository rules for coding agents |
