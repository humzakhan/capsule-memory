package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

func (m *Module) captureDiskPressure(ctx context.Context) (bool, error) {
	if m.Home == "" {
		return false, nil
	}
	free, err := diskFree(m.Home)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(filepath.Join(m.Home, "memory.db"))
	if err != nil {
		return false, memory.ErrStorage
	}
	size := info.Size()
	if wal, err := os.Stat(filepath.Join(m.Home, "memory.db-wal")); err == nil {
		size += wal.Size()
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, memory.ErrStorage
	}
	var pressure int
	err = m.svc.Read(ctx, func(q memory.Queryer) error {
		err := q.QueryRowContext(ctx, "SELECT value FROM local_seq WHERE name='disk_pressure'").Scan(&pressure)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return false, err
	}
	low := m.LowWatermark
	if low <= 0 {
		low = m.HighWatermark * 9 / 10
	}
	blocked := free < m.FreeSpaceFloor || (m.HighWatermark > 0 && size > m.HighWatermark) || (pressure != 0 && size > low)
	err = m.svc.Update(ctx, func(tx *sql.Tx) error {
		value := 0
		if blocked {
			value = 1
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO local_seq(name,value) VALUES('disk_pressure',?)
			ON CONFLICT(name) DO UPDATE SET value=excluded.value`, value)
		return err
	})
	return blocked, err
}

func (m *Module) captureGroup(ctx context.Context, q memory.Queryer, src capture.Source, st capture.Stream,
	info memory.ProfileInfo, policy Policy, project, scope string, observation capture.Observation) (*ingestGroup, error) {
	key := SessionKey(observation.Origin)
	group := &ingestGroup{scope: scope, project: project, branch: hash([]byte(src.ID + "\n" + info.Device + "\n" + st.Key))[:32]}
	err := q.QueryRowContext(ctx, `SELECT session FROM sessions WHERE session_key=? AND scope=?
		AND EXISTS(SELECT 1 FROM revisions root WHERE root.record=sessions.session
		AND root.kind='session_manifest' AND COALESCE(json_extract(root.body,'$.body.continues'),'')='')
		ORDER BY session LIMIT 1`, key, scope).Scan(&group.session)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if group.session == "" {
		group.session, err = memory.NewID()
		if err != nil {
			return nil, err
		}
		group.manifestRecord = group.session
		title, _ := Redact("title", observation.Title, policy.RedactionExtra)
		branch, _ := Redact("branch", observation.Branch, policy.RedactionExtra)
		group.manifest = records.SessionManifest{
			SessionKey: key, ProjectID: project, Origin: observation.Origin,
			Adapter: src.Adapter, HarnessVersion: observation.HarnessVersion,
			Title: title, Branch: branch, Parent: observation.Parent,
			Bindings: observation.Bindings, Links: observation.Links, State: observation.State,
			Policy:    records.CapturePolicy{Version: policy.Version, Redaction: "redact/v1", ToolDetail: policy.ToolDetail},
			ChunkHigh: map[string]int64{"main": -1},
		}
	} else {
		latest, merged, err := m.latestManifest(ctx, q, scope, group.session)
		if err != nil {
			return nil, err
		}
		group.manifestRecord = latest.Record
		group.manifest = merged
		if observation.State != "" {
			group.manifest.State = observation.State
		}
		var count, total int64
		err = q.QueryRowContext(ctx, "SELECT count(*),COALESCE(sum(length(body)),0) FROM revisions WHERE record=?", group.manifestRecord).Scan(&count, &total)
		if err != nil {
			return nil, err
		}
		if count >= 1024 || total >= 24<<20 {
			old := group.manifestRecord
			group.manifestRecord, err = memory.NewID()
			if err != nil {
				return nil, err
			}
			group.manifest.Continues = old
		}
	}
	group.manifest.Bindings = dedupBindings(append(group.manifest.Bindings, observation.Bindings...))
	group.manifest.Links = dedupLinks(append(group.manifest.Links, observation.Links...))
	if group.manifest.ChunkHigh == nil {
		group.manifest.ChunkHigh = map[string]int64{"main": -1}
	}
	if len(group.manifest.ChunkHigh) > 256 {
		return nil, memory.ErrInvalid
	}
	return group, nil
}

func (m *Module) prepareCapture(ctx context.Context, src capture.Source, st capture.Stream, ch capture.Changes,
	info memory.ProfileInfo, groups map[string]*ingestGroup, gaps []records.Omission, result *IngestResult) ([]memory.Prepared, error) {
	var prepared []memory.Prepared
	sourceKind := "capture"
	if src.Harness == "import" {
		sourceKind = "import"
	}
	for _, group := range groups {
		if len(group.manifest.Links) > 64 || len(group.manifest.Bindings) > 16 || len(group.manifest.ChunkHigh) > 256 {
			result.Blocked = "lineage_bound"
			return nil, m.quarantineCapture(ctx, src, st, ch, result.Blocked, 1)
		}
		if group.manifestRecord == group.session {
			err := m.svc.Read(ctx, func(q memory.Queryer) error {
				rows, err := q.QueryContext(ctx, `SELECT session FROM sessions
					WHERE session_key=? AND scope!=? AND deleted=0 ORDER BY session LIMIT 65`, group.manifest.SessionKey, group.scope)
				if err != nil {
					return err
				}
				defer rows.Close()
				for rows.Next() {
					var id string
					if err := rows.Scan(&id); err != nil {
						return memory.ErrStorage
					}
					group.manifest.Links = append(group.manifest.Links, records.SessionLink{Session: id, Origin: group.manifest.Origin, Relation: "continuation"})
				}
				return rows.Err()
			})
			if err != nil {
				return nil, err
			}
			for _, other := range groups {
				if other != group && other.manifest.SessionKey == group.manifest.SessionKey && other.scope != group.scope {
					group.manifest.Links = append(group.manifest.Links, records.SessionLink{Session: other.session, Origin: other.manifest.Origin, Relation: "continuation"})
				}
			}
			group.manifest.Links = dedupLinks(group.manifest.Links)
			if len(group.manifest.Links) > 64 {
				result.Blocked = "lineage_bound"
				return nil, m.quarantineCapture(ctx, src, st, ch, result.Blocked, len(group.manifest.Links))
			}
		}
		seq := int64(-1)
		previous := ""
		err := m.svc.Read(ctx, func(q memory.Queryer) error {
			err := q.QueryRowContext(ctx, "SELECT seq,record FROM chunks WHERE session=? AND branch=? ORDER BY seq DESC LIMIT 1", group.session, group.branch).Scan(&seq, &previous)
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		})
		if err != nil {
			return nil, err
		}
		for index, events := range chunks(group.events) {
			seq++
			chunk := records.SessionChunk{
				Session: group.session, Branch: group.branch, Seq: seq, Prev: previous,
				Generation: ch.Next.Generation, Events: events,
				Omissions:  chunkOmissions(events, group.events, append(append([]records.Omission{}, group.omissions...), gaps...), index == 0),
				Redactions: chunkRedactions(events, group.redactions, group.events),
			}
			body, err := json.Marshal(events)
			if err != nil {
				return nil, memory.ErrInvalid
			}
			chunk.Digest = hash(body)
			parts, err := m.prepareChunk(ctx, info, sourceKind, src.ID, src.Harness, group.scope, chunk)
			if err != nil {
				if errors.Is(err, errChunkMetadataBound) {
					result.Blocked = "chunk_metadata_bound"
					return nil, m.quarantineCapture(ctx, src, st, ch, result.Blocked, 1)
				}
				return nil, err
			}
			for _, part := range parts {
				prepared = append(prepared, part)
				previous = part.Revision.Record
				result.Chunks++
			}
			seq += int64(len(parts) - 1)
			result.Events += len(events)
		}
		if _, exists := group.manifest.ChunkHigh[group.branch]; !exists && len(group.manifest.ChunkHigh) >= 256 {
			return nil, memory.ErrInvalid
		}
		group.manifest.ChunkHigh[group.branch] = seq
		for _, omission := range append(group.omissions, gaps...) {
			group.manifest.Gaps = append(group.manifest.Gaps, records.Gap{From: omission.Event, Reason: omission.Reason, Count: 1})
		}
		group.manifest.Gaps = records.MergeGaps(group.manifest.Gaps)
		manifest, err := m.svc.Prepare(ctx, memory.PutInput{
			Scope: group.scope, Kind: records.KindSessionManifest, Record: group.manifestRecord, Body: group.manifest,
			Provenance: memory.Provenance{SourceKind: sourceKind, SourceRef: src.ID, Harness: src.Harness},
		})
		if err != nil {
			return nil, err
		}
		prepared = append(prepared, manifest)
	}
	return prepared, nil
}
