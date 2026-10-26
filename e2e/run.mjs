#!/usr/bin/env node
// Owner-machine BLACK-BOX integration journey. No imports from application code,
// no fake relay/SQLite, and no production endpoints. Source artifact: NOT RUN.
import assert from "node:assert/strict";
import {spawn, execFile} from "node:child_process";
import {createHash, createPrivateKey, createPublicKey, generateKeyPairSync, randomBytes, sign, createCipheriv} from "node:crypto";
import {once} from "node:events";
import {mkdtemp, mkdir, readFile, writeFile, copyFile, readdir, rm, stat} from "node:fs/promises";
import http from "node:http";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import readline from "node:readline";
import {fileURLToPath} from "node:url";
import {promisify} from "node:util";

const exec = promisify(execFile);
const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const args = process.argv.slice(2);
const cliIndex = args.indexOf("--cli");
assert(cliIndex >= 0 && args[cliIndex + 1], "usage: node e2e/run.mjs --cli /absolute/path/agent-memory [--keep]");
const cli = path.resolve(args[cliIndex + 1]);
const keep = args.includes("--keep");
assert(process.platform !== "win32", "this harness currently requires POSIX process groups; Windows is unsupported");
const wrangler = path.join(repo, "cloudflare/node_modules/wrangler/bin/wrangler.js");
await stat(cli); await stat(wrangler);
const root = await mkdtemp(path.join(os.tmpdir(), "agent-memory-e2e-"));
const owner = path.join(root, "owner"), child = path.join(root, "child"), recovered = path.join(root, "recovered");
const state = path.join(root, "relay-state");
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const sha = data => createHash("sha256").update(data).digest("hex");
const opaqueID = () => randomBytes(24).toString("base64url");
const children = new Set();
let worker, workerLog = "", proxy, offline = false, dropNextRecord = false, bootstrap;
const capturedRecords = [];
let capturedControl;
const completed = [];
function passed(name) { completed.push(name); process.stdout.write(`PASS ${name}\n`); }
function track(process) {
  children.add(process); process.once("exit", () => children.delete(process)); return process;
}
async function stop(process) {
  if (!process || process.exitCode !== null || process.signalCode !== null) return;
  const exited = once(process, "exit");
  try { process.kill("SIGTERM"); } catch {}
  let force;
  try { await Promise.race([exited, new Promise(resolve => { force = setTimeout(() => { try { process.kill("SIGKILL"); } catch {} resolve(); }, 5000); })]); }
  finally { clearTimeout(force); }
  if (process.exitCode === null && process.signalCode === null) await exited;
}
async function stopWorker() {
  if (!worker) return;
  const process = worker; worker = undefined;
  // Wrangler can have workerd/esbuild descendants. Stop this harness's private
  // process group before touching its emulator files, never a shared process.
  const exited = process.exitCode !== null || process.signalCode !== null ? Promise.resolve() : once(process, "exit");
  try { globalThis.process.kill(-process.pid, "SIGTERM"); } catch {}
  await Promise.race([exited, pause(3000)]);
  try { globalThis.process.kill(-process.pid, "SIGKILL"); } catch {}
  await exited;
}
async function availablePort() {
  const server = net.createServer(); server.listen(0, "127.0.0.1"); await once(server, "listening");
  const port = server.address().port; await new Promise(resolve => server.close(resolve)); return port;
}
async function command(home, commandArgs, {fail = false} = {}) {
  try {
    const {stdout} = await exec(cli, ["--home", home, ...commandArgs], {timeout: 150000, maxBuffer: 8 * 1024 * 1024, env: {...process.env, NO_COLOR:"1"}});
    assert(!fail, "command unexpectedly succeeded");
    return JSON.parse(stdout);
  } catch (error) {
    if (fail && typeof error.code === "number" && error.code !== 0) return {failed: true};
    // Child diagnostics may someday contain content. Do not dump stdout,
    // stderr, command arguments, configs, envelopes, or keys on failure.
    throw new Error(`CLI ${commandArgs[0]} failed (${typeof error.code === "number" ? error.code : "invalid output"})`);
  }
}
async function sync(home, scope) {
  // Real CLI reports HTTP status on stderr. Retry only temporary relay states;
  // syntax, auth, crypto, and domain errors fail this journey immediately.
  for (let attempt = 0; attempt < 40; attempt++) {
    try {
      const {stdout} = await exec(cli, ["--home",home,"sync","--scope",scope], {timeout:150000,maxBuffer:8*1024*1024});
      const result = JSON.parse(stdout);
      if (result.caught_up && result.pending === 0) return result;
    } catch (error) {
      const diagnostic = String(error.stderr ?? "");
      if (!/HTTP (503|429)/.test(diagnostic)) throw new Error("sync failed outside retryable relay states");
    }
    await pause(150 + Math.min(attempt * 100, 2000));
  }
  throw new Error("sync did not converge within the bounded retry budget");
}
async function startWorker(port) {
  workerLog = "";
  // No credentials passed to Wrangler. Local-only bindings need no account.
  const cleanEnv = Object.fromEntries(["PATH","HOME","TMPDIR","TEMP","SYSTEMROOT"].filter(k => process.env[k]).map(k => [k,process.env[k]]));
  worker = track(spawn(process.execPath, [wrangler,"dev","--local","--ip","127.0.0.1","--port",String(port),"--persist-to",state,
    "--var",`VAULT_ID:${bootstrap.vault_id}`,"--var",`OWNER_PUBLIC_KEY_B64:${bootstrap.OWNER_PUBLIC_KEY_B64}`,"--var","ALLOW_LOOPBACK_HTTP:true"],
    {cwd:path.join(repo,"cloudflare"),env:{...cleanEnv,WRANGLER_SEND_METRICS:"false",NO_UPDATE_NOTIFIER:"1",CI:"true"},detached:true,stdio:["ignore","pipe","pipe"]}));
  worker.stdout.on("data", chunk => { workerLog = (workerLog + chunk).slice(-16000); });
  worker.stderr.on("data", chunk => { workerLog = (workerLog + chunk).slice(-16000); });
  worker.on("error", () => {});
  for (let n = 0; n < 150; n++) {
    assert(worker.exitCode === null && worker.signalCode === null, "Wrangler exited before readiness; inspect the private artifact directory");
    try {
      const response = await fetch(`http://127.0.0.1:${port}/v1/vaults/${bootstrap.vault_id}/manifest`, {signal:AbortSignal.timeout(1000)});
      if ([200,409].includes(response.status)) return;
    } catch {}
    await pause(200);
  }
  throw new Error("local Wrangler readiness timed out");
}
function privateKey(raw64) {
  const key = Buffer.from(raw64,"base64"); assert.equal(key.length,64);
  return createPrivateKey({key:Buffer.concat([Buffer.from("302e020100300506032b657004220420","hex"),key.subarray(0,32)]),format:"der",type:"pkcs8"});
}
function signedPayload(kind, value, key) {
  const payload = Buffer.from(JSON.stringify(value)).toString("base64");
  return {payload,signature:sign(null,Buffer.from(`agent-memory/${kind}/v1\n${payload}\n`),key).toString("base64")};
}
async function authFetch(config, route, {method="GET",body=Buffer.alloc(0),scope=bootstrap.scope_id,time=String(Math.floor(Date.now()/1000))} = {}) {
  const pathname = `/v1/vaults/${config.vault_id}${route}`;
  const nonce = randomBytes(24).toString("hex");
  const input = `agent-memory/request/v1\n${method}\n${pathname}\n${config.vault_id}\n${scope}\n${config.device_id}\n${time}\n${nonce}\n${sha(body)}\n`;
  const signature = sign(null,Buffer.from(input),privateKey(config.device_private_key_b64)).toString("base64");
  return fetch(`${proxyURL}${pathname}`,{method,headers:{"Content-Type":"application/json","X-AM-Device":config.device_id,"X-AM-Scope":scope,"X-AM-Time":time,"X-AM-Nonce":nonce,"X-AM-Signature":signature},body:method==="GET"?undefined:body,signal:AbortSignal.timeout(10000)});
}
async function mcp(home, scope, run) {
  const process = track(spawn(cli,["--home",home,"mcp","--scope",scope],{stdio:["pipe","pipe","pipe"]}));
  const waiting = new Map(); let sequence = 0; let buffered = 0;
  process.stderr.resume();
  const lines = readline.createInterface({input:process.stdout});
  lines.on("line", line => {
    buffered += Buffer.byteLength(line); if (buffered > 8*1024*1024) { process.kill("SIGKILL"); return; }
    try {
      const response = JSON.parse(line); const pending = waiting.get(response.id);
      if (pending) { waiting.delete(response.id); response.error ? pending.reject(new Error("MCP protocol error")) : pending.resolve(response.result); }
    } catch { for (const pending of waiting.values()) pending.reject(new Error("MCP emitted non-JSON output")); }
  });
  function request(method, params) {
    const id = ++sequence;
    return new Promise((resolve,reject) => {
      const timer = setTimeout(() => { waiting.delete(id); reject(new Error("MCP request timed out")); },15000);
      waiting.set(id,{resolve:value=>{clearTimeout(timer);resolve(value);},reject:error=>{clearTimeout(timer);reject(error);}});
      process.stdin.write(JSON.stringify({jsonrpc:"2.0",id,method,params})+"\n");
    });
  }
  async function tool(name, args) {
    const result = await request("tools/call",{name,arguments:args}); assert(!result.isError,"MCP tool failed");
    const text = result.content.find(c=>c.type==="text"); assert(text,"MCP tool returned no text JSON"); return JSON.parse(text.text);
  }
  try {
    await request("initialize",{protocolVersion:"2025-03-26",capabilities:{},clientInfo:{name:"agent-memory-blackbox",version:"1"}});
    process.stdin.write(JSON.stringify({jsonrpc:"2.0",method:"notifications/initialized"})+"\n");
    return await run(tool, request);
  } finally { lines.close(); process.stdin.end(); await stop(process); }
}

