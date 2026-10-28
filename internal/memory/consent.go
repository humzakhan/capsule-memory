package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/humzakhan/capsule-memory/internal/records"
)

var ErrConsentWithdrawn = errors.New("source consent withdrawn; automatic work held")
var ErrConsentUnavailable = errors.New("source consent unavailable; open the source profile before publishing automatic descendants")
var ErrUnpublishedParent = errors.New("parent publication unconfirmed; revision retained without releasing private ancestry")

// Direct sync without a source router retains promoted descendants locally.
func (s *Service) SetPublicationCheck(fn func(context.Context, records.Projection) error) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	s.publicationCheck = fn
}

// SetProjectionPublicationCheck installs the projection consumer's pure local
// lifecycle/grant check. It must use q only, with no network or nested updates.
func (s *Service) SetProjectionPublicationCheck(fn func(context.Context, Queryer, Revision) (records.Projection, error)) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	s.projectionPublicationCheck = fn
}

func (s *Service) checkPublication(ctx context.Context, id string) error {
	s.configMu.RLock()
	remote := s.publicationCheck
	local := s.projectionPublicationCheck
	s.configMu.RUnlock()
	var contexts []records.Projection
	var revisions []string
	err := s.Read(ctx, func(q Queryer) error {
		r, err := s.Revision(ctx, q, id)
		if err != nil {
			return err
		}
		for _, parent := range r.Parents {
			var pending int
			if err = q.QueryRowContext(ctx, "SELECT count(*) FROM outbox WHERE revision=? AND receipt IS NULL", parent).Scan(&pending); err != nil {
				return ErrStorage
			}
			if pending != 0 {
				return ErrUnpublishedParent
			}
		}
		// Cleanup never releases private same-record parents. Independent markers
		// use their authenticated opaque binding and carry no assertion text.
		if r.Operation == "delete" {
			return nil
		}
		if r.Kind == records.KindProjection {
			var projection records.Projection
			if json.Unmarshal(r.Body, &projection) != nil {
				return ErrInvalid
			}
			if projection.WithdrawalMarker {
				return s.VerifyProjectionMarker(ctx, q, r, projection, true)
			}
			if projection.State == "hidden" || projection.State == "withdrawn" {
				return nil
			}
		}
		if err = CheckConsent(ctx, q, []records.SourceRef{{Revision: id}}); err != nil {
			return err
		}
		rows, err := q.QueryContext(ctx, `WITH RECURSIVE ancestry(revision) AS (
			SELECT ? UNION SELECT d.parent_revision FROM deps d JOIN ancestry a ON d.child_revision=a.revision
		) SELECT r.revision FROM ancestry a JOIN revisions r ON r.revision=a.revision LIMIT 100001`, id)
		if err != nil {
			return ErrStorage
		}
		var ids []string
		for rows.Next() {
			var revision string
			if rows.Scan(&revision) != nil {
				rows.Close()
				return ErrStorage
			}
			ids = append(ids, revision)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return ErrStorage
		}
		if len(ids) > 100000 {
			return ErrStaleOverflow
		}
		for _, revision := range ids {
			r, err := s.Revision(ctx, q, revision)
			if err != nil {
				return err
			}
			if r.Operation == "delete" {
				continue
			}
			if r.Kind == records.KindProjection {
				var projection records.Projection
				if json.Unmarshal(r.Body, &projection) != nil {
					return ErrInvalid
				}
				if projection.Role != "target" || projection.State == "hidden" || projection.State == "withdrawn" {
					continue
				}
			} else if r.Kind != records.KindKnowledge {
				continue
			}
			if local == nil || remote == nil {
				return ErrConsentUnavailable
			}
			projection, err := local(ctx, q, r)
			if err != nil {
				return err
			}
			contexts = append(contexts, projection)
			revisions = append(revisions, r.ID)
		}
		return nil
	})
	if err != nil {
		return err
	}
	// The source router may refresh remote control. It never runs inside a
	// target snapshot or under a write lock. Retry bytes remain unchanged.
	for _, projection := range contexts {
		if err = remote(ctx, projection); err != nil {
			return err
		}
	}
	if len(contexts) == 0 {
		return nil
	}
	return s.Read(ctx, func(q Queryer) error {
		if err := CheckConsent(ctx, q, []records.SourceRef{{Revision: id}}); err != nil {
			return err
		}
		for i, revision := range revisions {
			r, err := s.Revision(ctx, q, revision)
			if err != nil {
				return err
			}
			current, err := local(ctx, q, r)
			if err != nil {
				return err
			}
			actual, err := json.Marshal(current)
			if err != nil {
				return ErrInvalid
			}
			expected, err := json.Marshal(contexts[i])
			if err != nil || !equalBytes(actual, expected) {
				return ErrHeadsChanged
			}
		}
		return nil
	})
}

