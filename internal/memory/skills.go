package memory

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"errors"
)

type Release struct {
	Version      int      `json:"version"`
	Vault        string   `json:"vault_id"`
	Scope        string   `json:"scope_id"`
	Record       string   `json:"record_id"`
	Revision     string   `json:"revision_id"`
	Digest       string   `json:"sha256"`
	Runtime      string   `json:"runtime"`
	Capabilities []string `json:"capabilities"`
}

type SkillState struct {
	Approved bool `json:"approved"`
	Active   bool `json:"active"`
	Staged   bool `json:"staged"`
}

// This release selects reviewed local text only. No installer or executor is used.
func (s *Service) textRelease(r Revision) Release {
	return Release{1, s.configuration().Vault, r.Scope, r.Record, r.ID, digest([]byte(r.Payload.Text)), "text/markdown", []string{"read-as-text"}}
}
func (s *Service) validRelease(release Release, r Revision) bool {
	if r.Payload == nil || r.Kind != "skill" || r.Vault != s.configuration().Vault {
		return false
	}
	for _, id := range []string{release.Vault, release.Scope, release.Record, release.Revision} {
		if !idPattern.MatchString(id) {
			return false
		}
	}
	expected := s.textRelease(r)
	return release.Version == expected.Version && release.Vault == expected.Vault && release.Scope == expected.Scope && release.Record == expected.Record && release.Revision == expected.Revision && release.Digest == expected.Digest && release.Runtime == expected.Runtime && len(release.Capabilities) == 1 && release.Capabilities[0] == "read-as-text"
}

