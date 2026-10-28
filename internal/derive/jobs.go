package derive

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/projection"
	"github.com/humzakhan/capsule-memory/internal/records"
)

func (r *Runner) Assign(ctx context.Context, key, device string) error {
	var j records.Job
	var scope string
	err := r.svc.Read(ctx, func(q memory.Queryer) error {
		var raw []byte
		if e := q.QueryRowContext(ctx, "SELECT kind,scope,subject,inputs,destination FROM jobs WHERE job_key=?", key).Scan(&j.Kind, &scope, &j.Subject, &raw, &j.Destination); e != nil {
			return e
		}
		if json.Unmarshal(raw, &j.Inputs) != nil {
			return memory.ErrInvalid
		}
		j.JobKey = key
		j.AssignedDevice = device
		j.State = "requested"
		return nil
	})
	if err != nil {
		return err
	}
	p, err := r.svc.Prepare(ctx, memory.PutInput{Scope: scope, Kind: records.KindJob, Body: j, Provenance: memory.Provenance{SourceKind: "manual", SourceRef: key}})
	if err != nil {
		return err
	}
	return r.svc.Commit(ctx, []memory.Prepared{p}, func(ctx context.Context, tx *sql.Tx) error {
		if e := authorizeJob(ctx, tx, r.svc, p.Revision, j); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, "UPDATE jobs SET assigned_device=?,state=CASE WHEN state='leased' THEN 'pending' ELSE state END,attempt_token=NULL,lease_until=NULL WHERE job_key=?", device, key)
		return e
	})
}
func (r *Runner) Schedule(ctx context.Context, now time.Time) error {
	policy, err := r.sess.Policy(ctx)
	if err != nil {
		return err
	}
	if !policy.Derive || r.inf == nil {
		return nil
	}
	info, err := r.svc.Info(ctx)
	if err != nil {
		return err
	}
	if policy.RollupDevice != "" && policy.RollupDevice != info.Device {
		return nil
	}
	utc := now.UTC()
	today := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := today
	if utc.Before(today.Add(time.Hour)) {
		dayEnd = today.AddDate(0, 0, -1)
	}
	return r.svc.Update(ctx, func(tx *sql.Tx) error {
		if e := r.scheduleSessions(ctx, tx, info.Device, policy.Version, now); e != nil {
			return e
		}
		// Daily rollups are versioned by their input revision set; scheduling is idempotent.
		for _, scope := range info.Scopes {
			if scope.Role != "project" {
				continue
			}
			all, e := r.summaryInputs(ctx, tx, []string{scope.ID}, "session", time.Time{}, dayEnd)
			if e != nil {
				return e
			}
			groups, periods, e := r.groupPeriods(ctx, tx, all, false)
			if e != nil {
				return e
			}
			for _, day := range periods {
				inputs := groups[day]
				var subject string
				if e = tx.QueryRowContext(ctx, "SELECT project_id FROM projects WHERE scope=? LIMIT 1", scope.ID).Scan(&subject); e != nil {
					return e
				}
				_, e = r.svc.EnqueueJob(ctx, tx, memory.JobSpec{Kind: "project_rollup", Scope: scope.ID, Subject: subject, Destination: scope.ID, Inputs: inputs, Versions: r.limits.Model + "|" + RollupVersion + "|" + strconv.Itoa(policy.Version) + "|" + day.Format("2006-01-02"), NotBefore: day.AddDate(0, 0, 1).Add(time.Hour), Priority: 3, AssignedDevice: info.Device})
				if e != nil {
					return e
				}
			}
		}
		monday := today.AddDate(0, 0, -(int(today.Weekday())+6)%7)
		if utc.Before(monday.Add(2 * time.Hour)) {
			monday = monday.AddDate(0, 0, -7)
		}
		c, e := projection.SnapshotControl(ctx, tx, r.svc)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "DELETE FROM scheduler_blocked"); e != nil {
			return e
		}
		for _, id := range policy.Aggregations {
			grant, e := projection.Aggregation(ctx, tx, r.svc, id, c)
			if e != nil {
				if _, e = tx.ExecContext(ctx, "INSERT INTO scheduler_blocked VALUES(?,?)", id, "aggregation_unavailable_or_unauthorized"); e != nil {
					return e
				}
				continue
			}
			all, e := r.summaryInputs(ctx, tx, grant.Sources, "project_day", time.Time{}, monday)
			if e != nil {
				return e
			}
			groups, periods, e := r.groupPeriods(ctx, tx, all, true)
			if e != nil {
				return e
			}
			for _, week := range periods {
				inputs := groups[week]
				if e = projection.CheckAggregation(ctx, tx, r.svc, []string{id}, inputs, grant.Destination, c); e != nil {
					if _, e = tx.ExecContext(ctx, "INSERT OR REPLACE INTO scheduler_blocked VALUES(?,?)", id, "aggregation_inputs_unavailable_or_unauthorized"); e != nil {
						return e
					}
					continue
				}
				_, e = r.svc.EnqueueJob(ctx, tx, memory.JobSpec{Kind: "personal_rollup", Scope: grant.Destination, Subject: grant.Destination, Destination: grant.Destination, Inputs: inputs, Versions: r.limits.Model + "|" + RollupVersion + "|" + strconv.Itoa(policy.Version) + "|" + week.Format("2006-01-02"), NotBefore: week.AddDate(0, 0, 7).Add(2 * time.Hour), Priority: 3, AssignedDevice: info.Device})
				if e != nil {
					return e
				}
			}
		}
		return nil
	})
}
func (r *Runner) summaryInputs(ctx context.Context, q memory.Queryer, scopes []string, level string, start, end time.Time) ([]records.SourceRef, error) {
	out := []records.SourceRef{}
	for _, scope := range scopes {
		rows, err := q.QueryContext(ctx, "SELECT DISTINCT record FROM revisions WHERE scope=? AND kind=? ORDER BY record LIMIT 4097", scope, records.KindSummary)
		if err != nil {
			return nil, err
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if len(ids) > 4096 {
			return nil, memory.ErrInvalid
		}
		for _, id := range ids {
			visible, e := memory.IsVisible(ctx, q, id)
			if e != nil {
				return nil, e
			}
			if !visible {
				continue
			}
			hh, deleted, e := r.svc.Heads(ctx, q, scope, id)
			if e != nil {
				return nil, e
			}
			if deleted || len(hh) != 1 {
				continue
			}
			h := hh[0]
			var s records.Summary
			if json.Unmarshal(h.Body, &s) != nil {
				return nil, memory.ErrInvalid
			}
			if s.Level != level {
				continue
			}
			when, e := time.Parse(time.RFC3339Nano, h.Created)
			if s.PeriodStart != "" {
				when, e = time.Parse(time.RFC3339Nano, s.PeriodStart)
			}
			if e != nil || when.Before(start) || !when.Before(end) {
				continue
			}
			out = append(out, records.SourceRef{Record: h.Record, Revision: h.ID, Kind: h.Kind})
		}
	}
	if len(out) > 4096 {
		return nil, memory.ErrInvalid
	}
	return out, nil
}

// Enabling derivation also discovers archived sessions that predate the opt-in.
// This reads the local archive only; it does not authorize harness backfill.
func (r *Runner) scheduleSessions(ctx context.Context, tx *sql.Tx, device string, policyVersion int, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT s.rowid,session,scope,state,last_event_time FROM sessions s WHERE s.rowid>COALESCE((SELECT value FROM local_seq WHERE name='derive_schedule_cursor'),0) AND deleted=0 AND (view_of IS NULL OR view_of='')
 AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.subject=s.session AND j.kind='session_digest' AND j.state IN ('pending','leased','blocked'))
 AND NOT EXISTS(SELECT 1 FROM session_search x JOIN revisions v ON v.revision=x.revision WHERE x.session=s.session AND x.kind='summary' AND json_extract(v.body,'$.body.level')='session' AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=v.revision) AND NOT EXISTS(SELECT 1 FROM stale z WHERE z.record=x.record)) ORDER BY s.rowid LIMIT 64`)
	if err != nil {
		return err
	}
	type subject struct {
		seq              int64
		id, scope, state string
		last             sql.NullString
	}
	subjects := []subject{}
	for rows.Next() {
		var s subject
		if err = rows.Scan(&s.seq, &s.id, &s.scope, &s.state, &s.last); err != nil {
			rows.Close()
			return err
		}
		subjects = append(subjects, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(subjects) == 0 {
		_, err = tx.ExecContext(ctx, "INSERT INTO local_seq VALUES('derive_schedule_cursor',0) ON CONFLICT(name) DO UPDATE SET value=0")
		return err
	}
	for _, s := range subjects {
		if _, e := tx.ExecContext(ctx, "INSERT INTO local_seq VALUES('derive_schedule_cursor',?) ON CONFLICT(name) DO UPDATE SET value=excluded.value", s.seq); e != nil {
			return e
		}
		_, refs, e := r.sess.DigestInputs(ctx, tx, s.id)
		if e != nil {
			if e == memory.ErrInvalid || e == memory.ErrHeadsChanged {
				continue
			}
			return e
		}
		if len(refs) == 0 {
			continue
		}
		ready := now
		if s.state != "closed" {
			if when, e := time.Parse(time.RFC3339Nano, s.last.String); e == nil {
				ready = when.Add(r.cfg.Idle)
			} else {
				ready = now.Add(r.cfg.Idle)
			}
		}
		_, e = r.svc.EnqueueJob(ctx, tx, memory.JobSpec{Kind: "session_digest", Scope: s.scope, Subject: s.id, Destination: s.scope, Inputs: refs, Versions: r.limits.Model + "|" + PromptVersion + "|" + strconv.Itoa(policyVersion), Priority: 2, NotBefore: ready, AssignedDevice: device})
		if e != nil {
			return e
		}
	}
	return nil
}

func (r *Runner) groupPeriods(ctx context.Context, q memory.Queryer, inputs []records.SourceRef, weekly bool) (map[time.Time][]records.SourceRef, []time.Time, error) {
	groups := map[time.Time][]records.SourceRef{}
	for _, ref := range inputs {
		rev, err := r.svc.Revision(ctx, q, ref.Revision)
		if err != nil {
			return nil, nil, err
		}
		var summary records.Summary
		if json.Unmarshal(rev.Body, &summary) != nil {
			return nil, nil, memory.ErrInvalid
		}
		stamp := rev.Created
		if summary.PeriodStart != "" {
			stamp = summary.PeriodStart
		}
		when, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			return nil, nil, err
		}
		when = when.UTC()
		day := time.Date(when.Year(), when.Month(), when.Day(), 0, 0, 0, 0, time.UTC)
		if weekly {
			day = day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
		}
		groups[day] = append(groups[day], ref)
	}
	periods := []time.Time{}
	for period := range groups {
		periods = append(periods, period)
	}
	sort.Slice(periods, func(i, j int) bool { return periods[i].Before(periods[j]) })
	return groups, periods, nil
}
