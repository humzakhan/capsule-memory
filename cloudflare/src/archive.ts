import {
  Fault,
  MAX_BODY,
  MAX_CONTROL,
  decimal,
  fields,
  hash,
  object,
  parse,
  requireThat,
  signed,
  type Signed,
} from "./wire";

export interface Ref {
  key: string;
  digest: string;
}
export interface Head {
  version: 1;
  vault_id: string;
  cursor: string;
  journal: Ref | null;
  manifest: Signed | null;
}
export interface Journal {
  version: 1;
  cursor: string;
  previous: Ref | null;
  type: "record" | "control";
  object: Ref;
}
export interface VersionedHead {
  head: Head;
  etag: string | null;
}

// Narrow provider boundary: no list(), deletion, CDN, metadata-based commit inference,
// or provider credentials in domain code. Conditional writes are mandatory.
export interface Archive {
  head(): Promise<VersionedHead>;
  get(ref: Ref, max: number): Promise<Uint8Array>;
  immutable(kind: "objects" | "journals", body: Uint8Array): Promise<Ref>;
  compareAndSwap(expected: string | null, head: Head): Promise<boolean>;
}
export const utf8 = (value: unknown) => new TextEncoder().encode(JSON.stringify(value));
// A timeout does NOT cancel an in-flight provider write. Its outcome remains
// ambiguous and is resolved by immutable create-once/head CAS on an exact retry.
export async function provider<T>(operation: Promise<T>): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([
      operation,
      new Promise<never>((_, reject) => {
        timer = setTimeout(() => reject(new Fault(503, "archive_timeout")), 10000);
      }),
    ]);
  } finally {
    if (timer !== undefined) clearTimeout(timer);
  }
}
export function ref(value: unknown, prefix: string): Ref {
  requireThat(object(value), "archive_corrupt", 503);
  fields(value, ["key", "digest"]);
  requireThat(
    typeof value.digest === "string" &&
      /^[0-9a-f]{64}$/.test(value.digest) &&
      value.key === `${prefix}/${value.digest}`,
    "archive_corrupt",
    503,
  );
  return value as unknown as Ref;
}
export function journal(body: Uint8Array): Journal {
  const value = parse(body);
  requireThat(object(value), "archive_corrupt", 503);
  fields(value, ["version", "cursor", "previous", "type", "object"]);
  requireThat(
    value.version === 1 &&
      decimal(value.cursor) &&
      value.cursor !== "0" &&
      (value.type === "record" || value.type === "control"),
    "archive_corrupt",
    503,
  );
  ref(value.object, "objects");
  if (value.previous !== null) ref(value.previous, "journals");
  requireThat((value.cursor === "1") === (value.previous === null), "archive_corrupt", 503);
  return value as unknown as Journal;
}
export class R2Archive implements Archive {
  private prefix: string;
  constructor(
    private bucket: R2Bucket,
    private vault: string,
  ) {
    this.prefix = `v1/${vault}/`;
  }
  async head(): Promise<VersionedHead> {
    const value = await provider(this.bucket.get(this.prefix + "head"));
    if (!value)
      return {
        head: { version: 1, vault_id: this.vault, cursor: "0", journal: null, manifest: null },
        etag: null,
      };
    requireThat(value.size <= MAX_CONTROL * 2, "archive_corrupt", 503);
    const h = parse(new Uint8Array(await provider(value.arrayBuffer())));
    requireThat(object(h), "archive_corrupt", 503);
    fields(h, ["version", "vault_id", "cursor", "journal", "manifest"]);
    requireThat(
      h.version === 1 && h.vault_id === this.vault && decimal(h.cursor) && h.cursor !== "0",
      "archive_corrupt",
      503,
    );
    ref(h.journal, "journals");
    signed(h.manifest);
    return { head: h as unknown as Head, etag: value.etag };
  }
  async get(reference: Ref, max: number): Promise<Uint8Array> {
    const value = await provider(this.bucket.get(this.prefix + reference.key));
    requireThat(value && value.size <= max, "archive_missing_or_oversize", 503);
    const body = new Uint8Array(await provider(value.arrayBuffer()));
    requireThat((await hash(body)) === reference.digest, "archive_digest_mismatch", 503);
    return body;
  }
  async immutable(kind: "objects" | "journals", body: Uint8Array): Promise<Ref> {
    requireThat(body.length <= (kind === "objects" ? MAX_BODY : 4096), "body_too_large", 413);
    const digest = await hash(body);
    const key = `${kind}/${digest}`;
    // Headers give explicit RFC If-None-Match:* create-once semantics.
    const result = await provider(
      this.bucket.put(this.prefix + key, body, { onlyIf: new Headers({ "If-None-Match": "*" }) }),
    );
    if (!result) {
      const existing = await this.get({ key, digest }, body.length);
      requireThat(existing.length === body.length, "archive_collision", 503);
    }
    return { key, digest };
  }
  async compareAndSwap(expected: string | null, head: Head): Promise<boolean> {
    const onlyIf = new Headers(
      expected === null ? { "If-None-Match": "*" } : { "If-Match": `"${expected}"` },
    );
    const result = await provider(this.bucket.put(this.prefix + "head", utf8(head), { onlyIf }));
    return result !== null;
  }
}
