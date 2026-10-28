package daemon

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/session"
)

func captureSources(ctx context.Context, h *app.Host) {
	type worker struct {
		source capture.Source
		profile *app.Profile
		cancel context.CancelFunc
		done chan struct{}
	}
	workers := map[string]worker{}
	var wg sync.WaitGroup
	defer func() {
		for _, w := range workers {
			w.cancel()
		}
		wg.Wait()
	}()

	for ctx.Err() == nil {
		active := map[string]bool{}
		for _, profile := range h.Snapshot() {
			sources, err := profile.Session.Sources(ctx)
			if err != nil {
				continue
			}
			for _, source := range sources {
				allowed, err := profile.Session.CanCaptureSource(ctx, source)
				if err != nil || !allowed {
					continue
				}
				key := profile.Name + ":" + source.ID
				active[key] = true
				if w, ok := workers[key]; ok {
					if w.profile != profile || !session.SameSource(w.source, source) {
						w.cancel()
					}
					// Replacement starts only after the previous owner exits.
					select {
					case <-w.done:
						delete(workers, key)
					default:
						continue
					}
				}
				if len(workers) >= 128 {
					_ = profile.Session.MarkCaptureSource(ctx, source, "blocked", "capture_worker_limit")
					continue
				}
				run, cancel := context.WithCancel(ctx)
				done := make(chan struct{})
				workers[key] = worker{source: source, profile: profile, cancel: cancel, done: done}
				wg.Add(1)
				go func(p *app.Profile, s capture.Source) {
					defer wg.Done()
					defer close(done)
					captureSource(run, h, p, s)
				}(profile, source)
			}
		}
		for key, w := range workers {
			if !active[key] {
				w.cancel()
				select {
				case <-w.done:
					delete(workers, key)
				default:
				}
			}
		}
		if !delay(ctx, time.Second, 20) {
			return
		}
	}
}

func captureSource(ctx context.Context, h *app.Host, p *app.Profile, source capture.Source) {
	adapter := app.CaptureAdapters()[source.Harness]
	if adapter == nil {
		return
	}
	page := ""
	seen := map[string]bool{}
	for ctx.Err() == nil {
		allowed, err := p.Session.CanCaptureSource(ctx, source)
		if err != nil || !allowed {
			return
		}
		streams, next, err := adapter.Discover(ctx, source, page, h.Config.Schedule.ScanPage)
		if err != nil {
			state, reason := "unavailable", "source_unavailable"
			if errors.Is(err, capture.ErrInvalid) {
				state, reason = "quarantined", "source_shape_or_version_mismatch"
			}
			_ = p.Session.MarkCaptureSource(ctx, source, state, reason)
		} else {
			page = next
			for _, stream := range streams {
				seen[stream.Key] = true
				result, err := p.Session.Poll(ctx, source, stream)
				if errors.Is(err, memory.ErrLeaseLost) {
					return
				}
				if err != nil {
					_ = p.Session.MarkCaptureSource(ctx, source, "blocked", "ingest_failed")
				} else if result.Blocked == "" {
					_ = p.Session.MarkCaptureSource(ctx, source, "enabled", "")
				}
			}
			if next == "" {
				_ = p.Session.MarkCaptureMissingStreams(ctx, source, seen)
				seen = map[string]bool{}
			}
		}
		timer := time.NewTimer(time.Duration(h.Config.Schedule.CaptureFlushSeconds) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-p.Session.Wake():
			timer.Stop()
		case <-timer.C:
		}
	}
}
