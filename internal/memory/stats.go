package memory

import "context"

// Stats counts locally visible legacy memory records, not revisions or cloud records.
func (s *Service) Stats(ctx context.Context) (map[string]any, error) {
	scopes := map[string]any{}
	var devices []map[string]any
	var totalMemories, totalSkills, totalPending int
	err := s.Read(ctx, func(q Queryer) error {
		for scope := range s.configuration().Keys {
			var memories, skills, pending int
			err := q.QueryRowContext(ctx, `SELECT count(DISTINCT r.record) FROM revisions r
				WHERE r.scope=? AND r.kind='memory'
				AND r.revision NOT IN (SELECT parent FROM parents)
				AND NOT EXISTS(SELECT 1 FROM revisions d WHERE d.scope=r.scope AND d.record=r.record
					AND json_extract(d.body,'$.operation')='delete' AND d.revision NOT IN (SELECT deletion FROM restores))`, scope).Scan(&memories)
			if err != nil {
				return ErrStorage
			}
			if err := q.QueryRowContext(ctx, `SELECT count(DISTINCT r.record) FROM revisions r
				WHERE r.scope=? AND r.kind='skill'
				AND r.revision NOT IN (SELECT parent FROM parents)
				AND NOT EXISTS(SELECT 1 FROM revisions d WHERE d.scope=r.scope AND d.record=r.record
					AND json_extract(d.body,'$.operation')='delete' AND d.revision NOT IN (SELECT deletion FROM restores))`, scope).Scan(&skills); err != nil {
				return ErrStorage
			}
			if err := q.QueryRowContext(ctx, "SELECT count(*) FROM outbox WHERE scope=? AND receipt IS NULL", scope).Scan(&pending); err != nil {
				return ErrStorage
			}
			coverage, err := coverageQuery(ctx, q, scope)
			if err != nil {
				return err
			}
			scopes[scope] = map[string]any{"memories": memories, "skills": skills, "pending_uploads": pending, "coverage": coverage}
			totalMemories += memories
			totalSkills += skills
			totalPending += pending
		}
		control, _, err := s.currentTx(ctx, q)
		if err != nil {
			return err
		}
		devices = make([]map[string]any, 0, len(control.Devices))
		for _, d := range control.Devices {
			devices = append(devices, map[string]any{"device_id": d.ID, "revoked": d.Revoked, "this_device": d.ID == s.configuration().Device})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"scopes": scopes, "devices": devices, "memories": totalMemories, "skills": totalSkills, "pending_uploads": totalPending, "count_basis": "locally visible legacy memory records; excludes session capture and skills", "capture_status": "not reported", "freshness": "local snapshot; sync required for latest relay data"}, nil
}
