package derive

import (
	"context"
	"encoding/json"
	"strings"
	"sort"
	"errors"
	"time"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/projection"
	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/session"
)

type KnowledgeVisibility struct {Guard func(context.Context)bool;Revision string;Visible,Current bool;LastVerifiedAt string;LastVerifiedHeads []records.ScopeHead;Reason string}
type KnowledgeResolver func(context.Context,[]string,string)(map[string]KnowledgeVisibility,error)
func (r *Runner) SetKnowledgeResolver(fn KnowledgeResolver){r.mu.Lock();defer r.mu.Unlock();r.resolver=fn}
type KnowledgeResult struct {Results []KnowledgeHit `json:"results"`;Coverage session.Coverage `json:"coverage"`}
type KnowledgeHit struct {
	Kind,Record,Revision,Level,State string
	Text string
	Generated,Accepted,Stale,Conflict bool
	Contradictions int
	Evidence []EvidenceView
	Current bool `json:"current"`
 Reason string `json:"reason,omitempty"`
	LastVerifiedAt string `json:"last_verified_at"`
	LastVerifiedHeads []records.ScopeHead `json:"last_verified_heads"`
	Untrusted bool `json:"untrusted"`
	Coverage session.Coverage `json:"coverage"`
}
func (r *Runner) SearchKnowledge(ctx context.Context,g ops.Grant,query string,limit int)(KnowledgeResult,error){return r.SearchKnowledgeMode(ctx,g,query,limit,"historical")}
// Each scope/kind has an independent ranked cursor. Filtered lifecycle rows do
// not consume the requested result count or hide a different kind's candidates.
func (r *Runner) SearchKnowledgeMode(ctx context.Context, g ops.Grant, query string, limit int, mode string) (KnowledgeResult, error) {
	result := KnowledgeResult{Results: []KnowledgeHit{}, Coverage: session.Coverage{Complete: false}}
	if !g.Has(ops.CapKnowledge) {
		return result, ops.ErrForbidden
	}
	if mode != "historical" && mode != "strict" {
		return result, memory.ErrInvalid
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if len(query) > 4096 {
		return result, memory.ErrInvalid
	}
	fts := projection.SearchQuery(query)
	if fts == "" {
		return result, nil
	}
	if mode == "strict" {
		bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		ctx = bounded
	}
	r.mu.RLock()
	resolver := r.resolver
	r.mu.RUnlock()
	var searchFence func(context.Context)bool
	if err := r.svc.Read(ctx, func(q memory.Queryer) error {
		var err error
		searchFence, err = r.svc.SnapshotFence(ctx, q)
		return err
	}); err != nil {
		return result, err
	}
	ranks := map[string]float64{}
	guards := map[string]func(context.Context)bool{}
	seenScopes := map[string]bool{}
	for _, scope := range g.Scopes {
		if seenScopes[scope] {
			continue
		}
		seenScopes[scope] = true
		err := r.svc.Read(ctx, func(q memory.Queryer) error {
			status, err := r.svc.ScopeStatusQuery(ctx, q, scope)
			if err != nil {
				return err
			}
			if !status.Coverage.Complete {
				result.Coverage.Issues = append(result.Coverage.Issues, "scope_not_caught_up")
			}
			return nil
		})
		if err != nil {
			return result, err
		}
		for _, kind := range []string{records.KindSummary, records.KindInsight, records.KindKnowledge} {
			var cursor *searchCandidate
			accepted, scanned := 0, 0
			for accepted < limit {
				var page []searchCandidate
				views := map[string]KnowledgeHit{}
				err = r.svc.Read(ctx, func(q memory.Queryer) error {
					var err error
					page, err = searchPage(ctx, q, scope, kind, fts, cursor)
					if err != nil {
						return err
					}
					for _, candidate := range page {
						view, visible, err := r.searchView(ctx, q, g, candidate)
						if err != nil {
							return err
						}
						if visible {
							views[candidate.revision] = view
						}
					}
					return nil
				})
				if err != nil {
					return result, err
				}
				if len(page) == 0 {
					break
				}
				// Source freshness may perform network I/O only after Read closes.
				if kind == records.KindKnowledge {
					if err = r.resolvePage(ctx, page, views, guards, resolver, mode, &result.Coverage); err != nil {
						return result, err
					}
				}
				for _, candidate := range page {
					view, ok := views[candidate.revision]
					if !ok {
						continue
					}
					ranks[view.Revision] = candidate.rank
					result.Results = append(result.Results, view)
					accepted++
				}
				scanned += len(page)
				last := page[len(page)-1]
				cursor = &last
				if len(page) < 64 {
					break
				}
				// Bound rejected/lifecycle-hidden work per stream, not globally.
				// A bounded result is explicitly incomplete and the caller may retry.
				if scanned >= 4096 {
					result.Coverage.Issues = append(result.Coverage.Issues, "query_candidate_bound")
					break
				}
			}
		}
	}

	// A proof cannot authorize a row replaced during network work. Re-read all
	// returned revisions, lifecycle, input grants and aggregation in one snapshot.
	final := []KnowledgeHit{}
	err := r.svc.Read(ctx, func(q memory.Queryer) error {
		for _, hit := range result.Results {
			rev, err := r.svc.Revision(ctx, q, hit.Revision)
			if err != nil {
				return err
			}
			candidate := searchCandidate{scope: rev.Scope, record: hit.Record, revision: hit.Revision, kind: hit.Kind}
			current, visible, err := r.searchView(ctx, q, g, candidate)
			if err != nil {
				return err
			}
			if !visible {
				result.Coverage.Issues = append(result.Coverage.Issues, "query_changed")
				continue
			}
			hit.Conflict, hit.Evidence = current.Conflict, current.Evidence
			if !hit.Current {
				hit.LastVerifiedAt, hit.LastVerifiedHeads = current.LastVerifiedAt, current.LastVerifiedHeads
			}
			final = append(final, hit)
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	result.Results = nil
	for _, hit := range final {
		if hit.Kind == records.KindKnowledge && hit.Current {
			guard := guards[hit.Record]
			if guard == nil || !guard(ctx) {
				hit.Current = false
				hit.Reason = "not_current"
				if mode == "strict" {
					result.Coverage.Issues = append(result.Coverage.Issues, "query_changed")
					continue
				}
			}
		}
		result.Results = append(result.Results, hit)
	}
	sort.SliceStable(result.Results, func(i, j int) bool {
		a, b := result.Results[i], result.Results[j]
		if ranks[a.Revision] != ranks[b.Revision] {
			return ranks[a.Revision] < ranks[b.Revision]
		}
		return a.Revision < b.Revision
	})
	if len(result.Results) > limit {
		result.Results = result.Results[:limit]
	}
	// Ranked keyset pages use different snapshots around source I/O. Changes
	// cannot silently turn the merged scan into a claim of complete coverage.
	if searchFence == nil || !searchFence(ctx) {
		result.Coverage.Issues = append(result.Coverage.Issues, "query_changed")
	}
	result.Coverage.Complete = len(result.Coverage.Issues) == 0
	for _, hit := range result.Results {
		result.Coverage.SourceHeads = append(result.Coverage.SourceHeads, hit.Revision)
		if hit.Kind == records.KindKnowledge && !hit.Current || hit.Kind == records.KindSummary && !hit.Coverage.Complete {
			result.Coverage.Complete = false
		}
	}
	return result, nil
}

type searchCandidate struct {
	scope, record, revision, kind string
	rank float64
}

func searchPage(ctx context.Context, q memory.Queryer, scope, kind, query string, after *searchCandidate) ([]searchCandidate, error) {
	statement := `SELECT record,revision,kind,rank FROM session_search
WHERE scope=? AND session_search MATCH ? AND kind=?
AND (kind!='insight' OR EXISTS(SELECT 1 FROM revisions v WHERE v.revision=session_search.revision AND json_extract(v.body,'$.body.review') IN ('accepted','edited')))
AND NOT EXISTS(SELECT 1 FROM revisions d WHERE d.record=session_search.record AND d.scope=session_search.scope AND json_extract(d.body,'$.operation')='delete' AND NOT EXISTS(SELECT 1 FROM restores x WHERE x.deletion=d.revision))
AND NOT EXISTS(SELECT 1 FROM stale z WHERE z.record=session_search.record)
AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=session_search.revision)
AND NOT EXISTS(SELECT 1 FROM revisions v WHERE v.revision=session_search.revision AND v.kind='summary' AND json_extract(v.body,'$.body.level')='window')`
	args := []any{scope, query, kind}
	if after != nil {
		statement += " AND (rank>? OR (rank=? AND revision>?))"
		args = append(args, after.rank, after.rank, after.revision)
	}
	statement += " ORDER BY rank,revision LIMIT 64"
	rows, err := q.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := []searchCandidate{}
	for rows.Next() {
		candidate := searchCandidate{scope: scope}
		if err := rows.Scan(&candidate.record, &candidate.revision, &candidate.kind, &candidate.rank); err != nil {
			return nil, memory.ErrStorage
		}
		page = append(page, candidate)
	}
	return page, rows.Err()
}

func (r *Runner) searchView(ctx context.Context, q memory.Queryer, g ops.Grant, candidate searchCandidate) (KnowledgeHit, bool, error) {
	view := KnowledgeHit{Kind: candidate.kind, Record: candidate.record, Revision: candidate.revision, Untrusted: true, Evidence: []EvidenceView{}, LastVerifiedHeads: []records.ScopeHead{}}
	if err := r.svc.CheckLocalScope(ctx, q, candidate.scope); err != nil {
		return view, false, err
	}
	heads, deleted, err := r.svc.Heads(ctx, q, candidate.scope, candidate.record)
	if err != nil || deleted {
		return view, false, err
	}
	var rev memory.Revision
	for _, head := range heads {
		if head.ID == candidate.revision {
			rev = head
		}
	}
	if rev.ID == "" {
		return view, false, nil
	}
	view.Conflict = len(heads) > 1
	if candidate.kind == records.KindKnowledge {
		visibility, err := projection.TargetVisible(ctx, q, r.svc, candidate.record)
		if err != nil || !visibility.Visible || visibility.Revision != candidate.revision {
			return view, false, err
		}
		var knowledge records.Knowledge
		if json.Unmarshal(rev.Body, &knowledge) != nil {
			return view, false, memory.ErrInvalid
		}
		view.Text, view.State, view.Accepted = knowledge.Text, knowledge.Review, true
		view.LastVerifiedAt, view.LastVerifiedHeads = visibility.LastVerifiedAt, visibility.LastVerifiedHeads
		return view, true, nil
	}
	visible, err := memory.IsVisible(ctx, q, candidate.record)
	if err != nil || !visible {
		return view, false, err
	}
	if candidate.kind == records.KindInsight {
		var insight records.Insight
		if json.Unmarshal(rev.Body, &insight) != nil {
			return view, false, memory.ErrInvalid
		}
		if insight.Review != "accepted" && insight.Review != "edited" {
			return view, false, nil
		}
		view.Text, view.State, view.Accepted = insight.Assertion, insight.Review, true
		view.Generated, view.Contradictions = insight.Generator != nil, len(insight.Contradictions)
		view.Evidence = r.evidence(ctx, q, g, insight.Evidence)
		return view, true, nil
	}
	var summary records.Summary
	if json.Unmarshal(rev.Body, &summary) != nil {
		return view, false, memory.ErrInvalid
	}
	if summary.Level == "window" {
		return view, false, nil
	}
	if summary.Level == "personal_week" {
		if err := r.summaryGrant(ctx, q, candidate.scope, summary); err != nil {
			if errors.Is(err, projection.ErrReadersExceedSources) || errors.Is(err, memory.ErrInvalid) {
				return view, false, nil
			}
			return view, false, err
		}
	}
	text := []string{}
	for _, finding := range summary.Findings {
		text = append(text, finding.Text)
	}
	view.Text, view.Level, view.Generated, view.State = strings.Join(text, "\n"), summary.Level, true, "generated"
	view.Evidence = r.evidence(ctx, q, g, summary.Primary)
	view.Coverage.Gaps, view.Coverage.Complete = summary.Gaps, summary.Complete
	return view, true, nil
}

func (r *Runner) summaryGrant(ctx context.Context, q memory.Queryer, scope string, summary records.Summary) error {
	control, err := projection.SnapshotControl(ctx, q, r.svc)
	if err != nil {
		return err
	}
	rows, err := q.QueryContext(ctx, "SELECT record FROM aggregation_grants WHERE destination=? AND revoked=0 ORDER BY record LIMIT 10001", scope)
	if err != nil {
		return err
	}
	var grants []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return memory.ErrStorage
		}
		grants = append(grants, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(grants) > 10000 {
		return memory.ErrInvalid
	}
	return projection.CheckAggregation(ctx, q, r.svc, grants, summary.SourceHeads, scope, control)
}

func (r *Runner) resolvePage(ctx context.Context, page []searchCandidate, views map[string]KnowledgeHit, guards map[string]func(context.Context)bool, resolver KnowledgeResolver, mode string, coverage *session.Coverage) error {
	var ids []string
	for _, candidate := range page {
		if _, ok := views[candidate.revision]; ok {
			ids = append(ids, candidate.record)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var visibility map[string]KnowledgeVisibility
	var err error
	if resolver == nil {
		err = memory.ErrConsentUnavailable
	} else {
		visibility, err = resolver(ctx, ids, mode)
	}
	if err != nil {
		coverage.Issues = append(coverage.Issues, "knowledge_resolution_failed")
	}
	for _, candidate := range page {
		view, ok := views[candidate.revision]
		if !ok {
			continue
		}
		proof, found := visibility[candidate.record]
		if err != nil || !found {
			if !found && err == nil {
				coverage.Issues = append(coverage.Issues, "knowledge_proof_unavailable")
			}
			if mode == "strict" {
				delete(views, candidate.revision)
				continue
			}
			view.Reason = "fresh_check_unavailable"
		} else if !proof.Visible || proof.Revision != view.Revision || mode == "strict" && !proof.Current {
			delete(views, candidate.revision)
			continue
		} else {
			view.Current, view.Reason = proof.Current, proof.Reason
			view.LastVerifiedAt, view.LastVerifiedHeads = proof.LastVerifiedAt, proof.LastVerifiedHeads
			guards[view.Record] = proof.Guard
		}
		views[candidate.revision] = view
	}
	return ctx.Err()
}
