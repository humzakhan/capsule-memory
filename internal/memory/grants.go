package memory

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "database/sql"
 "encoding/base64"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "unicode/utf8"
 "github.com/humzakhan/capsule-memory/internal/strictjson"
 "github.com/humzakhan/capsule-memory/internal/records"
)
type GrantCheck func(context.Context,Queryer,Control)error
func (s *Service) OnGrantCheck(h GrantCheck){s.grantHooks=append(s.grantHooks,h)}
func (s *Service) checkGrants(ctx context.Context, proposed Control) error {
	return s.checkGrantsQuery(ctx, s.reader, proposed)
}

func (s *Service) checkGrantsQuery(ctx context.Context, q Queryer, proposed Control) error {
	if s.schemaVersion == 2 {
		if err := s.checkAggregationReaders(ctx, q, proposed); err != nil {
			return err
		}
		if err := s.checkProjectionReaders(ctx, q, proposed); err != nil {
			return err
		}
	}
	for _, hook := range s.grantHooks {
		if err := hook(ctx, q, proposed); err != nil {
			return err
		}
	}
	return nil
}
func contains(a []string,v string)bool{for _,s:=range a{if s==v{return true}};return false}
func syncDir(path string)error{d,e:=os.Open(path);if e!=nil{return e};defer d.Close();return d.Sync()}
func replaceConfig(home string,c Config)error{p:=filepath.Join(home,"device.json.tmp");if e:=writeNew(p,c);e!=nil{return e};if e:=os.Rename(p,filepath.Join(home,"device.json"));e!=nil{return e};return syncDir(home)}
func (s *Service) OwnerSign(domain string,value any)(Signed,error){if domain!="capsule-memory/bridge-grant/v1"&&domain!="capsule-memory/aggregation-grant/v1"&&domain!=ownerInsightDomain{return Signed{},ErrInvalid};k,e:=s.ownerKey();if e!=nil{return Signed{},e};return sign(domain,value,k)}
func (s *Service) VerifyOwner(domain string,signed Signed,dst any)error{if domain!="capsule-memory/bridge-grant/v1"&&domain!="capsule-memory/aggregation-grant/v1"&&domain!=ownerInsightDomain{return ErrInvalid};k,e:=unbase(s.configuration().OwnerPublic,32);if e!=nil{return e};b,e:=unbase(signed.Payload,-1);if e!=nil||len(b)>MaxEnvelope{return ErrInvalid};sig,e:=unbase(signed.Signature,ed25519.SignatureSize);if e!=nil||!ed25519.Verify(ed25519.PublicKey(k),[]byte(domain+"\n"+signed.Payload+"\n"),sig){return ErrInvalid};return strictjson.Decode(b,MaxEnvelope,dst)}
func (s *Service) saveControl(ctx context.Context,tx *sql.Tx,c Control,signed Signed)error{b,e:=json.Marshal(signed);if e!=nil{return e};if _,e=tx.ExecContext(ctx,"INSERT INTO controls(generation,signed) VALUES(?,?)",c.Generation,b);e!=nil{return ErrStorage};if _,e=tx.ExecContext(ctx,"INSERT INTO meta VALUES('current_control',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",b);e!=nil{return ErrStorage};return nil}
type packageKey struct{Epoch string `json:"epoch" cm:"required"`;Key string `json:"key_b64" cm:"required"`;Label string `json:"label" cm:"required"`;Role string `json:"role" cm:"required"`}
type keyPackage struct{Version int `json:"version" cm:"required"`;Vault string `json:"vault_id" cm:"required"`;Device string `json:"device_id" cm:"required"`;Keys map[string]packageKey `json:"scope_keys" cm:"required"`}
func (s *Service) GrantDevice(ctx context.Context,device string,scopes []string,out string)(map[string]any,error){
 s.wmu.Lock();defer s.wmu.Unlock();if e:=s.RequireV2();e!=nil{return nil,e};if e:=s.recoverPendingScope(ctx);e!=nil{return nil,e};owner,e:=s.ownerKey();if e!=nil{return nil,e};c,_,e:=s.current(ctx);if e!=nil{return nil,e};generation:=c.Generation
 idx:=-1;for i,d:=range c.Devices{if d.ID==device&&!d.Revoked{idx=i}};if idx<0||len(scopes)==0{return nil,ErrInvalid}
 p:=keyPackage{Version:1,Vault:c.Vault,Device:device,Keys:map[string]packageKey{}};changed:=false
 for _,scope:=range scopes{k,ok:=s.configuration().Keys[scope];if !ok{return nil,ErrInvalid};found:=false;for _,sc:=range c.Scopes{if sc.ID==scope&&sc.Epoch==k.Epoch{found=true}};if !found{return nil,ErrInvalid};if !contains(c.Devices[idx].Scopes,scope){c.Devices[idx].Scopes=append(c.Devices[idx].Scopes,scope);changed=true}
  v:=packageKey{Epoch:k.Epoch,Key:k.Key,Role:"default"};e=s.reader.QueryRowContext(ctx,"SELECT label,role FROM scope_labels WHERE scope=?",scope).Scan(&v.Label,&v.Role);if e!=nil&&e!=sql.ErrNoRows{return nil,ErrStorage};p.Keys[scope]=v
 };if changed{c.Generation=next(c.Generation)};if e=validateControl(c,c.Vault);e!=nil{return nil,e}
	outputAttempted := false
	e = s.withGrantAuthority(ctx, generation, c, func(tx *sql.Tx) error {
		var signed Signed
		if changed {
			var err error
			signed, err = sign("agent-memory/control/v1", c, owner)
			if err != nil {
				return err
			}
		}
		outputAttempted = true
		if err := writeNew(out, p); err != nil {
			return err
		}
		if changed {
			return s.saveControl(ctx, tx, c, signed)
		}
		return nil
	})
	if e != nil {
		if outputAttempted {
			return nil, errors.Join(e, errors.New("key-package operation not confirmed; preserve any output before retrying"))
		}
		return nil, e
	}
 return map[string]any{"device_id":device,"control_pending":changed,"key_package_written":true},nil
}
func (s *Service) RevokeScopeGrant(ctx context.Context,device string,scopes []string)(map[string]any,error){
 s.wmu.Lock();defer s.wmu.Unlock();if e:=s.RequireV2();e!=nil{return nil,e};if e:=s.recoverPendingScope(ctx);e!=nil{return nil,e};owner,e:=s.ownerKey();if e!=nil{return nil,e};c,_,e:=s.current(ctx);if e!=nil{return nil,e};found:=false
 for i,d:=range c.Devices{if d.ID!=device{continue};found=true;kept:=[]string{};for _,scope:=range d.Scopes{if !contains(scopes,scope){kept=append(kept,scope)}};c.Devices[i].Scopes=kept};if !found{return nil,ErrInvalid};c.Generation=next(c.Generation);if e=validateControl(c,c.Vault);e!=nil{return nil,e};if e=s.checkGrants(ctx,c);e!=nil{return nil,e};signed,e:=sign("agent-memory/control/v1",c,owner);if e!=nil{return nil,e};if e=s.updateLocked(ctx,func(tx *sql.Tx)error{return s.saveControl(ctx,tx,c,signed)});e!=nil{return nil,e};return map[string]any{"device_id":device,"control_pending":true,"old_keys_retained":true,"rotation":"manual"},nil
}
func ImportKeys(ctx context.Context,home,keyFile string)(map[string]any,error){
 s,e:=OpenWith(ctx,home,LockExclusive);if e!=nil{return nil,e};defer s.Close();if e=s.RequireV2();e!=nil{return nil,e};if e=safeFile(keyFile);e!=nil{return nil,e};b,e:=os.ReadFile(keyFile);if e!=nil{return nil,e};var p keyPackage;if strictjson.Decode(b,MaxEnvelope,&p)!=nil||p.Version!=1||p.Vault!=s.configuration().Vault||p.Device!=s.configuration().Device||len(p.Keys)==0||len(p.Keys)>64{return nil,ErrInvalid}
 c:=s.configuration();c.Keys=map[string]ScopeKey{};for id,k:=range s.configuration().Keys{c.Keys[id]=k};for id,k:=range p.Keys{if !idPattern.MatchString(id)||!validRole(k.Role)||len(k.Label)>256||!utf8.ValidString(k.Label){return nil,ErrInvalid};if _,e=unbase(k.Key,32);e!=nil{return nil,e};if _,e=compare(k.Epoch,"0");e!=nil{return nil,e};if old,ok:=c.Keys[id];ok{cmp,e:=compare(k.Epoch,old.Epoch);if e!=nil||cmp<0||cmp==0&&k.Key!=old.Key{return nil,ErrInvalid}};c.Keys[id]=ScopeKey{k.Epoch,k.Key}}
 // Journal contains only private data, permitting idempotent recovery across
 // the filesystem/SQLite boundary without changing device.json's schema.
 pending:=filepath.Join(home,"pending-keys.json");if e=writeNew(pending,p);e!=nil{return nil,e};if e=s.finishKeys(ctx,c,p);e!=nil{return nil,e};return map[string]any{"imported":len(p.Keys)},nil
}
func validRole(r string)bool{return r=="default"||r=="project"||r=="personal"||r=="ledger"}
func (s *Service) AddScope(ctx context.Context,label,role string)(ScopeInfo,error){
 s.wmu.Lock();defer s.wmu.Unlock();var out ScopeInfo;if e:=s.RequireV2();e!=nil{return out,e};if e:=s.recoverPendingScope(ctx);e!=nil{return out,e};if !validRole(role)||len(label)>256||!utf8.ValidString(label){return out,ErrInvalid};if _,e:=s.ownerKey();e!=nil{return out,e};c,_,e:=s.current(ctx);if e!=nil{return out,e};if len(c.Scopes)>=64{return out,ErrInvalid};id,e:=newID();if e!=nil{return out,e};key:=make([]byte,32);if _,e=rand.Read(key);e!=nil{return out,e};p:=pendingScope{Scope:id,Epoch:"1",Key:base64.StdEncoding.EncodeToString(key),Label:label,Role:role,Base:c.Generation}
 proposed:=scopeControl(c,p);for i,d:=range proposed.Devices{if d.ID==s.configuration().Device&&!d.Revoked{proposed.Devices[i].Scopes=append(d.Scopes,p.Scope)}};if e=validateControl(proposed,c.Vault);e!=nil{return out,e};if e=s.checkGrants(ctx,proposed);e!=nil{return out,e};if e=writeNew(filepath.Join(s.home,"pending-scope.json"),p);e!=nil{return out,e};if e=s.finishScope(ctx,p);e!=nil{return out,e};return ScopeInfo{ID:id,Epoch:"1",Label:label,Role:role},nil
}