let proxyURL;
let failed = false;
try {
  const workerPort = await availablePort();
  proxy = http.createServer(async (req,res) => {
    try {
      const chunks=[]; let count=0;
      for await (const chunk of req) { count+=chunk.length; assert(count<=3*1024*1024); chunks.push(chunk); }
      const body=Buffer.concat(chunks);
      if (offline) { res.writeHead(503); res.end(); return; }
      const isRecord=req.method==="POST"&&req.url.endsWith("/records");
      if (isRecord) capturedRecords.push({body,headers:{...req.headers}});
      if (req.method==="PUT"&&req.url.endsWith("/manifest")) capturedControl=Buffer.from(body);
      const upstream = await fetch(`http://127.0.0.1:${workerPort}${req.url}`,{method:req.method,headers:{...req.headers,host:`127.0.0.1:${workerPort}`},body:["GET","HEAD"].includes(req.method)?undefined:body,signal:AbortSignal.timeout(30000)});
      const response=Buffer.from(await upstream.arrayBuffer());
      // Real Worker has committed before we destroy the client response.
      if (isRecord && dropNextRecord && upstream.ok) { dropNextRecord=false; res.destroy(); return; }
      res.writeHead(upstream.status,{"Content-Type":"application/json"}); res.end(response);
    } catch { if (!res.destroyed) { res.writeHead(502); res.end(); } }
  });
  proxy.listen(0,"127.0.0.1"); await once(proxy,"listening");
  proxyURL=`http://127.0.0.1:${proxy.address().port}`;
  bootstrap=await command(owner,["init","--relay",proxyURL,"--allow-loopback-http"]);
  assert(bootstrap.vault_id&&bootstrap.scope_id&&bootstrap.OWNER_PUBLIC_KEY_B64,"init lacks public deployment values");
  const scope=bootstrap.scope_id;
  // Enrollment before the owner's first sync must preserve bootstrap generation
  // 1, then publish the successor in order rather than skipping the root grant.
  await command(owner,["device","create","--out",child]);
  await startWorker(workerPort);
  await sync(owner,scope);
  assert(capturedControl,"owner bootstrap was not published through the real Worker");
  const originalControl = await fetch(`${proxyURL}/v1/vaults/${bootstrap.vault_id}/manifest`,{method:"PUT",headers:{"Content-Type":"application/json"},body:capturedControl});
  assert.equal(originalControl.status,200,"exact owner bootstrap retry failed");
  await sync(child,scope);
  passed("pre-sync enrollment publishes ordered bootstrap controls; exact current control retry succeeds");

  const mcpRecord=await mcp(owner,scope,async tool=>{
    const record=await tool("memory_store",{kind:"memory",text:"synthetic provenance orchard"});
    const recall=await tool("memory_recall",{query:"orchard"});
    const hit=recall.results.find(r=>r.revision_id===record.revision_id);
    assert(hit?.provenance?.source_kind==="harness","MCP provenance missing");
    assert(hit.author_device_id===bootstrap.device_id&&recall.coverage,"MCP author/coverage missing"); return record;
  });
  await sync(owner,scope); await sync(child,scope);
  const childRecall=await command(child,["recall","--scope",scope,"--query","orchard"]);
  assert(childRecall.results.some(r=>r.revision_id===mcpRecord.revision_id&&r.provenance.source_kind==="harness"));
  passed("MCP store/recall provenance survives encrypted publication and replica replay");

  offline=true;
  const queued=await command(owner,["store","--scope",scope,"--text","synthetic offline apricot"]);
  await command(owner,["sync","--scope",scope],{fail:true});
  assert((await command(owner,["status"])).pending>0,"offline write lost its durable outbox");
  offline=false; await sync(owner,scope); await sync(child,scope);
  assert((await command(child,["recall","--scope",scope,"--query","apricot"])).results.some(r=>r.record_id===queued.record_id));
  passed("offline write remains queued and replays after reconnect");

  const base=await command(owner,["store","--scope",scope,"--text","synthetic conflict plum"]);
  await sync(owner,scope); await sync(child,scope);
  const editA=await command(owner,["store","--scope",scope,"--id",base.record_id,"--parents",base.revision_id,"--text","synthetic branch alpha"]);
  const editB=await command(child,["store","--scope",scope,"--id",base.record_id,"--parents",base.revision_id,"--text","synthetic branch beta"]);
  await sync(owner,scope); await sync(child,scope); await sync(owner,scope);
  const conflict=await command(owner,["get","--scope",scope,"--id",base.record_id,"--history"]);
  assert(conflict.conflict&&conflict.heads.length===2);
  assert.deepEqual(new Set(conflict.heads.map(r=>r.revision_id)),new Set([editA.revision_id,editB.revision_id]));
  const deletion=await command(owner,["delete","--scope",scope,"--id",base.record_id]);
  const stale=await command(child,["store","--scope",scope,"--id",base.record_id,"--parents",base.revision_id,"--text","synthetic resurrection forbidden"]);
  await sync(owner,scope); await sync(child,scope); await sync(owner,scope);
  for (const home of [owner,child]) {
    const view=await command(home,["get","--scope",scope,"--id",base.record_id,"--history"]);
    assert(view.deleted&&view.history.some(r=>r.revision_id===stale.revision_id)&&view.history.some(r=>r.revision_id===deletion.revision_id));
    assert(!(await command(home,["recall","--scope",scope,"--query","resurrection"])).results.some(r=>r.record_id===base.record_id));
  }
  passed("concurrent heads preserved; deletion suppresses a stale edit without losing history");

  const ambiguous=await command(owner,["store","--scope",scope,"--text","synthetic ambiguous nectarine"]);
  const beforePosts=capturedRecords.length; dropNextRecord=true;
  await command(owner,["sync","--scope",scope],{fail:true});
  assert(!dropNextRecord,"fault injection never observed a committed Worker response");
  const lost=capturedRecords.at(-1); assert(capturedRecords.length===beforePosts+1);
  assert((await command(owner,["status"])).pending>0);
  await sync(owner,scope);
  const ownerConfig=JSON.parse(await readFile(path.join(owner,"device.json"),"utf8"));
  const duplicate=await authFetch(ownerConfig,"/records",{method:"POST",body:lost.body});
  assert.equal(duplicate.status,200); const receipt=await duplicate.json(); assert.equal(receipt.duplicate,true);
  const onceOnly=await command(owner,["get","--scope",scope,"--id",ambiguous.record_id,"--history"]);
  assert.equal(onceOnly.history.length,1);
  passed("lost POST acknowledgement and exact duplicate recover one logical revision");

  const pending=await command(owner,["store","--scope",scope,"--text","synthetic rebuild pineapple"]);
  const pendingCount=(await command(owner,["status"])).pending;
  assert(pendingCount>0); await command(owner,["rebuild"]);
  assert.equal((await command(owner,["status"])).pending,pendingCount);
  assert((await command(owner,["recall","--scope",scope,"--query","pineapple"])).results.some(r=>r.revision_id===pending.revision_id));
  await sync(owner,scope); await sync(child,scope);
  await mkdir(recovered,{mode:0o700}); await copyFile(path.join(child,"device.json"),path.join(recovered,"device.json"));
  await sync(recovered,scope);
  const restored=await command(recovered,["get","--scope",scope,"--id",base.record_id,"--history"]);
  assert(restored.deleted&&restored.history.some(r=>r.revision_id===stale.revision_id));
  assert((await command(recovered,["recall","--scope",scope,"--query","orchard"])).results.length===1);
  passed("index rebuild preserves queued writes; empty replica replays archive and tombstones");

  const staged=await command(owner,["store","--scope",scope,"--kind","skill","--text","# synthetic skill\nRead only; no commands."]);
  await sync(owner,scope); await sync(child,scope);
  await command(child,["skill","activate","--scope",scope,"--id",staged.record_id,"--revision",staged.revision_id],{fail:true});
  await command(child,["skill","approve","--scope",scope,"--id",staged.record_id,"--revision",staged.revision_id],{fail:true});
  const stagedList=await command(child,["skill","list","--scope",scope]); assert(stagedList.staged&&stagedList.activation_supported===false);
  passed("incoming skill stays staged; non-owner approval and unapproved activation rejected");

  const captured=JSON.parse(lost.body); const outer=JSON.parse(Buffer.from(captured.payload,"base64"));
  const tampered={...outer,transport_id:opaqueID()};
  const ciphertext=Buffer.from(tampered.ciphertext_b64,"base64"); ciphertext[0]^=1; tampered.ciphertext_b64=ciphertext.toString("base64");
  const invalid=Buffer.from(JSON.stringify({...captured,payload:Buffer.from(JSON.stringify(tampered)).toString("base64")}));
  assert.equal((await authFetch(ownerConfig,"/records",{method:"POST",body:invalid})).status,400);
  const badSignature={...outer,transport_id:opaqueID()};
  const unsignedChange=Buffer.from(JSON.stringify({...captured,payload:Buffer.from(JSON.stringify(badSignature)).toString("base64")}));
  assert.equal((await authFetch(ownerConfig,"/records",{method:"POST",body:unsignedChange})).status,403);
  const substituted={...outer,scope_id:opaqueID(),transport_id:opaqueID()};
  const wrongScope=Buffer.from(JSON.stringify({...captured,payload:Buffer.from(JSON.stringify(substituted)).toString("base64")}));
  assert.equal((await authFetch(ownerConfig,"/records",{method:"POST",body:wrongScope})).status,403);
  assert.equal((await authFetch(ownerConfig,"/changes?after=0&limit=1",{scope:opaqueID()})).status,403);
  assert.equal((await authFetch(ownerConfig,"/changes?after=0&limit=1",{time:String(Math.floor(Date.now()/1000)-301)})).status,403);
  assert.equal((await authFetch(ownerConfig,"/changes?after=00&limit=1")).status,400);
  passed("tampering, cross-scope substitution, unauthorized scope, expired auth, noncanonical cursor rejected");

  // A separate HTTP-only synthetic scope checks omission+watermark semantics.
  // This is a real signed/encrypted request to the same local Worker, not a mock.
  let publicManifest=await (await fetch(`${proxyURL}/v1/vaults/${bootstrap.vault_id}/manifest`)).json();
  const ownerSecret=JSON.parse(await readFile(path.join(owner,"owner.json"),"utf8"));
  const ownerKey=privateKey(ownerSecret.private_key_b64);
  const nextControl=JSON.parse(Buffer.from(publicManifest.payload,"base64"));
  const hiddenScope=opaqueID(), hiddenDevice=opaqueID(); const pair=generateKeyPairSync("ed25519");
  const rawPublic=createPublicKey(pair.privateKey).export({format:"der",type:"spki"}).subarray(-32).toString("base64");
  nextControl.generation=(BigInt(nextControl.generation)+1n).toString(); nextControl.scopes.push({scope_id:hiddenScope,key_epoch:"1"});
  nextControl.devices.push({device_id:hiddenDevice,public_key_b64:rawPublic,scope_ids:[hiddenScope],revoked:false});
  publicManifest=signedPayload("control",nextControl,ownerKey);
  const ctrlResponse=await fetch(`${proxyURL}/v1/vaults/${bootstrap.vault_id}/manifest`,{method:"PUT",body:JSON.stringify(publicManifest)});
  assert.equal(ctrlResponse.status,200); const ctrlReceipt=await ctrlResponse.json();
  const hiddenTransport=opaqueID(), nonce=randomBytes(12), contentKey=randomBytes(32);
  const cipher=createCipheriv("aes-256-gcm",contentKey,nonce);
  cipher.setAAD(Buffer.from(`agent-memory/aead/v1\n${bootstrap.vault_id}\n${hiddenScope}\n${hiddenDevice}\n1\n${hiddenTransport}\n`));
  const encrypted=Buffer.concat([cipher.update(Buffer.from('{"synthetic":"hidden"}')),cipher.final(),cipher.getAuthTag()]);
  const hiddenEnvelope=signedPayload("envelope",{version:1,vault_id:bootstrap.vault_id,scope_id:hiddenScope,device_id:hiddenDevice,key_epoch:"1",transport_id:hiddenTransport,nonce_b64:nonce.toString("base64"),ciphertext_b64:encrypted.toString("base64"),ciphertext_sha256:sha(encrypted)},pair.privateKey);
  const hiddenSeed=pair.privateKey.export({format:"der",type:"pkcs8"}).subarray(-32);
  const hiddenConfig={vault_id:bootstrap.vault_id,device_id:hiddenDevice,device_private_key_b64:Buffer.concat([hiddenSeed,Buffer.from(rawPublic,"base64")]).toString("base64")};
  const hiddenPost=await authFetch(hiddenConfig,"/records",{method:"POST",scope:hiddenScope,body:Buffer.from(JSON.stringify(hiddenEnvelope))});
  assert.equal(hiddenPost.status,200); const hiddenReceipt=await hiddenPost.json();
  const filtered=await authFetch(ownerConfig,`/changes?after=${ctrlReceipt.cursor}&limit=1`); assert.equal(filtered.status,200);
  const filteredPage=await filtered.json(); assert.deepEqual(filteredPage.entries,[]); assert.equal(filteredPage.cursor,hiddenReceipt.cursor); assert.equal(filteredPage.has_more,false);
  passed("scope-filtered pagination withholds hidden ciphertext and advances scanned watermark");

  // Discard ONLY the disposable DO emulator cache, with Wrangler stopped.
  // R2 is a separate subtree and must remain intact. Refuse unknown layouts.
  await stopWorker();
  const layout=await readdir(path.join(state,"v3"),{withFileTypes:true});
  assert(layout.some(d=>d.name==="do"&&d.isDirectory())&&layout.some(d=>d.name==="r2"&&d.isDirectory()),"unsupported emulator persistence layout; DO reset scenario NOT COVERED");
  await rm(path.join(state,"v3","do"),{recursive:true});
  await startWorker(workerPort);
  await sync(owner,scope);
  const afterReset=await authFetch(ownerConfig,"/records",{method:"POST",body:lost.body});
  assert.equal(afterReset.status,200); const resetReceipt=await afterReset.json(); assert(resetReceipt.duplicate&&resetReceipt.cursor===receipt.cursor);
  passed("discarded SQLite DO cache rebuilds from retained R2 history and preserves duplicate receipt");

  // Revoke an author AFTER its legitimate publications; old entries must remain
  // valid to a fresh authorized replica, while current relay access is denied.
  const revokedControl=JSON.parse(Buffer.from(publicManifest.payload,"base64"));
  const childConfig=JSON.parse(await readFile(path.join(child,"device.json"),"utf8"));
  revokedControl.generation=(BigInt(revokedControl.generation)+1n).toString();
  revokedControl.devices.find(d=>d.device_id===childConfig.device_id).revoked=true;
  const revokeResponse=await fetch(`${proxyURL}/v1/vaults/${bootstrap.vault_id}/manifest`,{method:"PUT",body:JSON.stringify(signedPayload("control",revokedControl,ownerKey))});
  assert.equal(revokeResponse.status,200);
  assert.equal((await authFetch(childConfig,"/changes?after=0&limit=1")).status,403);
  const ownerRecovery=path.join(root,"owner-recovery"); await mkdir(ownerRecovery,{mode:0o700});
  await copyFile(path.join(owner,"device.json"),path.join(ownerRecovery,"device.json"));
  await sync(ownerRecovery,scope);
  const historical=await command(ownerRecovery,["get","--scope",scope,"--id",base.record_id,"--history"]);
  assert(historical.deleted&&historical.history.some(r=>r.revision_id===editB.revision_id&&r.author_device_id===childConfig.device_id));
  passed("revoked device blocked now; fresh authorized recovery validates its historical publications");
  process.stdout.write(`PASS ${completed.length} black-box scenarios (LOCAL EMULATOR ONLY; production durability/quotas NOT VALIDATED)\n`);
} catch (error) {
  failed=true;
  process.stderr.write(`FAIL ${String(error.message).slice(0,400)}\n`);
  process.exitCode=1;
} finally {
  await stopWorker();
  for (const process of [...children]) await stop(process);
  if (proxy) { proxy.closeAllConnections(); await new Promise(resolve=>proxy.close(resolve)); }
  if (keep || failed) {
    await writeFile(path.join(root,"wrangler-last.log"),workerLog,{mode:0o600});
    process.stdout.write(`Private synthetic artifacts retained: ${root}\n`);
  } else await rm(root,{recursive:true,force:true});
}
