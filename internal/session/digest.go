package session

import (
 "context"

 "github.com/humzakhan/capsule-memory/internal/memory"
 "github.com/humzakhan/capsule-memory/internal/records"
)

// DigestInputs includes every strong view and continuation. Capture branches
// retain their own sequence; they share one canonical derivation subject.
func(m *Module) DigestInputs(ctx context.Context,q memory.Queryer,session string)(string,[]records.SourceRef,error){
 var canonical,scope string;e:=q.QueryRowContext(ctx,"SELECT CASE WHEN COALESCE(view_of,'')='' THEN session ELSE view_of END,scope FROM sessions WHERE session=? AND deleted=0",session).Scan(&canonical,&scope);if e!=nil{return "",nil,e}
 rows,e:=q.QueryContext(ctx,`SELECT c.record,c.revision FROM chunks c JOIN sessions s ON s.session=c.session WHERE s.scope=? AND s.deleted=0 AND (s.session=? OR s.view_of=?) ORDER BY c.session,c.branch,c.seq,c.record LIMIT 4097`,scope,canonical,canonical);if e!=nil{return "",nil,e};var refs []records.SourceRef;for rows.Next(){r:=records.SourceRef{Kind:records.KindSessionChunk};if rows.Scan(&r.Record,&r.Revision)!=nil{rows.Close();return "",nil,memory.ErrStorage};refs=append(refs,r)};e=rows.Err();rows.Close();if e!=nil{return "",nil,e};if len(refs)>4096{return "",nil,memory.ErrInvalid}
 rows,e=q.QueryContext(ctx,"SELECT session FROM sessions WHERE scope=? AND deleted=0 AND (session=? OR view_of=?) ORDER BY session LIMIT 4097",scope,canonical,canonical);if e!=nil{return "",nil,e};var ids []string;for rows.Next(){var id string;if rows.Scan(&id)!=nil{rows.Close();return "",nil,memory.ErrStorage};ids=append(ids,id)};e=rows.Err();rows.Close();if e!=nil{return "",nil,e}
 for _,id:=range ids{hh,del,e:=m.svc.Heads(ctx,q,scope,id);if e!=nil{return "",nil,e};if del||len(hh)!=1{return "",nil,memory.ErrHeadsChanged};refs=append(refs,records.SourceRef{Record:id,Revision:hh[0].ID,Kind:records.KindSessionManifest})};if len(refs)>4096{return "",nil,memory.ErrInvalid};return canonical,refs,nil
}
