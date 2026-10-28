# First-time evaluation

Use this when the user wants to try the project from source, before creating a real vault.

~~~text
Evaluate Capsule Memory from its trusted repository:
https://github.com/humzakhan/capsule-memory

Inputs:
- CHECKOUT and SOURCE_REVISION: fill in the trusted checkout and commit.
- EXECUTION_ROUTE and ALLOWED_ACTIONS: fill in existing authorization;
  otherwise produce instructions only.
- Target OS and desired binary location: fill in.

Read AGENTS.md, prompts/README.md, docs/v1-status.md, and docs/run-v1.md.
Capsule Memory stores and synchronizes existing agent memories and skills;
the agents decide what to remember. Do not describe it as an autonomous memory manager.

1. Establish the source commit, intended platform, and permitted execution route.
   Inspect prerequisites without installing or running workloads unless allowed.
2. Explain the four roles: central relay, owner device, enrolled device, MCP client.
   Choose the local synthetic E2E for a first evaluation, not cloud deployment.
3. Follow runbook steps 1–2 exactly in the allowed environment. Resolve real Go/npm
   dependencies and review generated sums/locks. Do not invent frozen installs.
   No curl-to-shell installer, prebuilt release, or registry package is assumed.
   If the user also authorized installation, follow docs/install.md to install
   the selected release machine-wide and verify PATH/revision/rollback. An
   evaluation alone leaves the binary in disposable storage, not a global path.
4. Run only the single focused E2E if authorized. It uses local Wrangler/DO/R2,
   disposable synthetic profiles and a built CLI. Preserve actual output and failures.
   Stop after a concrete failure; do not add test suites or claim skipped cases passed.
5. Explain cleanup and retained failure artifacts without deleting unrelated paths.
   Do not create real memories, provision cloud resources, or initialize a real vault.

Return actual commands/results, source identity, unresolved failure or NOT RUN checks,
and whether to proceed to prompts/02-owner-setup.md and prompts/03-relay-setup.md.
An emulator pass does not establish production durability, security, cost, or platform support.
~~~
