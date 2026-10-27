package daemon

import (
	"context"
	"sync"
	"time"

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/capture"
)
func captureSources(ctx context.Context,h *app.Host){type worker struct{cancel context.CancelFunc;done chan struct{}};workers:=map[string]worker{};var wg sync.WaitGroup;defer func(){for _,w:=range workers{w.cancel()};wg.Wait()}();for ctx.Err()==nil{active:=map[string]bool{};for _,p:=range h.Snapshot(){sources,e:=p.Session.Sources(ctx);if e!=nil{continue};for _,s:=range sources{allowed,e:=p.Session.CanCapture(ctx,s.ID);if e!=nil||!allowed{continue};key:=p.Name+":"+s.ID;active[key]=true;if w,ok:=workers[key];ok{select{case <-w.done:delete(workers,key);default:continue}};if len(workers)>=128{_ = p.Session.MarkSource(ctx,s.ID,"blocked","capture_worker_limit");continue};run,cancel:=context.WithCancel(ctx);done:=make(chan struct{});workers[key]=worker{cancel,done};wg.Add(1);go func(p *app.Profile,s capture.Source){defer wg.Done();defer close(done);captureSource(run,h,p,s)}(p,s)}};for key,w:=range workers{if !active[key]{w.cancel();select{case <-w.done:delete(workers,key);default:}}};if !delay(ctx,time.Second,20){return}}}
func captureSource(ctx context.Context,h *app.Host,p *app.Profile,s capture.Source){a:=app.CaptureAdapters()[s.Harness];if a==nil{return};page:="";seen:=map[string]bool{};for ctx.Err()==nil{allowed,e:=p.Session.CanCapture(ctx,s.ID);if e!=nil||!allowed{return};streams,next,e:=a.Discover(ctx,s,page,h.Config.Schedule.ScanPage);if e!=nil{_ = p.Session.MarkSource(ctx,s.ID,"unavailable","source_unavailable")}else{page=next;for _,st:=range streams{seen[st.Key]=true;r,e:=p.Session.Poll(ctx,s,st);if e!=nil{_ = p.Session.MarkSource(ctx,s.ID,"blocked","ingest_failed")}else if r.Blocked==""{_ = p.Session.MarkSource(ctx,s.ID,"enabled","")}};if next==""{_ = p.Session.MarkMissingStreams(ctx,s.ID,seen);seen=map[string]bool{}}};timer:=time.NewTimer(time.Duration(h.Config.Schedule.CaptureFlushSeconds)*time.Second);select{case <-ctx.Done():timer.Stop();return;case <-p.Session.Wake():timer.Stop();case <-timer.C:}}}
