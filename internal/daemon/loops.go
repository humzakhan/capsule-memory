package daemon

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"time"

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/memory"
)

func delay(ctx context.Context, base time.Duration, jitter int) bool {
	var b [8]byte
	_, _ = rand.Read(b[:])
	if jitter > 0 {
		span := int64(base) * int64(jitter) / 100
		if span > 0 {
			base += time.Duration(int64(binary.BigEndian.Uint64(b[:])%uint64(2*span+1)) - span)
		}
	}
	timer := time.NewTimer(base)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type syncScopeKey struct {
	profile string
	scope   string
}

type retryState struct {
	until time.Time
	delay time.Duration
}

func (r retryState) ready(now time.Time) bool {
	return !now.Before(r.until)
}

func (r retryState) failed(now time.Time) retryState {
	if r.delay == 0 {
		r.delay = 2 * time.Second
	} else {
		r.delay *= 2
		if r.delay > 15*time.Minute {
			r.delay = 15 * time.Minute
		}
	}
	r.until = now.Add(r.delay)
	return r
}

func publishLoop(ctx context.Context, h *app.Host) {
	retries := map[syncScopeKey]retryState{}
	for ctx.Err() == nil {
		for _, p := range h.Snapshot() {
			publishProfile(ctx, h, p, retries)
		}
		if !delay(ctx, time.Duration(h.Config.Schedule.PublishSeconds)*time.Second, h.Config.Schedule.JitterPercent) {
			return
		}
	}
}

func publishProfile(ctx context.Context, h *app.Host, p *app.Profile, retries map[syncScopeKey]retryState) {
	policy, err := p.Session.Policy(ctx)
	if err != nil {
		return
	}
	if !policy.Publish && !policy.Capture && !policy.Derive {
		// There is no scheduled attempt while all session work is disabled.
		h.ClearSyncRetries(p.Name)
		for key := range retries {
			if key.profile == p.Name {
				delete(retries, key)
			}
		}
		return
	}
	info, err := p.Svc.Info(ctx)
	if err != nil {
		return
	}
	for _, scope := range info.Scopes {
		if ctx.Err() != nil {
			return
		}
		key := syncScopeKey{profile: p.Name, scope: scope.ID}
		retry := retries[key]
		if !retry.ready(time.Now()) {
			continue
		}
		publish := false
		if policy.Publish {
			for _, id := range policy.PublishScopes {
				if id == scope.ID {
					publish = true
					break
				}
			}
		}
		_, err = p.Svc.SyncScope(ctx, scope.ID, memory.SyncOptions{
			MaxPublish: 100,
			MaxPages:   100,
			ReplayOnly: !publish,
		})
		if err == nil {
			delete(retries, key)
			h.SetSyncRetry(p.Name, scope.ID, app.SyncRetryStatus{})
			continue
		}
		if ctx.Err() != nil {
			return
		}
		// Authorization failures are recoverable: every bounded retry refreshes
		// control and rechecks grants. Never regenerate queued encrypted bytes.
		retry = retry.failed(time.Now())
		retries[key] = retry
		reason := "sync_failed"
		if errors.Is(err, memory.ErrSyncAuthorization) {
			reason = "authorization_unavailable"
		}
		h.SetSyncRetry(p.Name, scope.ID, app.SyncRetryStatus{
			Reason:     reason,
			RetryAfter: retry.until.UTC(),
		})
	}
}

func inferenceLoop(ctx context.Context, h *app.Host) {
	scheduled := map[string]time.Time{}
	for ctx.Err() == nil {
		worked := false
		for _, p := range h.Snapshot() {
			policy, err := p.Session.Policy(ctx)
			if err != nil || !policy.Derive || !h.Config.Inference.Enabled {
				continue
			}
			if time.Since(scheduled[p.Name]) >= 10*time.Minute {
				_ = p.Derive.Schedule(ctx, time.Now())
				scheduled[p.Name] = time.Now()
			}
			did, _ := p.Derive.RunOnce(ctx)
			worked = worked || did
		}
		if !worked && !delay(ctx, time.Second, 20) {
			return
		}
	}
}

func bridgeLoop(ctx context.Context, h *app.Host) {
	for ctx.Err() == nil {
		worked := false
		for _, p := range h.Snapshot() {
			did, _ := h.BridgeFor(p.Name).RunOnce(ctx)
			worked = worked || did
		}
		if !worked && !delay(ctx, 2*time.Second, 20) {
			return
		}
	}
}

func retryLoop(ctx context.Context, h *app.Host) {
	for delay(ctx, 30*time.Second, 0) {
		_ = h.RetryBusy(ctx)
	}
}
