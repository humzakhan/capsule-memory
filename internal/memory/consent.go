package memory

import (
 "context"
 "database/sql"
 "errors"
 "encoding/json"
 "github.com/humzakhan/capsule-memory/internal/records"
)

var ErrConsentWithdrawn=errors.New("source consent withdrawn; automatic work held")
var ErrConsentUnavailable=errors.New("source consent unavailable; open the source profile before publishing automatic descendants")
var ErrUnpublishedParent=errors.New("parent publication unconfirmed; revision retained without releasing private ancestry")

// Direct sync without a source router retains cross-vault descendants locally.
func(s *Service) SetPublicationCheck(fn func(context.Context,records.ScopedRef)error){s.configMu.Lock();defer s.configMu.Unlock();s.publicationCheck=fn}
func(s *Service) checkPublication(ctx context.Context,id string)error{
 r,e:=s.Revision(ctx,s.reader,id);if e!=nil{return e}
 // Relay replay requires every same-record parent first. A privacy hold cannot
 // be bypassed by sending a child that would poison replay on another device.
 for _,parent:=range r.Parents{var pending int;if e=s.reader.QueryRowContext(ctx,"SELECT count(*) FROM outbox WHERE revision=? AND receipt IS NULL",parent).Scan(&pending);e!=nil{return ErrStorage};if pending!=0{return ErrUnpublishedParent}}
 // Cleanup carries no newly exported assertion and must survive source loss.
 if r.Operation=="delete"{return nil};if r.Kind==records.KindProjection{var p records.Projection;if json.Unmarshal(r.Body,&p)!=nil{return ErrInvalid};if p.WithdrawalMarker{return s.VerifyProjectionMarker(ctx,s.reader,r,p,true)};if p.State=="hidden"||p.State=="withdrawn"{return nil}}
 if e:=CheckConsent(ctx,s.reader,[]records.SourceRef{{Revision:id}});e!=nil{return e}
 rows,e:=s.reader.QueryContext(ctx,`WITH RECURSIVE ancestry(revision) AS (SELECT ? UNION SELECT d.parent_revision FROM deps d JOIN ancestry a ON d.child_revision=a.revision) SELECT r.body FROM ancestry a JOIN revisions r ON r.revision=a.revision LIMIT 100001`,id);if e!=nil{return ErrStorage}
 var page []Revision;for rows.Next(){var raw []byte;var r Revision;if rows.Scan(&raw)!=nil||json.Unmarshal(raw,&r)!=nil{rows.Close();return ErrStorage};page=append(page,r)};e=rows.Err();rows.Close();if e!=nil{return ErrStorage};if len(page)>100000{return ErrStaleOverflow}
 s.configMu.RLock();check:=s.publicationCheck;s.configMu.RUnlock()
 for _,r:=range page{if r.Operation=="delete"{continue};var ref records.ScopedRef
  switch r.Kind{case records.KindKnowledge:var k records.Knowledge;if json.Unmarshal(r.Body,&k)!=nil{return ErrInvalid};ref=k.Source
  case records.KindProjection:var p records.Projection;if json.Unmarshal(r.Body,&p)!=nil{return ErrInvalid};if p.Role!="target"||p.State=="hidden"||p.State=="withdrawn"{continue};ref=p.Source
  default:continue}
  if ref.Vault==r.Vault{continue};if check==nil{return ErrConsentUnavailable};if e=check(ctx,ref);e!=nil{return e}
 };return nil
}

// Durable roots refer to exact captured revisions, so enabling a source again
// grants no release of earlier consent. Derived indexes can be rebuilt freely.
func(s *Service) HoldSource(ctx context.Context,tx *sql.Tx,source string)error{
 if _,e:=tx.ExecContext(ctx,"CREATE TEMP TABLE IF NOT EXISTS consent_affected(revision TEXT PRIMARY KEY); DELETE FROM consent_affected");e!=nil{return ErrStorage}
 if _,e:=tx.ExecContext(ctx,`WITH RECURSIVE affected(revision) AS (SELECT revision FROM outbox_source WHERE source_id=? UNION SELECT d.child_revision FROM deps d JOIN affected a ON d.parent_revision=a.revision) INSERT INTO consent_affected SELECT revision FROM affected LIMIT 100001`,source);e!=nil{return ErrStorage}
 var n int;if tx.QueryRowContext(ctx,"SELECT count(*) FROM consent_affected").Scan(&n)!=nil{return ErrStorage};if n>100000{return ErrStaleOverflow}
 if _,e:=tx.ExecContext(ctx,"INSERT OR IGNORE INTO consent_roots SELECT revision,? FROM outbox_source WHERE source_id=?",source,source);e!=nil{return ErrStorage}
 if _,e:=tx.ExecContext(ctx,"INSERT OR IGNORE INTO outbox_hold SELECT o.revision,'consent_withdrawn' FROM outbox o JOIN consent_affected a ON a.revision=o.revision JOIN revisions r ON r.revision=o.revision WHERE o.receipt IS NULL AND json_extract(r.body,'$.operation')!='delete' AND NOT (r.kind='projection' AND json_extract(r.body,'$.body.state') IN ('hidden','withdrawn'))");e!=nil{return ErrStorage}
 _,e:=tx.ExecContext(ctx,`UPDATE jobs SET state='cancelled',error_code='consent_withdrawn',attempt_token=NULL,lease_until=NULL WHERE state IN ('pending','leased','blocked') AND (EXISTS(SELECT 1 FROM json_each(jobs.inputs) i JOIN consent_affected a ON a.revision=json_extract(i.value,'$.revision')) OR (kind='bridge_project' AND subject IN (SELECT link_id FROM projections WHERE source_revision IN (SELECT revision FROM consent_affected))))`);return e
}

func CheckConsent(ctx context.Context,q Queryer,refs []records.SourceRef)error{
 raw,e:=json.Marshal(refs);if e!=nil{return ErrInvalid}
 var count,held,missing int
 e=q.QueryRowContext(ctx,`WITH RECURSIVE ancestry(revision) AS (
 SELECT json_extract(value,'$.revision') FROM json_each(?)
 UNION SELECT d.parent_revision FROM deps d JOIN ancestry a ON d.child_revision=a.revision
 ), bounded AS (SELECT revision FROM ancestry LIMIT 100001)
 SELECT count(*),COALESCE(max(EXISTS(SELECT 1 FROM consent_roots c WHERE c.revision=bounded.revision)),0),COALESCE(max(NOT EXISTS(SELECT 1 FROM revisions r WHERE r.revision=bounded.revision)),0) FROM bounded`,raw).Scan(&count,&held,&missing)
 if e!=nil{return ErrStorage};if count>100000{return ErrStaleOverflow};if held!=0{return ErrConsentWithdrawn};if missing!=0{return ErrMissingRevision};return nil
}
