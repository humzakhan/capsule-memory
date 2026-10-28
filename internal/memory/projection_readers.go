package memory

import (
	"context"
	"database/sql"
	"errors"

	"github.com/humzakhan/capsule-memory/internal/records"
)

var ErrProjectionReaders = errors.New("same-vault projection readers exceed source readers")

// checkProjectionReaders runs before owner control or key-package disclosure,
// including direct and recovery paths where projection hooks are not installed.
// Immutable knowledge history is authoritative: tombstones, withdrawn links and
// rebuilt indexes do not erase ciphertext encrypted with the target scope key.
// Cross-vault reviewed exports have separate owner bridge authority.
func (s *Service) checkProjectionReaders(ctx context.Context, q Queryer, proposed Control) error {
	rows, err := q.QueryContext(ctx, `SELECT DISTINCT scope,
		json_extract(body,'$.body.source.scope') FROM revisions
		WHERE kind=? AND json_extract(body,'$.body.source.vault')=?
		ORDER BY scope,json_extract(body,'$.body.source.scope') LIMIT 4097`,
		records.KindKnowledge, proposed.Vault)
	if err != nil {
		return ErrStorage
	}
	defer rows.Close()
	readers := map[string]map[string]bool{}
	for _, device := range proposed.Devices {
		if device.Revoked {
			continue
		}
		for _, scope := range device.Scopes {
			if readers[scope] == nil {
				readers[scope] = map[string]bool{}
			}
			readers[scope][device.ID] = true
		}
	}
	count := 0
	for rows.Next() {
		var target, source string
		if err := rows.Scan(&target, &source); err != nil {
			return ErrStorage
		}
		count++
		// A valid control has at most 64 scopes: at most 64*64 bindings.
		// Inconsistent or excessive retained authority fails before key writes.
		if count > 4096 || !idPattern.MatchString(target) || !idPattern.MatchString(source) {
			return ErrInvalid
		}
		for device := range readers[target] {
			if !readers[source][device] {
				return ErrProjectionReaders
			}
		}
	}
	if rows.Err() != nil {
		return ErrStorage
	}
	return nil
}

// withGrantAuthority owns the local writer reservation through key disclosure.
// Callers already own wmu. The first write prevents another SQLite connection or
// process from adding protected history between the checks and the private file
// write. The callback performs local control/file work only, never network I/O.
func (s *Service) withGrantAuthority(ctx context.Context, generation string, proposed Control, disclose func(*sql.Tx) error) error {
	return s.updateLocked(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE meta SET value=value WHERE key='current_control'"); err != nil {
			return ErrStorage
		}
		current, _, err := s.currentTx(ctx, tx)
		if err != nil {
			return err
		}
		if current.Generation != generation {
			return ErrHeadsChanged
		}
		if err := s.checkGrantsQuery(ctx, tx, proposed); err != nil {
			return err
		}
		return disclose(tx)
	})
}
