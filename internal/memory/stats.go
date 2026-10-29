package memory

import "context"

// Stats counts locally visible legacy memory records, not revisions or cloud records.
func (s *Service) Stats(ctx context.Context) (map[string]any, error) {
	scopes := map[string]any{}
	err := s.Read(ctx, func(q Queryer) error {
		for scope := range s.configuration().Keys {
			var memories, pending int
			err := q.QueryRowContext(ctx, `SELECT count(DISTINCT r.record) FROM revisions r
				WHERE r.scope=? AND r.kind='memory'
				AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=r.revision)
				AND NOT EXISTS(SELECT 1 FROM revisions d WHERE d.scope=r.scope AND d.record=r.record
					AND json_extract(d.body,'$.operation')='delete' AND NOT EXISTS(SELECT 1 FROM restores x WHERE x.deletion=d.revision))`, scope).Scan(&memories)
			if err != nil {
				return ErrStorage
			}
			if err := q.QueryRowContext(ctx, "SELECT count(*) FROM outbox WHERE scope=? AND receipt IS NULL", scope).Scan(&pending); err != nil {
				return ErrStorage
			}
			coverage, err := coverageQuery(ctx, q, scope)
			if err != nil {
				return err
			}
			scopes[scope] = map[string]any{"memories": memories, "pending_uploads": pending, "coverage": coverage}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"scopes": scopes, "count_basis": "locally visible legacy memory records; excludes session capture and skills", "capture_status": "not reported", "freshness": "local snapshot; sync required for latest relay data"}, nil
}