// Durable roots refer to exact captured revisions, so enabling a source again
// grants no release of earlier consent. Derived indexes can be rebuilt freely.
func (s *Service) HoldSource(ctx context.Context, tx *sql.Tx, source string) error {
	if _, e := tx.ExecContext(ctx, "CREATE TEMP TABLE IF NOT EXISTS consent_affected(revision TEXT PRIMARY KEY); DELETE FROM consent_affected"); e != nil {
		return ErrStorage
	}
	if _, e := tx.ExecContext(ctx, `WITH RECURSIVE affected(revision) AS (SELECT revision FROM outbox_source WHERE source_id=? UNION SELECT d.child_revision FROM deps d JOIN affected a ON d.parent_revision=a.revision) INSERT INTO consent_affected SELECT revision FROM affected LIMIT 100001`, source); e != nil {
		return ErrStorage
	}
	var n int
	if tx.QueryRowContext(ctx, "SELECT count(*) FROM consent_affected").Scan(&n) != nil {
		return ErrStorage
	}
	if n > 100000 {
		return ErrStaleOverflow
	}
	if _, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO consent_roots SELECT revision,? FROM outbox_source WHERE source_id=?", source, source); e != nil {
		return ErrStorage
	}
	if _, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO outbox_hold SELECT o.revision,'consent_withdrawn' FROM outbox o JOIN consent_affected a ON a.revision=o.revision JOIN revisions r ON r.revision=o.revision WHERE o.receipt IS NULL AND json_extract(r.body,'$.operation')!='delete' AND NOT (r.kind='projection' AND json_extract(r.body,'$.body.state') IN ('hidden','withdrawn'))"); e != nil {
		return ErrStorage
	}
	_, e := tx.ExecContext(ctx, `UPDATE jobs SET state='cancelled',error_code='consent_withdrawn',attempt_token=NULL,lease_until=NULL WHERE state IN ('pending','leased','blocked') AND (EXISTS(SELECT 1 FROM json_each(jobs.inputs) i JOIN consent_affected a ON a.revision=json_extract(i.value,'$.revision')) OR (kind='bridge_project' AND subject IN (SELECT link_id FROM projections WHERE source_revision IN (SELECT revision FROM consent_affected))))`)
	return e
}

func CheckConsent(ctx context.Context, q Queryer, refs []records.SourceRef) error {
	raw, e := json.Marshal(refs)
	if e != nil {
		return ErrInvalid
	}
	var count, held, missing int
	e = q.QueryRowContext(ctx, `WITH RECURSIVE ancestry(revision) AS (
 SELECT json_extract(value,'$.revision') FROM json_each(?)
 UNION SELECT d.parent_revision FROM deps d JOIN ancestry a ON d.child_revision=a.revision
 ), bounded AS (SELECT revision FROM ancestry LIMIT 100001)
 SELECT count(*),COALESCE(max(EXISTS(SELECT 1 FROM consent_roots c WHERE c.revision=bounded.revision)),0),COALESCE(max(NOT EXISTS(SELECT 1 FROM revisions r WHERE r.revision=bounded.revision)),0) FROM bounded`, raw).Scan(&count, &held, &missing)
	if e != nil {
		return ErrStorage
	}
	if count > 100000 {
		return ErrStaleOverflow
	}
	if held != 0 {
		return ErrConsentWithdrawn
	}
	if missing != 0 {
		return ErrMissingRevision
	}
	return nil
}
