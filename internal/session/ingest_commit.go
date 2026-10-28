package session

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

// commitCapture is the sole acknowledgement boundary: prepared ciphertext,
// source identities, budgets and the read cursor all commit or all roll back.
func (m *Module) commitCapture(ctx context.Context, src capture.Source, st capture.Stream,
	ch capture.Changes, policy Policy, historical bool, prepared []memory.Prepared,
	observations map[string]capture.Observation, groups map[string]*ingestGroup,
	ids []identity, gaps []records.Omission) error {
	return m.svc.Commit(ctx, prepared, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := captureSnapshot(ctx, tx, src, ch.ObservedPolicyVersion); err != nil {
			return err
		}
		if err := checkpoint(ctx, tx, src.ID, ch.Next); err != nil {
			return err
		}
		for key, observation := range observations {
			if observation.CWD == "" || len(observation.CWD) > 4096 {
				continue
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO origin_paths VALUES(?,?)
				ON CONFLICT(session_key) DO UPDATE SET path=excluded.path`, key, observation.CWD)
			if err != nil {
				return err
			}
		}
		if historical {
			result, err := tx.ExecContext(ctx, `UPDATE backfills SET remaining=remaining-?
				WHERE source_id=? AND remaining>=?`, ch.ScannedBytes, src.ID, ch.ScannedBytes)
			if err != nil {
				return err
			}
			count, err := result.RowsAffected()
			if err != nil || count != 1 {
				return memory.ErrInvalid
			}
			if ch.CaughtUp {
				if _, err := tx.ExecContext(ctx, "UPDATE backfill_streams SET active=0 WHERE source_id=? AND stream=?", src.ID, st.Key); err != nil {
					return err
				}
			}
		}
		for _, group := range groups {
			gaps = append(gaps, group.omissions...)
		}
		counts := map[string]int{}
		for _, gap := range gaps {
			counts[gap.Reason]++
		}
		for reason, count := range counts {
			if err := auditGap(ctx, tx, src.ID, st.Key, ch.Next.Position, reason, count); err != nil {
				return err
			}
		}
		for _, id := range ids {
			_, err := tx.ExecContext(ctx, `INSERT INTO event_ids
				(source_id,stream,generation,position_key,event_id,content_digest) VALUES(?,?,?,?,?,?)
				ON CONFLICT(source_id,stream,generation,position_key) DO UPDATE
				SET event_id=excluded.event_id,content_digest=excluded.content_digest`,
				src.ID, st.Key, ch.Next.Generation, id.position, id.id, id.digest)
			if err != nil {
				return err
			}
		}
		for _, item := range prepared {
			if _, err := tx.ExecContext(ctx, "INSERT INTO outbox_source(revision,source_id) VALUES(?,?)", item.Revision.ID, src.ID); err != nil {
				return err
			}
		}
		if !policy.Derive {
			return nil
		}
		for _, group := range groups {
			subject, inputs, err := m.DigestInputs(ctx, tx, group.session)
			if err != nil {
				if !errors.Is(err, memory.ErrHeadsChanged) && !errors.Is(err, memory.ErrInvalid) {
					return err
				}
				if err := auditGap(ctx, tx, src.ID, st.Key, ch.Next.Position, "derive_inputs_incomplete", 1); err != nil {
					return err
				}
				continue
			}
			notBefore := time.Now().Add(15 * time.Minute)
			if group.manifest.State == "closed" {
				notBefore = time.Now()
			}
			spec := memory.JobSpec{
				Kind: "session_digest", Scope: group.scope, Subject: subject,
				Destination: group.scope, Inputs: inputs,
				Versions: "session/v1|policy/" + strconv.Itoa(policy.Version),
				Priority: 2, NotBefore: notBefore,
			}
			key, err := m.svc.EnqueueJob(ctx, tx, spec)
			if err != nil {
				return err
			}
			if err := m.svc.SupersedePending(ctx, tx, "session_digest", subject, key); err != nil {
				return err
			}
		}
		return nil
	})
}

// Quarantine records only a bounded reason and source/stream identity. It never
// acknowledges the failing page or persists raw adapter error text.
func (m *Module) quarantineCapture(ctx context.Context, src capture.Source, st capture.Stream, ch capture.Changes, reason string, count int) error {
	return m.svc.Update(ctx, func(tx *sql.Tx) error {
		if _, err := captureSnapshot(ctx, tx, src, ch.ObservedPolicyVersion); err != nil {
			return err
		}
		if reason == "protocol_or_version_mismatch" {
			if _, err := tx.ExecContext(ctx, "UPDATE sources SET state='quarantined',reason=? WHERE source_id=?", reason, src.ID); err != nil {
				return err
			}
		}
		if err := auditGap(ctx, tx, src.ID, st.Key, ch.Next.Position, reason, count); err != nil {
			return err
		}
		return quarantineStream(ctx, tx, src.ID, st.Key, reason)
	})
}
