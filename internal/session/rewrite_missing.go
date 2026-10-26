package session

import (
 "context"
 "encoding/json"

 "github.com/humzakhan/capsule-memory/internal/capture"
 "github.com/humzakhan/capsule-memory/internal/memory"
 "github.com/humzakhan/capsule-memory/internal/records"
)

// Absence proves retraction only at the end of a complete native-id generation.
// Unknown/positional identities abort instead of guessing what a rewrite meant.
func(m *Module) reconcileMissing(ctx context.Context,q memory.Queryer,src capture.Source,st capture.Stream,ch capture.Changes,staged []identity,groups map[string]*ingestGroup)error{
 current:=map[string]bool{};for _,id:=range staged{current[id.id]=true}
 rows,e:=q.QueryContext(ctx,"SELECT event_id FROM event_ids WHERE source_id=? AND stream=? AND generation=? LIMIT 100001",src.ID,st.Key,ch.Next.Generation);if e!=nil{return e};for rows.Next(){var id string;if rows.Scan(&id)!=nil{rows.Close();return memory.ErrStorage};current[id]=true};e=rows.Err();rows.Close();if e!=nil{return e};if len(current)>100000{return memory.ErrStaleOverflow}
 rows,e=q.QueryContext(ctx,`SELECT DISTINCT ids.event_id,ev.value,c.session,s.scope,s.project_id,s.session_key FROM event_ids ids JOIN chunks c JOIN sessions s ON s.session=c.session JOIN revisions r ON r.revision=c.revision JOIN json_each(r.body,'$.body.events') ev ON (json_extract(ev.value,'$.id')=ids.event_id OR json_extract(ev.value,'$.part.group')=ids.event_id) WHERE ids.source_id=? AND ids.stream=? AND ids.generation!=? AND json_extract(r.body,'$.provenance.source_ref')=ids.source_id AND json_extract(r.body,'$.body.generation')=ids.generation AND json_extract(ev.value,'$.type')!='correction' LIMIT 100001`,src.ID,st.Key,ch.Next.Generation);if e!=nil{return e}
 type missing struct{id,session,scope,project,key string;event records.Event};var candidates []missing
 for rows.Next(){var v missing;var raw []byte;if rows.Scan(&v.id,&raw,&v.session,&v.scope,&v.project,&v.key)!=nil||json.Unmarshal(raw,&v.event)!=nil{rows.Close();return memory.ErrStorage};candidates=append(candidates,v)};e=rows.Err();rows.Close();if e!=nil{return e};if len(candidates)>100000{return memory.ErrStaleOverflow}
 seen:=map[string]bool{}
 for _,v:=range candidates{if current[v.id]||seen[v.scope+":"+v.id]{continue};seen[v.scope+":"+v.id]=true;if v.event.NativeID==""{return ErrRewriteUnresolved};invalid,e:=memory.EventRetracted(ctx,q,v.session,v.event.ID,v.id);if e!=nil{return e};if invalid{continue}
  gkey:=v.key+":"+v.scope;g:=groups[gkey];if g==nil{rev,manifest,e:=m.latestManifest(ctx,q,v.scope,v.session);if e!=nil{return e};info,e:=m.svc.Info(ctx);if e!=nil{return e};g=&ingestGroup{session:v.session,scope:v.scope,project:v.project,manifestRecord:rev.Record,manifest:manifest,branch:hash([]byte(src.ID+"\n"+info.Device+"\n"+st.Key))[:32]};groups[gkey]=g}
  correction:=records.Event{ID:"missing:"+hash([]byte(src.ID+"\n"+ch.Next.Generation+"\n"+v.id)),Role:"system",Type:"correction",Retracts:[]string{v.id}};g.events=append(g.events,correction)
 };return nil
}
