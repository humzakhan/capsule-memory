package daemon

import (
	"context"
	"errors"
	"sync"

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/memory"
)

func Run(ctx context.Context, home string) (resultErr error) {
	if err := privateDir(home); err != nil {
		return err
	}
	unlock, err := singleton(home)
	if err != nil {
		return err
	}
	defer unlock()
	host, err := app.NewHost(ctx, home, memory.LockExclusive)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, host.Close())
	}()

	run, cancel := context.WithCancel(ctx)
	defer cancel()
	var workers sync.WaitGroup
	for _, loop := range []func(context.Context, *app.Host){
		captureSources, publishLoop, inferenceLoop, bridgeLoop, retryLoop,
	} {
		workers.Add(1)
		go func(loop func(context.Context, *app.Host)) {
			defer workers.Done()
			loop(run, host)
		}(loop)
	}

	resultErr = serve(run, home, host)
	cancel()
	// The profile and singleton locks outlive every user of the host. A timeout
	// must not close SQLite and release ownership beneath still-running work.
	workers.Wait()
	return resultErr
}
