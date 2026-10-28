// Uses the existing black-box local Worker harness and real compiled Go CLI.
// All identities, scope keys and records below are disposable synthetic data.
import assert from "node:assert/strict";
import {readFile, access, stat} from "node:fs/promises";
import path from "node:path";
import {createHash, createPrivateKey, randomBytes, sign} from "node:crypto";

export async function enrollmentCases({command,sync,owner,existingChild,root,scope,record,passed,loseDecisionReply,decisionWasDropped,beforeNextDecision,decisions,relay,vault}) {
  const privateScope=await command(owner,["scope","add","--label","synthetic private collection","--role","project"]);
  const hidden=privateScope.ID;
  assert(hidden,"scope creation did not return identity");
  await sync(owner,scope); await sync(owner,hidden);
  const home=path.join(root,"paired-device"), inviteFile=path.join(root,"pair-invite.json");
  const invite=await command(owner,["device","invite","--scopes",scope,"--out",inviteFile]);
  assert.equal(invite.grants_access,false);
  assert.equal((await stat(inviteFile)).mode & 0o777,0o600);
  const request=await command(home,["device","join","--invite-file",inviteFile,"--once"]);
  assert.equal(request.state,"pending"); assert.equal(request.ready,false);
  await assert.rejects(access(path.join(home,"device.json")));
  const stateBefore=await readFile(path.join(home,"enrollment.json"));
  const repeat=await command(home,["device","join","--once"]);
  assert.equal(repeat.device_id,request.device_id);
  assert.deepEqual(await readFile(path.join(home,"enrollment.json")),stateBefore);
  await command(path.join(root,"competing-device"),["device","join","--invite-file",inviteFile,"--once"],{fail:true});
  const requests=await command(owner,["device","requests"]);
  assert(requests.some(r=>r.request_id===invite.invite_id && r.fingerprint===request.fingerprint));
  assert(!JSON.stringify(requests).includes('"signature"'),"console exposed invitation bearer material");
  const unauth=await fetch(`${relay}/v1/vaults/${vault}/enrollment/requests`);
  assert.equal(unauth.status,403);
  await command(owner,["device","approve","--id",invite.invite_id,"--scopes",scope,"--fingerprint","wrong"],{fail:true});
  await command(owner,["device","approve","--id",invite.invite_id,"--scopes",hidden,"--fingerprint",request.fingerprint],{fail:true});
  passed("invitation-gated discovery preserves new-device keys; unsigned listing, wrong proof and excess scope rejected");

  loseDecisionReply();
  await command(owner,["device","approve","--id",invite.invite_id,"--scopes",scope,"--fingerprint",request.fingerprint],{fail:true});
  assert(decisionWasDropped(),"approval failed before the committed-response fault was injected");
  await command(owner,["store","--scope",scope,"--text","synthetic grant fence must block"],{fail:true});
  assert.equal((await command(owner,["devices","--json"])).pending_operation,true);
  const prepared=Buffer.from(decisions.at(-1));
  const resumed=await command(owner,["device","resume"]);
  assert.equal(resumed.state,"approved");
  assert.deepEqual(decisions.at(-1),prepared,"ambiguous retry changed signed decision or HPKE bytes");
  assert.equal((await command(owner,["devices","--json"])).pending_operation,false);
  // Signing-key proof alone is not proof of decrypting the HPKE package.
  const state=JSON.parse(stateBefore);
  const decision=JSON.parse(prepared);
  const sha=value=>createHash("sha256").update(value).digest("hex");
  const ackBody=Buffer.from(JSON.stringify({decision_sha256:sha(`${decision.payload}\n${decision.signature}\n`),activation_secret_b64:Buffer.alloc(32).toString("base64")}));
  const ackPath=`/v1/vaults/${vault}/enrollment/requests/${invite.invite_id}/ack`;
  const ts=String(Math.floor(Date.now()/1000)),nonce=randomBytes(24).toString("hex");
  const seed=Buffer.from(state.identity.signing_private_key_b64,"base64").subarray(0,32);
  const key=createPrivateKey({key:Buffer.concat([Buffer.from("302e020100300506032b657004220420","hex"),seed]),format:"der",type:"pkcs8"});
  const input=`capsule-memory/enrollment-http/v1\nPOST\n${ackPath}\n${vault}\n${request.device_id}\n${ts}\n${nonce}\n${sha(ackBody)}\n`;
  const falseAck=await fetch(`${relay}${ackPath}`,{method:"POST",body:ackBody,headers:{"Content-Type":"application/json","X-CM-Subject":request.device_id,"X-CM-Time":ts,"X-CM-Nonce":nonce,"X-CM-Signature":sign(null,Buffer.from(input),key).toString("base64")}});
  assert.equal(falseAck.status,400);
  assert.equal((await falseAck.json()).error,"activation_mismatch");
  const joined=await command(home,["device","join","--once"]);
  assert.equal(joined.state,"active"); assert.equal(joined.ready,true);
  assert.deepEqual(joined.scope_ids,[scope]);
  await assert.rejects(access(path.join(home,"owner.json")));
  const child=JSON.parse(await readFile(path.join(home,"device.json"),"utf8"));
  assert.deepEqual(Object.keys(child.scope_keys),[scope]);
  await sync(home,scope);
  assert((await command(home,["get","--scope",scope,"--id",record.record_id])).heads.some(h=>h.revision_id===record.revision_id));
  await command(home,["recall","--scope",hidden,"--query","synthetic"],{fail:true});
  const roundtrip=await command(home,["store","--scope",scope,"--text","synthetic paired device persimmon"]);
  await sync(home,scope); await sync(owner,scope);
  assert((await command(owner,["get","--scope",scope,"--id",roundtrip.record_id])).heads.length>0);
  passed("lost approval reply recovers exact encrypted bytes; scoped device activates without owner secrets and syncs both ways");

  const staleHome=path.join(root,"stale-device"),staleFile=path.join(root,"stale-invite.json");
  const staleInvite=await command(owner,["device","invite","--scopes",scope,"--out",staleFile]);
  const staleRequest=await command(staleHome,["device","join","--invite-file",staleFile,"--once"]);
  beforeNextDecision(async()=>{
    await command(existingChild,["store","--scope",scope,"--text","synthetic archive changed after approval replay"]);
    await sync(existingChild,scope);
  });
  await command(owner,["device","approve","--id",staleInvite.invite_id,"--scopes",scope,"--fingerprint",staleRequest.fingerprint],{fail:true});
  assert.equal((await command(owner,["devices","--json"])).pending_operation,true);
  await assert.rejects(access(path.join(staleHome,"device.json")));
  const cancelled=await command(owner,["device","cancel","--confirm"]);
  assert.equal(cancelled.state,"cancelled");assert.equal(cancelled.request_closed,true);
  assert.equal((await command(owner,["devices","--json"])).pending_operation,false);
  await command(staleHome,["device","join","--once"],{fail:true});
  await command(owner,["device","approve","--id",staleInvite.invite_id,"--scopes",scope,"--fingerprint",staleRequest.fingerprint],{fail:true});
  passed("archive-head race rejects a stale approval; cancellation fences its generation and closes the request");

  const cancelHome=path.join(root,"cancel-granted-device"),cancelFile=path.join(root,"cancel-granted.json");
  const cancelInvite=await command(owner,["device","invite","--scopes",scope,"--out",cancelFile]);
  const cancelRequest=await command(cancelHome,["device","join","--invite-file",cancelFile,"--once"]);
  loseDecisionReply();
  await command(owner,["device","approve","--id",cancelInvite.invite_id,"--scopes",scope,"--fingerprint",cancelRequest.fingerprint],{fail:true});
  assert(decisionWasDropped());
  assert.equal((await command(owner,["device","cancel","--confirm"])).state,"cancelled");
  await command(cancelHome,["device","join","--once"],{fail:true});
  assert.equal((await command(owner,["devices","--json"])).pending_operation,false);
  passed("cancelling an ambiguously committed approval publishes revocation before releasing the owner fence");

  const denyHome=path.join(root,"denied-device"), denyInvite=path.join(root,"deny-invite.json");
  const deniedInvite=await command(owner,["device","invite","--scopes",scope,"--out",denyInvite]);
  const pending=await command(denyHome,["device","join","--invite-file",denyInvite,"--once"]);
  assert.equal((await command(owner,["device","deny","--id",deniedInvite.invite_id])).state,"denied");
  await command(owner,["device","approve","--id",deniedInvite.invite_id,"--scopes",scope,"--fingerprint",pending.fingerprint],{fail:true});
  await command(denyHome,["device","join","--once"],{fail:true});
  await assert.rejects(access(path.join(denyHome,"device.json")));
  const revoked=await command(owner,["device","revoke","--id",request.device_id,"--confirm"]);
  assert(revoked.published && revoked.revoked);
  await command(home,["sync","--scope",scope],{fail:true});
  await command(home,["device","join","--once"],{fail:true});
  passed("denial remains terminal and published revocation blocks both archive sync and delayed key delivery");
}
