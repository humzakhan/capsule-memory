package session

import (
	"context"
	"encoding/json"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

func (m *Module) coverage(ctx context.Context, scopes []string) (Coverage, error) {
	var result Coverage
	err := m.svc.Read(ctx, func(q memory.Queryer) error {
		var err error
		result, err = m.coverageQuery(ctx, q, scopes)
		return err
	})
	return result, err
}

// Retrieval passes its own snapshot so coverage cannot describe a later state.
func (m *Module) coverageQuery(ctx context.Context, q memory.Queryer, scopes []string) (Coverage, error) {
	coverage := Coverage{Complete: true}
	var pending int
	err := q.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM checkpoints WHERE caught_up=0 OR state IN ('gap','quarantined'))+
		(SELECT count(*) FROM sources WHERE enabled=1 AND state IN ('paused','blocked','quarantined','unavailable'))`).Scan(&pending)
	if err != nil {
		return coverage, err
	}
	if pending > 0 {
		coverage.Complete = false
	}
	for _, scope := range scopes {
		var count, indexed, stale int
		err := q.QueryRowContext(ctx, "SELECT count(*) FROM chunks c JOIN sessions s ON s.session=c.session WHERE s.scope=? AND s.deleted=0", scope).Scan(&count)
		if err != nil {
			return coverage, err
		}
		coverage.AvailableChunks += count
		if err := q.QueryRowContext(ctx, "SELECT count(*) FROM session_search WHERE scope=? AND kind='session_chunk'", scope).Scan(&indexed); err != nil {
			return coverage, err
		}
		coverage.IndexedChunks += indexed
		status, err := m.svc.ScopeStatusQuery(ctx, q, scope)
		if err != nil {
			return coverage, err
		}
		coverage.SourceHeads = append(coverage.SourceHeads, status.Coverage.Head)
		if !status.Coverage.Complete || status.Blocked != "" || indexed != count {
			coverage.Complete = false
		}
		if err := q.QueryRowContext(ctx, "SELECT count(*) FROM stale x JOIN sessions s ON s.session=x.record WHERE s.scope=?", scope).Scan(&stale); err != nil {
			return coverage, err
		}
		coverage.StaleDependencies += stale
		rows, err := q.QueryContext(ctx, "SELECT session FROM sessions WHERE scope=? AND deleted=0 ORDER BY session LIMIT 100001", scope)
		if err != nil {
			return coverage, err
		}
		var manifests []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return coverage, memory.ErrStorage
			}
			manifests = append(manifests, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return coverage, err
		}
		if len(manifests) > 100000 {
			return coverage, memory.ErrStaleOverflow
		}
		for _, id := range manifests {
			heads, deleted, err := m.svc.Heads(ctx, q, scope, id)
			if err != nil {
				return coverage, err
			}
			if deleted || len(heads) != 1 {
				coverage.Complete = false
				continue
			}
			var manifest records.SessionManifest
			if json.Unmarshal(heads[0].Body, &manifest) != nil {
				return coverage, memory.ErrStorage
			}
			if len(manifest.Gaps) > 0 {
				coverage.Complete = false
				coverage.Gaps = append(coverage.Gaps, manifest.Gaps...)
			}
			root := id
			for n := 0; manifest.Continues != ""; n++ {
				if n >= 1024 {
					return coverage, memory.ErrInvalid
				}
				root = manifest.Continues
				prior, deleted, err := m.svc.Heads(ctx, q, scope, root)
				if err != nil {
					return coverage, err
				}
				if deleted || len(prior) != 1 {
					coverage.Complete = false
					break
				}
				var previous records.SessionManifest
				if json.Unmarshal(prior[0].Body, &previous) != nil {
					return coverage, memory.ErrStorage
				}
				manifest.Continues = previous.Continues
			}
			for branch, high := range manifest.ChunkHigh {
				var got int
				if err := q.QueryRowContext(ctx, "SELECT count(DISTINCT seq) FROM chunks WHERE session=? AND branch=?", root, branch).Scan(&got); err != nil {
					return coverage, err
				}
				if int64(got) < high+1 {
					coverage.Complete = false
				}
			}
		}
	}
	rows, err := q.QueryContext(ctx, "SELECT reason,sum(count) FROM source_gaps GROUP BY reason ORDER BY reason")
	if err != nil {
		return coverage, err
	}
	defer rows.Close()
	for rows.Next() {
		var gap records.Gap
		if err := rows.Scan(&gap.Reason, &gap.Count); err != nil {
			return coverage, memory.ErrStorage
		}
		coverage.Gaps = append(coverage.Gaps, gap)
		coverage.Complete = false
	}
	return coverage, rows.Err()
}