// The core fallback also runs during pending-file recovery, before application
// hooks can be installed. It verifies owner signatures and every current head.
func(s *Service) checkAggregationReaders(ctx context.Context,q Queryer,c Control)error{
 rows,e:=q.QueryContext(ctx,"SELECT DISTINCT record,scope FROM revisions WHERE kind='aggregation_grant' LIMIT 10001");if e!=nil{return ErrStorage};type ref struct{id,scope string};var refs []ref;for rows.Next(){var r ref;if rows.Scan(&r.id,&r.scope)!=nil{rows.Close();return ErrStorage};refs=append(refs,r)};e=rows.Err();rows.Close();if e!=nil{return ErrStorage};if len(refs)>10000{return ErrInvalid}
 has:=func(device,scope string)bool{for _,d:=range c.Devices{if d.ID==device&&!d.Revoked{for _,s:=range d.Scopes{if s==scope{return true}}}};return false}
 for _,r:=range refs{hh,del,e:=s.Heads(ctx,q,r.scope,r.id);if e!=nil{return e};if del{continue};for _,h:=range hh{var signed records.AggregationGrant;var body records.AggregationGrantBody;if json.Unmarshal(h.Body,&signed)!=nil{return ErrInvalid};if e=s.VerifyOwner("capsule-memory/aggregation-grant/v1",Signed{Payload:signed.Payload,Signature:signed.Signature},&body);e!=nil{return e};if body.Validate()!=nil||body.Vault!=c.Vault||body.Destination!=r.scope{return ErrInvalid};if body.Revoked{continue};for _,d:=range c.Devices{if has(d.ID,body.Destination){for _,scope:=range body.Sources{if !has(d.ID,scope){return errors.New("destination_readers_exceed_sources")}}}}}}
 return nil
}
