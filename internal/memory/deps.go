package memory

import (
	"context"
	"database/sql"
	"errors"

	"github.com/humzakhan/capsule-memory/internal/records"
)

var ErrStaleOverflow = errors.New("invalidation closure exceeds 100000 rows; operation rolled back")

type StaleRef struct{ Record, Kind, Scope, Reason string }
type StaleHook func(context.Context, *sql.Tx, []StaleRef) error

func (s *Service) OnStale(h StaleHook) { s.staleHooks = append(s.staleHooks, h) }
func (s *Service) SetDeps(ctx context.Context, tx *sql.Tx, child Revision, parents []records.SourceRef) error {
	if e := s.RequireV2(); e != nil {
		return e
	}
	if len(parents) > 32768 {
		return ErrInvalid
	}
	if _, e := tx.ExecContext(ctx, "DELETE FROM deps WHERE child_revision=?", child.ID); e != nil {
		return ErrStorage
	}
	for _, p := range parents {
		if !idPattern.MatchString(p.Record) || !idPattern.MatchString(p.Revision) {
			return ErrInvalid
		}
		if _, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO deps VALUES(?,?,?,?,?,?,1)", child.Record, child.ID, p.Record, p.Revision, p.EventFrom, p.EventTo); e != nil {
			return ErrStorage
		}
	}
	return nil
}

const liveDeps = `SELECT d.child,d.child_revision,d.parent,d.parent_revision,d.event_from,d.event_to,d.evidence FROM deps d WHERE NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=d.child_revision)`
const deletedSQL = `EXISTS(SELECT 1 FROM revisions d WHERE d.record=r.record AND json_extract(d.body,'$.operation')='delete' AND NOT EXISTS(SELECT 1 FROM restores x WHERE x.deletion=d.revision))`

func (s *Service) MarkStale(ctx context.Context, tx *sql.Tx, roots []string, reason string) ([]StaleRef, error) {
	if e := s.RequireV2(); e != nil {
		return nil, e
	}
	if len(roots) > 100000 {
		return nil, ErrStaleOverflow
	}
	// A temporary seed table avoids SQLite variable limits for large deletion sets.
	if _, e := tx.ExecContext(ctx, "CREATE TEMP TABLE IF NOT EXISTS stale_seed(record TEXT PRIMARY KEY); DELETE FROM stale_seed"); e != nil {
		return nil, ErrStorage
	}
	for _, r := range roots {
		if !idPattern.MatchString(r) {
			return nil, ErrInvalid
		}
		if _, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO stale_seed VALUES(?)", r); e != nil {
			return nil, ErrStorage
		}
	}
	rows, e := tx.QueryContext(ctx, `WITH RECURSIVE live AS (`+liveDeps+`), closure(record) AS (SELECT record FROM stale_seed UNION SELECT d.child FROM live d JOIN closure c ON d.parent=c.record) SELECT record FROM closure LIMIT 100001`)
	if e != nil {
		return nil, ErrStorage
	}
	all := []string{}
	for rows.Next() {
		var r string
		if rows.Scan(&r) != nil {
			rows.Close()
			return nil, ErrStorage
		}
		all = append(all, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, ErrStorage
	}
	if len(all) > 100000 {
		return nil, ErrStaleOverflow
	}
	out := []StaleRef{}
	for _, id := range all {
		marker := reason
		result, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO stale(record,reason,since) VALUES(?,?,COALESCE((SELECT MAX(rowid) FROM revisions),0))", id, marker)
		if e != nil {
			return nil, ErrStorage
		}
		n, e := result.RowsAffected()
		if e != nil {
			return nil, ErrStorage
		}
		if n == 0 {
			continue
		}
		var r StaleRef
		r.Record = id
		r.Reason = marker
		e = tx.QueryRowContext(ctx, "SELECT kind,scope FROM revisions WHERE record=? LIMIT 1", id).Scan(&r.Kind, &r.Scope)
		if e == sql.ErrNoRows {
			continue
		}
		if e != nil {
			return nil, ErrStorage
		}
		out = append(out, r)
	}
	for _, h := range s.staleHooks {
		if e = h(ctx, tx, out); e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (s *Service) ClearStale(ctx context.Context, tx *sql.Tx, record string) error {
	_, e := tx.ExecContext(ctx, "DELETE FROM stale WHERE record=?", record)
	if e != nil {
		return ErrStorage
	}
	return nil
}
func (s *Service) markSuperseded(ctx context.Context, tx *sql.Tx, r Revision) error {
	if r.Operation == "delete" {
		_, e := s.MarkStale(ctx, tx, []string{r.Record}, "deleted")
		return e
	}
	// Seed only dependents whose exact parent revision ceased being a head.
	rows, e := tx.QueryContext(ctx, `SELECT DISTINCT d.child FROM (`+liveDeps+`) d WHERE d.parent=? AND EXISTS(SELECT 1 FROM parents p WHERE p.parent=d.parent_revision)`, r.Record)
	if e != nil {
		return ErrStorage
	}
	roots := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return ErrStorage
		}
		roots = append(roots, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return ErrStorage
	}
	if len(roots) == 0 {
		return nil
	}
	_, e = s.MarkStale(ctx, tx, roots, "superseded")
	return e
}
func (s *Service) RecomputeStale(ctx context.Context, tx *sql.Tx) error {
	// Every root is reconstructed from immutable evidence and current heads.
	if _, e := tx.ExecContext(ctx, "DELETE FROM stale"); e != nil {
		return ErrStorage
	}
	rows, e := tx.QueryContext(ctx, `SELECT DISTINCT r.record FROM revisions r WHERE `+deletedSQL+` UNION SELECT DISTINCT r.record FROM revisions r WHERE r.kind='session_chunk' AND NOT EXISTS(SELECT 1 FROM sessions s WHERE s.session=json_extract(r.body,'$.body.session') AND s.scope=r.scope AND s.deleted=0) UNION SELECT DISTINCT d.child FROM (`+liveDeps+`) d WHERE NOT EXISTS(SELECT 1 FROM revisions r WHERE r.revision=d.parent_revision AND r.record=d.parent) OR EXISTS(SELECT 1 FROM parents p WHERE p.parent=d.parent_revision) OR (SELECT count(*) FROM revisions h WHERE h.record=d.parent AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=h.revision))!=1 OR `+secondaryViewSQL+` OR `+invalidRangeSQL+` UNION SELECT DISTINCT d.child FROM (`+liveDeps+`) d WHERE `+retractedDepSQL+` LIMIT 100001`)
	if e != nil {
		return ErrStorage
	}
	roots := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return ErrStorage
		}
		roots = append(roots, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return ErrStorage
	}
	if len(roots) > 100000 {
		return ErrStaleOverflow
	}
	if len(roots) == 0 {
		return nil
	}
	_, e = s.MarkStale(ctx, tx, roots, "source_invalid")
	return e
}

