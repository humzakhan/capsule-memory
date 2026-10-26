package session

import (
	"context"
	"encoding/json"
 "sort"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)
func(m *Module) latestManifest(ctx context.Context,q memory.Queryer,scope,session string)(memory.Revision,records.SessionManifest,error){var current memory.Revision;var body records.SessionManifest;current,body,e:=m.manifestHead(ctx,q,scope,session);if e!=nil{return current,body,e};rows,e:=q.QueryContext(ctx,"SELECT DISTINCT record FROM revisions WHERE kind='session_manifest' AND scope=? AND json_extract(body,'$.body.session_key')=? LIMIT 10001",scope,body.SessionKey);if e!=nil{return current,body,e};var ids []string;for rows.Next(){var id string;if rows.Scan(&id)!=nil{rows.Close();return current,body,memory.ErrStorage};ids=append(ids,id)};rows.Close();if len(ids)>10000{return current,body,memory.ErrInvalid};type candidate struct{revision memory.Revision;body records.SessionManifest};next:=map[string]candidate{};for _,id:=range ids{h,b,e:=m.manifestHead(ctx,q,scope,id);if e!=nil{return current,body,e};if b.Continues!=""{if old,exists:=next[b.Continues];!exists||h.Record<old.revision.Record{next[b.Continues]=candidate{h,b}}}};seen:=map[string]bool{};for{if seen[current.Record]{return current,body,memory.ErrInvalid};seen[current.Record]=true;c,ok:=next[current.Record];if !ok{return current,body,nil};current=c.revision;body=c.body}}

// A local continuation write resolves every concurrent head of the chosen
// manifest. Union coverage prevents one writer from erasing another branch.
func(m *Module) manifestHead(ctx context.Context,q memory.Queryer,scope,record string)(memory.Revision,records.SessionManifest,error){
 hh,del,e:=m.svc.Heads(ctx,q,scope,record);var out records.SessionManifest;if e!=nil{return memory.Revision{},out,e};if del||len(hh)==0{return memory.Revision{},out,memory.ErrHeadsChanged};sort.Slice(hh,func(i,j int)bool{return hh[i].ID<hh[j].ID});if json.Unmarshal(hh[0].Body,&out)!=nil{return memory.Revision{},out,memory.ErrStorage};for _,h:=range hh[1:]{var b records.SessionManifest;if json.Unmarshal(h.Body,&b)!=nil||b.SessionKey!=out.SessionKey||b.ProjectID!=out.ProjectID||b.Continues!=out.Continues{return memory.Revision{},out,memory.ErrHeadsChanged};for branch,seq:=range b.ChunkHigh{old,ok:=out.ChunkHigh[branch];if !ok||seq>old{out.ChunkHigh[branch]=seq}};out.Gaps=append(out.Gaps,b.Gaps...);out.Bindings=append(out.Bindings,b.Bindings...);out.Links=append(out.Links,b.Links...)};out.Gaps=dedupGaps(out.Gaps);out.Bindings=dedupBindings(out.Bindings);out.Links=dedupLinks(out.Links);if out.Validate()!=nil{return memory.Revision{},out,memory.ErrHeadsChanged};return hh[0],out,nil
}
func dedupGaps(in []records.Gap)[]records.Gap{out:=[]records.Gap{};seen:=map[records.Gap]bool{};for _,v:=range in{if !seen[v]{seen[v]=true;out=append(out,v)}};return out}
func dedupBindings(in []records.Binding)[]records.Binding{out:=[]records.Binding{};seen:=map[records.Binding]bool{};for _,v:=range in{if !seen[v]{seen[v]=true;out=append(out,v)}};return out}
func dedupLinks(in []records.SessionLink)[]records.SessionLink{out:=[]records.SessionLink{};seen:=map[records.SessionLink]bool{};for _,v:=range in{if !seen[v]{seen[v]=true;out=append(out,v)}};return out}

// ManifestRoot resolves a continuation through exact current heads with a bound.
func(m *Module) ManifestRoot(ctx context.Context,q memory.Queryer,scope,record string)(string,error){
 seen:=map[string]bool{}
 for n:=0;n<1024;n++{if seen[record]{return "",memory.ErrInvalid};seen[record]=true;hh,del,e:=m.svc.Heads(ctx,q,scope,record);if e!=nil{return "",e};if del||len(hh)!=1{return "",memory.ErrHeadsChanged};var body records.SessionManifest;if json.Unmarshal(hh[0].Body,&body)!=nil{return "",memory.ErrStorage};if body.Continues==""{return record,nil};record=body.Continues};return "",memory.ErrInvalid
}
