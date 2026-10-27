package memory

import (
 "context"
 "encoding/json"
 "github.com/humzakhan/capsule-memory/internal/records"
)

// Marker authority depends only on authenticated scope/control/grant history,
// never on the presence of private assertion revisions on a target-only replica.
func(s *Service) VerifyProjectionMarker(ctx context.Context,q Queryer,r Revision,p records.Projection,published bool)error{
 if !p.WithdrawalMarker||p.Validate()!=nil{return ErrInvalid}
 scope:=p.Source.Scope;vault:=p.Source.Vault;if p.Role=="target"{scope=p.Target.Scope;vault=p.Target.Vault};if r.Scope!=scope||r.Vault!=vault{return ErrInvalid}
 allowed,e:=s.scopeAuthority(ctx,q,r.Author,scope);if e!=nil{return e};if !allowed{return ErrInvalid}
 if p.Role=="source"{return nil}
 if p.Source.Vault==p.Target.Vault{allowed,e=s.scopeAuthority(ctx,q,r.Author,p.Source.Scope);if e!=nil{return e};if !allowed{return ErrInvalid};return nil}
 query:="SELECT r.body FROM revisions r WHERE r.record=? AND r.scope=? AND r.kind='bridge_grant'"
 if published{query+=" AND NOT EXISTS(SELECT 1 FROM outbox o WHERE o.revision=r.revision AND o.receipt IS NULL)"};query+=" LIMIT 4097"
 rows,e:=q.QueryContext(ctx,query,p.Grant,p.Target.Scope);if e!=nil{return ErrStorage};var values [][]byte;for rows.Next(){var raw []byte;if rows.Scan(&raw)!=nil{rows.Close();return ErrStorage};values=append(values,raw)};e=rows.Err();rows.Close();if e!=nil{return ErrStorage};if len(values)>4096{return ErrInvalid}
 for _,raw:=range values{var rev Revision;var signed records.BridgeGrant;var body records.BridgeGrantBody;if json.Unmarshal(raw,&rev)!=nil{return ErrStorage};if rev.Operation=="delete"{continue};if json.Unmarshal(rev.Body,&signed)!=nil{return ErrInvalid}
  if s.VerifyOwner("capsule-memory/bridge-grant/v1",Signed{Payload:signed.Payload,Signature:signed.Signature},&body)!=nil||body.Validate()!=nil{return ErrInvalid}
  if !body.Revoked&&body.SourceVault==p.Source.Vault&&body.TargetVault==p.Target.Vault&&body.LedgerScope==p.Target.Scope&&body.Bridge==r.Author{return nil}
 };if published{return ErrMissingRevision};return ErrInvalid
}

func(s *Service) scopeAuthority(ctx context.Context,q Queryer,author,scope string)(bool,error){
 c,_,e:=s.currentTx(ctx,q);if e!=nil{return false,e};if _,_,e=c.grant(author,scope);e==nil{return true,nil}
 var after int64
 for{rows,e:=q.QueryContext(ctx,"SELECT rowid,signed FROM controls WHERE rowid>? ORDER BY rowid LIMIT 16",after);if e!=nil{return false,ErrStorage};var values [][]byte;for rows.Next(){var raw []byte;if rows.Scan(&after,&raw)!=nil{rows.Close();return false,ErrStorage};values=append(values,raw)};e=rows.Err();rows.Close();if e!=nil{return false,ErrStorage};if len(values)==0{return false,nil}
  for _,raw:=range values{var signed Signed;if json.Unmarshal(raw,&signed)!=nil{return false,ErrStorage};c,e:=s.control(signed);if e!=nil{return false,e};if _,_,e=c.grant(author,scope);e==nil{return true,nil}}
 }
}
