package projection

import (
	"context"
	"encoding/json"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

// publicationProjection binds queued assertion bytes to one authenticated,
// current target lifecycle. A source reference alone never supplies this authority.
func publicationProjection(ctx context.Context, q memory.Queryer, target *memory.Service, r memory.Revision) (records.Projection, error) {
	var want records.Projection
	if r.Kind == records.KindKnowledge {
		var knowledge records.Knowledge
		if json.Unmarshal(r.Body, &knowledge) != nil {
			return want, memory.ErrInvalid
		}
		want.LinkID, want.Source = knowledge.LinkID, knowledge.Source
		want.Target = records.ScopedRef{Vault: r.Vault, Scope: r.Scope, Record: r.Record, Revision: r.ID}
		want.TextSHA256 = knowledge.TextSHA256
	} else if r.Kind == records.KindProjection {
		if json.Unmarshal(r.Body, &want) != nil {
			return want, memory.ErrInvalid
		}
		if want.Role != "target" || want.WithdrawalMarker || want.State != "projected" || r.Vault != want.Target.Vault || r.Scope != want.Target.Scope {
			return want, memory.ErrInvalid
		}
	} else {
		return want, memory.ErrInvalid
	}
	if err := target.CheckLocalScope(ctx, q, want.Target.Scope); err != nil {
		return want, err
	}
	visibility, err := TargetVisible(ctx, q, target, want.Target.Record)
	if err != nil {
		return want, err
	}
	if visibility.Reason == "withdrawn" {
		return want, memory.ErrConsentWithdrawn
	}
	if !visibility.Visible || visibility.Revision != want.Target.Revision {
		return want, memory.ErrConsentUnavailable
	}
	heads, err := linkHeads(ctx, q, target, want.LinkID, "target", want.Target.Scope, want.Target.Record)
	if err != nil {
		return want, err
	}
	var found *records.Projection
	for _, head := range heads {
		var actual records.Projection
		if json.Unmarshal(head.Body, &actual) != nil {
			return want, memory.ErrInvalid
		}
		if actual.WithdrawalMarker {
			continue
		}
		if actual.Role != "target" || actual.State != "projected" || actual.OwnerWithdrawn || actual.LinkID != want.LinkID || actual.Source != want.Source || actual.Target != want.Target || actual.TextSHA256 != want.TextSHA256 || head.Vault != want.Target.Vault || head.Scope != want.Target.Scope {
			return want, memory.ErrInvalid
		}
		if r.Kind == records.KindProjection && head.ID != r.ID {
			return want, memory.ErrHeadsChanged
		}
		if found != nil {
			return want, memory.ErrConsentUnavailable
		}
		copy := actual
		found = &copy
	}
	if found == nil {
		return want, memory.ErrConsentUnavailable
	}
	return *found, nil
}

// CheckSourcePublication uses only the supplied source snapshot. Binds the
// completed source receipt/lifecycle to exactly the target's reviewed bytes.
func (b *Bridge) CheckSourcePublication(ctx context.Context, q memory.Queryer, expected records.Projection) error {
	if expected.Role != "target" || expected.State != "projected" || expected.WithdrawalMarker || expected.OwnerWithdrawn {
		return memory.ErrInvalid
	}
	actual, err := b.sourceProjection(ctx, q, expected.LinkID)
	if err != nil {
		return err
	}
	if actual.OwnerWithdrawn || actual.State == "withdrawn" {
		return memory.ErrConsentWithdrawn
	}
	if actual.State != "projected" || actual.WithdrawalMarker {
		return memory.ErrConsentUnavailable
	}
	if actual.Source != expected.Source || actual.Target != expected.Target || actual.LinkID != expected.LinkID || actual.Generation != expected.Generation || actual.TextSHA256 != expected.TextSHA256 || actual.Grant != expected.Grant {
		return memory.ErrHeadsChanged
	}
	heads, err := linkHeads(ctx, q, b.source, actual.LinkID, "source", actual.Source.Scope, actual.Target.Record)
	if err != nil {
		return err
	}
	if len(heads) != 1 || heads[0].Vault != actual.Source.Vault || heads[0].Scope != actual.Source.Scope {
		return memory.ErrInvalid
	}
	control, err := SnapshotControl(ctx, q, b.source)
	if err != nil {
		return err
	}
	if !HasScope(control, heads[0].Author, actual.Source.Scope) {
		return memory.ErrInvalid
	}
	var generation int64
	var terminal bool
	var state string
	if err := q.QueryRowContext(ctx, "SELECT generation,owner_withdrawn,state FROM projections WHERE link_id=?", actual.LinkID).Scan(&generation, &terminal, &state); err != nil {
		return memory.ErrConsentUnavailable
	}
	if terminal {
		return memory.ErrConsentWithdrawn
	}
	if state != "projected" {
		return memory.ErrConsentUnavailable
	}
	if generation != expected.Generation {
		return memory.ErrHeadsChanged
	}
	if err := b.source.CheckLocalScope(ctx, q, actual.Source.Scope); err != nil {
		return err
	}
	refs := append([]records.SourceRef{}, actual.Dependencies...)
	refs = append(refs, records.SourceRef{Record: actual.Source.Record, Revision: actual.Source.Revision, Kind: records.KindInsight})
	return memory.CheckConsent(ctx, q, refs)
}
