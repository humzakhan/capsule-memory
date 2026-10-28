package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

type applier struct {
	m    *Module
	kind string
}

func (a *applier) Apply(ctx context.Context, tx *sql.Tx, r memory.Revision, _ memory.ApplyOrigin) error {
	switch r.Kind {
	case records.KindProject:
		if r.Operation == "delete" {
			_, e := tx.ExecContext(ctx, "DELETE FROM projects WHERE record=?", r.Record)
			return e
		}
		var p records.Project
		if json.Unmarshal(r.Body, &p) != nil || p.ProjectID != r.Record {
			return memory.ErrInvalid
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO projects(project_id,scope,record) VALUES(?,?,?) ON CONFLICT(project_id) DO UPDATE SET scope=excluded.scope,record=excluded.record", p.ProjectID, r.Scope, r.Record)
		return e
	case records.KindSuppression:
		return a.applySuppression(ctx, tx, r)
	case records.KindSessionManifest:
		if e := a.indexManifest(ctx, tx, r); e != nil {
			return e
		}
		return a.reconcileManifests(ctx, tx, r.Record)
	case records.KindSessionChunk:
		if r.Operation == "delete" {
			_, e := tx.ExecContext(ctx, "DELETE FROM session_search WHERE record=?", r.Record)
			return e
		}
		var c records.SessionChunk
		if json.Unmarshal(r.Body, &c) != nil {
			return memory.ErrInvalid
		}
		b, _ := json.Marshal(c.Events)
		if hash(b) != c.Digest {
			return memory.ErrInvalid
		}
		if _, e := tx.ExecContext(ctx, "INSERT INTO chunks(record,session,branch,seq,revision,events,bytes) VALUES(?,?,?,?,?,?,?) ON CONFLICT(record,revision) DO NOTHING", r.Record, c.Session, c.Branch, c.Seq, r.ID, len(c.Events), len(b)); e != nil {
			return e
		}
		for ordinal, ev := range c.Events {
			if _, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO event_lookup VALUES(?,?,?,?,?,?)", r.Scope, c.Session, ev.NativeID, ev.ID, r.ID, ordinal); e != nil {
				return e
			}
			if _, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO seen_events(session,event_id,chunk) VALUES(?,?,?)", c.Session, ev.ID, r.Record); e != nil {
				return e
			}
		}
		if _, e := tx.ExecContext(ctx, "UPDATE sessions SET last_event_time=? WHERE session=?", r.Created, c.Session); e != nil {
			return e
		}
		if e := a.m.applyRetractions(ctx, tx, r); e != nil {
			return e
		}
		return a.indexChunk(ctx, tx, r, c)
	}
	return nil
}

func (a *applier) indexChunk(ctx context.Context, tx *sql.Tx, r memory.Revision, c records.SessionChunk) error {
	if _, e := tx.ExecContext(ctx, "DELETE FROM session_search WHERE revision=?", r.ID); e != nil {
		return e
	}
	var deleted int
	var scope string
	if e := tx.QueryRowContext(ctx, "SELECT deleted,scope FROM sessions WHERE session=?", c.Session).Scan(&deleted, &scope); errors.Is(e, sql.ErrNoRows) {
		return nil
	} else if e != nil {
		return e
	}
	if deleted != 0 || scope != r.Scope {
		return nil
	}
	visible, e := memory.IsVisible(ctx, tx, c.Session)
	if e != nil || !visible {
		return e
	}
	var text strings.Builder
	for _, ev := range c.Events {
		if ev.ViewOf != "" {
			continue
		}
		text.WriteString(ev.Role)
		text.WriteByte(' ')
		text.WriteString(ev.ToolName)
		text.WriteByte(' ')
		text.WriteString(ev.Text)
		text.WriteByte('\n')
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO session_search(scope,record,revision,kind,session,text) VALUES(?,?,?,?,?,?)", r.Scope, r.Record, r.ID, r.Kind, c.Session, text.String())
	return e
}
func (a *applier) reindexSession(ctx context.Context, tx *sql.Tx, session string) error {
	var after int64
	for {
		rows, e := tx.QueryContext(ctx, "SELECT r.rowid,r.body FROM chunks c JOIN revisions r ON r.revision=c.revision WHERE c.session=? AND r.rowid>? ORDER BY r.rowid LIMIT 16", session, after)
		if e != nil {
			return e
		}
		var page []memory.Revision
		for rows.Next() {
			var raw []byte
			var r memory.Revision
			if rows.Scan(&after, &raw) != nil || json.Unmarshal(raw, &r) != nil {
				rows.Close()
				return memory.ErrStorage
			}
			page = append(page, r)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(page) == 0 {
			return nil
		}
		for _, r := range page {
			var chunk records.SessionChunk
			if json.Unmarshal(r.Body, &chunk) != nil {
				return memory.ErrStorage
			}
			if e = a.indexChunk(ctx, tx, r, chunk); e != nil {
				return e
			}
		}
	}
}
func (a *applier) Rebuild(ctx context.Context, tx *sql.Tx) error {
	if a.kind == records.KindSessionManifest {
		if e := memory.ScanRevisions(ctx, tx, a.kind, func(r memory.Revision) error { return a.indexManifest(ctx, tx, r) }); e != nil {
			return e
		}
		return a.rebuildManifestGroups(ctx, tx)
	}
	if a.kind == records.KindSuppression {
		return memory.ScanRevisions(ctx, tx, a.kind, func(r memory.Revision) error { return a.applySuppression(ctx, tx, r) })
	}
	return memory.ScanRevisions(ctx, tx, a.kind, func(r memory.Revision) error { return a.Apply(ctx, tx, r, memory.ApplyReplay) })
}
