package app

import (
	"context"
	"encoding/json"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/projection"
	"github.com/humzakhan/capsule-memory/internal/records"
)

// routedFresh binds a target link to its declared source vault before choosing
// a local key holder. Unrelated same-named links cannot attest this target.
type routedFresh struct {
	target      *memory.Service
	sources     []*Profile
	targets     map[string]*memory.Service
	unavailable bool
}

func (f routedFresh) VerifyFresh(ctx context.Context, ids []string) (map[string]projection.FreshResult, error) {
	out := map[string]projection.FreshResult{}
	if len(ids) > 1000 {
		return out, memory.ErrInvalid
	}
	expected, err := f.targetBindings(ctx, ids)
	if err != nil {
		return out, err
	}
	if f.unavailable {
		return out, memory.ErrConsentUnavailable
	}
	// Classify copies before looking at their link state. An unavailable,
	// withdrawn or outdated copy must not disappear merely because another
	// local profile can produce a current proof for the same vault and scope.
	type sourceCopy struct {
		profile *Profile
		info    memory.ProfileInfo
	}
	copies := make([]sourceCopy, 0, len(f.sources))
	for _, source := range f.sources {
		info, err := source.Svc.Info(ctx)
		if err != nil {
			return out, memory.ErrConsentUnavailable
		}
		copies = append(copies, sourceCopy{profile: source, info: info})
	}
	requests := map[*Profile]map[string]records.Projection{}
	// Classify each link once; batch reconciliation by its unique source.
	for id, target := range expected {
		var source *Profile
		matches := 0
		for _, copy := range copies {
			if copy.info.Vault != target.Source.Vault {
				continue
			}
			for _, scope := range copy.info.Scopes {
				if scope.ID == target.Source.Scope {
					matches++
					source = copy.profile
					break
				}
			}
		}
		if matches != 1 {
			out[id] = projection.FreshResult{Reason: "source_ambiguous_or_unavailable"}
			continue
		}
		if requests[source] == nil {
			requests[source] = map[string]records.Projection{}
		}
		requests[source][id] = target
	}
	// All target/source snapshots end before live bridge reconciliation.
	for source, bindings := range requests {
		var vault string
		for _, target := range bindings {
			vault = target.Source.Vault
			break
		}
		links, err := matchingSourceLinks(ctx, source.Svc, vault, bindings)
		if err != nil {
			for id := range bindings {
				out[id] = projection.FreshResult{Reason: "source_binding_unavailable"}
			}
			continue
		}
		for id := range bindings {
			out[id] = projection.FreshResult{Reason: "source_binding_unavailable"}
		}
		if len(links) == 0 {
			continue
		}
		verified, err := projection.NewBridge(source.Svc, source.Session, f.targets).VerifyFresh(ctx, links)
		for _, id := range links {
			result, ok := verified[id]
			if err != nil || !ok {
				out[id] = projection.FreshResult{Reason: "source_check_unavailable"}
				continue
			}
			// Keep the bridge's exact binding and live guard intact. Resolve
			// checks both again. Each link has only one eligible source copy.
			out[id] = result
		}
	}
	return out, nil
}

func (f routedFresh) targetBindings(ctx context.Context, ids []string) (map[string]records.Projection, error) {
	expected := map[string]records.Projection{}
	err := f.target.Read(ctx, func(q memory.Queryer) error {
		for _, id := range ids {
			rows, err := q.QueryContext(ctx, "SELECT body FROM revisions r WHERE kind='projection' AND json_extract(body,'$.body.role')='target' AND json_extract(body,'$.body.link_id')=? AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=r.revision) LIMIT 2", id)
			if err != nil {
				return err
			}
			var found []records.Projection
			for rows.Next() {
				var body []byte
				var revision memory.Revision
				var link records.Projection
				if rows.Scan(&body) != nil || json.Unmarshal(body, &revision) != nil || json.Unmarshal(revision.Body, &link) != nil {
					rows.Close()
					return memory.ErrStorage
				}
				found = append(found, link)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return memory.ErrStorage
			}
			if len(found) == 1 {
				expected[id] = found[0]
			}
		}
		return nil
	})
	return expected, err
}

func matchingSourceLinks(ctx context.Context, source *memory.Service, vault string, expected map[string]records.Projection) ([]string, error) {
	var links []string
	err := source.Read(ctx, func(q memory.Queryer) error {
		for id, target := range expected {
			if target.Source.Vault != vault {
				continue
			}
			var count int
			err := q.QueryRowContext(ctx, "SELECT count(*) FROM revisions r WHERE kind='projection' AND json_extract(body,'$.body.role')='source' AND json_extract(body,'$.body.link_id')=? AND json_extract(body,'$.body.source.record')=? AND json_extract(body,'$.body.source.revision')=? AND json_extract(body,'$.body.source.scope')=? AND json_extract(body,'$.body.target.vault')=? AND json_extract(body,'$.body.target.scope')=? AND json_extract(body,'$.body.target.record')=? AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=r.revision)", id, target.Source.Record, target.Source.Revision, target.Source.Scope, target.Target.Vault, target.Target.Scope, target.Target.Record).Scan(&count)
			if err != nil {
				return err
			}
			if count == 1 {
				links = append(links, id)
			}
		}
		return nil
	})
	return links, err
}
