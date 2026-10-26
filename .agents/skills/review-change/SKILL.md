---
name: review-change
description: Review a concrete capsule-memory diff or design revision for data loss, privacy, synchronization, skill trust, Go correctness, and provider assumptions. Use when review is requested; does not launch another agent.
---

# Review a change

Read the [reviewer persona](../../personas/reviewer.md), `AGENTS.md`, and the affected design sections. Start from the requested commit or diff; do not review a remembered earlier state.

1. Record the base, head, and scope. Identify changed invariants and the path to an acknowledgement, query result, or activated skill.
2. Inspect the implementation and its callers. Trace cancellation, transaction boundaries, duplicate delivery, offline replay, revocation, deletion, and scope enforcement where affected.
3. Compare claimed evidence with [verification](../../../docs/verification.md). Separate source inspection from executed checks. Do not run tests or launch workers without the authorized route and task scope.
4. Report concrete findings first: severity, location, trigger, impact, and a focused correction. Label design questions and unverified risks as such. Avoid style-only findings unless they obscure correctness or violate an explicit rule.
5. State the reviewed source identity and remaining coverage limits. If there are no findings, say so without implying runtime or security approval.

For protocol changes, compare Go/TypeScript semantics, draft schemas, limits, and fixtures. For cryptography, require exact reviewed signing bytes and real vectors. For Cloudflare claims, consult current primary documentation.

Return the review in the local conversation unless posting is requested. Before every GitHub or Linear write, invoke the account's required writing skill freshly and check the actual outgoing text. Do not add a published review, merge, or code fix as a side effect of read-only review.