// Drive invalidation from the affected records, rather than scanning the archive
// and probing the temporary seed table for every historical revision.
const refreshInvalidSQL = `WITH RECURSIVE live AS (` + liveDeps + `), roots(record) AS (
 SELECT r.record FROM refresh_affected a CROSS JOIN revisions r ON r.record=a.record WHERE ` + deletedSQL + `
 UNION SELECT r.record FROM refresh_affected a CROSS JOIN revisions r ON r.record=a.record WHERE r.kind='session_chunk' AND NOT EXISTS(SELECT 1 FROM sessions s WHERE s.session=json_extract(r.body,'$.body.session') AND s.scope=r.scope AND s.deleted=0)
 UNION SELECT d.child FROM live d JOIN refresh_affected a ON a.record=d.child WHERE NOT EXISTS(SELECT 1 FROM revisions r WHERE r.revision=d.parent_revision AND r.record=d.parent) OR EXISTS(SELECT 1 FROM parents p WHERE p.parent=d.parent_revision) OR (SELECT count(*) FROM revisions h WHERE h.record=d.parent AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=h.revision))!=1 OR ` + secondaryViewSQL + ` OR ` + invalidRangeSQL + ` OR ` + retractedDepSQL + ` OR EXISTS(SELECT 1 FROM stale s WHERE s.record=d.parent AND NOT EXISTS(SELECT 1 FROM refresh_affected x WHERE x.record=s.record))
 ), invalid(record) AS (SELECT record FROM roots UNION SELECT d.child FROM live d JOIN invalid i ON d.parent=i.record JOIN refresh_affected a ON a.record=d.child) INSERT INTO refresh_invalid SELECT record FROM invalid`

