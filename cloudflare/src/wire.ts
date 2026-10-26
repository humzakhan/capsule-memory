// Wire framing is shared by agreement with docs/v1-contract.md, not JSON canonicalization.
export const MAX_BODY = 2 * 1024 * 1024;
export const MAX_CONTROL = 128 * 1024;
const enc = new TextEncoder();
const dec = new TextDecoder("utf-8", { fatal: true });
export class Fault extends Error {
  constructor(public status: number, public code: string) { super(code); }
}
export function requireThat(ok: unknown, code = "invalid_input", status = 400): asserts ok {
  if (!ok) throw new Fault(status, code);
}
export function id(v: unknown): v is string { return typeof v === "string" && /^[A-Za-z0-9_-]{8,128}$/.test(v); }
export function decimal(v: unknown): v is string { return typeof v === "string" && /^(0|[1-9][0-9]{0,127})$/.test(v); }
export function object(v: unknown): v is Record<string, unknown> { return !!v && typeof v === "object" && !Array.isArray(v); }
export function fields(v: Record<string, unknown>, names: string[]) {
  requireThat(Object.keys(v).length === names.length && names.every(k => Object.hasOwn(v, k)));
}
export function parse(bytes: Uint8Array): any {
  try {
    const text = dec.decode(bytes);
    const value = JSON.parse(text);
    uniqueJSON(text);
    return value;
  } catch { throw new Fault(400, "invalid_json"); }
}
// JSON.parse owns syntax and value semantics. Walk the validated source once
// because parsing erases duplicate properties. Decode key tokens with the same
// parser so "version" and "\u0076ersion" compare equal. No signed bytes change.
function uniqueJSON(text: string): void {
  let i = 0;
  const space = () => { while (i < text.length && /[ \t\r\n]/.test(text[i])) i++; };
  const stringEnd = () => {
    const start = i++;
    while (text[i] !== '"') { i += text[i] === "\\" ? 2 : 1; }
    i++;
    return start;
  };
  const walk = (depth: number): void => {
    // Match Go uniqueJSON: root value is depth 0, including scalar children.
    requireThat(depth <= 32, "invalid_json");
    space();
    const kind = text[i];
    if (kind === "{" || kind === "[") {
      i++; space();
      const end = kind === "{" ? "}" : "]";
      const keys = kind === "{" ? new Set<string>() : null;
      if (text[i] !== end) for (;;) {
        if (keys) {
          const start = stringEnd();
          const key: string = JSON.parse(text.slice(start, i));
          requireThat(!keys.has(key), "invalid_json"); keys.add(key);
          space(); i++; // Colon; syntax was already validated by JSON.parse.
        }
        walk(depth + 1); space();
        if (text[i] === end) break;
        i++; space(); // Comma.
      }
      i++;
    } else if (kind === '"') {
      stringEnd();
    } else {
      while (i < text.length && !/[ \t\r\n,\]}]/.test(text[i])) i++;
    }
  };
  walk(0);
}
export function bytes(v: unknown): Uint8Array {
  requireThat(typeof v === "string" && v.length <= MAX_BODY * 2 && v.length % 4 === 0 && /^[A-Za-z0-9+/]*={0,2}$/.test(v));
  const out = Uint8Array.from(atob(v), c => c.charCodeAt(0));
  // Reject nonzero padding bits as well as alternate encodings.
  let canonical = "";
  for (let i = 0; i < out.length; i += 8192) canonical += String.fromCharCode(...out.subarray(i, i + 8192));
  requireThat(btoa(canonical) === v);
  return out;
}
export async function hash(v: Uint8Array | string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", typeof v === "string" ? enc.encode(v) : v);
  return Array.from(new Uint8Array(digest), b => b.toString(16).padStart(2, "0")).join("");
}
export interface Signed { payload: string; signature: string }
export interface Control {
  version: 1; vault_id: string; generation: string;
  scopes: {scope_id: string; key_epoch: string}[];
  devices: {device_id: string; public_key_b64: string; scope_ids: string[]; revoked: boolean}[];
}
export interface Outer {
  version: 1; vault_id: string; scope_id: string; device_id: string; key_epoch: string;
  transport_id: string; nonce_b64: string; ciphertext_b64: string; ciphertext_sha256: string;
}
export function signed(v: unknown): Signed {
  requireThat(object(v)); fields(v, ["payload", "signature"]);
  requireThat(typeof v.payload === "string" && typeof v.signature === "string");
  requireThat(bytes(v.signature).length === 64);
  return v as unknown as Signed;
}
export function equalSigned(a: Signed, b: Signed): boolean { return a.payload === b.payload && a.signature === b.signature; }
export async function verify(s: Signed, keyB64: string, kind: "envelope" | "control") {
  requireThat(bytes(keyB64).length === 32, "invalid_key");
  const key = await crypto.subtle.importKey("raw", bytes(keyB64), { name: "Ed25519" }, false, ["verify"]);
  requireThat(await crypto.subtle.verify("Ed25519", key, bytes(s.signature), enc.encode(`agent-memory/${kind}/v1\n${s.payload}\n`)), "invalid_signature", 403);
}
export async function control(s: Signed, vault: string, owner: string): Promise<Control> {
  requireThat(s.payload.length <= Math.ceil(MAX_CONTROL / 3) * 4, "control_too_large", 413);
  await verify(s, owner, "control");
  const raw = bytes(s.payload); requireThat(raw.length <= MAX_CONTROL, "control_too_large", 413);
  const c = parse(raw);
  requireThat(object(c)); fields(c, ["version", "vault_id", "generation", "scopes", "devices"]);
  requireThat(c.version === 1 && c.vault_id === vault && decimal(c.generation) && c.generation !== "0");
  requireThat(Array.isArray(c.scopes) && c.scopes.length > 0 && c.scopes.length <= 64);
  const scopes = new Set<string>();
  for (const scope of c.scopes) {
    requireThat(object(scope)); fields(scope, ["scope_id", "key_epoch"]);
    requireThat(id(scope.scope_id) && decimal(scope.key_epoch) && !scopes.has(scope.scope_id)); scopes.add(scope.scope_id);
  }
  requireThat(Array.isArray(c.devices) && c.devices.length > 0 && c.devices.length <= 256);
  const devices = new Set<string>();
  for (const d of c.devices) {
    requireThat(object(d)); fields(d, ["device_id", "public_key_b64", "scope_ids", "revoked"]);
    requireThat(id(d.device_id) && !devices.has(d.device_id) && typeof d.revoked === "boolean"); devices.add(d.device_id);
    requireThat(bytes(d.public_key_b64).length === 32 && Array.isArray(d.scope_ids) && d.scope_ids.length <= 64);
    requireThat(new Set(d.scope_ids).size === d.scope_ids.length && d.scope_ids.every((x: unknown) => typeof x === "string" && scopes.has(x)));
  }
  return c as unknown as Control;
}
export function member(c: Control, device: string, scope: string) {
  const d = c.devices.find(x => x.device_id === device);
  requireThat(d && !d.revoked && d.scope_ids.includes(scope) && c.scopes.some(s => s.scope_id === scope), "not_authorized", 403);
  return d;
}
export function outerIdentity(s: Signed): Outer {
  const p = parse(bytes(s.payload));
  requireThat(object(p)); fields(p, ["version", "vault_id", "scope_id", "device_id", "key_epoch", "transport_id", "nonce_b64", "ciphertext_b64", "ciphertext_sha256"]);
  requireThat(p.version === 1 && id(p.vault_id) && id(p.scope_id) && id(p.device_id) && id(p.transport_id) && decimal(p.key_epoch));
  return p as unknown as Outer;
}
export async function envelope(s: Signed, c: Control): Promise<Outer> {
  const p = outerIdentity(s);
  requireThat(p.vault_id === c.vault_id);
  const d = member(c, p.device_id, p.scope_id);
  requireThat(c.scopes.find(x => x.scope_id === p.scope_id)?.key_epoch === p.key_epoch, "stale_epoch", 403);
  requireThat(bytes(p.nonce_b64).length === 12);
  const ciphertext = bytes(p.ciphertext_b64);
  requireThat(ciphertext.length >= 16 && typeof p.ciphertext_sha256 === "string" && /^[0-9a-f]{64}$/.test(p.ciphertext_sha256));
  requireThat(await hash(ciphertext) === p.ciphertext_sha256, "digest_mismatch");
  await verify(s, d.public_key_b64, "envelope");
  return p as unknown as Outer;
}
export async function authenticate(req: Request, body: Uint8Array, c: Control): Promise<{device: string; scope: string}> {
  const device = req.headers.get("X-AM-Device"), scope = req.headers.get("X-AM-Scope");
  const time = req.headers.get("X-AM-Time"), nonce = req.headers.get("X-AM-Nonce"), signature = req.headers.get("X-AM-Signature");
  requireThat(id(device) && id(scope) && decimal(time) && typeof nonce === "string" && /^[0-9a-f]{32,128}$/.test(nonce) && signature, "invalid_auth", 403);
  const delta = BigInt(time) - BigInt(Math.floor(Date.now() / 1000));
  requireThat(delta >= -300n && delta <= 300n, "expired_auth", 403);
  const d = member(c, device, scope);
  const url = new URL(req.url);
  const input = `agent-memory/request/v1\n${req.method}\n${url.pathname}${url.search}\n${c.vault_id}\n${scope}\n${device}\n${time}\n${nonce}\n${await hash(body)}\n`;
  const sig = bytes(signature); requireThat(sig.length === 64, "invalid_auth", 403);
  const key = await crypto.subtle.importKey("raw", bytes(d.public_key_b64), {name:"Ed25519"}, false, ["verify"]);
  requireThat(await crypto.subtle.verify("Ed25519", key, sig, enc.encode(input)), "invalid_auth", 403);
  return {device, scope};
}
export async function readBody(req: Request, max: number): Promise<Uint8Array> {
  const length = req.headers.get("content-length");
  if (length) requireThat(/^[0-9]+$/.test(length) && BigInt(length) <= BigInt(max), "body_too_large", 413);
  if (!req.body) return new Uint8Array();
  const reader = req.body.getReader(); const chunks: Uint8Array[] = []; let size = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => {
      reject(new Fault(408, "body_timeout"));
      void reader.cancel().catch(() => {});
    }, 10000);
  });
  try {
    for (;;) {
      const {value, done} = await Promise.race([reader.read(), timeout]); if (done) break;
      size += value.length;
      if (size > max) { await reader.cancel(); throw new Fault(413, "body_too_large"); }
      chunks.push(value);
    }
  } finally { if (timer !== undefined) clearTimeout(timer); reader.releaseLock(); }
  const out = new Uint8Array(size); let offset = 0;
  for (const chunk of chunks) { out.set(chunk, offset); offset += chunk.length; }
  return out;
}
