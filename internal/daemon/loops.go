package daemon

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"time"

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/memory"
)
func delay(ctx context.Context,base time.Duration,jitter int)bool{var b [8]byte;_,_=rand.Read(b[:]);if jitter>0{span:=int64(base)*int64(jitter)/100;if span>0{base+=time.Duration(int64(binary.BigEndian.Uint64(b[:])%uint64(2*span+1))-span)}};timer:=time.NewTimer(base);defer timer.Stop();select{case <-ctx.Done():return false;case <-timer.C:return true}}
type retryState struct{until time.Time;delay time.Duration;blocked bool}
func publishLoop(ctx context.Context,h *app.Host){retry:=map[string]retryState{};for ctx.Err()==nil{for _,p:=range h.Snapshot(){policy,e:=p.Session.Policy(ctx);if e!=nil{continue};if !policy.Publish&&!policy.Capture&&!policy.Derive{continue};info,e:=p.Svc.Info(ctx);if e!=nil{continue};for _,scope:=range info.Scopes{key:=p.Name+":"+scope.ID;r:=retry[key];if r.blocked||time.Now().Before(r.until){continue};publish:=false;if policy.Publish{for _,id:=range policy.PublishScopes{if id==scope.ID{publish=true}}};_,e=p.Svc.SyncScope(ctx,scope.ID,memory.SyncOptions{MaxPublish:100,MaxPages:100,ReplayOnly:!publish});if e==nil{delete(retry,key);continue};if e.Error()=="relay rejected request (HTTP 403)"||e.Error()=="device revoked or scope not granted"{r.blocked=true};if r.delay==0{r.delay=2*time.Second}else{r.delay*=2;if r.delay>15*time.Minute{r.delay=15*time.Minute}};r.until=time.Now().Add(r.delay);retry[key]=r}};if !delay(ctx,time.Duration(h.Config.Schedule.PublishSeconds)*time.Second,h.Config.Schedule.JitterPercent){return}}}
func inferenceLoop(ctx context.Context,h *app.Host){scheduled:=map[string]time.Time{};for ctx.Err()==nil{worked:=false;for _,p:=range h.Snapshot(){policy,e:=p.Session.Policy(ctx);if e!=nil||!policy.Derive||!h.Config.Inference.Enabled{continue};if time.Since(scheduled[p.Name])>=10*time.Minute{_ = p.Derive.Schedule(ctx,time.Now());scheduled[p.Name]=time.Now()};did,_:=p.Derive.RunOnce(ctx);worked=worked||did};if !worked&&!delay(ctx,time.Second,20){return}}}
func bridgeLoop(ctx context.Context,h *app.Host){for ctx.Err()==nil{worked:=false;for _,p:=range h.Snapshot(){did,_:=h.BridgeFor(p.Name).RunOnce(ctx);worked=worked||did};if !worked&&!delay(ctx,2*time.Second,20){return}}}
func retryLoop(ctx context.Context,h *app.Host){for delay(ctx,30*time.Second,0){_ = h.RetryBusy(ctx)}}
