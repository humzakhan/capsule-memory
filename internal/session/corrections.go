package session

import (
 "context"
 "database/sql"
 "github.com/humzakhan/capsule-memory/internal/memory"
)
// Reconstruct from immutable corrections, including corrections received before
// their source chunk. Raw revisions remain available for provenance and export.
func(m *Module) recomputeRetractions(ctx context.Context,tx *sql.Tx,session string)error{
 _,e:=tx.ExecContext(ctx,`INSERT OR IGNORE INTO retracted_events(scope,session,event_id,correction_revision)
 SELECT r.scope,json_extract(r.body,'$.body.session'),x.value,r.revision FROM revisions r
 JOIN json_each(r.body,'$.body.events') ev JOIN json_each(ev.value,'$.retracts') x
 WHERE r.kind='session_chunk' AND json_extract(ev.value,'$.type')='correction'`)
 if e!=nil{return memory.ErrStorage};var n int;if tx.QueryRowContext(ctx,"SELECT count(*) FROM retracted_events").Scan(&n)!=nil{return memory.ErrStorage};if n>100000{return memory.ErrStaleOverflow};return m.svc.RecomputeStale(ctx,tx)
}
