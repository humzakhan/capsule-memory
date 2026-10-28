package session

import (
	"context"
	"database/sql"
	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"strings"
	"time"
)

func (m *Module) MarkMissingStreams(ctx context.Context, source string, seen map[string]bool) error {
	src, err := m.Source(ctx, source)
	if err != nil {
		return err
	}
	return m.MarkCaptureMissingStreams(ctx, src, seen)
}

// The seen set belongs to the source snapshot used for discovery. A later
// configuration must not inherit an old worker's absence observations.
func (m *Module) MarkCaptureMissingStreams(ctx context.Context, src capture.Source, seen map[string]bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.captureSnapshot(ctx, src, 0); err != nil {
		return err
	}
	var flushErr error
	prefix := src.ID + ":"
	for key, pending := range m.buffers {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		stream := strings.TrimPrefix(key, prefix)
		if !seen[stream] {
			if _, err := m.flushBuffer(ctx, src, capture.Stream{Key: stream}, key, pending); err != nil && flushErr == nil {
				flushErr = err
			}
		}
	}
	err := m.svc.Update(ctx, func(tx *sql.Tx) error {
		if _, err := captureSnapshot(ctx, tx, src, 0); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, "SELECT stream FROM checkpoints WHERE source_id=?", src.ID)
		if err != nil {
			return err
		}
		var missing []string
		for rows.Next() {
			var stream string
			if err := rows.Scan(&stream); err != nil {
				rows.Close()
				return memory.ErrStorage
			}
			if !seen[stream] {
				missing = append(missing, stream)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, stream := range missing {
			if err := auditGap(ctx, tx, src.ID, stream, "", "source_lost", 1); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `UPDATE checkpoints
				SET state=CASE WHEN state='quarantined' THEN state ELSE 'gap' END,
				reason=CASE WHEN state='quarantined' THEN reason ELSE 'source_lost' END
				WHERE source_id=? AND stream=?`, src.ID, stream)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return flushErr
}

func auditGap(ctx context.Context, tx *sql.Tx, source, stream, position, reason string, count int) error {
	_, e := tx.ExecContext(ctx, "INSERT INTO source_gaps VALUES(?,?,?,?,?) ON CONFLICT(source_id,stream,position,reason) DO UPDATE SET count=max(source_gaps.count,excluded.count)", source, stream, position, reason, count)
	return e
}

func quarantineStream(ctx context.Context, tx *sql.Tx, source, stream, reason string) error {
	_, e := tx.ExecContext(ctx, "INSERT INTO checkpoints(source_id,stream,generation,position,prefix_digest,updated,state,reason) VALUES(?,?, '', '', '',?,'quarantined',?) ON CONFLICT(source_id,stream) DO UPDATE SET state='quarantined',reason=excluded.reason", source, stream, time.Now().Unix(), reason)
	return e
}
