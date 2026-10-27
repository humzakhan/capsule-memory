package session

import (
 "context"
 "database/sql"
 "github.com/humzakhan/capsule-memory/internal/memory"
)

// Index this immutable correction exactly once. Targets need not have arrived.
func(m *Module) applyRetractions(ctx context.Context,tx *sql.Tx,r memory.Revision)error{
 _,e:=tx.ExecContext(ctx,`INSERT OR IGNORE INTO retracted_events(scope,session,event_id,correction_revision)
 SELECT ?,json_extract(?,'$.session'),x.value,? FROM json_each(?,'$.events') ev JOIN json_each(ev.value,'$.retracts') x
 WHERE json_extract(ev.value,'$.type')='correction'`,r.Scope,string(r.Body),r.ID,string(r.Body))
 if e!=nil{return memory.ErrStorage}
 return m.svc.RefreshRetractions(ctx,tx,r.ID)
}
