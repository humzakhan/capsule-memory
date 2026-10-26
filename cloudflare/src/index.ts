import { DurableObject } from "cloudflare:workers";
import { R2Archive, journal, utf8, type Archive, type Head, type Journal, type Ref, type VersionedHead } from "./archive";
import { Fault, MAX_BODY, MAX_CONTROL, authenticate, control, decimal, envelope, equalSigned, hash, id, outerIdentity, parse, readBody, requireThat, signed, type Control, type Signed } from "./wire";

export interface Env {
  VAULTS: DurableObjectNamespace<Vault>;
  ARCHIVE: R2Bucket;
  VAULT_ID: string;
  OWNER_PUBLIC_KEY_B64: string;
  ALLOW_LOOPBACK_HTTP?: string;
}
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), {status, headers: {"content-type": "application/json", "cache-control": "no-store", "x-content-type-options": "nosniff"}});
function failure(error: unknown): Response {
  const f = error instanceof Fault ? error : new Fault(503, "temporarily_unavailable");
  const response = json({error: f.code}, f.status);
  if (f.status === 503 || f.status === 429) response.headers.set("retry-after", "2");
  return response;
}
function route(req: Request, env: Env): string {
  const url = new URL(req.url);
  requireThat(url.pathname.length + url.search.length <= 4096, "url_too_large", 414);
  requireThat(id(env.VAULT_ID) && typeof env.OWNER_PUBLIC_KEY_B64 === "string", "not_configured", 503);
  requireThat(url.protocol === "https:" || (env.ALLOW_LOOPBACK_HTTP === "true" && url.protocol === "http:" && ["127.0.0.1", "localhost", "[::1]"].includes(url.hostname)), "https_required", 403);
  const base = `/v1/vaults/${env.VAULT_ID}`;
  requireThat(url.pathname.startsWith(base + "/"), "not_found", 404);
  const path = url.pathname.slice(base.length);
  requireThat((path === "/manifest" && ["GET", "PUT"].includes(req.method)) || (path === "/records" && req.method === "POST") || (path === "/changes" && req.method === "GET"), "not_found", 404);
  requireThat(path === "/changes" || url.search === "", "unexpected_query");
  return path;
}
export default {
  async fetch(req: Request, env: Env): Promise<Response> {
    try {
      route(req, env);
      const stub = env.VAULTS.get(env.VAULTS.idFromName(env.VAULT_ID));
      return await stub.fetch(req);
    } catch (error) { return failure(error); }
  }
};

interface Meta { cursor: string; journal: Ref | null; manifest: Signed | null }
interface Walk { target: Head; next: Ref | null; cursor: string; phase: "walk" | "apply" }
type CachedEntry = {
  cursor: string; type: "record" | "control"; object_key: string; object_digest: string;
  journal_key: string; journal_digest: string; scope_id: string | null;
  device_id: string | null; transport_id: string | null; signed_digest: string;
  generation: string | null;
}
const sameRef = (a: Ref | null, b: Ref | null) => a?.key === b?.key && a?.digest === b?.digest;
const signDigest = (s: Signed) => hash(`${s.payload}\n${s.signature}\n`);

