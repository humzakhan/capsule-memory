package projection

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

// Local coordination pins all logical identities. A lifecycle tombstone or a
// later conflicting body cannot erase a terminal intent already authenticated
// under that binding, even before its target receipt is recorded.
func localTerminal(ctx context.Context, q memory.Queryer, link string) (*records.Projection, error) {
	rows, e := q.QueryContext(ctx, `SELECT r.body FROM projections p JOIN projection_records i ON i.link_id=p.link_id AND i.role='source' JOIN revisions r ON r.record=i.record AND r.scope=i.scope
 WHERE p.link_id=? AND json_extract(r.body,'$.body.owner_withdrawn')=1
 AND json_extract(r.body,'$.body.role')='source' AND json_extract(r.body,'$.body.link_id')=p.link_id
 AND json_extract(r.body,'$.body.source.vault')=p.source_vault AND json_extract(r.body,'$.body.source.scope')=p.source_scope AND json_extract(r.body,'$.body.source.record')=p.source_record
 AND json_extract(r.body,'$.body.target.vault')=p.target_vault AND json_extract(r.body,'$.body.target.scope')=p.target_scope AND json_extract(r.body,'$.body.target.record')=p.target_record ORDER BY r.rowid LIMIT 1`, link)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var raw []byte
	var r memory.Revision
	var p records.Projection
	if rows.Scan(&raw) != nil || json.Unmarshal(raw, &r) != nil || json.Unmarshal(r.Body, &p) != nil {
		return nil, memory.ErrStorage
	}
	if p.State != "withdrawn" || r.Vault != p.Source.Vault || r.Scope != p.Source.Scope {
		return nil, memory.ErrInvalid
	}
	return &p, nil
}

