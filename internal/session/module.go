package session

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/records"
)

type Module struct {
	svc                                         *memory.Service
	mu                                          sync.Mutex
	polls                                       sync.Map
	adapters                                    map[string]capture.Adapter
	wake                                        chan struct{}
	FlushInterval                               time.Duration
	diskBlocked                                 bool
	buffers                                     map[string]streamBuffer
	Home                                        string
	HighWatermark, LowWatermark, FreeSpaceFloor int64
}

func Install(svc *memory.Service) *Module {
	m := &Module{svc: svc, adapters: capture.Registry(), wake: make(chan struct{}, 1), buffers: map[string]streamBuffer{}, FlushInterval: 5 * time.Second, FreeSpaceFloor: 1 << 30}
	for _, k := range []string{records.KindProject, records.KindSessionManifest, records.KindSessionChunk, records.KindSuppression} {
		svc.RegisterApplier(k, &applier{m: m, kind: k})
	}
	return m
}
func (m *Module) SetAdapters(a map[string]capture.Adapter) { m.adapters = a }
func (m *Module) Wake() <-chan struct{}                    { return m.wake }
func (m *Module) Hint(sourceID, stream string) {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func SessionKey(o records.Origin) string {
	h := sha256.Sum256([]byte("capsule-memory/session-key/v1\n" + o.SourceStore + "\n" + o.Harness + "\n" + o.Environment + "\n" + o.NativeSessionID + "\n"))
	return hex.EncodeToString(h[:])
}
func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func (m *Module) Source(ctx context.Context, id string) (capture.Source, error) {
	var s capture.Source
	e := m.svc.Read(ctx, func(q memory.Queryer) error {
		return q.QueryRowContext(ctx, "SELECT source_id,harness,adapter,store_identity,config,COALESCE(consent_at,'') FROM sources WHERE source_id=?", id).Scan(&s.ID, &s.Harness, &s.Adapter, &s.StoreIdentity, &s.Config, &s.Consent)
	})
	if e != nil {
		return s, memory.ErrInvalid
	}
	return s, nil
}
func (m *Module) Sources(ctx context.Context) ([]capture.Source, error) {
	var out []capture.Source
	e := m.svc.Read(ctx, func(q memory.Queryer) error {
		rows, e := q.QueryContext(ctx, "SELECT source_id,harness,adapter,store_identity,config,COALESCE(consent_at,'') FROM sources ORDER BY source_id")
		if e != nil {
			return memory.ErrStorage
		}
		defer rows.Close()
		for rows.Next() {
			var s capture.Source
			if rows.Scan(&s.ID, &s.Harness, &s.Adapter, &s.StoreIdentity, &s.Config, &s.Consent) != nil {
				return memory.ErrStorage
			}
			out = append(out, s)
		}
		return rows.Err()
	})
	return out, e
}
func (m *Module) CanCapture(ctx context.Context, id string) (bool, error) {
	p, e := m.Policy(ctx)
	if e != nil || !p.Capture {
		return false, e
	}
	ok := false
	e = m.svc.Read(ctx, func(q memory.Queryer) error {
		var enabled, pause int
		var state string
		if e := q.QueryRowContext(ctx, "SELECT enabled,state FROM sources WHERE source_id=?", id).Scan(&enabled, &state); e != nil {
			return memory.ErrInvalid
		}
		if e := q.QueryRowContext(ctx, "SELECT count(*) FROM pauses WHERE source_id=? AND resolution IS NULL", id).Scan(&pause); e != nil {
			return memory.ErrStorage
		}
		ok = enabled == 1 && pause == 0 && state != "withdrawn" && state != "quarantined"
		return nil
	})
	return ok, e
}
func (m *Module) Checkpoint(ctx context.Context, id, stream string) (capture.Checkpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pending, ok := m.buffers[id+":"+stream]; ok {
		return pending.changes.Next, nil
	}
	cp := capture.Checkpoint{Stream: stream}
	e := m.svc.Read(ctx, func(q memory.Queryer) error {
		e := q.QueryRowContext(ctx, "SELECT generation,position,prefix_digest,tail_digest,COALESCE(file_identity,''),adapter_state,observed_modified,observed_size,reconciling,discarding,caught_up FROM checkpoints WHERE source_id=? AND stream=?", id, stream).Scan(&cp.Generation, &cp.Position, &cp.PrefixDigest, &cp.TailDigest, &cp.FileIdentity, &cp.AdapterState, &cp.ObservedModified, &cp.ObservedSize, &cp.Reconciling, &cp.Discarding, &cp.CaughtUp)
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		return e
	})
	if len(cp.AdapterState) > 65536 {
		return cp, memory.ErrInvalid
	}
	return cp, e
}
func checkpoint(ctx context.Context, tx *sql.Tx, source string, cp capture.Checkpoint) error {
	if len(cp.AdapterState) > 65536 {
		return memory.ErrInvalid
	}
	_, e := tx.ExecContext(ctx, "INSERT INTO checkpoints(source_id,stream,generation,position,prefix_digest,tail_digest,file_identity,adapter_state,observed_modified,observed_size,reconciling,discarding,caught_up,updated,state,reason) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,'ok',NULL) ON CONFLICT(source_id,stream) DO UPDATE SET generation=excluded.generation,position=excluded.position,prefix_digest=excluded.prefix_digest,tail_digest=excluded.tail_digest,file_identity=excluded.file_identity,adapter_state=excluded.adapter_state,observed_modified=excluded.observed_modified,observed_size=excluded.observed_size,reconciling=excluded.reconciling,discarding=excluded.discarding,caught_up=excluded.caught_up,updated=excluded.updated", source, cp.Stream, cp.Generation, cp.Position, cp.PrefixDigest, cp.TailDigest, cp.FileIdentity, cp.AdapterState, cp.ObservedModified, cp.ObservedSize, cp.Reconciling, cp.Discarding, cp.CaughtUp, time.Now().Unix())
	return e
}
func (m *Module) MarkSource(ctx context.Context, id, state, reason string) error {
	return m.svc.Update(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, "UPDATE sources SET state=?,reason=? WHERE source_id=?", state, reason, id)
		return e
	})
}