// RefreshStale recomputes only a changed record and its live descendants. The
// bound is on this operation's affected closure, never total archive history.
// Unchanged stale rows remain in place, so hooks observe transitions only.
func (s *Service) RefreshStale(ctx context.Context, tx *sql.Tx, seeds []string) error {
	if len(seeds) == 0 {
		return nil
	}
	if len(seeds) > 100000 {
		return ErrStaleOverflow
	}
	if _, e := tx.ExecContext(ctx, "CREATE TEMP TABLE IF NOT EXISTS refresh_seed(record TEXT PRIMARY KEY); DELETE FROM refresh_seed; CREATE TEMP TABLE IF NOT EXISTS refresh_affected(record TEXT PRIMARY KEY); DELETE FROM refresh_affected; CREATE TEMP TABLE IF NOT EXISTS refresh_invalid(record TEXT PRIMARY KEY); DELETE FROM refresh_invalid"); e != nil {
		return ErrStorage
	}
	for _, id := range seeds {
		if _, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO refresh_seed VALUES(?)", id); e != nil {
			return ErrStorage
		}
	}
	if _, e := tx.ExecContext(ctx, `WITH RECURSIVE live AS (`+liveDeps+`), affected(record) AS (SELECT record FROM refresh_seed UNION SELECT d.child FROM live d JOIN affected a ON d.parent=a.record) INSERT INTO refresh_affected SELECT record FROM affected LIMIT 100001`); e != nil {
		return ErrStorage
	}
	var count int
	if tx.QueryRowContext(ctx, "SELECT count(*) FROM refresh_affected").Scan(&count) != nil {
		return ErrStorage
	}
	if count > 100000 {
		return ErrStaleOverflow
	}
	_, e := tx.ExecContext(ctx, refreshInvalidSQL)
	if e != nil {
		return ErrStorage
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM stale WHERE record IN (SELECT record FROM refresh_affected) AND record NOT IN (SELECT record FROM refresh_invalid)"); e != nil {
		return ErrStorage
	}
	rows, e := tx.QueryContext(ctx, "SELECT record FROM refresh_invalid WHERE record NOT IN (SELECT record FROM stale)")
	if e != nil {
		return ErrStorage
	}
	var roots []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return ErrStorage
		}
		roots = append(roots, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return ErrStorage
	}
	if len(roots) == 0 {
		return nil
	}
	_, e = s.MarkStale(ctx, tx, roots, "source_invalid")
	return e
}
func IsVisible(ctx context.Context, q Queryer, record string) (bool, error) {
	// Dynamic dependency inspection also fails closed for out-of-order sources.
	rows, e := q.QueryContext(ctx, `WITH RECURSIVE live AS (`+liveDeps+`), closure(record) AS (SELECT ? UNION SELECT d.parent FROM live d JOIN closure c ON d.child=c.record) SELECT record FROM closure LIMIT 100001`, record)
	if e != nil {
		return false, ErrStorage
	}
	all := []string{}
	for rows.Next() {
		var r string
		if rows.Scan(&r) != nil {
			rows.Close()
			return false, ErrStorage
		}
		all = append(all, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return false, ErrStorage
	}
	if len(all) > 100000 {
		return false, ErrStaleOverflow
	}
	for _, id := range all {
		var count int
		e = q.QueryRowContext(ctx, `SELECT count(*) FROM revisions r WHERE r.record=? AND r.kind!='knowledge' AND NOT `+deletedSQL+` AND NOT EXISTS(SELECT 1 FROM stale s WHERE s.record=r.record)`, id).Scan(&count)
		if e != nil {
			return false, ErrStorage
		}
		if count == 0 {
			return false, nil
		}
		e = q.QueryRowContext(ctx, `SELECT count(*) FROM revisions r WHERE r.record=? AND r.kind='session_chunk' AND NOT EXISTS(SELECT 1 FROM sessions s WHERE s.session=json_extract(r.body,'$.body.session') AND s.deleted=0 AND s.scope=r.scope)`, id).Scan(&count)
		if e != nil {
			return false, ErrStorage
		}
		if count > 0 {
			return false, nil
		}
		e = q.QueryRowContext(ctx, `SELECT count(*) FROM (`+liveDeps+`) d WHERE d.child=? AND (NOT EXISTS(SELECT 1 FROM revisions r WHERE r.revision=d.parent_revision AND r.record=d.parent) OR EXISTS(SELECT 1 FROM parents p WHERE p.parent=d.parent_revision) OR (SELECT count(*) FROM revisions h WHERE h.record=d.parent AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=h.revision))!=1 OR `+secondaryViewSQL+` OR `+invalidRangeSQL+` OR `+retractedDepSQL+`)`, id).Scan(&count)
		if e != nil {
			return false, ErrStorage
		}
		if count > 0 {
			return false, nil
		}
	}
	return true, nil
}

// Coverage heads still participate in supersession, but never assert that every
// event in a chunk was cited. Evidence rows preserve the exact event interval.
func (s *Service) SetCoverageDeps(ctx context.Context, tx *sql.Tx, child Revision, refs []records.SourceRef) error {
	for _, p := range refs {
		if !idPattern.MatchString(p.Record) || !idPattern.MatchString(p.Revision) {
			return ErrInvalid
		}
		if _, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO deps VALUES(?,?,?,?,?,?,0)", child.Record, child.ID, p.Record, p.Revision, "", ""); e != nil {
			return ErrStorage
		}
	}
	return nil
}

