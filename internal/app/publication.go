package app

import (
	"context"
	"encoding/json"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/session"
)

// Publication invokes this router before HTTP submission, outside target write
// transactions. Only one source snapshot is open; no network call occurs here.
func (h *Host) checkPublicationSource(ctx context.Context, expected records.Projection) error {
	ref := expected.Source
	h.mu.RLock()
	blocked := len(h.Blocked) != 0
	h.mu.RUnlock()
	if blocked {
		return memory.ErrConsentUnavailable
	}
	var source *Profile
	for _, profile := range h.Snapshot() {
		info, err := profile.Svc.Info(ctx)
		if err != nil {
			return memory.ErrConsentUnavailable
		}
		if info.Vault != ref.Vault {
			continue
		}
		for _, scope := range info.Scopes {
			if scope.ID != ref.Scope {
				continue
			}
			if source != nil {
				return memory.ErrConsentUnavailable
			}
			source = profile
			break
		}
	}
	if source == nil {
		return memory.ErrConsentUnavailable
	}
	bridge := h.BridgeFor(source.Name)
	if bridge == nil {
		return memory.ErrConsentUnavailable
	}
	return source.Svc.Read(ctx, func(q memory.Queryer) error {
		revision, err := source.Svc.Revision(ctx, q, ref.Revision)
		if err != nil {
			return err
		}
		if revision.Vault != ref.Vault || revision.Scope != ref.Scope || revision.Record != ref.Record || revision.Kind != records.KindInsight {
			return memory.ErrInvalid
		}
		if err := source.Svc.CheckLocalScope(ctx, q, ref.Scope); err != nil {
			return err
		}
		sourceRef := records.SourceRef{Record: ref.Record, Revision: ref.Revision, Kind: records.KindInsight}
		if err := memory.CheckConsent(ctx, q, []records.SourceRef{sourceRef}); err != nil {
			return err
		}
		visible, err := memory.RefVisible(ctx, q, sourceRef, true)
		if err != nil {
			return err
		}
		if !visible {
			return memory.ErrInvalid
		}
		var insight records.Insight
		if json.Unmarshal(revision.Body, &insight) != nil || insight.AutoAccepted ||
			(insight.Review != "accepted" && insight.Review != "edited") || insight.Reviewer != revision.Author {
			return memory.ErrInvalid
		}
		if err := checkPublicationScopes(ctx, q, source.Svc, ref.Revision); err != nil {
			return err
		}
		if err := bridge.CheckSourcePublication(ctx, q, expected); err != nil {
			return err
		}
		if ref.Vault != expected.Target.Vault {
			var raw []byte
			if err := q.QueryRowContext(ctx, "SELECT body FROM policy WHERE name='session'").Scan(&raw); err != nil {
				return memory.ErrConsentUnavailable
			}
			var policy session.Policy
			if json.Unmarshal(raw, &policy) != nil || policy.CrossVault != "enabled_unaccepted" {
				return memory.ErrConsentUnavailable
			}
		}
		return nil
	})
}

// Consent and RefVisible have already bounded/validated this dependency graph.
// Recheck every transitive input scope in the same source snapshot: possession
// of a copied key is insufficient after a local control/grant change.
func checkPublicationScopes(ctx context.Context, q memory.Queryer, source *memory.Service, revision string) error {
	rows, err := q.QueryContext(ctx, `WITH RECURSIVE ancestry(revision) AS (
		SELECT ? UNION SELECT d.parent_revision FROM deps d JOIN ancestry a ON d.child_revision=a.revision
	) SELECT DISTINCT r.scope FROM ancestry a JOIN revisions r ON r.revision=a.revision LIMIT 65`, revision)
	if err != nil {
		return memory.ErrStorage
	}
	var scopes []string
	for rows.Next() {
		var scope string
		if rows.Scan(&scope) != nil {
			rows.Close()
			return memory.ErrStorage
		}
		scopes = append(scopes, scope)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return memory.ErrStorage
	}
	// A control contains at most 64 scopes; never truncate an authority check.
	if len(scopes) == 0 || len(scopes) > 64 {
		return memory.ErrInvalid
	}
	for _, scope := range scopes {
		if err := source.CheckLocalScope(ctx, q, scope); err != nil {
			return err
		}
	}
	return nil
}
