package memory

import (
	"context"
	"encoding/json"
	"github.com/humzakhan/capsule-memory/internal/records"
)

func (s *Service) publicationStatus(ctx context.Context, q Queryer, scope string, out *ScopeStatus) error {
	queries := []struct {
		sql string
		dst *int
	}{
		{"SELECT count(*) FROM outbox o JOIN outbox_hold h ON h.revision=o.revision WHERE o.scope=? AND o.receipt IS NULL", &out.Held},
		{"SELECT count(*) FROM outbox o JOIN publication_block b ON b.revision=o.revision WHERE o.scope=? AND o.receipt IS NULL", &out.PublicationBlocked},
		{"SELECT count(*) FROM outbox o JOIN publication_block b ON b.revision=o.revision WHERE o.scope=? AND o.receipt IS NULL AND b.reason='waiting_ancestry'", &out.WaitingAncestry},
		{"SELECT count(*) FROM outbox o JOIN publication_block b ON b.revision=o.revision JOIN revisions r ON r.revision=o.revision WHERE o.scope=? AND o.receipt IS NULL AND b.reason='waiting_ancestry' AND (json_extract(r.body,'$.operation')='delete' OR (r.kind='projection' AND json_extract(r.body,'$.body.state') IN ('hidden','withdrawn')))", &out.CleanupWaitingAncestry},
		{"SELECT count(*) FROM outbox o JOIN revisions r ON r.revision=o.revision WHERE o.scope=? AND o.receipt IS NULL AND r.kind='projection' AND json_extract(r.body,'$.body.withdrawal_marker')=1", &out.PendingMarkers},
	}
	for _, v := range queries {
		if q.QueryRowContext(ctx, v.sql, scope).Scan(v.dst) != nil {
			return ErrStorage
		}
	}
	rows, e := q.QueryContext(ctx, "SELECT r.body FROM outbox o JOIN revisions r ON r.revision=o.revision WHERE o.scope=? AND o.receipt IS NULL AND r.kind='projection' AND json_extract(r.body,'$.body.withdrawal_marker')=1 AND NOT EXISTS(SELECT 1 FROM outbox_hold h WHERE h.revision=o.revision) ORDER BY o.seq LIMIT 1001", scope)
	if e != nil {
		return ErrStorage
	}
	var candidates []Revision
	for rows.Next() {
		var raw []byte
		var r Revision
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &r) != nil {
			rows.Close()
			return ErrStorage
		}
		candidates = append(candidates, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return ErrStorage
	}
	if len(candidates) > 1000 {
		out.MarkerStatusBounded = true
		candidates = candidates[:1000]
	}
	for _, r := range candidates {
		pending := false
		for _, parent := range r.Parents {
			var n int
			if q.QueryRowContext(ctx, "SELECT count(*) FROM outbox WHERE revision=? AND receipt IS NULL", parent).Scan(&n) != nil {
				return ErrStorage
			}
			pending = pending || n != 0
		}
		if pending {
			continue
		}
		var p records.Projection
		if json.Unmarshal(r.Body, &p) != nil {
			return ErrStorage
		}
		if s.VerifyProjectionMarker(ctx, q, r, p, true) == nil {
			out.ReadyMarkers++
		}
	}
	return nil
}
