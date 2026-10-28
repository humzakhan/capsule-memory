package session

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/records"
)

func (m *Module) Search(ctx context.Context, grant Grant, query string, limit int) (SearchResult, error) {
	result := SearchResult{Results: []SearchHit{}}
	if !grant.Has(ops.CapSessionRead) || len(grant.Scopes) > 128 {
		return result, ops.ErrForbidden
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	err := m.svc.Read(ctx, func(q memory.Queryer) error {
		var scopes []string
		var placeholders []string
		arguments := []any{query}
		seen := map[string]bool{}
		for _, scope := range grant.Scopes {
			if seen[scope] {
				continue
			}
			seen[scope] = true
			if err := m.svc.CheckLocalScope(ctx, q, scope); err != nil {
				return ops.ErrForbidden
			}
			scopes = append(scopes, scope)
			placeholders = append(placeholders, "?")
			arguments = append(arguments, scope)
		}
		if len(scopes) == 0 {
			return nil
		}
		// Rank all granted scopes together. Visibility checks happen before
		// counting results; an invisible page cannot consume the caller's limit.
		statement := `SELECT canonical.session,f.record,f.revision,
			snippet(session_search,5,'','', ' … ',24) FROM session_search f
			JOIN sessions s ON s.session=f.session AND s.scope=f.scope AND s.deleted=0
			JOIN sessions canonical ON canonical.session=CASE WHEN COALESCE(s.view_of,'')=''
				THEN s.session ELSE s.view_of END AND canonical.scope=s.scope AND canonical.deleted=0
			JOIN revisions r ON r.revision=f.revision AND r.record=f.record AND r.scope=f.scope
			WHERE session_search MATCH ? AND f.scope IN (` + strings.Join(placeholders, ",") + `)
			AND f.kind='session_chunk' AND json_extract(r.body,'$.operation')!='delete'
			AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=r.revision)
			AND (SELECT count(*) FROM revisions h WHERE h.record=r.record
				AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=h.revision))=1
			ORDER BY rank,f.record,f.revision,f.rowid LIMIT 64 OFFSET ?`
		exhausted := false
		for offset := 0; offset < 100000 && len(result.Results) < limit; offset += 64 {
			args := append(append([]any{}, arguments...), offset)
			rows, err := q.QueryContext(ctx, statement, args...)
			if err != nil {
				return memory.ErrInvalid
			}
			var page []SearchHit
			for rows.Next() {
				var hit SearchHit
				if err := rows.Scan(&hit.Session, &hit.Chunk, &hit.Revision, &hit.Snippet); err != nil {
					rows.Close()
					return memory.ErrStorage
				}
				hit.Untrusted = true
				page = append(page, hit)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return memory.ErrStorage
			}
			for _, hit := range page {
				visible, err := memory.IsVisible(ctx, q, hit.Chunk)
				if err != nil {
					return err
				}
				if visible {
					visible, err = memory.IsVisible(ctx, q, hit.Session)
					if err != nil {
						return err
					}
				}
				if visible && len(result.Results) < limit {
					result.Results = append(result.Results, hit)
				}
			}
			if len(page) < 64 {
				exhausted = true
				break
			}
		}
		var err error
		result.Coverage, err = m.coverageQuery(ctx, q, scopes)
		if err != nil {
			return err
		}
		if !exhausted && len(result.Results) < limit {
			result.Coverage.Complete = false
			result.Coverage.Issues = append(result.Coverage.Issues, "search_scan_bound")
		}
		return nil
	})
	if err != nil {
		return SearchResult{Results: []SearchHit{}}, err
	}
	return result, nil
}

func (m *Module) Events(ctx context.Context, q memory.Queryer, session, branch string, fromSeq int64, maxBytes int) ([]records.SessionChunk, error) {
	if fromSeq < 0 {
		return nil, memory.ErrInvalid
	}
	if maxBytes <= 0 || maxBytes > 4<<20 {
		maxBytes = 4 << 20
	}
	var scope string
	if err := q.QueryRowContext(ctx, "SELECT scope FROM sessions WHERE session=? AND deleted=0", session).Scan(&scope); err != nil {
		return nil, memory.ErrInvalid
	}
	if err := m.svc.CheckLocalScope(ctx, q, scope); err != nil {
		return nil, ops.ErrForbidden
	}
	visible, err := memory.IsVisible(ctx, q, session)
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, memory.ErrInvalid
	}
	var result []records.SessionChunk
	size := 0
	sequences := map[int64]bool{}
	for offset := 0; offset < 100000; offset += 4 {
		rows, err := q.QueryContext(ctx, `SELECT r.body,c.record FROM chunks c JOIN revisions r ON r.revision=c.revision
			JOIN sessions s ON s.session=c.session AND s.scope=r.scope AND s.deleted=0
			WHERE s.scope=? AND (s.session=? OR s.view_of=?) AND c.branch=? AND c.seq>=?
			AND json_extract(r.body,'$.operation')!='delete'
			AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=r.revision)
			AND (SELECT count(*) FROM revisions h WHERE h.record=r.record
				AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=h.revision))=1
			ORDER BY c.seq,c.record,c.revision LIMIT 4 OFFSET ?`, scope, session, session, branch, fromSeq, offset)
		if err != nil {
			return nil, err
		}
		type candidate struct {
			body []byte
			id string
		}
		var page []candidate
		for rows.Next() {
			var item candidate
			if err := rows.Scan(&item.body, &item.id); err != nil {
				rows.Close()
				return nil, memory.ErrStorage
			}
			page = append(page, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, memory.ErrStorage
		}
		for _, item := range page {
			visible, err := memory.IsVisible(ctx, q, item.id)
			if err != nil {
				return nil, err
			}
			if !visible {
				continue
			}
			var revision memory.Revision
			var chunk records.SessionChunk
			if json.Unmarshal(item.body, &revision) != nil || json.Unmarshal(revision.Body, &chunk) != nil {
				return nil, memory.ErrStorage
			}
			if sequences[chunk.Seq] {
				return nil, memory.ErrHeadsChanged
			}
			if size+len(item.body) > maxBytes {
				if len(result) == 0 {
					return nil, memory.ErrInvalid
				}
				return result, nil
			}
			sequences[chunk.Seq] = true
			size += len(item.body)
			result = append(result, chunk)
		}
		if len(page) < 4 {
			return result, nil
		}
	}
	return nil, memory.ErrStaleOverflow
}

func (m *Module) ReadSession(ctx context.Context, grant Grant, session, branch string, fromSeq int64, maxBytes int) (ReadResult, error) {
	result := ReadResult{}
	if !grant.Has(ops.CapSessionRead) {
		return result, ops.ErrForbidden
	}
	if branch == "" {
		branch = "main"
	}
	err := m.svc.Read(ctx, func(q memory.Queryer) error {
		var scope string
		err := q.QueryRowContext(ctx, `SELECT s.scope,canonical.session FROM sessions s
			JOIN sessions canonical ON canonical.session=CASE WHEN COALESCE(s.view_of,'')=''
				THEN s.session ELSE s.view_of END AND canonical.scope=s.scope AND canonical.deleted=0
			WHERE s.session=? AND s.deleted=0`, session).Scan(&scope, &session)
		if err != nil || !grant.AllowsScope(scope) {
			return ops.ErrForbidden
		}
		if err := m.svc.CheckLocalScope(ctx, q, scope); err != nil {
			return ops.ErrForbidden
		}
		rows, err := q.QueryContext(ctx, `SELECT DISTINCT c.branch FROM chunks c JOIN sessions s ON s.session=c.session
			WHERE s.scope=? AND s.deleted=0 AND (s.session=? OR s.view_of=?) ORDER BY c.branch`, scope, session, session)
		if err != nil {
			return err
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				return memory.ErrStorage
			}
			result.Branches = append(result.Branches, value)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if branch == "main" && len(result.Branches) > 0 {
			found := false
			for _, value := range result.Branches {
				found = found || value == branch
			}
			if !found {
				branch = result.Branches[0]
			}
		}
		result.Branch = branch
		result.Chunks, err = m.Events(ctx, q, session, branch, fromSeq, maxBytes)
		if err != nil {
			return err
		}
		result.Next = fromSeq
		contiguous := true
		for _, chunk := range result.Chunks {
			if chunk.Seq != result.Next {
				contiguous = false
			}
			result.Next = chunk.Seq + 1
		}
		result.Coverage, err = m.coverageQuery(ctx, q, []string{scope})
		if err != nil {
			return err
		}
		var high int64
		err = q.QueryRowContext(ctx, `SELECT COALESCE(max(c.seq),-1) FROM chunks c JOIN sessions s ON s.session=c.session
			WHERE s.scope=? AND s.deleted=0 AND (s.session=? OR s.view_of=?) AND c.branch=?`, scope, session, session, branch).Scan(&high)
		if err != nil {
			return err
		}
		result.Complete = contiguous && result.Coverage.Complete && result.Next > high
		return nil
	})
	if err != nil {
		return ReadResult{}, err
	}
	return result, nil
}
