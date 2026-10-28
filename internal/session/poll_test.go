package session

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

// This adapter performs no file/history I/O. Its read callback lets a control
// change occur after Poll's authority snapshot but before its ingest attempt.
type pollAdapter struct {
	read func(capture.Checkpoint) (capture.Changes, error)
}

func (*pollAdapter) Name() string                                 { return "synthetic" }
func (*pollAdapter) Version() string                              { return "1" }
func (*pollAdapter) ParseConfig(raw json.RawMessage) (any, error) { return raw, nil }
func (*pollAdapter) Preview(context.Context, capture.Source) (capture.Preview, error) {
	return capture.Preview{}, nil
}
func (*pollAdapter) Discover(context.Context, capture.Source, string, int) ([]capture.Stream, string, error) {
	return []capture.Stream{{Key: "stream", Generation: "generation", Size: 0}}, "", nil
}
func (a *pollAdapter) ReadChanges(_ context.Context, _ capture.Source, _ capture.Stream, cp capture.Checkpoint, _ capture.Limits) (capture.Changes, error) {
	return a.read(cp)
}

func pollFixture(t *testing.T) (*Module, *pollAdapter, capture.Source, capture.Stream) {
	t.Helper()
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "synthetic-profile")
	if _, err := memory.Init(ctx, home, "http://127.0.0.1:1", true); err != nil {
		t.Fatal(err)
	}
	svc, err := memory.Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	m := Install(svc)
	m.FlushInterval = time.Hour
	adapter := &pollAdapter{}
	m.SetAdapters(map[string]capture.Adapter{"synthetic": adapter})
	if _, err := m.SetPolicy(ctx, func(p *Policy) error { p.Capture = true; return nil }); err != nil {
		t.Fatal(err)
	}
	source, _, err := m.AddSource(ctx, "synthetic", json.RawMessage(`{"pin":"old"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.EnableSource(ctx, source.ID, nil); err != nil {
		t.Fatal(err)
	}
	source, err = m.Source(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	return m, adapter, source, capture.Stream{Key: "stream", Generation: "generation"}
}

func durablePosition(t *testing.T, m *Module, source capture.Source) string {
	t.Helper()
	var position string
	err := m.svc.Read(context.Background(), func(q memory.Queryer) error {
		return q.QueryRowContext(context.Background(), "SELECT position FROM checkpoints WHERE source_id=? AND stream='stream'", source.ID).Scan(&position)
	})
	if err != nil {
		t.Fatal(err)
	}
	return position
}

func TestPollRejectsConfigurationChangedDuringRead(t *testing.T) {
	m, adapter, source, stream := pollFixture(t)
	ctx := context.Background()
	calls := 0
	adapter.read = func(cp capture.Checkpoint) (capture.Changes, error) {
		calls++
		if err := m.DisableSourceWithConsent(ctx, source.ID, false); err != nil {
			t.Fatal(err)
		}
		if _, err := m.ConfigureSource(ctx, source.ID, json.RawMessage(`{"pin":"new"}`)); err != nil {
			t.Fatal(err)
		}
		if err := m.EnableSource(ctx, source.ID, nil); err != nil {
			t.Fatal(err)
		}
		cp.Position = "obsolete-page"
		return capture.Changes{Next: cp, CaughtUp: true}, nil
	}
	if _, err := m.Poll(ctx, source, stream); !errors.Is(err, memory.ErrLeaseLost) {
		t.Fatalf("old read committed: %v", err)
	}
	if got := durablePosition(t, m, source); got != "0" {
		t.Fatalf("obsolete page acknowledged: %q", got)
	}
	// A sleeping old worker must be refused before invoking its adapter again.
	if _, err := m.Poll(ctx, source, stream); !errors.Is(err, memory.ErrLeaseLost) || calls != 1 {
		t.Fatalf("old source reused: calls=%d err=%v", calls, err)
	}
}

func TestPollBufferCursorAndPolicyFence(t *testing.T) {
	m, adapter, source, stream := pollFixture(t)
	ctx := context.Background()
	positions := []string{}
	adapter.read = func(cp capture.Checkpoint) (capture.Changes, error) {
		positions = append(positions, cp.Position)
		cp.Position += "x"
		return capture.Changes{Next: cp, ScannedBytes: 1}, nil
	}
	for i := 0; i < 2; i++ {
		if _, err := m.Poll(ctx, source, stream); err != nil {
			t.Fatal(err)
		}
	}
	if len(positions) != 2 || positions[0] != "0" || positions[1] != "0x" || durablePosition(t, m, source) != "0" {
		t.Fatalf("buffer cursor repeated or acknowledged early: %v", positions)
	}
	if _, err := m.SetPolicy(ctx, func(p *Policy) error { p.ToolDetail = "full"; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Poll(ctx, source, stream); !errors.Is(err, memory.ErrLeaseLost) {
		t.Fatalf("obsolete buffer retained: %v", err)
	}
	adapter.read = func(cp capture.Checkpoint) (capture.Changes, error) {
		if cp.Position != "0" {
			t.Fatalf("obsolete buffer skipped uncommitted bytes: %q", cp.Position)
		}
		cp.Position = "final"
		return capture.Changes{Next: cp, CaughtUp: true}, nil
	}
	if _, err := m.Poll(ctx, source, stream); err != nil {
		t.Fatal(err)
	}
	if got := durablePosition(t, m, source); got != "final" {
		t.Fatalf("caught-up page not committed: %q", got)
	}
}

func TestPollPersistsQuarantineButRetriesTransport(t *testing.T) {
	m, adapter, source, stream := pollFixture(t)
	ctx := context.Background()
	adapter.read = func(cp capture.Checkpoint) (capture.Changes, error) {
		return capture.Changes{Next: cp}, capture.ErrUnavailable
	}
	if _, err := m.Poll(ctx, source, stream); !errors.Is(err, capture.ErrUnavailable) {
		t.Fatal(err)
	}
	if allowed, err := m.StreamAllowed(ctx, source.ID, stream.Key); err != nil || !allowed {
		t.Fatalf("transport failure quarantined: allowed=%v err=%v", allowed, err)
	}
	adapter.read = func(cp capture.Checkpoint) (capture.Changes, error) {
		cp.Position = "unacknowledged"
		return capture.Changes{Next: cp, Quarantine: "protocol_or_version_mismatch"}, capture.ErrInvalid
	}
	result, err := m.Poll(ctx, source, stream)
	if err != nil || result.Blocked != "protocol_or_version_mismatch" {
		t.Fatalf("quarantine dropped: result=%+v err=%v", result, err)
	}
	if err := m.svc.Read(ctx, func(q memory.Queryer) error {
		var state, reason string
		if err := q.QueryRowContext(ctx, "SELECT state,reason FROM sources WHERE source_id=?", source.ID).Scan(&state, &reason); err != nil {
			return err
		}
		if state != "quarantined" || reason != result.Blocked {
			t.Fatalf("quarantine not durable: %q %q", state, reason)
		}
		var gaps int
		if err := q.QueryRowContext(ctx, "SELECT count(*) FROM source_gaps WHERE source_id=? AND reason=?", source.ID, result.Blocked).Scan(&gaps); err != nil {
			return err
		}
		if gaps != 1 {
			t.Fatalf("quarantine gap missing: %d", gaps)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := durablePosition(t, m, source); got != "0" {
		t.Fatalf("quarantined page acknowledged: %q", got)
	}
	// A stale worker status write must not clear the durable quarantine.
	if err := m.MarkCaptureSource(ctx, source, "enabled", ""); !errors.Is(err, memory.ErrLeaseLost) {
		t.Fatalf("quarantine overwritten: %v", err)
	}
}

func TestPollPreservesOldGenerationBeforeBackpressure(t *testing.T) {
	m, adapter, source, stream := pollFixture(t)
	ctx := context.Background()
	calls := 0
	adapter.read = func(cp capture.Checkpoint) (capture.Changes, error) {
		calls++
		switch calls {
		case 1:
			cp.Position = "old-page"
			return capture.Changes{Next: cp, Omissions: []records.Omission{{Reason: "old_generation_gap"}}}, nil
		case 2:
			cp.Generation = "rotated"
			cp.Position = "oversized-page"
			return capture.Changes{Next: cp, Observations: make([]capture.Observation, 8193)}, nil
		default:
			if cp.Position != "old-page" {
				t.Fatalf("lost old cursor or resurrected empty buffer: %+v", cp)
			}
			cp.Generation = "rotated"
			cp.Position = "new-page"
			return capture.Changes{Next: cp, CaughtUp: true}, nil
		}
	}
	if _, err := m.Poll(ctx, source, stream); err != nil {
		t.Fatal(err)
	}
	result, err := m.Poll(ctx, source, stream)
	if err == nil || result.Blocked != "buffer_bound" || durablePosition(t, m, source) != "old-page" {
		t.Fatalf("old generation not committed before backpressure: %+v %v", result, err)
	}
	if err := m.svc.Read(ctx, func(q memory.Queryer) error {
		var gaps int
		if err := q.QueryRowContext(ctx, "SELECT count(*) FROM source_gaps WHERE source_id=? AND reason='old_generation_gap'", source.ID).Scan(&gaps); err != nil {
			return err
		}
		if gaps != 1 {
			t.Fatalf("old page audit lost: %d", gaps)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Poll(ctx, source, stream); err != nil {
		t.Fatal(err)
	}
	if got := durablePosition(t, m, source); got != "new-page" {
		t.Fatalf("rotation did not recover: %q", got)
	}
}

func TestPollDoesNotAttributeUnmappedChildToParent(t *testing.T) {
	m, adapter, source, stream := pollFixture(t)
	ctx := context.Background()
	parentPath := filepath.Join(t.TempDir(), "mapped-parent")
	if _, err := m.MapProject(ctx, parentPath, "", "synthetic-parent"); err != nil {
		t.Fatal(err)
	}
	parent := records.Origin{Harness: "codex", SourceStore: source.StoreIdentity, NativeSessionID: "parent"}
	child := parent
	child.NativeSessionID = "child"
	adapter.read = func(cp capture.Checkpoint) (capture.Changes, error) {
		cp.Position = "final"
		return capture.Changes{
			Next: cp, CaughtUp: true,
			Observations: []capture.Observation{
				{Origin: parent, CWD: parentPath, State: "open"},
				{Origin: child, Parent: &records.SessionLink{Origin: parent, Relation: "fork"}, State: "open"},
			},
			Events: []capture.Event{{Origin: child, CWD: filepath.Join(t.TempDir(), "unmapped-child"),
				PositionKey: "child-event", Event: records.Event{ID: "child-event", Role: "user", Type: "message",
					Text: "Synthetic child content", Time: time.Now().UTC().Format(time.RFC3339Nano)}}},
		}, nil
	}
	result, err := m.Poll(ctx, source, stream)
	if err != nil || result.Events != 0 || result.Omitted != 1 || result.Checkpoint != "final" {
		t.Fatalf("child inherited an unauthorized project: %+v %v", result, err)
	}
}
