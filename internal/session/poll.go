package session

import (
	"context"
	"sync"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
)

// Poll serializes manual and background reads of a source. Adapter I/O stays
// outside the database transaction and the consent/ingest mutex.
func (m *Module) Poll(ctx context.Context, source capture.Source, stream capture.Stream) (IngestResult, error) {
	value, _ := m.polls.LoadOrStore(source.ID, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	policy, err := m.captureSnapshot(ctx, source, 0)
	if err != nil {
		return IngestResult{Blocked: "capture_snapshot_changed"}, err
	}
	allowed, err := m.StreamAllowed(ctx, source.ID, stream.Key)
	if err != nil || !allowed {
		return IngestResult{Blocked: "quarantined"}, err
	}
	blocked, err := m.FlushPending(ctx, source, stream)
	if err != nil || blocked {
		return IngestResult{Blocked: "flush_pending"}, err
	}
	cp, err := m.Checkpoint(ctx, source.ID, stream.Key)
	if err != nil {
		return IngestResult{}, err
	}
	limits, err := m.CaptureLimits(ctx, source.ID, stream.Key)
	if err != nil {
		return IngestResult{Blocked: "backfill_budget_exhausted"}, err
	}
	adapter := m.adapters[source.Harness]
	if adapter == nil {
		return IngestResult{}, capture.ErrUnavailable
	}
	changes, err := adapter.ReadChanges(ctx, source, stream, cp, limits)
	if err != nil && changes.Quarantine == "" {
		return IngestResult{}, err
	}
	// Confirmed shape/version failures must reach the durable quarantine path.
	// Transport failures have no quarantine and leave the checkpoint retryable.
	changes.ObservedConsent = source.Consent
	changes.ObservedPolicyVersion = policy.Version
	return m.Ingest(ctx, source, stream, changes)
}

func (m *Module) ImportStream(ctx context.Context, source capture.Source, path string) (IngestResult, error) {
	if source.Harness != "import" {
		return IngestResult{}, memory.ErrInvalid
	}
	a := m.adapters["import"]
	if a == nil {
		return IngestResult{}, capture.ErrUnavailable
	}
	page := ""
	for n := 0; n < 1563; n++ {
		if _, err := m.captureSnapshot(ctx, source, 0); err != nil {
			return IngestResult{}, err
		}
		streams, next, err := a.Discover(ctx, source, page, 64)
		if err != nil {
			return IngestResult{}, err
		}
		for _, stream := range streams {
			if stream.Key == path {
				return m.Poll(ctx, source, stream)
			}
		}
		if next == "" || next == page {
			return IngestResult{}, memory.ErrInvalid
		}
		page = next
	}
	return IngestResult{}, memory.ErrInvalid
}