type Grant = ops.Grant
type Backfill struct {
	Since    time.Time
	MaxBytes int64
	Streams  []string
}
type IngestResult struct {
	Chunks, Events, Omitted int
	Checkpoint, Blocked     string
}
type Coverage struct {
	Issues                         []string
	SourceKinds, Ranges            []string
	Gaps                           []records.Gap
	SourceHeads                    []string
	IndexedChunks, AvailableChunks int
	LastReconcile                  string
	StaleDependencies              int
	Complete                       bool
}
type SearchResult struct {
	Results  []SearchHit `json:"results"`
	Coverage Coverage    `json:"coverage"`
}
type SearchHit struct {
	Session, Chunk, Revision, EventID, Role, Snippet string
	Untrusted                                        bool
}
type ReadResult struct {
	Branch   string
	Branches []string
	Chunks   []records.SessionChunk
	Next     int64
	Complete bool
	Coverage Coverage
}
type CaptureStatus struct {
	Publication                                                    map[string]memory.ScopeStatus
	Sources                                                        []SourceStatus
	CapturedThrough                                                map[string]string
	PendingBytes                                                   int64
	PublishedThrough                                               map[string]string
	IndexCoverage                                                  Coverage
	Quarantined, Exclusions                                        int
	Blocked                                                        []string
	Saved, Published, Summarized                                   int
	CacheEviction                                                  string
	DuplicateCapture                                               int
	BufferedStreams, BufferedEvents, BufferedBytes, PendingStreams int
	Queues                                                         map[string]map[string]int
}
type SourceStatus struct {
	ID, Harness, Adapter, State, Reason string
	Streams, Quarantined                int
	Token                               string
}

func (m *Module) StreamAllowed(ctx context.Context, source, stream string) (bool, error) {
	allowed := true
	e := m.svc.Read(ctx, func(q memory.Queryer) error {
		var state string
		e := q.QueryRowContext(ctx, "SELECT state FROM checkpoints WHERE source_id=? AND stream=?", source, stream).Scan(&state)
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		allowed = state != "quarantined"
		return nil
	})
	return allowed, e
}
