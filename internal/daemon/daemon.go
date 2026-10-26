package daemon

import (
	"context"
	"sync"
	"time"

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/memory"
)
func Run(ctx context.Context,home string)error{if e:=privateDir(home);e!=nil{return e};unlock,e:=singleton(home);if e!=nil{return e};defer unlock();h,e:=app.NewHost(ctx,home,memory.LockExclusive);if e!=nil{return e};defer h.Close();run,cancel:=context.WithCancel(ctx);defer cancel();var wg sync.WaitGroup;for _,fn:=range []func(context.Context,*app.Host){captureSources,publishLoop,inferenceLoop,bridgeLoop,retryLoop}{wg.Add(1);go func(fn func(context.Context,*app.Host)){defer wg.Done();fn(run,h)}(fn)};e=serve(run,home,h);cancel();done:=make(chan struct{});go func(){wg.Wait();close(done)}();select{case <-done:case <-time.After(10*time.Second):};return e}
