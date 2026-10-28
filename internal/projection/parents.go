package projection

import (
	"context"
	"database/sql"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

func (b *Bridge) sourceParents(ctx context.Context, q memory.Queryer, p records.Projection) ([]string, error) {
	hh, deleted, e := b.source.Heads(ctx, q, p.Source.Scope, linkRecord("source", p.LinkID))
	if e != nil {
		return nil, e
	}
	if deleted && !p.OwnerWithdrawn || len(hh) == 0 {
		return nil, memory.ErrHeadsChanged
	}
	ids := []string{}
	for _, r := range hh {
		ids = append(ids, r.ID)
	}
	return ids, nil
}

// Called after apply in the same transaction: a concurrent, unobserved head
// survives explicit ancestry and makes this transaction roll back.
func (b *Bridge) sourceHead(ctx context.Context, tx *sql.Tx, p memory.Prepared) error {
	hh, deleted, e := b.source.Heads(ctx, tx, p.Revision.Scope, p.Revision.Record)
	if e != nil {
		return e
	}
	if deleted || len(hh) != 1 || hh[0].ID != p.Revision.ID {
		return memory.ErrHeadsChanged
	}
	return nil
}