export class Vault extends DurableObject<Env> {
  private archive: Archive;
  // Awaiting R2 yields the event loop. Explicit queue serializes the complete
  // read/auth/publish operation without holding blockConcurrencyWhile across I/O.
  private tail: Promise<void> = Promise.resolve();
  private waiting = 0;
  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env); this.archive = new R2Archive(env.ARCHIVE, env.VAULT_ID);
    ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`);
    ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS pending (cursor TEXT PRIMARY KEY, journal_key TEXT NOT NULL, journal_digest TEXT NOT NULL, body TEXT NOT NULL)`);
    ctx.storage.sql.exec(`CREATE TABLE IF NOT EXISTS entries (
      cursor TEXT PRIMARY KEY, type TEXT NOT NULL, object_key TEXT NOT NULL, object_digest TEXT NOT NULL,
      journal_key TEXT NOT NULL, journal_digest TEXT NOT NULL, scope_id TEXT, device_id TEXT,
      transport_id TEXT, signed_digest TEXT NOT NULL, generation TEXT)`);
    ctx.storage.sql.exec(`CREATE UNIQUE INDEX IF NOT EXISTS transports ON entries(device_id, transport_id) WHERE type='record'`);
    ctx.storage.sql.exec(`CREATE UNIQUE INDEX IF NOT EXISTS controls ON entries(generation) WHERE type='control'`);
    ctx.storage.sql.exec(`CREATE INDEX IF NOT EXISTS entries_order ON entries(length(cursor),cursor)`);
    ctx.storage.sql.exec(`CREATE INDEX IF NOT EXISTS pending_order ON pending(length(cursor),cursor)`);
  }
  async fetch(req: Request): Promise<Response> {
    if (this.waiting >= 8) return failure(new Fault(429, "vault_busy"));
    this.waiting++;
    const previous = this.tail; let release!: () => void;
    this.tail = new Promise<void>(resolve => { release = resolve; });
    await previous;
    try { return await this.handle(req); } catch (error) { return failure(error); }
    finally { this.waiting--; release(); }
  }
  private get<T>(key: string): T | null {
    const row = this.ctx.storage.sql.exec<{value: string}>("SELECT value FROM meta WHERE key=?", key).toArray()[0];
    return row ? JSON.parse(row.value) as T : null;
  }
  private set(key: string, value: unknown) { this.ctx.storage.sql.exec("INSERT OR REPLACE INTO meta(key,value) VALUES(?,?)", key, JSON.stringify(value)); }
  private meta(): Meta { return this.get<Meta>("committed") ?? {cursor: "0", journal: null, manifest: null}; }
  private async decodeControl(value: Signed) { return control(value, this.env.VAULT_ID, this.env.OWNER_PUBLIC_KEY_B64); }
  private async current(head: Head): Promise<Control> {
    requireThat(head.manifest, "not_bootstrapped", 409); return this.decodeControl(head.manifest);
  }
  // At most eight backward links and eight authenticated forward entries per
  // request. Recovery is resumable; callers retry 503 with bounded backoff.
  private async catchUp(observed: Head): Promise<Meta> {
    let m = this.meta();
    requireThat(BigInt(m.cursor) <= BigInt(observed.cursor), "archive_rollback", 503);
    if (m.cursor === observed.cursor) {
      requireThat(sameRef(m.journal, observed.journal) && (m.manifest === null ? observed.manifest === null : !!observed.manifest && equalSigned(m.manifest, observed.manifest)), "archive_fork", 503);
      return m;
    }
    let w = this.get<Walk>("walk");
    if (!w) { w = {target: observed, next: observed.journal, cursor: observed.cursor, phase: "walk"}; this.set("walk", w); }
    for (let n = 0; w.phase === "walk" && n < 8; n++) {
      if (w.cursor === m.cursor) {
        requireThat(sameRef(w.next, m.journal), "archive_fork", 503); w.phase = "apply"; this.set("walk", w); break;
      }
      requireThat(BigInt(w.cursor) > BigInt(m.cursor) && w.next, "archive_corrupt", 503);
      const reference = w.next;
      const j = journal(await this.archive.get(reference, 4096));
      requireThat(j.cursor === w.cursor, "archive_corrupt", 503);
      this.ctx.storage.transactionSync(() => {
        this.ctx.storage.sql.exec("INSERT OR REPLACE INTO pending(cursor,journal_key,journal_digest,body) VALUES(?,?,?,?)", j.cursor, reference.key, reference.digest, JSON.stringify(j));
        w!.next = j.previous; w!.cursor = (BigInt(j.cursor) - 1n).toString(); this.set("walk", w);
      });
    }
    if (w.phase === "walk" && w.cursor === m.cursor) {
      requireThat(sameRef(w.next, m.journal), "archive_fork", 503); w.phase = "apply"; this.set("walk", w);
    }
    for (let n = 0; w.phase === "apply" && n < 8; n++) {
      const row = this.ctx.storage.sql.exec<{cursor: string; journal_key: string; journal_digest: string; body: string}>("SELECT * FROM pending ORDER BY length(cursor),cursor LIMIT 1").toArray()[0];
      if (!row) break;
      const j = JSON.parse(row.body) as Journal;
      requireThat(j.cursor === (BigInt(m.cursor) + 1n).toString() && sameRef(j.previous, m.journal), "archive_corrupt", 503);
      const raw = await this.archive.get(j.object, MAX_BODY);
      const s = signed(parse(raw));
      const entry: CachedEntry = {cursor: j.cursor, type: j.type, object_key: j.object.key, object_digest: j.object.digest,
        journal_key: row.journal_key, journal_digest: row.journal_digest, scope_id: null, device_id: null,
        transport_id: null, signed_digest: await signDigest(s), generation: null};
      let manifest = m.manifest;
      if (j.type === "control") {
        const c = await this.decodeControl(s);
        const old = m.manifest ? await this.decodeControl(m.manifest) : null;
        requireThat(c.generation === (BigInt(old?.generation ?? "0") + 1n).toString(), "archive_control_gap", 503);
        entry.generation = c.generation; manifest = s;
      } else {
        requireThat(m.manifest, "archive_missing_control", 503);
        const p = await envelope(s, await this.decodeControl(m.manifest));
        entry.scope_id = p.scope_id; entry.device_id = p.device_id; entry.transport_id = p.transport_id;
      }
      m = {cursor: j.cursor, journal: {key: row.journal_key, digest: row.journal_digest}, manifest};
      this.ctx.storage.transactionSync(() => { this.insert(entry); this.set("committed", m); this.ctx.storage.sql.exec("DELETE FROM pending WHERE cursor=?", j.cursor); });
    }
    if (m.cursor === w.target.cursor) {
      requireThat(sameRef(m.journal, w.target.journal) && m.manifest && w.target.manifest && equalSigned(m.manifest, w.target.manifest), "archive_corrupt", 503);
      this.ctx.storage.sql.exec("DELETE FROM meta WHERE key='walk'");
      if (m.cursor === observed.cursor && sameRef(m.journal, observed.journal)) return m;
    }
    throw new Fault(503, "archive_rebuilding");
  }
  private insert(e: CachedEntry) {
    this.ctx.storage.sql.exec(`INSERT INTO entries(cursor,type,object_key,object_digest,journal_key,journal_digest,scope_id,device_id,transport_id,signed_digest,generation) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
      e.cursor,e.type,e.object_key,e.object_digest,e.journal_key,e.journal_digest,e.scope_id,e.device_id,e.transport_id,e.signed_digest,e.generation);
  }
  private async publish(observed: VersionedHead, body: Uint8Array, type: "record" | "control", manifest: Signed | null): Promise<string> {
    const next = (BigInt(observed.head.cursor) + 1n).toString(); requireThat(decimal(next), "cursor_exhausted", 503);
    const blob = await this.archive.immutable("objects", body);
    const j: Journal = {version: 1, cursor: next, previous: observed.head.journal, type, object: blob};
    const link = await this.archive.immutable("journals", utf8(j));
    const head: Head = {version: 1, vault_id: this.env.VAULT_ID, cursor: next, journal: link, manifest};
    // An exception or failed CAS is ambiguous to the caller. Never roll back or
    // acknowledge from SQLite. Retrying exact signed bytes repairs the cache and
    // discovers publication exclusively through head-reachable history.
    requireThat(await this.archive.compareAndSwap(observed.etag, head), "head_changed_retry", 503);
    // The successful conditional R2 response establishes durable reachability.
    // Cache catch-up may fail afterward; a retry still discovers the commit.
    await this.catchUp(head);
    return next;
  }
  private async handle(req: Request): Promise<Response> {
    const path = route(req, this.env);
    const body = await readBody(req, req.method === "GET" ? 0 : (path === "/manifest" ? MAX_CONTROL * 2 : MAX_BODY));
    const observed = await this.archive.head();
    if (path === "/manifest" && req.method === "GET") {
      await this.current(observed.head); return json(observed.head.manifest);
    }
    if (path === "/manifest") {
      const s = signed(parse(body)); const c = await this.decodeControl(s);
      const old = observed.head.manifest ? await this.current(observed.head) : null;
      if (old && c.generation === old.generation) {
        requireThat(equalSigned(s, observed.head.manifest!), "control_conflict", 409);
        await this.catchUp(observed.head);
        const receipt = this.ctx.storage.sql.exec<{cursor: string}>("SELECT cursor FROM entries WHERE generation=?", c.generation).toArray()[0];
        requireThat(receipt, "archive_corrupt", 503); return json({cursor: receipt.cursor, generation: c.generation});
      }
      requireThat(c.generation === (BigInt(old?.generation ?? "0") + 1n).toString(), "control_generation_conflict", 409);
      await this.catchUp(observed.head);
      const cursor = await this.publish(observed, body, "control", s);
      return json({cursor, generation: c.generation});
    }
    const c = await this.current(observed.head);
    const auth = await authenticate(req, body, c);
    await this.catchUp(observed.head);
    if (path === "/records") {
      const s = signed(parse(body)); const p = outerIdentity(s);
      requireThat(p.vault_id === this.env.VAULT_ID && p.scope_id === auth.scope && p.device_id === auth.device, "context_mismatch", 403);
      const duplicate = this.ctx.storage.sql.exec<{cursor: string; scope_id: string; signed_digest: string}>("SELECT cursor,scope_id,signed_digest FROM entries WHERE device_id=? AND transport_id=?", p.device_id,p.transport_id).toArray()[0];
      if (duplicate) {
        requireThat(duplicate.scope_id === p.scope_id && duplicate.signed_digest === await signDigest(s), "transport_conflict", 409);
        return json({cursor: duplicate.cursor, duplicate: true});
      }
      await envelope(s, c);
      const cursor = await this.publish(observed, body, "record", observed.head.manifest);
      return json({cursor, duplicate: false});
    }
    return this.changes(req, observed.head, auth.scope);
  }
  private async changes(req: Request, head: Head, scope: string): Promise<Response> {
    const params = new URL(req.url).searchParams;
    requireThat([...params.keys()].every(k => k === "after" || k === "limit") && params.getAll("after").length === 1 && params.getAll("limit").length <= 1, "invalid_pagination");
    const after = params.get("after"); const limitText = params.get("limit") ?? "100";
    requireThat(decimal(after) && /^(?:[1-9][0-9]?|100)$/.test(limitText), "invalid_pagination");
    requireThat(BigInt(after) <= BigInt(head.cursor), "cursor_ahead", 409);
    // Limit bounds SCANNED entries (including invisible scopes), not only
    // returned entries. A second byte bound keeps responses and R2 reads small.
    const rows = this.ctx.storage.sql.exec<CachedEntry>(`SELECT * FROM entries WHERE (length(cursor),cursor)>(?,?) ORDER BY length(cursor),cursor LIMIT ?`, after.length, after, Number(limitText)).toArray();
    const entries: unknown[] = []; let cursor = after; let size = 0; let reads = 0;
    for (const row of rows) {
      if (row.type === "record" && row.scope_id !== scope) { cursor = row.cursor; continue; }
      if (reads >= 8 || (entries.length > 0 && size + MAX_BODY > 4 * MAX_BODY)) break;
      const raw = await this.archive.get({key: row.object_key, digest: row.object_digest}, MAX_BODY); reads++;
      const s = signed(parse(raw)); size += raw.length;
      entries.push(row.type === "control" ? {cursor: row.cursor, type: "control", manifest: s} : {cursor: row.cursor, type: "record", envelope: s});
      cursor = row.cursor;
    }
    return json({cursor, head: head.cursor, entries, has_more: BigInt(cursor) < BigInt(head.cursor)});
  }
}
