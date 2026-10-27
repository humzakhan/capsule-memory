package memory

import (
 "context"

 "github.com/humzakhan/capsule-memory/internal/records"
)

// RefVisible checks the exact revision and interval in the caller's snapshot.
// Coverage is exempt only from event retraction, never from revision freshness.
func RefVisible(ctx context.Context,q Queryer,ref records.SourceRef,evidence bool)(bool,error){
 visible,e:=IsVisible(ctx,q,ref.Record);if e!=nil||!visible{return false,e}
 var n int;e=q.QueryRowContext(ctx,`SELECT count(*) FROM revisions r WHERE r.record=? AND r.revision=? AND r.kind=? AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=r.revision) AND (SELECT count(*) FROM revisions h WHERE h.record=r.record AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=h.revision))=1`,ref.Record,ref.Revision,ref.Kind).Scan(&n);if e!=nil{return false,ErrStorage};if n!=1{return false,nil}
 flag:=0;if evidence{flag=1};e=q.QueryRowContext(ctx,`SELECT count(*) FROM (SELECT ? AS parent_revision,? AS event_from,? AS event_to,? AS evidence) d WHERE `+invalidRangeSQL+` OR `+retractedDepSQL,ref.Revision,ref.EventFrom,ref.EventTo,flag).Scan(&n);if e!=nil{return false,ErrStorage};return n==0,nil
}
