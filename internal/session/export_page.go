package session

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/humzakhan/capsule-memory/internal/capture/importv1"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

type ExportPage struct {
	JSONL     string `json:"jsonl"`
	Next      int64  `json:"next"`
	Done      bool   `json:"done"`
	Snapshot  string `json:"snapshot"`
	Untrusted bool   `json:"untrusted"`
}

func (m *Module) ExportPage(ctx context.Context, session string, from int64, maxBytes int) (ExportPage, error) {
	return m.ExportSnapshotPage(ctx, session, from, maxBytes, "")
}

// The token pins an immutable manifest revision and local insertion high-water
// mark. New branches/chunks cannot shift subsequent OFFSETs. Each request still
// checks live deletion/suppression before disclosing any pinned content.
func (m *Module) ExportSnapshotPage(ctx context.Context, session string, from int64, maxBytes int, snapshot string) (ExportPage, error) {
	out := ExportPage{Next: from, Done: true, Untrusted: true, Snapshot: snapshot}
	if from < 0 || len(snapshot) > 128 || from > 0 && snapshot == "" {
		return out, memory.ErrInvalid
	}
	if maxBytes < 1 || maxBytes > 1<<20 {
		maxBytes = 256 << 10
	}
	var buf bytes.Buffer
	e := m.svc.Read(ctx, func(q memory.Queryer) error {
		var scope, project, cwd string
		if q.QueryRowContext(ctx, "SELECT scope,project_id FROM sessions WHERE session=? AND deleted=0", session).Scan(&scope, &project) != nil {
			return memory.ErrInvalid
		}
		visible, e := memory.IsVisible(ctx, q, session)
		if e != nil {
			return e
		}
		if !visible {
			return memory.ErrInvalid
		}
		var rev memory.Revision
		var manifest records.SessionManifest
		var high int64
		if snapshot == "" {
			rev, manifest, e = m.latestManifest(ctx, q, scope, session)
			if e != nil {
				return e
			}
			if e = q.QueryRowContext(ctx, "SELECT COALESCE(max(rowid),0) FROM revisions").Scan(&high); e != nil {
				return e
			}
			out.Snapshot = rev.ID + ":" + strconv.FormatInt(high, 10)
		} else {
			id, value, ok := strings.Cut(snapshot, ":")
			if !ok {
				return memory.ErrInvalid
			}
			high, e = strconv.ParseInt(value, 10, 64)
			if e != nil || high < 0 {
				return memory.ErrInvalid
			}
			rev, e = m.svc.Revision(ctx, q, id)
			if e != nil || rev.Scope != scope || rev.Kind != records.KindSessionManifest || json.Unmarshal(rev.Body, &manifest) != nil {
				return memory.ErrInvalid
			}
			var inserted int64
			if q.QueryRowContext(ctx, "SELECT rowid FROM revisions WHERE revision=?", id).Scan(&inserted) != nil || inserted > high {
				return memory.ErrInvalid
			}
			var maximum int64
			if q.QueryRowContext(ctx, "SELECT COALESCE(max(rowid),0) FROM revisions").Scan(&maximum) != nil || high > maximum {
				return memory.ErrInvalid
			}
			var key string
			if q.QueryRowContext(ctx, "SELECT session_key FROM sessions WHERE session=?", session).Scan(&key) != nil || key != manifest.SessionKey {
				return memory.ErrInvalid
			}
		}
		emit := func(line importv1.Line) (bool, error) {
			b, e := json.Marshal(line)
			if e != nil {
				return false, e
			}
			if buf.Len() > 0 && buf.Len()+len(b)+1 > maxBytes {
				out.Done = false
				return false, nil
			}
			if len(b) > 1<<20 {
				return false, memory.ErrInvalid
			}
			buf.Write(b)
			buf.WriteByte('\n')
			out.Next++
			return true, nil
		}
		if from == 0 {
			_ = q.QueryRowContext(ctx, "SELECT path FROM project_paths WHERE project_id=? ORDER BY path LIMIT 1", project).Scan(&cwd)
			ok, e := emit(importv1.Line{Version: 1, Kind: "session", Harness: manifest.Origin.Harness, Store: manifest.Origin.SourceStore, SessionID: manifest.Origin.NativeSessionID, Environment: manifest.Origin.Environment, CWD: cwd, Title: manifest.Title, HarnessVersion: manifest.HarnessVersion})
			if e != nil || !ok {
				return e
			}
		}
		offset := from - 1
		if offset < 0 {
			offset = 0
		}
		rows, e := q.QueryContext(ctx, `SELECT ev.value,c.seq FROM chunks c JOIN revisions r ON r.revision=c.revision JOIN json_each(r.body,'$.body.events') ev WHERE c.session=? AND r.rowid<=? ORDER BY c.branch,c.seq,c.record,c.revision,CAST(ev.key AS INTEGER) LIMIT 4097 OFFSET ?`, session, high, offset)
		if e != nil {
			return e
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			if count == 4096 {
				out.Done = false
				break
			}
			var raw []byte
			var seq int64
			var ev records.Event
			if rows.Scan(&raw, &seq) != nil || json.Unmarshal(raw, &ev) != nil {
				return memory.ErrStorage
			}
			ok, e := emit(importv1.Line{Version: 1, Kind: "event", SessionID: manifest.Origin.NativeSessionID, Seq: seq, EventID: ev.ID, Role: ev.Role, Type: ev.Type, Text: ev.Text, Status: "final", Time: ev.Time, Event: &ev})
			if e != nil || !ok {
				return e
			}
			count++
		}
		return rows.Err()
	})
	out.JSONL = buf.String()
	return out, e
}
