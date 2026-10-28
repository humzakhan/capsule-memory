package session

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

type DeleteResult struct {
	Session                  string
	Tombstones, Withdrawn    []string
	Stale, ProjectionsQueued int
}

func (m *Module) DeleteSession(ctx context.Context, session string, keepInsights []string) (DeleteResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := DeleteResult{Session: session}
	var manifests []memory.Revision
	var origin records.Origin
	var key string
	var keys []string
	keep := map[string]bool{}
	for _, id := range keepInsights {
		keep[id] = true
	}
	var dependent []memory.Revision
	e := m.svc.Read(ctx, func(q memory.Queryer) error {
		if e := q.QueryRowContext(ctx, "SELECT session_key FROM sessions WHERE session=?", session).Scan(&key); e != nil {
			return memory.ErrInvalid
		}
		related, err := relatedKeys(ctx, q, session)
		if err != nil {
			return err
		}
		keys = related
		rows, e := q.QueryContext(ctx, relatedCTE+" SELECT DISTINCT r.record,r.scope FROM revisions r JOIN related s ON s.session=r.record WHERE r.kind='session_manifest'", session)
		if e != nil {
			return e
		}
		type ref struct{ record, scope string }
		var refs []ref
		for rows.Next() {
			var r ref
			if rows.Scan(&r.record, &r.scope) != nil {
				rows.Close()
				return memory.ErrStorage
			}
			refs = append(refs, r)
		}
		rows.Close()
		for _, ref := range refs {
			heads, deleted, e := m.svc.Heads(ctx, q, ref.scope, ref.record)
			if e != nil {
				return e
			}
			if !deleted && len(heads) > 0 {
				manifests = append(manifests, heads[0])
				var body records.SessionManifest
				if json.Unmarshal(heads[0].Body, &body) != nil {
					return memory.ErrStorage
				}
				origin = body.Origin
			}
		}
		rows, e = q.QueryContext(ctx, relatedCTE+", roots(record) AS(SELECT session FROM related UNION SELECT c.record FROM chunks c JOIN related s ON s.session=c.session), affected(record) AS(SELECT record FROM roots UNION SELECT d.child FROM deps d JOIN affected a ON d.parent=a.record WHERE NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=d.child_revision) LIMIT 100001) SELECT DISTINCT r.record,r.scope FROM revisions r JOIN affected a ON a.record=r.record WHERE r.kind='insight' LIMIT 100001", session)
		if e != nil {
			return e
		}
		refs = nil
		for rows.Next() {
			var r ref
			if rows.Scan(&r.record, &r.scope) != nil {
				rows.Close()
				return memory.ErrStorage
			}
			refs = append(refs, r)
		}
		rows.Close()
		if len(refs) > 100000 {
			return memory.ErrStaleOverflow
		}
		for _, ref := range refs {
			heads, deleted, e := m.svc.Heads(ctx, q, ref.scope, ref.record)
			if e != nil {
				return e
			}
			if deleted {
				continue
			}
			for _, h := range heads {
				var in records.Insight
				if json.Unmarshal(h.Body, &in) != nil {
					return memory.ErrStorage
				}
				if in.Review == "accepted" || in.Review == "edited" {
					dependent = append(dependent, h)
					break
				}
			}
		}
		return nil
	})
	if e != nil {
		return result, e
	}
	if len(manifests) == 0 {
		return result, memory.ErrInvalid
	}
	info, e := m.svc.Info(ctx)
	if e != nil {
		return result, e
	}
	if len(keep) > 0 && !info.Owner {
		return result, memory.ErrInvalid
	}
	var prepared []memory.Prepared
	for _, r := range manifests {
		p, e := m.svc.Prepare(ctx, memory.PutInput{Scope: r.Scope, Kind: r.Kind, Record: r.Record, Operation: "delete", Provenance: memory.Provenance{SourceKind: "manual", SourceRef: "session-delete"}})
		if e != nil {
			return result, e
		}
		prepared = append(prepared, p)
		result.Tombstones = append(result.Tombstones, r.Record)
	}
	for _, r := range dependent {
		in := memory.PutInput{Scope: r.Scope, Kind: r.Kind, Record: r.Record, Operation: "delete", Provenance: memory.Provenance{SourceKind: "manual", SourceRef: "session-delete"}}
		if keep[r.Record] {
			var body records.Insight
			if json.Unmarshal(r.Body, &body) != nil {
				return result, memory.ErrStorage
			}
			body.Evidence = []records.SourceRef{}
			body.Influence = nil
			body.OwnerProof = nil
			body.Contradictions = nil
			body.Basis = "explicit"
			body.Generator = nil
			body.Reviewer = info.Device
			body.Review = "edited"
			body.AutoAccepted = false
			body.Verification = "none"
			in.Operation = "put"
			in.Body = body
		} else {
			result.Withdrawn = append(result.Withdrawn, r.Record)
		}
		var p memory.Prepared
		var e error
		if keep[r.Record] {
			p, e = m.svc.PrepareOwnerInsight(ctx, in)
		} else {
			p, e = m.svc.Prepare(ctx, in)
		}
		if e != nil {
			return result, e
		}
		prepared = append(prepared, p)
	}
	for _, sessionKey := range keys {
		for _, r := range manifests {
			var b records.SessionManifest
			if json.Unmarshal(r.Body, &b) != nil {
				return result, memory.ErrInvalid
			}
			if b.SessionKey == sessionKey {
				origin = b.Origin
				break
			}
		}
		sup := records.Suppression{SessionKey: sessionKey, Origin: origin, Session: session, Reason: "deleted"}
		p, e := m.svc.Prepare(ctx, memory.PutInput{Scope: manifests[0].Scope, Kind: records.KindSuppression, Body: sup, Provenance: memory.Provenance{SourceKind: "manual", SourceRef: "session-delete"}})
		if e != nil {
			return result, e
		}
		prepared = append(prepared, p)
	}
	e = m.svc.Commit(ctx, prepared, func(ctx context.Context, tx *sql.Tx) error {
		if e := tx.QueryRowContext(ctx, "SELECT count(*) FROM stale").Scan(&result.Stale); e != nil {
			return e
		}
		if e := tx.QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE kind='bridge_invalidate' AND state='pending'").Scan(&result.ProjectionsQueued); e != nil {
			return e
		}
		for _, r := range dependent {
			if keep[r.Record] {
				if e := m.svc.ClearStale(ctx, tx, r.Record); e != nil {
					return e
				}
			}
		}
		return nil
	})
	return result, e
}
func (m *Module) RestoreSession(ctx context.Context, session string) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var key string
	var candidates []memory.Revision
	var suppressions []memory.Revision
	var keys []string
	e := m.svc.Read(ctx, func(q memory.Queryer) error {
		if e := q.QueryRowContext(ctx, "SELECT session_key FROM sessions WHERE session=?", session).Scan(&key); e != nil {
			return memory.ErrInvalid
		}
		related, err := relatedKeys(ctx, q, session)
		if err != nil {
			return err
		}
		keys = related
		rows, e := q.QueryContext(ctx, relatedCTE+" SELECT DISTINCT r.record,r.scope FROM revisions r JOIN related s ON s.session=r.record WHERE r.kind='session_manifest'", session)
		if e != nil {
			return e
		}
		type ref struct{ record, scope string }
		var refs []ref
		for rows.Next() {
			var r ref
			if rows.Scan(&r.record, &r.scope) != nil {
				rows.Close()
				return memory.ErrStorage
			}
			refs = append(refs, r)
		}
		rows.Close()
		for _, r := range refs {
			_, deleted, e := m.svc.Heads(ctx, q, r.scope, r.record)
			if e != nil {
				return e
			}
			if !deleted {
				continue
			}
			var previous memory.Revision
			rr, e := q.QueryContext(ctx, "SELECT body FROM revisions WHERE record=? AND json_extract(body,'$.operation')!='delete' ORDER BY rowid DESC LIMIT 1", r.record)
			if e != nil {
				return e
			}
			if rr.Next() {
				var b []byte
				if rr.Scan(&b) != nil || json.Unmarshal(b, &previous) != nil {
					rr.Close()
					return memory.ErrStorage
				}
			}
			rr.Close()
			if previous.Record == "" {
				return memory.ErrInvalid
			}
			previous.Restores, e = m.svc.UnrestoredDeletions(ctx, q, r.record)
			if e != nil {
				return e
			}
			candidates = append(candidates, previous)
		}
		keySet := map[string]bool{}
		for _, k := range keys {
			keySet[k] = true
		}
		rows, e = q.QueryContext(ctx, "SELECT DISTINCT record,scope FROM revisions WHERE kind='suppression' LIMIT 100001")
		if e != nil {
			return e
		}
		refs = nil
		for rows.Next() {
			var ref ref
			if rows.Scan(&ref.record, &ref.scope) != nil {
				rows.Close()
				return memory.ErrStorage
			}
			refs = append(refs, ref)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(refs) > 100000 {
			return memory.ErrStaleOverflow
		}
		for _, ref := range refs {
			hh, del, e := m.svc.Heads(ctx, q, ref.scope, ref.record)
			if e != nil {
				return e
			}
			if del {
				continue
			}
			for _, h := range hh {
				var sup records.Suppression
				if json.Unmarshal(h.Body, &sup) != nil {
					return memory.ErrInvalid
				}
				if keySet[sup.SessionKey] {
					suppressions = append(suppressions, h)
					break
				}
			}
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	var set []memory.Prepared
	// Remove suppression first in the commit so manifest restore can reindex.
	for _, sup := range suppressions {
		p, e := m.svc.Prepare(ctx, memory.PutInput{Scope: sup.Scope, Kind: sup.Kind, Record: sup.Record, Operation: "delete", Provenance: memory.Provenance{SourceKind: "manual", SourceRef: "session-restore"}})
		if e != nil {
			return nil, e
		}
		set = append(set, p)
	}
	for _, r := range candidates {
		p, e := m.svc.Prepare(ctx, memory.PutInput{Scope: r.Scope, Kind: r.Kind, Record: r.Record, Operation: "restore", Restores: r.Restores, Body: r.Body, Provenance: memory.Provenance{SourceKind: "manual", SourceRef: "session-restore"}})
		if e != nil {
			return nil, e
		}
		set = append(set, p)
	}
	if len(set) == 0 {
		return nil, memory.ErrInvalid
	}
	if e = m.svc.Commit(ctx, set, nil); e != nil {
		return nil, e
	}
	return map[string]any{"session": session, "restored": true, "derived_outputs": "visibility_recomputed_from_current_dependencies"}, nil
}
