package memory

import (
	"context"
	"encoding/json"
)

// ScanRevisions pages immutable payloads inside the caller's transaction. A page
// holds at most sixteen bounded revisions; callbacks may update derived tables.
func ScanRevisions(ctx context.Context, q Queryer, kind string, fn func(Revision) error) error {
	var after int64
	for {
		rows, e := q.QueryContext(ctx, "SELECT rowid,body FROM revisions WHERE kind=? AND rowid>? ORDER BY rowid LIMIT 16", kind, after)
		if e != nil {
			return e
		}
		var page []Revision
		for rows.Next() {
			var raw []byte
			var r Revision
			if rows.Scan(&after, &raw) != nil || json.Unmarshal(raw, &r) != nil {
				rows.Close()
				return ErrStorage
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
			if e = fn(r); e != nil {
				return e
			}
		}
	}
}
