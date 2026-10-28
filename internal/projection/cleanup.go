package projection

import (
	"context"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

func cleanupProjection(p records.Projection) records.Projection {
	p.WithdrawalMarker = true
	p.Source.Revision = ""
	p.Target.Revision = ""
	p.Dependencies = []records.SourceRef{}
	p.Attestation = nil
	p.TextSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	return p
}
func prepareCleanup(ctx context.Context, target *memory.Service, p records.Projection) (memory.Prepared, error) {
	p = cleanupProjection(p)
	input := memory.PutInput{Scope: p.Target.Scope, Kind: records.KindProjection, Record: linkRecord("target-cleanup", p.LinkID), Body: p, Provenance: memory.Provenance{SourceKind: "projection", SourceRef: p.LinkID}}
	e := target.Read(ctx, func(q memory.Queryer) error {
		_, deleted, e := target.Heads(ctx, q, input.Scope, input.Record)
		if e != nil {
			return e
		}
		if deleted {
			input.Operation = "restore"
			input.Restores, e = target.UnrestoredDeletions(ctx, q, input.Record)
		}
		return e
	})
	if e != nil {
		return memory.Prepared{}, e
	}
	return target.Prepare(ctx, input)
}
func retireCleanup(ctx context.Context, target *memory.Service, p records.Projection) ([]memory.Prepared, error) {
	record := linkRecord("target-cleanup", p.LinkID)
	active := false
	e := target.Read(ctx, func(q memory.Queryer) error {
		hh, deleted, e := target.Heads(ctx, q, p.Target.Scope, record)
		active = len(hh) > 0 && !deleted
		return e
	})
	if e != nil {
		return nil, e
	}
	if !active {
		return nil, nil
	}
	prepared, e := target.Prepare(ctx, memory.PutInput{Scope: p.Target.Scope, Kind: records.KindProjection, Record: record, Operation: "delete", Provenance: memory.Provenance{SourceKind: "projection", SourceRef: p.LinkID}})
	if e != nil {
		return nil, e
	}
	return []memory.Prepared{prepared}, nil
}
