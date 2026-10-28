# Development

Use the [explicit check gate](#explicit-check-gate) for source quality and the [maintainability contract](#maintainability-contract) for implementation and review. Dependency locks are now checked in; earlier delivery reports retain their historical meaning. Setup checks do not establish product runtime acceptance; see [verification](verification.md).

## Execution boundaries

Apply the installed host policy and current task authorization. On the Mac, read `~/.agents/host-policy.md` and `~/.agents/skills/execution-policy/SKILL.md`; authorized local validation is allowed. For writing, load the installed `~/.agents/skills/i-have-adhd/SKILL.md` (or the account-equivalent path) freshly before GitHub/Linear writes. Do not import OVH restrictions onto a different host.

On the shared OVH host, read `/srv/t3code/.agents/skills/execution-policy/SKILL.md` before preparing any project job. Installs, project validators, tests, typechecks, builds, benchmarks, and runtimes remain blocked until an execution route and per-job isolation are authorized.

On OVH, do not substitute direct host execution, another account, an old runner, or unapproved CI. A separate directory, worktree, normal T3 thread, or systemd service is not isolation. This setup does not authorize Boat, cloud provisioning, paid usage, or background services.

Documentation authoring and read-only file/Git inspection are permitted within the current task. Record missing checks clearly. On OVH, do not run a skill validator or schema checker merely because it is small.

## Language and library choices

V1 targets Go 1.24.0 and pins the official Go MCP SDK v1.4.0, modernc SQLite v1.39.1, and Wrangler v4.129.1 via npm with Node 22+. See the implementation runbooks for source evidence and platform limits. The table below separates source choices from pending acceptance; installed compatibility remains unverified.

| Boundary | Current source choice | Pending evidence or deferred work |
| --- | --- | --- |
| Local CLI | Go 1.24.0, standard packages, explicit construction | Platform builds; background service deferred |
| MCP | Official Go SDK v1.4.0; fixed-scope stdio process | SDK runtime behavior and specific harness compatibility |
| Local data | modernc SQLite v1.39.1 and FTS5, explicit SQL | Runtime FTS5, packaging, backups, migrations |
| Relay | TypeScript, native Workers/DO APIs, Wrangler v4.129.1, npm | No-emit typecheck and formatter gate; provider acceptance pending |
| Cryptography | AES-256-GCM, Ed25519, explicit v1 framing | Runtime interoperability, security review; automated key distribution deferred |

No general web framework, ORM, vector database, distributed broker, or frontend is needed for the first slice. Dependencies must earn their maintenance cost. The [Go SDK list](https://modelcontextprotocol.io/docs/sdk) is the source for MCP SDK support, not a promise about every harness feature.

## Go practice

Keep domain behavior independent of transport and harness details. The current entrypoint is `cmd/capsule-memory`, with domain code under `internal/memory`; extend cohesive packages rather than adding an empty clean-architecture tree. Consumer-owned interfaces belong at storage, crypto, and transport boundaries where testing or alternate implementations require them.

Use explicit errors and transactions. Keep context cancellation and shutdown observable. Bound goroutines, buffers, retries, body sizes, and archive batches. Avoid global state, hidden work in constructors, reflection-heavy plumbing, speculative caches, and optimization without measurements.

Use standard formatting, descriptive operation errors, and focused documentation for exported contracts. Follow [Go review comments](https://go.dev/wiki/CodeReviewComments); [Effective Go](https://go.dev/doc/effective_go) remains useful for core idioms but does not cover all modern Go features.

Pin dependency versions and document platform implications when modules are introduced. Verify FTS5 at the driver/build boundary, not just through an SQL example. Never claim portable single-binary delivery before checking native-library and keystore dependencies.

## Testing practice

Design tests around durable outcomes: lost acknowledgements, crashes between writes, replay, concurrent offline changes, deletion, rekeying, cross-scope input, and search provenance. Use synthetic content and deterministic clocks where time matters. Do not write tests that merely mirror a function's implementation or assert documentation wording.

With the authorized route required by the actual host, run the check gate below. Add focused regressions and race testing for affected concurrency where supported. Go/TypeScript contract parity requires separate vectors; typechecking alone cannot prove it.

Provider conformance and quota/hibernation behavior need controlled integration jobs against explicitly authorized test resources. Emulator success alone cannot prove real R2 conditional-write or billing behavior. See [verification](verification.md) for acceptance scenarios and evidence format.

## Harness configuration

`AGENTS.md` is the shared project entrypoint. `CLAUDE.md` imports it, avoiding a second rulebook. Codex discovers project skills under `.agents/skills`; Claude uses the `.claude/skills` symlink. If a filesystem or harness cannot follow the symlink, use the canonical skill paths explicitly rather than maintaining divergent copies.

`.agents/personas` contains canonical engineer and reviewer guidance. Claude's `.claude/agents` files are thin native adapters that instruct a delegated worker to read that guidance. They do not select a model, bypass permissions, launch automatically by policy, or authorize additional concurrency. Codex can read the canonical persona directly; no unsupported native persona registration is implied.

The adapter format follows [Claude subagent documentation](https://code.claude.com/docs/en/sub-agents); shared instructions follow [Claude memory/import documentation](https://code.claude.com/docs/en/memory) and [Codex AGENTS.md](https://learn.chatgpt.com/docs/agent-configuration/agents-md). Discovery is documented by the providers but has not been exercised in a fresh project session. Do not restart active harness sessions to load these files.

Default to one agent. Do not create new top-level T3 threads, workers, hooks, MCP server registrations, or scheduled tasks without the relevant user authorization. The project defines future runtime hooks; it does not install account-policy enforcement hooks.

## Writing and delivery

Use main-point-first, short paragraphs, and a few useful bullets. On this account, freshly load the required writing skill before each GitHub or Linear write. Keep the actual public-facing text concise and preserve evidence and material risks.

Before publishing, inspect the staged diff for personal data and credentials. `.gitignore` is a convenience, not a security boundary. Use synthetic fixtures only. Never put decryption keys into GitHub Actions or a remote review prompt.

## Maintainability contract

Implementation and review share this standard, including generated contributions:

- Expand critical control flow into ordinary multiline statements. Make validation, state transitions, error paths and acknowledgement order readable without decoding compressed chains of actions.
- Keep each function/file cohesive around a responsibility. Keep CLI/MCP/IPC adapters thin. Use meaningful operation and boundary names; avoid blanket renames, arbitrary line/file/function size limits, generic frameworks and speculative optimizations.
- Make error propagation, partial-write handling, resource cleanup and finalization explicit. Show who owns each transaction, lock and goroutine, when it ends, how cancellation propagates, and which durable event permits success. Preserve inspectable errors without logging sensitive data.
- Retrieve candidates, current heads, deletion/approval/conflict state and coverage from one consistent snapshot where they form one answer. Apply scope, current-revision and visibility filtering before the returned-result limit. Bound scanning/pagination and repeated work; report incomplete coverage rather than hiding eligible results behind filtered historical IDs.
- Give relay recovery and publication named phases: load head, traverse bounded history, validate, apply, checkpoint; stage immutable payloads/journal, conditionally publish the head, then acknowledge. Keep ordering and recovery paths visible and preserve exact protocol bytes.

Review a complete affected operation and its failure/recovery paths. Concrete obscurity that prevents reliable review blocks acceptance even without a reproduced runtime defect. Subjective style preferences alone do not. Formatters and static checks complement human review; they do not prove cohesion, efficiency, security or durability. Separate mechanical formatting, structural refactoring and behavior fixes into logical commits.

## Explicit check gate

After separately authorized dependency setup (`go mod download` and `npm ci --prefix cloudflare --ignore-scripts`), run from any working directory:

```sh
/path/to/capsule-memory/scripts/check.sh --range BASE..HEAD
```

Replace both endpoints with explicit commits for the actual delivery. The default `all` phase checks Go formatting, vet, tests, build, relay no-emit TypeScript and Prettier checks, and every commit subject in the nonempty ancestor range. It reports source HEAD, dirty paths, tool versions and individual exit codes; retain the reviewed diff or patch identity alongside dirty-tree evidence. Missing tools/dependencies fail. Tests reporting `[no test files]` establish no behavioral coverage. Source checks use the current working tree; commit checks use the supplied immutable endpoints.

Targeted diagnostics: `--phase quality` (Go + relay), `--phase go`, `--phase relay`, or `--phase commits --range BASE..HEAD`. Targeted runs do not replace the full gate. Invalid arguments/ranges exit 2; failed/missing checks exit 1; only completed passing checks exit 0. Run `--help` for usage.

The gate never installs dependencies, rewrites source/manifests, generates Worker declarations, starts services or deploys. Go commands disable automatic module/toolchain downloads, use `-mod=readonly` with disposable manifest copies (and fail if checks require manifest changes), two package workers and `GOMAXPROCS=2`; tests have a two-minute per-package timeout. Build outputs live in a task-owned temporary directory removed on exit. Build caches outside the checkout may be populated; resolve missing modules separately. Caller `GOFLAGS` are cleared to prevent accidental module/output overrides. Source must be trusted before running its tests. Stop a stalled job and report it as incomplete; this script is not an OS resource sandbox.

The relay uses TypeScript 5.9.3 (Apache-2.0) for no-emit checks, Prettier 3.6.2 (MIT) for formatting inspection, and `@cloudflare/workers-types` 5.20260907.1 (MIT OR Apache-2.0), matching Wrangler 4.129.1's peer range and workerd compatibility date. These are development-only dependencies with update/lockfile maintenance costs. Native APIs need no routing framework. `tsconfig.json` explicitly loads Worker definitions without the DOM library or generated declarations, so missing Worker types fail. See [Cloudflare TypeScript guidance](https://developers.cloudflare.com/workers/languages/typescript/), [TypeScript noEmit](https://www.typescriptlang.org/tsconfig/noEmit.html), and [Prettier check mode](https://prettier.io/docs/cli#--check). Versions were verified against official npm metadata during setup; this is not a claim that they are the latest versions.

Verify the gate itself with `python3 scripts/check_test.py` on an authorized host with Git, Go, Node/npm and the reviewed local relay dependencies. Its disposable Git/module fixtures test observable success/failure and prerequisite handling without changing product source. Python 3 is only needed for this tooling test, not the gate.

## Commit and PR subjects

Use Conventional Commits for every new commit and the final PR title: `type: description`, `type(scope): description`, `type!: description` or `type(scope)!: description`. Accepted types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`. Use a nonempty description and scope when present. Explain breaking changes in the body when applicable. Do not rewrite historical commits merely to rename them.

Before committing, review `git diff --cached` for the exact staged contents, unrelated edits, generated artifacts and secrets; review dependency pins, integrity hashes and registry URLs too. Before delivery, check the actual commit range and final PR title against this syntax. The gate validates commit subjects; the reviewer checks the PR title and maintainability contract. Neither these commands nor this standard grants commit/push/merge authority.

Incremental stale-state refresh starts with its affected-record set and uses the
schema-v2 `revision_record` index for archive lookups. Opening an existing v2
profile installs this additive index without changing its schema version or
protocol bytes. The query-plan regression guards against a full archive scan on
every append; it imposes no machine-specific timing threshold.
