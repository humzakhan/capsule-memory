package session

import (
	"context"
	"database/sql"
	"github.com/humzakhan/capsule-memory/internal/memory"
)

// Bound the daemon's source worker population at registration, without discovering any sources.
func (m *Module) sourceCapacity(ctx context.Context) error {
	return m.svc.Read(ctx, func(q memory.Queryer) error {
		var n int
		if e := q.QueryRowContext(ctx, "SELECT count(*) FROM sources").Scan(&n); e != nil {
			return e
		}
		if n >= 64 {
			return memory.ErrInvalid
		}
		return nil
	})
}
func (m *Module) pausedGap(ctx context.Context, tx *sql.Tx, id string) error {
	if e := auditGap(ctx, tx, id, "", "", "paused_skipped", 1); e != nil {
		return e
	}
	_, e := tx.ExecContext(ctx, "UPDATE checkpoints SET reason='paused_skipped' WHERE source_id=?", id)
	return e
}
