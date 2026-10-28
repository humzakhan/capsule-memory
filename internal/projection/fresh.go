package projection

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"time"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/session"
)

func Resolve(ctx context.Context, target *memory.Service, recordsIDs []string, mode Mode, fresh Freshness) (map[string]Visibility, error) {
	result := map[string]Visibility{}
	links := map[string]string{}
	ids := []string{}
	if mode != Historical && mode != Strict {
		return result, memory.ErrInvalid
	}
	if mode == Strict {
		bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		ctx = bounded
	}
	if len(recordsIDs) > 1000 {
		return result, memory.ErrInvalid
	}
	err := target.Read(ctx, func(q memory.Queryer) error {
		for _, id := range recordsIDs {
			v, e := TargetVisible(ctx, q, target, id)
			if e != nil {
				return e
			}
			result[id] = v
			if !v.Visible {
				continue
			}
			var link string
			e = q.QueryRowContext(ctx, "SELECT link_id FROM knowledge WHERE record=?", id).Scan(&link)
			if e != nil {
				v.Visible = false
				v.Reason = "pending"
				result[id] = v
				continue
			}
			links[id] = link
			ids = append(ids, link)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if fresh == nil {
		if mode == Strict {
			for id, v := range result {
				if v.Visible {
					v.Visible = false
					v.Reason = "fresh_check_unavailable"
					result[id] = v
				}
			}
		}
		return result, nil
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	checks, checkErr := fresh.VerifyFresh(bounded, ids)
	// Re-check lifecycle after network work; a concurrent withdrawal must hide immediately.
	err = target.Read(ctx, func(q memory.Queryer) error {
		for id := range result {
			v, e := TargetVisible(ctx, q, target, id)
			if e != nil {
				return e
			}
			f, ok := checks[links[id]]
			// Bind this request's proof to the exact source revision carried by the target.
			var scope string
			var k records.Knowledge
			match := q.QueryRowContext(ctx, "SELECT scope FROM knowledge WHERE record=?", id).Scan(&scope) == nil
			if match {
				hh, del, e := target.Heads(ctx, q, scope, id)
				match = e == nil && !del && len(hh) == 1
				if match {
					match = json.Unmarshal(hh[0].Body, &k) == nil && k.Source == f.source && hh[0].Vault == f.target.Vault && scope == f.target.Scope && id == f.target.Record && hh[0].ID == f.target.Revision && k.LinkID == links[id] && f.link == links[id]
				}
			}
			v.Current = v.Visible && checkErr == nil && ok && f.Current && match
			if v.Current {
				v.Guard = f.guard
				v.LastVerifiedAt = f.VerifiedAt
				v.LastVerifiedHeads = f.VerifiedHeads
			}
			if !v.Current && v.Visible {
				v.Reason = f.Reason
				if checkErr != nil || !ok {
					v.Reason = "fresh_check_unavailable"
				}
				if v.Reason == "" {
					v.Reason = "not_current"
				}
			}
			if mode == Strict && !v.Current {
				v.Visible = false
				if v.Reason == "" {
					v.Reason = "not_current"
				}
			}
			result[id] = v
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	for id, v := range result {
		if v.Current && (v.Guard == nil || !v.Guard(ctx)) {
			v.Current = false
			if mode == Strict {
				v.Visible = false
				v.Reason = "not_current"
			}
			result[id] = v
		}
	}
	return result, nil
}
func (b *Bridge) VerifyFresh(ctx context.Context, links []string) (map[string]FreshResult, error) {
	policy, policyErr := b.sess.Policy(ctx)
	if policyErr != nil {
		return nil, policyErr
	}
	out := map[string]FreshResult{}
	if len(links) > 1000 {
		return out, memory.ErrInvalid
	}
	type dependency struct {
		ref      records.SourceRef
		evidence bool
	}
	type check struct {
		source, target records.ScopedRef
		deps           []dependency
		generation     int64
	}
	checks := map[string]check{}
	scopes := map[string]bool{}
	err := b.source.Read(ctx, func(q memory.Queryer) error {
		for _, link := range links {
			out[link] = FreshResult{Reason: "not_current"}
			p, e := b.sourceProjection(ctx, q, link)
			if e != nil || p.OwnerWithdrawn || p.State == "withdrawn" || p.State == "hidden" {
				continue
			}
			if p.Source.Vault != p.Target.Vault && policy.CrossVault != "enabled_unaccepted" {
				out[link] = FreshResult{Reason: "policy_disabled"}
				continue
			}
			deps := []dependency{}
			for _, ref := range p.Dependencies {
				deps = append(deps, dependency{ref: ref, evidence: true})
			}
			deps = append(deps, dependency{ref: records.SourceRef{Record: p.Source.Record, Revision: p.Source.Revision, Kind: records.KindInsight}, evidence: true})
			valid := true
			seen := map[dependency]bool{}
			expanded := []dependency{}
			for len(deps) > 0 {
				dep := deps[0]
				deps = deps[1:]
				if seen[dep] {
					continue
				}
				seen[dep] = true
				d := dep.ref
				if len(seen) > 100000 {
					return memory.ErrStaleOverflow
				}
				r, e := b.source.Revision(ctx, q, d.Revision)
				if e != nil || r.Record != d.Record || r.Vault != p.Source.Vault || (d.Record == p.Source.Record && d.Revision == p.Source.Revision && r.Scope != p.Source.Scope) {
					valid = false
					break
				}
				scopes[r.Scope] = true
				expanded = append(expanded, dep)
				rows, e := q.QueryContext(ctx, "SELECT d.parent,d.parent_revision,d.event_from,d.event_to,d.evidence,COALESCE(r.kind,'') FROM deps d LEFT JOIN revisions r ON r.revision=d.parent_revision WHERE d.child_revision=?", d.Revision)
				if e != nil {
					return e
				}
				for rows.Next() {
					var parent records.SourceRef
					var evidence bool
					if e = rows.Scan(&parent.Record, &parent.Revision, &parent.EventFrom, &parent.EventTo, &evidence, &parent.Kind); e != nil {
						rows.Close()
						return e
					}
					deps = append(deps, dependency{ref: parent, evidence: evidence})
				}
				e = rows.Err()
				rows.Close()
				if e != nil {
					return e
				}
			}
			if valid {
				checks[link] = check{p.Source, p.Target, expanded, p.Generation}
			}
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	ordered := []string{}
	for scope := range scopes {
		ordered = append(ordered, scope)
	}
	sort.Strings(ordered)
	heads := map[string]string{}
	for _, scope := range ordered {
		res, e := b.source.SyncScope(ctx, scope, memory.SyncOptions{ReplayOnly: true, MaxPages: 100})
		if e != nil || !res.CaughtUp {
			return out, nil
		}
		heads[scope] = res.Coverage.Head
	}
	err = b.source.Read(ctx, func(q memory.Queryer) error {
		var raw []byte
		policyErr := q.QueryRowContext(ctx, "SELECT body FROM policy WHERE name='session'").Scan(&raw)
		if policyErr != nil && policyErr != sql.ErrNoRows {
			return policyErr
		}
		currentPolicy := session.Policy{CrossVault: "off"}
		if policyErr == nil && json.Unmarshal(raw, &currentPolicy) != nil {
			return memory.ErrInvalid
		}
		guard, e := b.source.SnapshotFence(ctx, q)
		if e != nil {
			return e
		}
		for link, c := range checks {
			latest, e := b.sourceProjection(ctx, q, link)
			current := e == nil && latest.Source == c.source && latest.Target == c.target && latest.Generation == c.generation && !latest.OwnerWithdrawn && (latest.Source.Vault == latest.Target.Vault || currentPolicy.CrossVault == "enabled_unaccepted") && latest.State != "withdrawn" && latest.State != "hidden"
			used := map[string]bool{}
			for _, dep := range c.deps {
				d := dep.ref
				r, e := b.source.Revision(ctx, q, d.Revision)
				if e != nil || b.source.CheckLocalScope(ctx, q, r.Scope) != nil {
					current = false
					break
				}
				visible, e := memory.RefVisible(ctx, q, d, dep.evidence)
				if e != nil || !visible {
					current = false
					break
				}
				hh, deleted, e := b.source.Heads(ctx, q, r.Scope, r.Record)
				if e != nil || deleted || len(hh) != 1 || hh[0].ID != d.Revision {
					current = false
					break
				}
				used[r.Scope] = true
			}
			if current && memory.CheckConsent(ctx, q, latest.Dependencies) != nil {
				current = false
			}
			if current {
				refs := []ScopeHead{}
				for _, scope := range ordered {
					if used[scope] {
						refs = append(refs, ScopeHead{Scope: scope, Head: heads[scope]})
					}
				}
				out[link] = FreshResult{Current: true, VerifiedHeads: refs, VerifiedAt: time.Now().UTC().Format(time.RFC3339Nano), source: c.source, target: c.target, link: link, guard: guard}
			}
		}
		return nil
	})
	return out, err
}
func (b *Bridge) sourceProjection(ctx context.Context, q memory.Queryer, link string) (records.Projection, error) {
	if terminal, e := localTerminal(ctx, q, link); e != nil {
		return records.Projection{}, e
	} else if terminal != nil {
		return *terminal, nil
	}
	var src, dst records.ScopedRef
	bindingErr := q.QueryRowContext(ctx, "SELECT source_vault,source_scope,source_record,target_vault,target_scope,target_record FROM projections WHERE link_id=?", link).Scan(&src.Vault, &src.Scope, &src.Record, &dst.Vault, &dst.Scope, &dst.Record)
	if bindingErr != nil && bindingErr != sql.ErrNoRows {
		return records.Projection{}, bindingErr
	}
	all, err := linkHeads(ctx, q, b.source, link, "source", "", "")
	if err != nil {
		return records.Projection{}, err
	}
	var found *records.Projection
	count := 0
	for _, r := range all {
		var p records.Projection
		if json.Unmarshal(r.Body, &p) != nil {
			return records.Projection{}, memory.ErrInvalid
		}
		if p.Role == "source" && p.LinkID == link {
			if bindingErr == nil && (p.Source.Vault != src.Vault || p.Source.Scope != src.Scope || p.Source.Record != src.Record || p.Target.Vault != dst.Vault || p.Target.Scope != dst.Scope || p.Target.Record != dst.Record) {
				continue
			}
			count++
			if found != nil && (found.Source.Vault != p.Source.Vault || found.Source.Scope != p.Source.Scope || found.Source.Record != p.Source.Record || found.Target.Vault != p.Target.Vault || found.Target.Scope != p.Target.Scope || found.Target.Record != p.Target.Record) {
				return records.Projection{}, memory.ErrInvalid
			}
			copy := p
			found = &copy
		}
	}
	if found == nil {
		return records.Projection{}, memory.ErrInvalid
	}
	terminal, e := terminalProjection(ctx, q, b.source, *found)
	if e != nil {
		return records.Projection{}, e
	}
	if terminal != nil {
		found = terminal
		found.OwnerWithdrawn = true
		found.State = "withdrawn"
	} else if count != 1 {
		return records.Projection{}, memory.ErrInvalid
	}
	return *found, nil
}
