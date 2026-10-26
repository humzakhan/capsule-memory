package app

import (
	"context"
	"encoding/json"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/projection"
	"github.com/humzakhan/capsule-memory/internal/records"
)
// routedFresh binds a target link to its declared source vault before choosing a local key holder.
// A same-named link in an unrelated profile can never attest this target's knowledge.
type routedFresh struct{target *memory.Service;sources []*Profile;targets map[string]*memory.Service}
func(f routedFresh) VerifyFresh(ctx context.Context,ids []string)(map[string]projection.FreshResult,error){out:=map[string]projection.FreshResult{};expected:=map[string]records.Projection{};e:=f.target.Read(ctx,func(q memory.Queryer)error{for _,id:=range ids{rows,e:=q.QueryContext(ctx,"SELECT body FROM revisions r WHERE kind='projection' AND json_extract(body,'$.body.role')='target' AND json_extract(body,'$.body.link_id')=? AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=r.revision)",id);if e!=nil{return e};var found []records.Projection;for rows.Next(){var b []byte;var r memory.Revision;var p records.Projection;if rows.Scan(&b)!=nil||json.Unmarshal(b,&r)!=nil||json.Unmarshal(r.Body,&p)!=nil{rows.Close();return memory.ErrStorage};found=append(found,p)};rows.Close();if len(found)==1{expected[id]=found[0]}};return nil});if e!=nil{return out,e}
	for _,source:=range f.sources{info,e:=source.Svc.Info(ctx);if e!=nil{continue};var links []string;e=source.Svc.Read(ctx,func(q memory.Queryer)error{for id,target:=range expected{if target.Source.Vault!=info.Vault{continue};var count int;e:=q.QueryRowContext(ctx,"SELECT count(*) FROM revisions r WHERE kind='projection' AND json_extract(body,'$.body.role')='source' AND json_extract(body,'$.body.link_id')=? AND json_extract(body,'$.body.source.record')=? AND json_extract(body,'$.body.source.revision')=? AND json_extract(body,'$.body.source.scope')=? AND json_extract(body,'$.body.target.vault')=? AND json_extract(body,'$.body.target.scope')=? AND json_extract(body,'$.body.target.record')=? AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=r.revision)",id,target.Source.Record,target.Source.Revision,target.Source.Scope,target.Target.Vault,target.Target.Scope,target.Target.Record).Scan(&count);if e!=nil{return e};if count==1{links=append(links,id)}};return nil});if e!=nil{continue};if len(links)==0{continue};v,e:=projection.NewBridge(source.Svc,source.Session,f.targets).VerifyFresh(ctx,links);if e!=nil{continue};for id,r:=range v{out[id]=r}};return out,nil
}
