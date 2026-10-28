// Additive pairing protocol. The inbox contains public identities and opaque
// owner-signed ciphertext, never content keys or device/owner private keys.
import {
  bytes,
  control,
  decimal,
  equalSigned,
  fields,
  hash,
  id,
  object,
  parse,
  requireThat,
  signed,
  type Control,
  type Signed,
} from "./wire";
import { provider, utf8, type Archive, type Ref } from "./archive";

const PREFIX = "capsule-memory/enrollment-";
const MAX_STATE = 256 * 1024;
const MAX_ENTRIES = 32;
const RETAIN_SECONDS = 86400;
const now = () => Math.floor(Date.now() / 1000);
const digest = (s: Signed) => hash(`${s.payload}\n${s.signature}\n`);

interface Invitation {
  binding: string;
  version: 1;
  vault_id: string;
  invite_id: string;
  relay: string;
  owner_public_key_b64: string;
  scope_ids: string[];
  issued_at: string;
  expires_at: string;
  allow_loopback_http: boolean;
}
interface JoinRequest {
  invitation_sha256: string;
  version: 1;
  vault_id: string;
  invite_id: string;
  device_id: string;
  public_key_b64: string;
  encryption_key_b64: string;
}
interface Decision {
  activation_sha256: string;
  version: 1;
  vault_id: string;
  invite_id: string;
  request_sha256: string;
  action: "approve" | "deny";
  scope_ids: string[];
  parent_sha256: string;
  archive_cursor: string;
  control?: Signed;
  key_package_b64: string;
}
interface Entry {
  invitation: Signed;
  request?: Signed;
  decision?: Ref;
  decision_sha256: string;
  retain_until: number;
  active: boolean;
}
interface State {
  version: 1;
  entries: Entry[];
}
export interface CommitResult {
  cursor: string;
  manifest: Signed;
  state: "approved" | "conflicted";
}

