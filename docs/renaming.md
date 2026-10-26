# Capsule Memory rename compatibility

Capsule Memory is the current product name; `capsule-memory` is the repository,
CLI, MCP server identity, and new default profile-directory name. The canonical
Go module is `github.com/humzakhan/capsule-memory`, with CLI source under
`cmd/capsule-memory`. This is a source and branding rename, not a protocol or
private-data migration. Runtime checks remain **NOT RUN**.

## Fixed legacy protocol bytes

These exact v1 domain labels intentionally retain the previous name:

- `agent-memory/aead/v1`: authenticated encryption additional data.
- `agent-memory/envelope/v1`: device-signed encrypted envelopes.
- `agent-memory/control/v1`: owner-signed public controls.
- `agent-memory/request/v1`: authenticated relay requests.
- `agent-memory/skill-approval/v1`: local owner skill approvals.

Go, TypeScript, and the E2E source retain their exact framing, including the
`agent-memory/${kind}/v1` template, encodings, separators, and final newlines.
The contract and client guide retain these labels to describe the actual bytes.
They are compatibility identifiers, not current branding. Changing them would
invalidate authentication of existing archived envelopes, saved controls,
approvals, queued outbox bytes, or recorded signatures. This rename changes no
keys, schema fields, content, versions, or ciphertext and requires no
re-encryption. `AM_*` environment variables and `X-AM-*` headers also remain
compatibility identifiers.

## Reuse an existing private profile explicitly

The new default is `capsule-memory` beneath Go's `os.UserConfigDir()`. Existing
users must explicitly select their previous private profile on every invocation:

```sh
capsule-memory --home /previous/private/path status
```

Before the rename, the default subdirectory was `agent-memory`. Typical previous
locations were `$XDG_CONFIG_HOME/agent-memory` or `$HOME/.config/agent-memory` on
Linux, and `$HOME/Library/Application Support/agent-memory` on macOS. Use the
actual path previously configured, including any custom `--home` location.
The CLI does not automatically inspect, read, copy, or move old profiles. This
rename work inspected no private state. Keep profile contents and keys intact;
never initialize a replacement identity to make a rename work.

## Historical source links

Immutable commit links use the renamed GitHub repository URL with their original
SHA. A historical tree may still contain `cmd/agent-memory`; that path accurately
describes the old commit and must not be rewritten to a path absent from it.
Git history is preserved.
