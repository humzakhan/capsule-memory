package session

import (
 "context"
 "database/sql"
 "encoding/json"
 "sort"

 "github.com/humzakhan/capsule-memory/internal/memory"
 "github.com/humzakhan/capsule-memory/internal/records"
)

// Every manifest keeps its own row, including continuations. Recompute groups
// from current immutable heads so arrival order cannot select a different view.
func(a *applier) reconcileManifests(ctx context.Context,tx *sql.Tx)error{
 if e:=a.reconcileSuppressions(ctx,tx);e!=nil{return e};rows,e:=tx.QueryContext(ctx,"SELECT DISTINCT record,scope FROM revisions WHERE kind='session_manifest' LIMIT 100001");if e!=nil{return e}
 type entry struct{id,scope string;body records.SessionManifest;hidden,withdrawn bool}
 var entries []entry
 for rows.Next(){var v entry;if rows.Scan(&v.id,&v.scope)!=nil{rows.Close();return memory.ErrStorage};entries=append(entries,v)};e=rows.Err();rows.Close();if e!=nil{return e};if len(entries)>100000{return memory.ErrStaleOverflow}
 parent:=map[string]string{};byKey:=map[string]string{};bindings:=map[string]string{}
 var root func(string)string;root=func(id string)string{p:=parent[id];if p==""||p==id{return id};parent[id]=root(p);return parent[id]}
 join:=func(x,y string){x=root(x);y=root(y);if x<y{parent[y]=x}else if y<x{parent[x]=y}}
 for i:=range entries{v:=&entries[i];hh,del,err:=a.m.svc.Heads(ctx,tx,v.scope,v.id);if err!=nil{return err};v.hidden=del||len(hh)!=1;v.withdrawn=del
  // A tombstone or conflict still retains its origin identity for suppression.
  var raw []byte;if err=tx.QueryRowContext(ctx,"SELECT body FROM revisions WHERE record=? AND json_extract(body,'$.operation')!='delete' ORDER BY revision LIMIT 1",v.id).Scan(&raw);err!=nil{return err};var r memory.Revision;if json.Unmarshal(raw,&r)!=nil||json.Unmarshal(r.Body,&v.body)!=nil{return memory.ErrInvalid}
  if !v.hidden{if json.Unmarshal(hh[0].Body,&v.body)!=nil{return memory.ErrInvalid}}
  // Retain strong origin association through conflicts and tombstones.
  bindingRows,err:=tx.QueryContext(ctx,"SELECT DISTINCT json_extract(b.value,'$.harness'),json_extract(b.value,'$.native_id'),json_extract(b.value,'$.strength') FROM revisions r JOIN json_each(r.body,'$.body.bindings') b WHERE r.record=? LIMIT 17",v.id);if err!=nil{return err};for bindingRows.Next(){var b records.Binding;if bindingRows.Scan(&b.Harness,&b.NativeID,&b.Strength)!=nil{bindingRows.Close();return memory.ErrStorage};v.body.Bindings=append(v.body.Bindings,b)};err=bindingRows.Err();bindingRows.Close();if err!=nil{return err};v.body.Bindings=dedupBindings(v.body.Bindings);if len(v.body.Bindings)>16{return memory.ErrInvalid}
  var suppressed int;if err=tx.QueryRowContext(ctx,"SELECT count(*) FROM suppressions WHERE session_key=?",v.body.SessionKey).Scan(&suppressed);err!=nil{return err};v.hidden=v.hidden||suppressed>0;v.withdrawn=v.withdrawn||suppressed>0
  parent[v.id]=v.id;k:=v.scope+":"+v.body.SessionKey;if old:=byKey[k];old!=""{join(v.id,old)}else{byKey[k]=v.id}
 }
 // Strong bindings establish an equivalence relation only within a scope.
 for _,v:=range entries{for _,b:=range v.body.Bindings{if b.Strength!="strong"{continue};k:=v.scope+":"+b.Harness+":"+b.NativeID;if old:=bindings[k];old!=""{join(v.id,old)}else{bindings[k]=v.id}}}
 for _,v:=range entries{if old:=bindings[v.scope+":"+v.body.Origin.Harness+":"+v.body.Origin.NativeSessionID];old!=""{join(v.id,old)}}
 groupHidden:=map[string]bool{};for _,v:=range entries{if v.withdrawn{groupHidden[root(v.id)]=true}};for i:=range entries{entries[i].hidden=entries[i].hidden||groupHidden[root(entries[i].id)]}
 canonical:=map[string]string{}
 sort.Slice(entries,func(i,j int)bool{if (entries[i].body.Origin.Harness=="t3")!=(entries[j].body.Origin.Harness=="t3"){return entries[i].body.Origin.Harness=="t3"};return entries[i].id<entries[j].id})
 for _,v:=range entries{if v.hidden||v.body.Continues!=""{continue};g:=root(v.id);if canonical[g]==""{canonical[g]=v.id}}
 for _,v:=range entries{if v.body.Continues==""&&canonical[root(v.id)]==""{canonical[root(v.id)]=v.id}}
 if _,e=tx.ExecContext(ctx,"DELETE FROM bindings");e!=nil{return e}
 for _,v:=range entries{del:=0;if v.hidden{del=1};view:=canonical[root(v.id)];if view==v.id{view=""};if view==""&&v.body.Continues!=""{view=v.body.Continues}
  if _,e=tx.ExecContext(ctx,`INSERT INTO sessions(session,session_key,scope,project_id,harness,state,view_of,deleted) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(session) DO UPDATE SET session_key=excluded.session_key,scope=excluded.scope,project_id=excluded.project_id,harness=excluded.harness,state=excluded.state,view_of=excluded.view_of,deleted=excluded.deleted`,v.id,v.body.SessionKey,v.scope,v.body.ProjectID,v.body.Origin.Harness,v.body.State,view,del);e!=nil{return e}
  for _,b:=range v.body.Bindings{if b.Strength=="strong"{if _,e=tx.ExecContext(ctx,"INSERT OR IGNORE INTO bindings VALUES(?,?,?)",b.Harness,b.NativeID,v.id);e!=nil{return e}}}
 }
 // Publish the complete group state before visibility checks and indexing.
 if e=a.m.recomputeRetractions(ctx,tx,"");e!=nil{return e}
 for _,v:=range entries{if v.hidden{if _,e=tx.ExecContext(ctx,"DELETE FROM session_search WHERE session=?",v.id);e!=nil{return e}}else if e=a.reindexSession(ctx,tx,v.id);e!=nil{return e}}
 return nil
}
