package memory

import (
	"context"
	"database/sql"
	"path/filepath"
)

// CommitWithSource excludes local writes to both profiles until the target
// transaction commits. Source validation finishes and closes its snapshot before the target
// transaction begins. Both callbacks must use only their supplied queries, with
// no network I/O or nested updates. The same Service validates in the target
// transaction. This is local write exclusion, not a distributed transaction.
func (s *Service) CommitWithSource(ctx context.Context, set []Prepared, source *Service, checkSource func(context.Context, Queryer) error, targetExtra func(context.Context, *sql.Tx) error) error {
	if source == nil || checkSource == nil || targetExtra == nil {
		return ErrInvalid
	}
	if s == source {
		return s.Commit(ctx, set, func(ctx context.Context, tx *sql.Tx) error {
			if err := checkSource(ctx, tx); err != nil {
				return err
			}
			return targetExtra(ctx, tx)
		})
	}
	// Two handles for one profile cannot provide mutual exclusion through wmu.
	targetHome, err := filepath.Abs(s.home)
	if err != nil {
		return ErrInvalid
	}
	sourceHome, err := filepath.Abs(source.home)
	if err != nil {
		return ErrInvalid
	}
	if targetHome == sourceHome {
		return ErrInvalid
	}
	first, second := s, source
	if targetHome > sourceHome {
		first, second = second, first
	}
	first.wmu.Lock()
	defer first.wmu.Unlock()
	second.wmu.Lock()
	defer second.wmu.Unlock()

	// Cross-profile work needs exclusive process ownership as well as wmu.
	// A shared direct-mode handle cannot fence another process's source writes.
	if s.lockMode != LockExclusive || source.lockMode != LockExclusive || s.profileLock == nil || source.profileLock == nil {
		return ErrProfileBusy
	}
	if err := source.Read(ctx, func(q Queryer) error {
		return checkSource(ctx, q)
	}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.commitLocked(ctx, set, targetExtra)
}
