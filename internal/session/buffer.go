package session

import (
 "context"
 "database/sql"
 "errors"
 "strconv"
 "encoding/json"
 "time"

 "github.com/humzakhan/capsule-memory/internal/capture"
 "github.com/humzakhan/capsule-memory/internal/memory"
)

var ErrRewriteUnresolved=errors.New("rewrite_unresolved")

type streamBuffer struct{changes capture.Changes;started time.Time;bytes int;retry bool}

// Unacknowledged buffers are memory-only. The checkpoint moves only when the
// complete sanitized chunk/outbox/identity transaction commits.
func(m *Module) Ingest(ctx context.Context,src capture.Source,st capture.Stream,ch capture.Changes)(IngestResult,error){
 m.mu.Lock();defer m.mu.Unlock();key:=src.ID+":"+st.Key
 if ch.Quarantine!=""{delete(m.buffers,key);return m.ingestNow(ctx,src,st,ch)}
 pending:=m.buffers[key];if pending.changes.Next.Generation!=""&&pending.changes.Next.Generation!=ch.Next.Generation{delete(m.buffers,key);pending=streamBuffer{}};if pending.started.IsZero(){pending.started=time.Now()}
 raw,_:=json.Marshal(ch);total:=len(raw);for _,buffer:=range m.buffers{total+=buffer.bytes};_,exists:=m.buffers[key];if len(m.buffers)>=128&&!exists||total>32<<20{return IngestResult{Blocked:"buffer_bound"},memory.ErrInvalid};if pending.bytes+len(raw)>8<<20||len(pending.changes.Events)+len(ch.Events)>8192||len(pending.changes.Observations)+len(ch.Observations)>8192||len(pending.changes.Omissions)+len(ch.Omissions)>8192{return IngestResult{Blocked:"buffer_bound"},memory.ErrInvalid}
 pending.bytes+=len(raw);pending.changes.Events=append(pending.changes.Events,ch.Events...);pending.changes.Observations=append(pending.changes.Observations,ch.Observations...);pending.changes.Omissions=append(pending.changes.Omissions,ch.Omissions...);ch.Next.CaughtUp=ch.CaughtUp;pending.changes.Next=ch.Next;pending.changes.ObservedConsent=ch.ObservedConsent;pending.changes.CaughtUp=ch.CaughtUp;pending.changes.ScannedBytes+=ch.ScannedBytes;pending.changes.Reconcile=pending.changes.Reconcile||ch.Reconcile
 m.buffers[key]=pending
 closed:=false;for _,o:=range ch.Observations{closed=closed||o.State=="closed"}
 if pending.bytes<256<<10&&time.Since(pending.started)<m.FlushInterval&&!ch.CaughtUp&&!closed{return IngestResult{Blocked:"buffering"},nil}
 return m.flushBuffer(ctx,src,st,key,pending)
}
func(m *Module) flushBuffer(ctx context.Context,src capture.Source,st capture.Stream,key string,pending streamBuffer)(IngestResult,error){
 result,e:=m.ingestNow(ctx,src,st,pending.changes)
 if errors.Is(e,ErrRewriteUnresolved){
  result.Blocked="rewrite_unresolved"
  auditErr:=m.svc.Update(ctx,func(tx *sql.Tx)error{if e:=auditGap(ctx,tx,src.ID,st.Key,pending.changes.Next.Position,"rewrite_unresolved",1);e!=nil{return e};return quarantineStream(ctx,tx,src.ID,st.Key,"rewrite_unresolved")})
  if auditErr!=nil{pending.retry=true;m.buffers[key]=pending;return result,auditErr};delete(m.buffers,key);return result,e
 }
 if errors.Is(e,memory.ErrLeaseLost){delete(m.buffers,key);return result,e};if e==nil&&result.Blocked==""{delete(m.buffers,key)}else{pending.retry=true;m.buffers[key]=pending};return result,e
}
// FlushPending retries a failed commit before the adapter can read more bytes.
// Time-based flushing also runs when a source has stopped appending.
func(m *Module) FlushPending(ctx context.Context,src capture.Source,st capture.Stream)(bool,error){
 m.mu.Lock();defer m.mu.Unlock();key:=src.ID+":"+st.Key;pending,ok:=m.buffers[key];if !ok||!pending.retry&&time.Since(pending.started)<m.FlushInterval{return false,nil};r,e:=m.flushBuffer(ctx,src,st,key,pending);return e!=nil||r.Blocked!="",e

}

// CaptureLimits is a durable historical budget. Missing or exhausted bounds
// never turn a historical import into an unlimited read.
func(m *Module) CaptureLimits(ctx context.Context,source,stream string)(capture.Limits,error){
 m.mu.Lock();defer m.mu.Unlock();limits:=capture.DefaultLimits(capture.Limits{})
 e:=m.svc.Read(ctx,func(q memory.Queryer)error{var active int;var remain,end int64;e:=q.QueryRowContext(ctx,"SELECT s.active,b.remaining,s.end_position FROM backfill_streams s JOIN backfills b ON b.source_id=s.source_id WHERE s.source_id=? AND s.stream=?",source,stream).Scan(&active,&remain,&end);if errors.Is(e,sql.ErrNoRows){return nil};if e!=nil{return e};if active!=0{if pending,ok:=m.buffers[source+":"+stream];ok{remain-=pending.changes.ScannedBytes};if remain<=0{return memory.ErrInvalid};if remain<limits.MaxBytes{limits.MaxBytes=remain};if end>=0{limits.StopAt=end;limits.EndBounded=true};if end>=0{var position string;if e=q.QueryRowContext(ctx,"SELECT position FROM checkpoints WHERE source_id=? AND stream=?",source,stream).Scan(&position);e!=nil{return e};pos:=int64(0);if position!=""{pos,e=strconv.ParseInt(position,10,64);if e!=nil{return memory.ErrInvalid}};if end-pos>0&&end-pos<limits.MaxBytes{limits.MaxBytes=end-pos}}};return nil});return limits,e
}
