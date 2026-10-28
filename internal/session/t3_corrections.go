package session

import (
	"context"
	"sort"
	"strings"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

// A rolled-back run may have vanished from the upstream visible rows. Match its
// stable run id against already archived events, never against message text.
func expandT3Corrections(ctx context.Context, q memory.Queryer, source capture.Source, input []capture.Event) ([]capture.Event, error) {
	if source.Harness != "t3" {
		return input, nil
	}
	var out []capture.Event
	for _, ce := range input {
		if ce.Event.Type != "correction" || ce.Event.Turn == "" {
			out = append(out, ce)
			continue
		}
		ids := map[string]bool{}
		scoped := map[string]map[string]bool{}
		for _, id := range ce.Event.Retracts {
			ids[id] = true
		}
		rows, e := q.QueryContext(ctx, `SELECT DISTINCT json_extract(ev.value,'$.id'),s.scope FROM chunks c JOIN sessions s ON s.session=c.session JOIN revisions r ON r.revision=c.revision JOIN json_each(r.body,'$.body.events') ev WHERE s.session_key=? AND json_extract(ev.value,'$.turn')=? AND json_extract(ev.value,'$.type')!='correction' LIMIT 100001`, SessionKey(ce.Origin), ce.Event.Turn)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var id, scope string
			if rows.Scan(&id, &scope) != nil {
				rows.Close()
				return nil, memory.ErrStorage
			}
			ids[id] = true
			if scoped[scope] == nil {
				scoped[scope] = map[string]bool{}
			}
			scoped[scope][id] = true
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		if len(ids) > 100000 {
			return nil, memory.ErrStaleOverflow
		}
		// Preserve a current-scope correction for not-yet-arrived source events,
		// plus exact original-scope copies for archived cross-project events.
		scoped[""] = ids
		for scope, targets := range scoped {
			ordered := make([]string, 0, len(targets))
			for id := range targets {
				ordered = append(ordered, id)
			}
			sort.Strings(ordered)
			for start := 0; start < len(ordered); start += 512 {
				end := start + 512
				if end > len(ordered) {
					end = len(ordered)
				}
				ev := records.Event{ID: "rollback:" + hash([]byte(ce.Event.Turn+"\n"+scope+"\n"+strings.Join(ordered[start:end], "\n"))), Role: "system", Type: "correction", Turn: ce.Event.Turn, Retracts: ordered[start:end]}
				copy := ce
				copy.Event = ev
				copy.CorrectionScope = scope
				copy.PositionKey = ev.ID
				out = append(out, copy)
			}
		}
	}
	if len(out) > 8192 {
		return nil, memory.ErrInvalid
	}
	return out, nil
}
