package session

import (
 "context"
 "database/sql"
 "encoding/json"

 "github.com/humzakhan/capsule-memory/internal/memory"
 "github.com/humzakhan/capsule-memory/internal/records"
)

func(a *applier) reconcileSuppressions(ctx context.Context,tx *sql.Tx)error{
 rows,e:=tx.QueryContext(ctx,"SELECT DISTINCT record,scope FROM revisions WHERE kind='suppression' LIMIT 100001");if e!=nil{return e};type ref struct{id,scope string};var refs []ref;for rows.Next(){var r ref;if rows.Scan(&r.id,&r.scope)!=nil{rows.Close();return memory.ErrStorage};refs=append(refs,r)};e=rows.Err();rows.Close();if e!=nil{return e};if len(refs)>100000{return memory.ErrStaleOverflow};if _,e=tx.ExecContext(ctx,"DELETE FROM suppressions");e!=nil{return e}
 for _,r:=range refs{hh,del,e:=a.m.svc.Heads(ctx,tx,r.scope,r.id);if e!=nil{return e};if del{continue};for _,h:=range hh{var s records.Suppression;if json.Unmarshal(h.Body,&s)!=nil{return memory.ErrInvalid};if _,e=tx.ExecContext(ctx,"INSERT OR IGNORE INTO suppressions VALUES(?,?,?)",s.SessionKey,h.Record,h.ID);e!=nil{return e}}};return nil
}
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