func terminalLink(ctx context.Context, q memory.Queryer, svc *memory.Service, want records.Projection) (bool, error) {
	p, e := terminalProjection(ctx, q, svc, want)
	return p != nil, e
}
func terminalProjection(ctx context.Context, q memory.Queryer, svc *memory.Service, want records.Projection) (*records.Projection, error) {
	rows, e := q.QueryContext(ctx, `SELECT r.body FROM projection_records p JOIN revisions r ON r.record=p.record AND r.scope=p.scope WHERE p.link_id=? AND p.role=? AND json_extract(r.body,'$.body.owner_withdrawn')=1 ORDER BY r.rowid LIMIT 4097`, want.LinkID, want.Role)
	if e != nil {
		return nil, e
	}
	var revisions []memory.Revision
	for rows.Next() {
		var raw []byte
		var r memory.Revision
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &r) != nil {
			rows.Close()
			return nil, memory.ErrStorage
		}
		revisions = append(revisions, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if len(revisions) > 4096 {
		return nil, memory.ErrInvalid
	}
	for _, r := range revisions {
		var p records.Projection
		if json.Unmarshal(r.Body, &p) != nil {
			return nil, memory.ErrInvalid
		}
		if !p.OwnerWithdrawn || p.State != "withdrawn" || p.LinkID != want.LinkID || p.Role != want.Role || p.Source.Vault != want.Source.Vault || p.Source.Scope != want.Source.Scope || p.Source.Record != want.Source.Record || p.Target.Vault != want.Target.Vault || p.Target.Scope != want.Target.Scope || p.Target.Record != want.Target.Record {
			continue
		}
		if p.Role == "source" {
			if r.Vault == p.Source.Vault && r.Scope == p.Source.Scope {
				return &p, nil
			}
			continue
		}
		if r.Vault != p.Target.Vault || r.Scope != p.Target.Scope {
			continue
		}
		if p.Source.Vault == p.Target.Vault {
			c, e := SnapshotControl(ctx, q, svc)
			if e != nil {
				return nil, e
			}
			if HasScope(c, r.Author, p.Source.Scope) {
				return &p, nil
			}
			allowed, e := historicalScope(ctx, q, r.Author, p.Source.Scope)
			if e != nil {
				return nil, e
			}
			if allowed {
				return &p, nil
			}
			continue
		}
		// A later grant revocation cannot erase an already authenticated withdrawal.
		// Verify a historical owner grant for the exact author and vault/scope pair.
		grants, e := q.QueryContext(ctx, "SELECT body FROM revisions WHERE record=? AND scope=? AND kind='bridge_grant' LIMIT 4097", p.Grant, r.Scope)
		if e != nil {
			return nil, e
		}
		var rawGrants [][]byte
		for grants.Next() {
			var raw []byte
			if grants.Scan(&raw) != nil {
				grants.Close()
				return nil, memory.ErrStorage
			}
			rawGrants = append(rawGrants, raw)
		}
		e = grants.Err()
		grants.Close()
		if e != nil {
			return nil, e
		}
		if len(rawGrants) > 4096 {
			return nil, memory.ErrInvalid
		}
		for _, raw := range rawGrants {
			var grantRev memory.Revision
			var signed records.BridgeGrant
			var body records.BridgeGrantBody
			if json.Unmarshal(raw, &grantRev) != nil || json.Unmarshal(grantRev.Body, &signed) != nil {
				continue
			}
			if svc.VerifyOwner(bridgeDomain, memory.Signed{Payload: signed.Payload, Signature: signed.Signature}, &body) != nil {
				continue
			}
			if !body.Revoked && body.Validate() == nil && body.SourceVault == p.Source.Vault && body.TargetVault == p.Target.Vault && body.LedgerScope == r.Scope && body.Bridge == r.Author {
				return &p, nil
			}
		}
	}
	return nil, nil
}

// The cache locates records only. Current heads remain the authority, including
// every conflicting variant; a second writer cannot replace the first cache row.
func linkHeads(ctx context.Context, q memory.Queryer, svc *memory.Service, link, role, scope, target string) ([]memory.Revision, error) {
	rows, e := q.QueryContext(ctx, "SELECT DISTINCT scope,record FROM projection_records WHERE link_id=? AND role=? AND (?='' OR scope=?) AND (?='' OR target_record=?) ORDER BY scope,record LIMIT 1001", link, role, scope, scope, target, target)
	if e != nil {
		return nil, e
	}
	type key struct{ scope, record string }
	var keys []key
	for rows.Next() {
		var k key
		if e = rows.Scan(&k.scope, &k.record); e != nil {
			rows.Close()
			return nil, e
		}
		keys = append(keys, k)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if len(keys) > 1000 {
		return nil, memory.ErrInvalid
	}
	var out []memory.Revision
	for _, k := range keys {
		hh, del, e := svc.Heads(ctx, q, k.scope, k.record)
		if e != nil {
			return nil, e
		}
		if !del {
			out = append(out, hh...)
		}
	}
	return out, nil
}

func historicalScope(ctx context.Context, q memory.Queryer, author, scope string) (bool, error) {
	var after int64
	for {
		rows, e := q.QueryContext(ctx, "SELECT rowid,signed FROM controls WHERE rowid>? ORDER BY rowid LIMIT 16", after)
		if e != nil {
			return false, e
		}
		var values [][]byte
		for rows.Next() {
			var raw []byte
			if rows.Scan(&after, &raw) != nil {
				rows.Close()
				return false, memory.ErrStorage
			}
			values = append(values, raw)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return false, e
		}
		if len(values) == 0 {
			return false, nil
		}
		for _, raw := range values {
			var signed memory.Signed
			var c memory.Control
			if json.Unmarshal(raw, &signed) != nil {
				return false, memory.ErrStorage
			}
			payload, e := base64.StdEncoding.Strict().DecodeString(signed.Payload)
			if e != nil || json.Unmarshal(payload, &c) != nil {
				return false, memory.ErrStorage
			}
			if HasScope(c, author, scope) {
				return true, nil
			}
		}
	}
}
