package session

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
)

// SameSource compares the complete local capture authority, including the
// consent generation. Config bytes are compared exactly, without normalization.
func SameSource(a, b capture.Source) bool {
	return a.ID == b.ID && a.Harness == b.Harness && a.Adapter == b.Adapter &&
		a.StoreIdentity == b.StoreIdentity && a.Consent == b.Consent &&
		string(a.Config) == string(b.Config)
}

// captureSnapshot runs in the caller's snapshot, including the ingest commit.
// A zero policy version requests the current version before adapter I/O.
func captureSnapshot(ctx context.Context, q memory.Queryer, expected capture.Source, version int) (Policy, error) {
	var current capture.Source
	var enabled, paused int
	var state string
	var raw []byte
	err := q.QueryRowContext(ctx, `SELECT source_id,harness,adapter,store_identity,config,
		COALESCE(consent_at,''),enabled,state,
		(SELECT count(*) FROM pauses WHERE source_id=? AND resolution IS NULL),
		(SELECT body FROM policy WHERE name='session') FROM sources WHERE source_id=?`,
		expected.ID, expected.ID).Scan(&current.ID, &current.Harness, &current.Adapter,
		&current.StoreIdentity, &current.Config, &current.Consent, &enabled, &state, &paused, &raw)
	if err != nil {
		return Policy{}, memory.ErrStorage
	}
	var policy Policy
	if json.Unmarshal(raw, &policy) != nil {
		return policy, memory.ErrLeaseLost
	}
	if !SameSource(current, expected) || current.Consent == "" || enabled != 1 ||
		paused != 0 || state == "withdrawn" || state == "quarantined" || !policy.Capture ||
		(version != 0 && version != policy.Version) {
		return policy, memory.ErrLeaseLost
	}
	return policy, nil
}

func (m *Module) captureSnapshot(ctx context.Context, source capture.Source, version int) (Policy, error) {
	var policy Policy
	err := m.svc.Read(ctx, func(q memory.Queryer) error {
		var err error
		policy, err = captureSnapshot(ctx, q, source, version)
		return err
	})
	return policy, err
}

func (m *Module) CanCaptureSource(ctx context.Context, source capture.Source) (bool, error) {
	_, err := m.captureSnapshot(ctx, source, 0)
	if err == memory.ErrLeaseLost {
		return false, nil
	}
	return err == nil, err
}

// MarkCaptureSource prevents an obsolete worker from overwriting quarantine or
// status belonging to a later configuration/consent generation.
func (m *Module) MarkCaptureSource(ctx context.Context, source capture.Source, state, reason string) error {
	return m.svc.Update(ctx, func(tx *sql.Tx) error {
		if _, err := captureSnapshot(ctx, tx, source, 0); err != nil {
			return err
		}
		if state == "quarantined" {
			if err := auditGap(ctx, tx, source.ID, "", "", reason, 1); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, "UPDATE sources SET state=?,reason=? WHERE source_id=?", state, reason, source.ID)
		return err
	})
}
