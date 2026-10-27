package session

import (
 "context"
 "sync"

 "github.com/humzakhan/capsule-memory/internal/capture"
 "github.com/humzakhan/capsule-memory/internal/memory"
)

// Poll serializes manual and background reads of a source. Adapter I/O stays
// outside the database transaction and the consent/ingest mutex.
func(m *Module) Poll(ctx context.Context,source capture.Source,stream capture.Stream)(IngestResult,error){
 value,_:=m.polls.LoadOrStore(source.ID,&sync.Mutex{});lock:=value.(*sync.Mutex);lock.Lock();defer lock.Unlock()
 allowed,e:=m.CanCapture(ctx,source.ID);if e!=nil||!allowed{return IngestResult{Blocked:"consent_disabled"},e};allowed,e=m.StreamAllowed(ctx,source.ID,stream.Key);if e!=nil||!allowed{return IngestResult{Blocked:"quarantined"},e}
 blocked,e:=m.FlushPending(ctx,source,stream);if e!=nil||blocked{return IngestResult{Blocked:"flush_pending"},e}
 var consent string;if e=m.svc.Read(ctx,func(q memory.Queryer)error{return q.QueryRowContext(ctx,"SELECT consent_at FROM sources WHERE source_id=?",source.ID).Scan(&consent)});e!=nil{return IngestResult{},e}
 cp,e:=m.Checkpoint(ctx,source.ID,stream.Key);if e!=nil{return IngestResult{},e};limits,e:=m.CaptureLimits(ctx,source.ID,stream.Key);if e!=nil{return IngestResult{Blocked:"backfill_budget_exhausted"},e};adapter:=m.adapters[source.Harness];if adapter==nil{return IngestResult{},capture.ErrUnavailable};changes,e:=adapter.ReadChanges(ctx,source,stream,cp,limits);if e!=nil{return IngestResult{},e};changes.ObservedConsent=consent;return m.Ingest(ctx,source,stream,changes)
}
func(m *Module) ImportStream(ctx context.Context,source capture.Source,path string)(IngestResult,error){
 if source.Harness!="import"{return IngestResult{},memory.ErrInvalid};a:=m.adapters["import"];if a==nil{return IngestResult{},capture.ErrUnavailable};page:=""
 for n:=0;n<1563;n++{streams,next,e:=a.Discover(ctx,source,page,64);if e!=nil{return IngestResult{},e};for _,stream:=range streams{if stream.Key==path{return m.Poll(ctx,source,stream)}};if next==""{return IngestResult{},memory.ErrInvalid};if next==page{return IngestResult{},memory.ErrInvalid};page=next};return IngestResult{},memory.ErrInvalid
}
