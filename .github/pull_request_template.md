<!-- Use a Conventional Commit PR title: type(scope): description or type: description; optional ! before :. -->
## Outcome

Describe the problem and resulting behavior in a short paragraph.

## Evidence

- Reviewed base/head, working-tree patch identity if dirty, and explicit delivery commit range:
- `scripts/check.sh --range BASE..HEAD`: tool versions, exact exit/results (including failures):
- Checks not run and the reason:

## Review

Confirm the [maintainability contract](../docs/development.md#maintainability-contract), reviewed staged/delivery diff, Conventional Commit subjects and final PR title. Explain any unmet check or acceptance criterion. Formatter success does not establish maintainability or runtime acceptance.

## Risks and decisions

State affected durability, privacy, protocol, migration, or skill-trust boundaries. Link the updated decision or design when relevant. For a documentation-only change, say that runtime behavior remains unverified.