async function verify(domain: string, s: Signed, publicKey: string): Promise<any> {
  requireThat(s.payload.length <= 2 * 1024 * 1024, "enrollment_too_large", 413);
  requireThat(bytes(publicKey).length === 32, "invalid_key");
  const key = await crypto.subtle.importKey("raw", bytes(publicKey), "Ed25519", false, ["verify"]);
  requireThat(
    await crypto.subtle.verify(
      "Ed25519",
      key,
      bytes(s.signature),
      new TextEncoder().encode(`${domain}\n${s.payload}\n`),
    ),
    "invalid_enrollment_signature",
    403,
  );
  return parse(bytes(s.payload));
}
function scopes(value: unknown): value is string[] {
  return (
    Array.isArray(value) &&
    value.length > 0 &&
    value.length <= 64 &&
    value.every(id) &&
    new Set(value).size === value.length
  );
}
async function invitation(
  s: Signed,
  req: Request,
  vault: string,
  owner: string,
): Promise<Invitation> {
  requireThat(s.payload.length <= 16000, "invitation_too_large", 413);
  const i = await verify(PREFIX + "invite/v1", s, owner);
  requireThat(object(i));
  fields(i, [
    "version",
    "vault_id",
    "invite_id",
    "relay",
    "owner_public_key_b64",
    "scope_ids",
    "issued_at",
    "expires_at",
    "allow_loopback_http",
  ]);
  requireThat(
    i.version === 1 &&
      i.vault_id === vault &&
      id(i.invite_id) &&
      i.owner_public_key_b64 === owner &&
      typeof i.relay === "string" &&
      scopes(i.scope_ids) &&
      typeof i.allow_loopback_http === "boolean" &&
      decimal(i.issued_at) &&
      decimal(i.expires_at),
  );
  const relay = new URL(i.relay as string);
  requireThat(
    relay.origin === i.relay &&
      !relay.username &&
      !relay.password &&
      (relay.protocol === "https:" ||
        (i.allow_loopback_http &&
          relay.protocol === "http:" &&
          ["127.0.0.1", "localhost", "[::1]"].includes(relay.hostname))),
    "invalid_invitation_relay",
  );
  const issued = BigInt(i.issued_at),
    expires = BigInt(i.expires_at);
  requireThat(
    issued <= BigInt(now() + 300) &&
      expires > issued &&
      expires - issued <= 3600n &&
      expires <= BigInt(Number.MAX_SAFE_INTEGER),
    "invalid_invitation_time",
  );
  return { ...i, binding: await digest(s) } as unknown as Invitation;
}
function live(i: Invitation): void {
  requireThat(BigInt(i.expires_at) > BigInt(now()), "invitation_expired", 409);
}
async function joinRequest(s: Signed, i: Invitation): Promise<JoinRequest> {
  requireThat(s.payload.length <= 8192, "request_too_large", 413);
  const hint = parse(bytes(s.payload));
  requireThat(object(hint) && typeof hint.public_key_b64 === "string");
  const r = await verify(PREFIX + "request/v1", s, hint.public_key_b64);
  fields(r, [
    "version",
    "vault_id",
    "invite_id",
    "invitation_sha256",
    "device_id",
    "public_key_b64",
    "encryption_key_b64",
  ]);
  requireThat(
    r.version === 1 &&
      r.vault_id === i.vault_id &&
      r.invite_id === i.invite_id &&
      r.invitation_sha256 === i.binding &&
      id(r.device_id) &&
      bytes(r.public_key_b64).length === 32 &&
      bytes(r.encryption_key_b64).length === 32,
  );
  return r as JoinRequest;
}
async function authenticate(
  req: Request,
  body: Uint8Array,
  vault: string,
  subject: string,
  publicKey: string,
) {
  const ts = req.headers.get("X-CM-Time"),
    nonce = req.headers.get("X-CM-Nonce"),
    signature = req.headers.get("X-CM-Signature");
  requireThat(
    req.headers.get("X-CM-Subject") === subject &&
      decimal(ts) &&
      nonce &&
      /^[0-9a-f]{48}$/.test(nonce) &&
      signature,
    "invalid_enrollment_auth",
    403,
  );
  const delta = BigInt(ts) - BigInt(now());
  requireThat(delta >= -300n && delta <= 300n, "expired_enrollment_auth", 403);
  const url = new URL(req.url);
  const input = `${PREFIX}http/v1\n${req.method}\n${url.pathname}${url.search}\n${vault}\n${subject}\n${ts}\n${nonce}\n${await hash(body)}\n`;
  const key = await crypto.subtle.importKey("raw", bytes(publicKey), "Ed25519", false, ["verify"]);
  requireThat(
    bytes(signature).length === 64 &&
      (await crypto.subtle.verify(
        "Ed25519",
        key,
        bytes(signature),
        new TextEncoder().encode(input),
      )),
    "invalid_enrollment_auth",
    403,
  );
}

// The R2 index is small and CAS-updated. Prepared decisions are immutable R2
// objects; receipts survive loss of every DO SQLite row. Expiry bounds index
// retention, not removal of ciphertext already disclosed to a device.
class InboxStore {
  private key: string;
  constructor(
    private bucket: R2Bucket,
    vault: string,
  ) {
    this.key = `v1/${vault}/enrollment/state-v1`;
  }
  async load(): Promise<{ state: State; etag: string | null }> {
    const obj = await provider(this.bucket.get(this.key));
    if (!obj) return { state: { version: 1, entries: [] }, etag: null };
    requireThat(obj.size <= MAX_STATE, "enrollment_state_oversize", 503);
    const state = parse(new Uint8Array(await provider(obj.arrayBuffer())));
    requireThat(
      object(state) &&
        state.version === 1 &&
        Array.isArray(state.entries) &&
        state.entries.length <= MAX_ENTRIES,
      "enrollment_state_invalid",
      503,
    );
    for (const e of state.entries)
      requireThat(
        object(e) &&
          Number.isSafeInteger(e.retain_until) &&
          typeof e.active === "boolean" &&
          typeof e.decision_sha256 === "string",
        "enrollment_state_invalid",
        503,
      );
    return { state: state as unknown as State, etag: obj.etag };
  }
  async save(state: State, etag: string | null) {
    const body = utf8(state);
    requireThat(body.length <= MAX_STATE, "enrollment_state_full", 429);
    const condition: Record<string, string> =
      etag === null ? { "If-None-Match": "*" } : { "If-Match": `"${etag}"` };
    const saved = await provider(
      this.bucket.put(this.key, body, { onlyIf: new Headers(condition) }),
    );
    requireThat(saved, "enrollment_changed_retry", 503);
  }
}