// Event ids are opaque. Intervals follow source array order, never lexical id order.
// Part groups allow an adapter correction of a native event to retract all its parts.
const retractedDepSQL = `d.evidence=1 AND EXISTS(
 SELECT 1 FROM revisions src JOIN json_each(src.body,'$.body.events') ev
 JOIN retracted_events x ON (x.event_id=json_extract(ev.value,'$.id') OR x.event_id=json_extract(ev.value,'$.part.group'))
 WHERE src.revision=d.parent_revision AND src.kind='session_chunk' AND x.scope=src.scope
 AND (x.session=json_extract(src.body,'$.body.session') OR EXISTS(
 SELECT 1 FROM sessions a JOIN sessions b ON a.session_key=b.session_key AND a.scope=b.scope
 WHERE a.session=x.session AND b.session=json_extract(src.body,'$.body.session')))
 AND (d.event_from='' OR (CAST(ev.key AS INTEGER) BETWEEN
 (SELECT CAST(f.key AS INTEGER) FROM json_each(src.body,'$.body.events') f WHERE json_extract(f.value,'$.id')=d.event_from)
 AND (SELECT CAST(t.key AS INTEGER) FROM json_each(src.body,'$.body.events') t WHERE json_extract(t.value,'$.id')=d.event_to))))`

func EventRetracted(ctx context.Context, q Queryer, session, event, group string) (bool, error) {
	var n int
	e := q.QueryRowContext(ctx, `SELECT count(*) FROM retracted_events x WHERE x.scope=(SELECT scope FROM sessions WHERE session=?) AND (x.event_id=? OR (?!='' AND x.event_id=?)) AND (x.session=? OR EXISTS(SELECT 1 FROM sessions a JOIN sessions b ON a.session_key=b.session_key AND a.scope=b.scope WHERE a.session=x.session AND b.session=?))`, session, event, group, group, session, session).Scan(&n)
	if e != nil {
		return false, ErrStorage
	}
	return n > 0, nil
}

const invalidRangeSQL = `d.event_from!='' AND NOT EXISTS(SELECT 1 FROM revisions src JOIN json_each(src.body,'$.body.events') f JOIN json_each(src.body,'$.body.events') t WHERE src.revision=d.parent_revision AND src.kind='session_chunk' AND json_extract(f.value,'$.id')=d.event_from AND json_extract(t.value,'$.id')=d.event_to AND CAST(f.key AS INTEGER)<=CAST(t.key AS INTEGER))`

// A strong native second view remains raw provenance, but cannot independently
// influence a derivation once its T3 canonical view is known.
const secondaryViewSQL = `d.evidence=1 AND EXISTS(SELECT 1 FROM revisions src JOIN sessions s ON s.session=json_extract(src.body,'$.body.session') JOIN sessions canonical ON canonical.session=s.view_of WHERE src.revision=d.parent_revision AND src.kind='session_chunk' AND s.harness!='t3' AND canonical.harness='t3')`

func (s *Service) RefreshRetractions(ctx context.Context, tx *sql.Tx, correction string) error {
	rows, e := tx.QueryContext(ctx, `SELECT DISTINCT d.child FROM retracted_events x JOIN chunks c ON (c.session=x.session OR c.session IN (SELECT b.session FROM sessions a JOIN sessions b ON a.session_key=b.session_key AND a.scope=b.scope WHERE a.session=x.session)) JOIN revisions src ON src.revision=c.revision JOIN (`+liveDeps+`) d ON d.parent_revision=src.revision WHERE x.correction_revision=? AND `+retractedDepSQL+` LIMIT 100001`, correction)
	if e != nil {
		return ErrStorage
	}
	var seeds []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return ErrStorage
		}
		seeds = append(seeds, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return ErrStorage
	}
	return s.RefreshStale(ctx, tx, seeds)
}
