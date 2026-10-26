package session

import (
 "context"
 "database/sql"

 "github.com/humzakhan/capsule-memory/internal/memory"
)

// Origin continuations may span project scopes. Deletion follows both exact
// origin identity and the scope-local strong-view equivalence transitively.
const relatedCTE=`WITH RECURSIVE related(session,session_key,view_of) AS (
 SELECT session,session_key,view_of FROM sessions WHERE session=?
 UNION SELECT s.session,s.session_key,s.view_of FROM sessions s JOIN related r ON
 s.session_key=r.session_key OR s.session=r.view_of OR s.view_of=r.session OR
 (COALESCE(s.view_of,'')!='' AND s.view_of=r.view_of) LIMIT 100001)`
func relatedKeys(ctx context.Context,q memory.Queryer,session string)([]string,error){
 var count int;if e:=q.QueryRowContext(ctx,relatedCTE+" SELECT count(*) FROM related",session).Scan(&count);e!=nil{return nil,e};if count>100000{return nil,memory.ErrStaleOverflow}
 rows,e:=q.QueryContext(ctx,relatedCTE+" SELECT DISTINCT session_key FROM related",session);if e!=nil{return nil,e};defer rows.Close();var out []string;for rows.Next(){var key string;if rows.Scan(&key)!=nil{return nil,memory.ErrStorage};out=append(out,key)};return out,rows.Err()
}

func(a *applier) applySuppression(ctx context.Context,tx *sql.Tx,r memory.Revision)error{
 rows,e:=tx.QueryContext(ctx,"SELECT DISTINCT json_extract(body,'$.body.session_key') FROM revisions WHERE record=? AND json_type(body,'$.body.session_key')='text'",r.Record);if e!=nil{return e}
 var keys []string;for rows.Next(){var key string;if rows.Scan(&key)!=nil{rows.Close();return memory.ErrStorage};keys=append(keys,key)};e=rows.Err();rows.Close();if e!=nil{return e};var seeds []string
 for _,key:=range keys{
  if _,e=tx.ExecContext(ctx,"DELETE FROM suppressions WHERE session_key=?",key);e!=nil{return e}
  rows,e=tx.QueryContext(ctx,"SELECT DISTINCT record,scope FROM revisions WHERE kind='suppression' AND json_extract(body,'$.body.session_key')=? LIMIT 100001",key);if e!=nil{return e};type ref struct{id,scope string};var refs []ref
  for rows.Next(){var v ref;if rows.Scan(&v.id,&v.scope)!=nil{rows.Close();return memory.ErrStorage};refs=append(refs,v)};e=rows.Err();rows.Close();if e!=nil{return e};if len(refs)>100000{return memory.ErrStaleOverflow}
  for _,v:=range refs{hh,del,e:=a.m.svc.Heads(ctx,tx,v.scope,v.id);if e!=nil{return e};if del{continue};for _,h:=range hh{if _,e=tx.ExecContext(ctx,"INSERT OR IGNORE INTO suppressions VALUES(?,?,?)",key,v.id,h.ID);e!=nil{return e}}}
  rows,e=tx.QueryContext(ctx,"SELECT record FROM manifest_origins WHERE session_key=? LIMIT 100001",key);if e!=nil{return e};for rows.Next(){var id string;if rows.Scan(&id)!=nil{rows.Close();return memory.ErrStorage};seeds=append(seeds,id);if len(seeds)>100000{rows.Close();return memory.ErrStaleOverflow}};e=rows.Err();rows.Close();if e!=nil{return e}
 };if len(seeds)==0{return nil};return a.reconcileManifests(ctx,tx,seeds...)
}
