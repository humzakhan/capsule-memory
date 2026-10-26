package memory

import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "unicode/utf8"
 "github.com/humzakhan/capsule-memory/internal/strictjson"
)
var errScopeIncomplete=errors.New("scope_add_incomplete: pending scope requires owner recovery")
type pendingScope struct{Scope string `json:"scope_id" cm:"required"`;Epoch string `json:"epoch" cm:"required"`;Key string `json:"key_b64" cm:"required"`;Label string `json:"label" cm:"required"`;Role string `json:"role" cm:"required"`;Base string `json:"base_generation" cm:"required"`}
func scopeControl(c Control,p pendingScope)Control{c.Scopes=append(append([]Scope{},c.Scopes...),Scope{p.Scope,p.Epoch});c.Devices=append([]Device{},c.Devices...);for i,d:=range c.Devices{if d.ID!=""{d.Scopes=append([]string{},d.Scopes...);c.Devices[i]=d}};c.Generation=next(c.Generation);return c}
func (s *Service) recoverPendingScope(ctx context.Context)error{
 keyPath:=filepath.Join(s.home,"pending-keys.json");if _,e:=os.Lstat(keyPath);e==nil{if e=safeFile(keyPath);e!=nil{return e};b,e:=os.ReadFile(keyPath);if e!=nil{return e};var p keyPackage;if strictjson.Decode(b,MaxEnvelope,&p)!=nil||p.Version!=1||p.Vault!=s.configuration().Vault||p.Device!=s.configuration().Device||len(p.Keys)==0||len(p.Keys)>64{return ErrInvalid};c:=s.configuration();c.Keys=map[string]ScopeKey{};for id,k:=range s.configuration().Keys{c.Keys[id]=k};for id,k:=range p.Keys{if !idPattern.MatchString(id)||!validRole(k.Role)||len(k.Label)>256||!utf8.ValidString(k.Label){return ErrInvalid};if _,e=unbase(k.Key,32);e!=nil{return e};if _,e=compare(k.Epoch,"0");e!=nil{return e};if old,ok:=c.Keys[id];ok{cmp,e:=compare(k.Epoch,old.Epoch);if e!=nil||cmp<0||cmp==0&&k.Key!=old.Key{return ErrInvalid}};c.Keys[id]=ScopeKey{k.Epoch,k.Key}};if e=s.finishKeys(ctx,c,p);e!=nil{return e}}else if !os.IsNotExist(e){return e}
 path:=filepath.Join(s.home,"pending-scope.json");if _,e:=os.Lstat(path);os.IsNotExist(e){return nil}else if e!=nil{return e};if e:=safeFile(path);e!=nil{return e};b,e:=os.ReadFile(path);if e!=nil{return e};var p pendingScope;if strictjson.Decode(b,MaxEnvelope,&p)!=nil||!idPattern.MatchString(p.Scope)||p.Epoch!="1"||!validRole(p.Role)||len(p.Label)>256||!utf8.ValidString(p.Label){return ErrInvalid};if _,e=unbase(p.Key,32);e!=nil{return e};return s.finishScope(ctx,p)
}
func (s *Service) installConfig(c Config)error{
 // A leftover temporary file is accepted only if it contains exactly the
 // intended replacement. Never discard an unrelated interrupted operation.
 tmp:=filepath.Join(s.home,"device.json.tmp");if _,e:=os.Lstat(tmp);e==nil{if e=safeFile(tmp);e!=nil{return e};b,e:=os.ReadFile(tmp);if e!=nil{return e};want,e:=json.Marshal(c);if e!=nil{return e};if !equalBytes(b,want){return errScopeIncomplete};if e=os.Rename(tmp,filepath.Join(s.home,"device.json"));e!=nil{return e};if e=syncDir(s.home);e!=nil{return e}}else if !os.IsNotExist(e){return e}else if e=replaceConfig(s.home,c);e!=nil{return e};s.setConfig(c);return nil
}
func (s *Service) finishKeys(ctx context.Context,c Config,p keyPackage)error{
 if e:=s.installConfig(c);e!=nil{return e};if e:=s.updateLocked(ctx,func(tx *sql.Tx)error{for id,k:=range p.Keys{if _,e:=tx.ExecContext(ctx,"INSERT INTO scope_labels VALUES(?,?,?) ON CONFLICT(scope) DO UPDATE SET label=excluded.label,role=excluded.role",id,k.Label,k.Role);e!=nil{return ErrStorage}};return nil});e!=nil{return e};if e:=os.Remove(filepath.Join(s.home,"pending-keys.json"));e!=nil{return e};return syncDir(s.home)
}
func (s *Service) finishScope(ctx context.Context,p pendingScope)error{
 c,_,e:=s.current(ctx);if e!=nil{return e};exists:=false;for _,sc:=range c.Scopes{if sc.ID==p.Scope{if sc.Epoch!=p.Epoch{return errScopeIncomplete};exists=true}}
 if !exists&&c.Generation!=p.Base{return errScopeIncomplete}
 var signed Signed
 if !exists{owner,e:=s.ownerKey();if e!=nil{return errScopeIncomplete};c=scopeControl(c,p);for i,d:=range c.Devices{if d.ID==s.configuration().Device&&!d.Revoked{c.Devices[i].Scopes=append(d.Scopes,p.Scope)}};if e=validateControl(c,s.configuration().Vault);e!=nil{return e};if e=s.checkGrants(ctx,c);e!=nil{return e};signed,e=sign("agent-memory/control/v1",c,owner);if e!=nil{return e}}
 cfg:=s.configuration();cfg.Keys=map[string]ScopeKey{};for id,k:=range s.configuration().Keys{cfg.Keys[id]=k};cfg.Keys[p.Scope]=ScopeKey{p.Epoch,p.Key};if e=s.installConfig(cfg);e!=nil{return e}
 if e=s.updateLocked(ctx,func(tx *sql.Tx)error{if !exists{if e:=s.saveControl(ctx,tx,c,signed);e!=nil{return e}};_,e:=tx.ExecContext(ctx,"INSERT INTO scope_labels VALUES(?,?,?) ON CONFLICT(scope) DO UPDATE SET label=excluded.label,role=excluded.role",p.Scope,p.Label,p.Role);return e});e!=nil{return e}
 if e=os.Remove(filepath.Join(s.home,"pending-scope.json"));e!=nil{return e};return syncDir(s.home)
}