// Invalid or absent approvals are unapproved, regardless of the stored active bit.
// q is the caller's transaction, so current heads and approval share one snapshot.
func (s *Service) skillState(ctx context.Context, q queryer, heads []Revision, deleted bool) (SkillState, error) {
	result := SkillState{Staged: true}
	if deleted || len(heads) != 1 || heads[0].Kind != "skill" || heads[0].Payload == nil {
		return result, nil
	}
	r := heads[0]
	var raw []byte
	var active int
	err := q.QueryRowContext(ctx, "SELECT release,active FROM approvals WHERE revision=?", r.ID).Scan(&raw, &active)
	if err == sql.ErrNoRows {
		return result, nil
	}
	if err != nil {
		return result, ErrStorage
	}
	var signed Signed
	var release Release
	if decode(raw, &signed) != nil {
		return result, nil
	}
	pub, err := unbase(s.configuration().OwnerPublic, ed25519.PublicKeySize)
	if err != nil {
		return result, nil
	}
	// Legacy v1 protocol domain is fixed for compatibility; see docs/renaming.md.
	if verify("agent-memory/skill-approval/v1", signed, ed25519.PublicKey(pub), &release) != nil || !s.validRelease(release, r) {
		return result, nil
	}
	result.Approved = true
	result.Active = active == 1
	result.Staged = !result.Active
	return result, nil
}
func (s *Service) recordView(ctx context.Context, q queryer, scope, record string, history bool) (RecordView, error) {
	revs, err := recordRevisions(ctx, q, scope, record)
	if err != nil {
		return RecordView{}, err
	}
	if len(revs) == 0 {
		return RecordView{}, errors.New("record not found")
	}
	if revs[0].Kind != "memory" && revs[0].Kind != "skill" {
		return RecordView{}, ErrInvalid
	}
	for _, r := range revs {
		if r.Vault != s.configuration().Vault || r.Scope != scope || r.Record != record {
			return RecordView{}, ErrInvalid
		}
	}
	heads, deleted := state(revs)
	v := RecordView{Record: record, Heads: heads, Deleted: deleted, Conflict: len(heads) > 1}
	if revs[0].Kind == "skill" {
		status, err := s.skillState(ctx, q, heads, deleted)
		if err != nil {
			return RecordView{}, err
		}
		v.SkillState = &status
	}
	if history {
		v.History = revs
	} else if deleted {
		for i := range v.Heads {
			v.Heads[i].Payload = nil
		}
	}
	return v, nil
}
func currentSkill(ctx context.Context, q queryer, scope, record, revision string) (Revision, error) {
	revs, err := recordRevisions(ctx, q, scope, record)
	if err != nil {
		return Revision{}, err
	}
	heads, deleted := state(revs)
	if deleted || len(heads) != 1 || heads[0].ID != revision || heads[0].Scope != scope || heads[0].Record != record || heads[0].Kind != "skill" || heads[0].Payload == nil {
		return Revision{}, errors.New("approval and activation require the unique current visible skill revision")
	}
	return heads[0], nil
}
func skillResult(r Revision, status SkillState) map[string]any {
	return map[string]any{"record_id": r.Record, "revision_id": r.ID, "approved": status.Approved, "active": status.Active, "staged": status.Staged, "runtime": "text/markdown", "capabilities": []string{"read-as-text"}}
}
func (s *Service) ApproveSkill(ctx context.Context, scope, record, revision string) (map[string]any, error) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if err := s.scope(scope); err != nil {
		return nil, err
	}
	owner, err := s.ownerKey()
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, ErrStorage
	}
	defer tx.Rollback()
	r, err := currentSkill(ctx, tx, scope, record, revision)
	if err != nil {
		return nil, err
	}
	release := s.textRelease(r)
	if !s.validRelease(release, r) {
		return nil, ErrInvalid
	}
	signed, err := sign("agent-memory/skill-approval/v1", release, owner)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(signed)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO approvals(revision,release,active) VALUES(?,?,0) ON CONFLICT(revision) DO UPDATE SET release=excluded.release,active=0", revision, b); err != nil {
		return nil, ErrStorage
	}
	if err = tx.Commit(); err != nil {
		return nil, ErrStorage
	}
	result := skillResult(r, SkillState{Approved: true, Staged: true})
	result["sha256"] = release.Digest
	return result, nil
}
func (s *Service) ActivateSkill(ctx context.Context, scope, record, revision string) (map[string]any, error) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if s.enrollmentPending.Load() {
		return nil, ErrEnrollmentPending
	}
	if err := s.scope(scope); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, ErrStorage
	}
	defer tx.Rollback()
	r, err := currentSkill(ctx, tx, scope, record, revision)
	if err != nil {
		return nil, err
	}
	status, err := s.skillState(ctx, tx, []Revision{r}, false)
	if err != nil {
		return nil, err
	}
	if !status.Approved {
		return nil, errors.New("activation requires a valid exact owner-signed local text approval")
	}
	if _, err = tx.ExecContext(ctx, "UPDATE approvals SET active=1 WHERE revision=?", revision); err != nil {
		return nil, ErrStorage
	}
	if err = tx.Commit(); err != nil {
		return nil, ErrStorage
	}
	return skillResult(r, SkillState{Approved: true, Active: true}), nil
}
func (s *Service) FetchSkill(ctx context.Context, scope, record string, history bool) (map[string]any, error) {
	v, err := s.Get(ctx, scope, record, history)
	if err != nil {
		return nil, err
	}
	if v.SkillState == nil {
		return nil, ErrInvalid
	}
	return map[string]any{"skill": v, "approved": v.Approved, "active": v.Active, "staged": v.Staged}, nil
}
func (s *Service) ListSkills(ctx context.Context, scope string) ([]RecordView, error) {
	if err := s.scope(scope); err != nil {
		return nil, err
	}
	tx, err := s.reader.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, ErrStorage
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT r.record FROM revisions r
		WHERE r.scope=? AND r.kind='skill'
		AND NOT EXISTS (
			SELECT 1 FROM revisions d WHERE d.scope=r.scope AND d.record=r.record
			AND json_extract(d.body,'$.operation')='delete'
			AND NOT EXISTS (SELECT 1 FROM restores x WHERE x.deletion=d.revision)
		) ORDER BY r.record LIMIT 100`, scope)
	if err != nil {
		return nil, ErrStorage
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return nil, ErrStorage
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, ErrStorage
	}
	out := []RecordView{}
	for _, id := range ids {
		v, err := s.recordView(ctx, tx, scope, id, false)
		if err != nil {
			return nil, err
		}
		if !v.Deleted {
			out = append(out, v)
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, ErrStorage
	}
	return out, nil
}
func SkillListResult(skills []RecordView) map[string]any {
	return map[string]any{"skills": skills, "activation_supported": true, "runtime": "text/markdown", "capabilities": []string{"read-as-text"}}
}
