package derive

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/projection"
	"github.com/humzakhan/capsule-memory/internal/records"
)

type applier struct {
	svc    *memory.Service
	kind   string
	device string
}

func Install(svc *memory.Service) {
	// Device identity is checked from revision-independent profile metadata by job appliers.
	for _, kind := range []string{records.KindSummary, records.KindInsight, records.KindJob} {
		svc.RegisterApplier(kind, &applier{svc: svc, kind: kind})
	}
	svc.OnStale(func(ctx context.Context, tx *sql.Tx, affected []memory.StaleRef) error {
		for _, ref := range affected {
			if ref.Kind != records.KindSummary {
				continue
			}
			hh, deleted, err := svc.Heads(ctx, tx, ref.Scope, ref.Record)
			if err != nil {
				return err
			}
			if deleted || len(hh) != 1 {
				continue
			}
			var summary records.Summary
			if json.Unmarshal(hh[0].Body, &summary) != nil {
				return memory.ErrInvalid
			}
			if summary.Level == "window" {
				continue
			}
			visible, err := memory.IsVisible(ctx, tx, summary.Subject)
			if err != nil {
				return err
			}
			if !visible {
				continue
			}
			kind := "session_digest"
			priority := 2
			if summary.Level == "project_day" {
				kind = "project_rollup"
				priority = 3
			}
			if summary.Level == "personal_week" {
				kind = "personal_rollup"
				priority = 3
			}
			inputs := []records.SourceRef{}
			for _, old := range summary.SourceHeads {
				var scope string
				if tx.QueryRowContext(ctx, "SELECT scope FROM revisions WHERE record=? LIMIT 1", old.Record).Scan(&scope) != nil {
					continue
				}
				heads, del, e := svc.Heads(ctx, tx, scope, old.Record)
				if e != nil {
					return e
				}
				if del {
					continue
				}
				for _, h := range heads {
					inputs = append(inputs, records.SourceRef{Record: h.Record, Revision: h.ID, Kind: h.Kind})
				}
			}
			var generation int64
			if err = tx.QueryRowContext(ctx, "SELECT since FROM stale WHERE record=?", ref.Record).Scan(&generation); err != nil {
				return err
			}
			if len(inputs) > 0 {
				if _, err = svc.EnqueueJob(ctx, tx, memory.JobSpec{Kind: kind, Scope: ref.Scope, Subject: summary.Subject, Destination: ref.Scope, Inputs: inputs, Priority: priority, Versions: summary.Generator.Model + "|" + summary.Generator.PromptVersion + "|" + summary.Generator.PolicyVersion + "|stale/" + strconv.FormatInt(generation, 10)}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
func (a *applier) Apply(ctx context.Context, tx *sql.Tx, r memory.Revision, origin memory.ApplyOrigin) error {
	if r.Operation == "delete" {
		if _, err := tx.ExecContext(ctx, "DELETE FROM session_search WHERE record=?", r.Record); err != nil {
			return err
		}
		_, err := a.svc.MarkStale(ctx, tx, []string{r.Record}, "source_deleted")
		return err
	}
	switch r.Kind {
	case records.KindSummary:
		var s records.Summary
		if json.Unmarshal(r.Body, &s) != nil {
			return memory.ErrInvalid
		}
		deps := append(append([]records.SourceRef{}, s.Sources...), s.Primary...)
		for _, finding := range s.Findings {
			deps = append(deps, finding.Evidence...)
		}
		deps = uniqueRefs(append(deps, s.Influence...))
		if err := a.svc.SetDeps(ctx, tx, r, deps); err != nil {
			return err
		}
		if err := a.svc.SetCoverageDeps(ctx, tx, r, s.SourceHeads); err != nil {
			return err
		}
		text := []string{}
		for _, f := range s.Findings {
			text = append(text, f.Text)
		}
		if err := a.index(ctx, tx, r, s.Subject, strings.Join(text, "\n")); err != nil {
			return err
		}
		if origin == memory.ApplyReplay && s.Level != "window" {
			if _, err := tx.ExecContext(ctx, "UPDATE jobs SET state='done',result_record=?,attempt_token=NULL,lease_until=NULL WHERE job_key=? AND scope=? AND destination=? AND state NOT IN ('cancelled','done')", r.Record, s.Generator.JobKey, r.Scope, r.Scope); err != nil {
				return err
			}
		}
		return nil
	case records.KindInsight:
		if e := a.svc.VerifyOwnerInsight(r); e != nil {
			return e
		}
		var in records.Insight
		if json.Unmarshal(r.Body, &in) != nil {
			return memory.ErrInvalid
		}
		deps := append(append([]records.SourceRef{}, in.Evidence...), in.Contradictions...)
		deps = append(deps, in.Influence...)
		if err := a.svc.SetDeps(ctx, tx, r, deps); err != nil {
			return err
		}
		// A human decision on any head suppresses regeneration. Rejection wins this cache tie;
		// retrieval still exposes all conflicting heads and never resolves by this cache.
		if _, err := tx.ExecContext(ctx, "INSERT INTO candidate_keys(candidate_key,record,review) VALUES(?,?,?) ON CONFLICT(candidate_key) DO UPDATE SET record=CASE WHEN candidate_keys.review IN ('accepted','edited','rejected') THEN candidate_keys.record ELSE excluded.record END,review=CASE WHEN candidate_keys.review IN ('accepted','edited','rejected') THEN candidate_keys.review ELSE excluded.review END", in.CandidateKey, r.Record, in.Review); err != nil {
			return err
		}
		if err := a.index(ctx, tx, r, in.ProjectID, in.Assertion); err != nil {
			return err
		}
		return nil
	case records.KindJob:
		var j records.Job
		if json.Unmarshal(r.Body, &j) != nil {
			return memory.ErrInvalid
		}
		info, err := a.svc.Info(ctx)
		if err != nil {
			return err
		}
		if j.AssignedDevice != info.Device {
			return nil
		}
		if err = authorizeJob(ctx, tx, a.svc, r, j); err != nil {
			_, e := tx.ExecContext(ctx, "INSERT INTO local_seq(name,value) VALUES('rejected_jobs',1) ON CONFLICT(name) DO UPDATE SET value=value+1")
			return e
		}
		if j.State == "cancelled" {
			_, err = tx.ExecContext(ctx, "UPDATE jobs SET state='cancelled',attempt_token=NULL,lease_until=NULL WHERE job_key=? AND scope=? AND destination=? AND subject=?", j.JobKey, r.Scope, j.Destination, j.Subject)
			return err
		}
		localKey, e := a.svc.EnqueueJob(ctx, tx, memory.JobSpec{Kind: j.Kind, Scope: r.Scope, Subject: j.Subject, Destination: j.Destination, Inputs: j.Inputs, AssignedDevice: j.AssignedDevice, Versions: "remote/" + j.JobKey, Priority: 2, NotBefore: time.Now()})
		if e != nil {
			return e
		}
		if localKey != j.JobKey {
			var exists int
			if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE job_key=?", j.JobKey).Scan(&exists); e != nil {
				return e
			}
			if exists == 0 {
				_, e = tx.ExecContext(ctx, "UPDATE jobs SET job_key=? WHERE job_key=?", j.JobKey, localKey)
			} else {
				_, e = tx.ExecContext(ctx, "DELETE FROM jobs WHERE job_key=? AND state='pending'", localKey)
			}
			return e
		}
		return nil
	}
	return memory.ErrInvalid
}
func authorizeJob(ctx context.Context, q memory.Queryer, svc *memory.Service, r memory.Revision, j records.Job) error {
	c, err := projection.SnapshotControl(ctx, q, svc)
	if err != nil {
		return err
	}
	if !projection.HasScope(c, r.Author, j.Destination) {
		return memory.ErrInvalid
	}
	for _, input := range j.Inputs {
		source, e := svc.Revision(ctx, q, input.Revision)
		if e != nil || source.Record != input.Record || !projection.HasScope(c, r.Author, source.Scope) {
			return memory.ErrInvalid
		}
		if j.Kind != "personal_rollup" && source.Scope != r.Scope {
			return memory.ErrInvalid
		}
	}
	if j.Kind == "session_digest" || j.Kind == "project_rollup" {
		if j.Destination != r.Scope {
			return memory.ErrInvalid
		}
		return nil
	}
	if j.Kind != "personal_rollup" || j.Destination != r.Scope {
		return memory.ErrInvalid
	}
	rows, err := q.QueryContext(ctx, "SELECT record FROM aggregation_grants WHERE destination=? AND revoked=0", j.Destination)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	return projection.CheckAggregation(ctx, q, svc, ids, j.Inputs, j.Destination, c)
}
func (a *applier) index(ctx context.Context, tx *sql.Tx, r memory.Revision, subject, text string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM session_search WHERE revision=?", r.ID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO session_search(scope,record,revision,kind,session,text) VALUES(?,?,?,?,?,?)", r.Scope, r.Record, r.ID, r.Kind, subject, text)
	return err
}
func (a *applier) Rebuild(ctx context.Context, tx *sql.Tx) error {
	return memory.ScanRevisions(ctx, tx, a.kind, func(r memory.Revision) error {
		if r.Operation == "delete" {
			return nil
		}
		return a.Apply(ctx, tx, r, memory.ApplyReplay)
	})
}
