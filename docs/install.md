# Install the CLI on a trusted computer

For **v0.1.0-beta.5**, build the Go CLI from the release tag and install it as
`/usr/local/bin/capsule-memory` on macOS or Linux. This is a machine-wide command,
not a root-running service. Profiles, databases and credentials stay in private
directories owned by the user running it. Distribution remains source-only;
there is no downloadable executable or curl-to-shell installer.

For a MacBook that will approve other devices, use the
[MacBook controller prompt](../prompts/12-macbook-controller.md). For an ordinary
additional device, use the [enrollment prompt](../prompts/04-enroll-device.md).
Budget about 15–30 minutes for prerequisites and the first build; pairing also
requires the existing owner and a matching relay.

## 1. Check the host and release

Read `AGENTS.md` and the host's execution policy before running workloads. Use Go
**1.26.10** and Git. On macOS, the native Keychain adapter also needs Apple's
command-line developer tools and `CGO_ENABLED=1`; install missing prerequisites
only through the host's authorized tool-management route. Do not replace system
tools or change unrelated services. Linux can build with `CGO_ENABLED=0`;
Windows retains the legacy direct workflow, not the new pairing console.

Use a fresh checkout under the host's normal projects directory. Do not reset an
existing dirty checkout or build from floating `main` for a pinned installation:

```sh
git clone --branch v0.1.0-beta.5 --single-branch \
  https://github.com/humzakhan/capsule-memory.git capsule-memory
cd capsule-memory
git rev-parse HEAD
git status --short
go version
go env CGO_ENABLED
```

Confirm the commit matches the release tag and keep that revision in the install
record. A GitHub source tag identifies the selected source; it is not a security
audit. Node **22+** and `npm ci --prefix cloudflare --ignore-scripts` are needed
for relay tooling or the local E2E, not for ordinary CLI operation. A joining
device does not need Cloudflare account credentials.

## 2. Build as your normal user

From the clean tagged checkout, with the pinned Go toolchain selected:

```sh
umask 077
CAPSULE_STAGE="$(mktemp -d "${TMPDIR:-/tmp}/capsule-cli.XXXXXX")"
GOTOOLCHAIN=local go mod download
GOTOOLCHAIN=local GOMAXPROCS=2 go build -mod=readonly -p 2 \
  -trimpath -buildvcs=true -o "$CAPSULE_STAGE/capsule-memory" ./cmd/capsule-memory
"$CAPSULE_STAGE/capsule-memory" device --help
GOTOOLCHAIN=local go version -m "$CAPSULE_STAGE/capsule-memory"
```

Stop on a failed command. Never run the build, package installation, enrollment
or normal CLI operation with `sudo`. A macOS build without cgo explicitly rejects
`owner protect`; do not report native custody as installed in that case.

## 3. Install on PATH

If a command already exists, inspect its resolved path and revision first.
Preserve its executable or symlink target and checksum outside the checkout for
rollback. Do not overwrite a different application's binary, change a running
installation or restart services without the relevant upgrade authority.

For the authorized machine-wide install, use the host's approved privilege
mechanism only for these filesystem writes. A human may need to authorize them
locally; never ask for an administrator password in chat:

```sh
test -d /usr/local/bin || sudo install -d -m 755 /usr/local/bin
sudo install -m 755 "$CAPSULE_STAGE/capsule-memory" /usr/local/bin/capsule-memory
```

If the host already permits writing that directory as the current user, use
`install` without `sudo`. If machine-wide writes are unavailable, report that
part as blocked. A user-owned `~/.local/bin/capsule-memory` is an explicit fallback,
not a successful machine-wide install; do not silently change the requested target.

Ensure `/usr/local/bin` is on the user's PATH, preserving existing shell settings
and adding the entry once if needed. In a new terminal, verify:

```sh
command -v capsule-memory
/usr/local/bin/capsule-memory device --help
go version -m /usr/local/bin/capsule-memory
```

The resolved command must be this installation, not an older copy earlier on
PATH. Use the absolute path in MCP configuration; do not depend on a harness
expanding `~` or inheriting an interactive shell's PATH. Record the commit, cgo
mode, installed path, executable checksum and any previous executable's location
outside Git. Remove only the task-owned staging directory after verification.

## 4. Set up authority and verify use

Installing the executable does not enroll the computer, create a vault, connect
an assistant, enable synchronization or install a background service.

- **Existing vault controller:** securely transfer the existing owner authority
  and a consistent private profile as described in [device enrollment](device-enrollment.md#local-owner-credentials).
  Do not run `init`, clone a live SQLite file, or operate two active owner clones.
- **Ordinary new device:** use `device join` with a privately transferred
  invitation; it generates its own device keys. The owner approves exact scopes.
- **New vault:** only an explicit new-vault request uses [owner setup](../prompts/02-owner-setup.md).

Both the CLI and existing Worker must come from this release before pairing.
Keep the existing vault ID, owner pin, Worker identity, DO and R2 bindings.
An authorized relay operator upgrades that Worker; installing on a laptop is not
permission to create infrastructure, replace the archive or spend money.

After enrollment, run scoped sync and a synthetic memory round trip between two
authorized profiles. Then connect the chosen harness using the
[MCP guide](../prompts/05-connect-mcp.md), with a separate non-owner profile.
Background sync requires its own [daemon setup](daemon.md); capture, inference
and automatic acceptance stay off. Skills synchronize as text for local review.

For rollback, stop only task-owned processes and restore the preserved executable
or symlink under the same filesystem permissions. Keep profiles and queued work;
do not delete a database or attempt an older client's replay to reverse an upgrade.
An older Worker also lacks these pairing endpoints. Report install, pairing,
sync and harness checks separately as passed, failed, pending or NOT RUN.