export class Enrollment {
  private store: InboxStore;
  constructor(
    private bucket: R2Bucket,
    private archive: Archive,
    private vault: string,
    private owner: string,
  ) {
    this.store = new InboxStore(bucket, vault);
  }
  async handle(
    req: Request,
    path: string,
    body: Uint8Array,
    manifest: Signed,
    cursor: string,
    commit: (s: Signed) => Promise<CommitResult>,
  ): Promise<unknown> {
    const relative = path.slice("/enrollment".length);
    const list = relative === "/requests" && req.method === "GET";
    const decisionRoute = relative.endsWith("/decision");
    if (list || decisionRoute) await authenticate(req, body, this.vault, "owner", this.owner);
    const loaded = await this.store.load();
    const state = loaded.state;
    state.entries = state.entries.filter((e) => e.retain_until > now());

    if (relative === "/invites") {
      const s = signed(parse(body));
      const i = await invitation(s, req, this.vault, this.owner);
      live(i);
      const previous = await this.find(state, i.invite_id, req);
      if (previous)
        requireThat(equalSigned(previous.entry.invitation, s), "invitation_conflict", 409);
      else {
        requireThat(state.entries.length < MAX_ENTRIES, "enrollment_inbox_full", 429);
        state.entries.push({
          invitation: s,
          decision_sha256: "",
          retain_until: Number(i.expires_at) + RETAIN_SECONDS,
          active: false,
        });
        await this.store.save(state, loaded.etag);
      }
      return { state: "invited", cursor: "" };
    }
    if (relative === "/requests" && req.method === "POST") {
      const value = parse(body);
      requireThat(object(value));
      fields(value, ["invitation", "request"]);
      const signedInvite = signed(value.invitation),
        signedRequest = signed(value.request);
      const i = await invitation(signedInvite, req, this.vault, this.owner);
      await joinRequest(signedRequest, i);
      let found = await this.find(state, i.invite_id, req);
      if (!found) {
        live(i);
        requireThat(state.entries.length < MAX_ENTRIES, "enrollment_inbox_full", 429);
        const entry: Entry = {
          invitation: signedInvite,
          decision_sha256: "",
          retain_until: Number(i.expires_at) + RETAIN_SECONDS,
          active: false,
        };
        state.entries.push(entry);
        found = { entry, invite: i };
      }
      requireThat(equalSigned(found.entry.invitation, signedInvite), "invitation_conflict", 409);
      if (found.entry.request)
        requireThat(
          equalSigned(found.entry.request, signedRequest),
          "invitation_already_claimed",
          409,
        );
      else {
        live(i);
        found.entry.request = signedRequest;
        await this.store.save(state, loaded.etag);
      }
      return { state: now() < Number(i.expires_at) ? "pending" : "expired", cursor: "" };
    }
    if (list) {
      const entries = [];
      for (const entry of state.entries) {
        const i = await invitation(entry.invitation, req, this.vault, this.owner);
        let status = entry.request ? "pending" : "invited";
        if (entry.request) await joinRequest(entry.request, i);
        if (entry.decision) {
          const saved = signed(parse(await this.archive.get(entry.decision, 2 * 1024 * 1024)));
          const d = await this.decision(saved, entry, i);
          status = d.action === "deny" ? "denied" : entry.active ? "active" : "approval_prepared";
        } else if (Number(i.expires_at) <= now()) status = "expired";
        entries.push({
          invitation: entry.invitation,
          ...(entry.request ? { request: entry.request } : {}),
          state: status,
          decision_sha256: entry.decision_sha256,
        });
      }
      return { entries };
    }
    const match = /^\/requests\/([A-Za-z0-9_-]{8,128})(?:\/(decision|ack))?$/.exec(relative);
    requireThat(match, "not_found", 404);
    const found = await this.find(state, match[1], req);
    requireThat(found?.entry.request, "enrollment_request_missing", 404);
    const { entry, invite: i } = found;
    const r = await joinRequest(entry.request!, i);
    if (!decisionRoute) {
      const subject = req.headers.get("X-CM-Subject");
      requireThat(subject === "owner" || subject === r.device_id, "invalid_enrollment_auth", 403);
      await authenticate(
        req,
        body,
        this.vault,
        subject,
        subject === "owner" ? this.owner : r.public_key_b64,
      );
    }
    if (decisionRoute) {
      const incoming = signed(parse(body));
      const d = await this.decision(incoming, entry, i);
      if (entry.decision)
        requireThat(
          entry.decision_sha256 === (await digest(incoming)),
          "enrollment_decision_terminal",
          409,
        );
      else {
        if (d.action === "approve") {
          live(i);
          requireThat(
            d.archive_cursor === cursor && d.parent_sha256 === (await digest(manifest)),
            "enrollment_review_stale",
            409,
          );
          const old = await control(manifest, this.vault, this.owner);
          const next = await control(d.control!, this.vault, this.owner);
          this.extension(old, next, r, d);
        }
        entry.decision = await this.archive.immutable("objects", utf8(incoming));
        entry.decision_sha256 = await digest(incoming);
        // Persist the owner-approved operation before attempting its membership
        // publication. Only these exact bytes may resume after a lost response.
        await this.store.save(state, loaded.etag);
      }
    }
    if (!entry.decision)
      return { state: Number(i.expires_at) <= now() ? "expired" : "pending", cursor: "" };
    const saved = signed(parse(await this.archive.get(entry.decision, 2 * 1024 * 1024)));
    requireThat((await digest(saved)) === entry.decision_sha256, "enrollment_state_invalid", 503);
    const d = await this.decision(saved, entry, i);
    if (d.action === "deny") return { state: "denied", decision: saved, cursor: "" };
    const published = await commit(d.control!);
    if (published.state === "conflicted") return { decision: saved, ...published };
    const current = await control(published.manifest, this.vault, this.owner);
    const granted = current.devices.find((x) => x.device_id === r.device_id);
    const expected = await control(d.control!, this.vault, this.owner);
    const permitted =
      granted &&
      !granted.revoked &&
      granted.public_key_b64 === r.public_key_b64 &&
      granted.scope_ids.length === d.scope_ids.length &&
      d.scope_ids.every(
        (scope) =>
          granted.scope_ids.includes(scope) &&
          current.scopes.find((x) => x.scope_id === scope)?.key_epoch ===
            expected.scopes.find((x) => x.scope_id === scope)?.key_epoch,
      );
    if (!permitted)
      return {
        state: "revoked",
        cursor: published.cursor,
        manifest: published.manifest,
        ...(req.headers.get("X-CM-Subject") === "owner" ? { decision: saved } : {}),
      };
    if (match[2] === "ack") {
      requireThat(req.headers.get("X-CM-Subject") === r.device_id, "invalid_enrollment_auth", 403);
      const ack = parse(body);
      requireThat(object(ack));
      fields(ack, ["decision_sha256", "activation_secret_b64"]);
      requireThat(ack.decision_sha256 === entry.decision_sha256, "acknowledgement_mismatch");
      requireThat(
        bytes(ack.activation_secret_b64).length === 32 &&
          (await hash(bytes(ack.activation_secret_b64))) === d.activation_sha256,
        "activation_mismatch",
      );
      if (!entry.active) {
        // A decision POST may have changed the index ETag in this request.
        const latest = await this.store.load();
        const same = await this.find(latest.state, i.invite_id, req);
        requireThat(
          same && same.entry.decision_sha256 === entry.decision_sha256,
          "enrollment_changed_retry",
          503,
        );
        same.entry.active = true;
        await this.store.save(latest.state, latest.etag);
      }
      entry.active = true;
    }
    return {
      state: entry.active ? "active" : "approved",
      decision: saved,
      cursor: published.cursor,
      manifest: published.manifest,
    };
  }
  private async find(state: State, id: string, req: Request) {
    for (const entry of state.entries) {
      const invite = await invitation(entry.invitation, req, this.vault, this.owner);
      if (invite.invite_id === id) return { entry, invite };
    }
    return null;
  }
  private async decision(s: Signed, entry: Entry, i: Invitation): Promise<Decision> {
    const d = await verify(PREFIX + "decision/v1", s, this.owner);
    requireThat(object(d));
    const names = [
      "version",
      "vault_id",
      "invite_id",
      "request_sha256",
      "action",
      "scope_ids",
      "parent_sha256",
      "archive_cursor",
      "activation_sha256",
      "key_package_b64",
    ];
    fields(d, d.action === "approve" ? [...names, "control"] : names);
    requireThat(
      d.version === 1 &&
        d.vault_id === this.vault &&
        d.invite_id === i.invite_id &&
        entry.request &&
        d.request_sha256 === (await digest(entry.request)),
      "enrollment_context_mismatch",
      403,
    );
    if (d.action === "deny")
      requireThat(
        Array.isArray(d.scope_ids) &&
          d.scope_ids.length === 0 &&
          d.parent_sha256 === "" &&
          d.archive_cursor === "" &&
          d.activation_sha256 === "" &&
          d.key_package_b64 === "",
      );
    else {
      requireThat(
        d.action === "approve" &&
          scopes(d.scope_ids) &&
          d.scope_ids.every((x) => i.scope_ids.includes(x)) &&
          typeof d.parent_sha256 === "string" &&
          /^[0-9a-f]{64}$/.test(d.parent_sha256) &&
          decimal(d.archive_cursor) &&
          typeof d.activation_sha256 === "string" &&
          /^[0-9a-f]{64}$/.test(d.activation_sha256) &&
          bytes(d.key_package_b64).length >= 48 &&
          bytes(d.key_package_b64).length <= 1024 * 1024 + 128,
      );
      const c = await control(signed(d.control), this.vault, this.owner);
      const r = await joinRequest(entry.request!, i);
      const device = c.devices.find((x) => x.device_id === r.device_id);
      requireThat(
        device &&
          !device.revoked &&
          device.public_key_b64 === r.public_key_b64 &&
          device.scope_ids.length === d.scope_ids.length &&
          d.scope_ids.every((x) => device.scope_ids.includes(x)),
        "enrollment_context_mismatch",
        403,
      );
    }
    return d as unknown as Decision;
  }
  private extension(old: Control, next: Control, r: JoinRequest, d: Decision) {
    requireThat(
      next.generation === (BigInt(old.generation) + 1n).toString() &&
        old.scopes.length === next.scopes.length &&
        old.scopes.every(
          (value, n) =>
            value.scope_id === next.scopes[n].scope_id &&
            value.key_epoch === next.scopes[n].key_epoch,
        ) &&
        next.devices.length === old.devices.length + 1 &&
        old.devices.every(
          (value, n) =>
            value.device_id === next.devices[n].device_id &&
            value.public_key_b64 === next.devices[n].public_key_b64 &&
            value.revoked === next.devices[n].revoked &&
            value.scope_ids.length === next.devices[n].scope_ids.length &&
            value.scope_ids.every((scope, k) => scope === next.devices[n].scope_ids[k]),
        ) &&
        !old.devices.some(
          (x) => x.device_id === r.device_id || x.public_key_b64 === r.public_key_b64,
        ) &&
        next.devices.at(-1)?.device_id === r.device_id &&
        scopes(d.scope_ids),
      "enrollment_control_conflict",
      409,
    );
  }
}
