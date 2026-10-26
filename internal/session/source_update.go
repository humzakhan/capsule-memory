package session

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
)
// ConfigureSource preserves origin identity and checkpoints; disable capture before changing a pin.
func(m *Module) ConfigureSource(ctx context.Context,id string,cfg json.RawMessage)(capture.Preview,error){m.mu.Lock();defer m.mu.Unlock();s,e:=m.Source(ctx,id);if e!=nil{return capture.Preview{},e};a:=m.adapters[s.Harness];if a==nil{return capture.Preview{},memory.ErrInvalid};if _,e=a.ParseConfig(cfg);e!=nil{return capture.Preview{},e};if stable,ok:=a.(interface{Identity(json.RawMessage)(string,error)});ok{identity,e:=stable.Identity(cfg);if e!=nil||identity!=s.StoreIdentity{return capture.Preview{},memory.ErrInvalid}};s.Config=cfg;preview,e:=a.Preview(ctx,s);if e!=nil{return preview,e};e=m.svc.Update(ctx,func(tx *sql.Tx)error{var enabled int;if tx.QueryRowContext(ctx,"SELECT enabled FROM sources WHERE source_id=?",id).Scan(&enabled)!=nil||enabled!=0{return memory.ErrInvalid};_,e:=tx.ExecContext(ctx,"UPDATE sources SET config=?,state='disabled',reason=NULL WHERE source_id=?",[]byte(cfg),id);return e});return preview,e}
