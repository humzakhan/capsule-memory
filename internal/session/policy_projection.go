package session

import (
 "context"
 "database/sql"
 "strconv"
 "github.com/humzakhan/capsule-memory/internal/memory"
)

func(m *Module) withdrawCrossVault(ctx context.Context,tx *sql.Tx)error{
 rows,e:=tx.QueryContext(ctx,`SELECT link_id,source_scope FROM projections WHERE owner_withdrawn=0 AND source_vault!=target_vault LIMIT 100001`);if e!=nil{return e}
 type item struct{link,scope string};var items []item;for rows.Next(){var i item;if rows.Scan(&i.link,&i.scope)!=nil{rows.Close();return memory.ErrStorage};items=append(items,i)};e=rows.Err();rows.Close();if e!=nil{return e};if len(items)>100000{return memory.ErrStaleOverflow}
 for _,i:=range items{
  if _,e=tx.ExecContext(ctx,"UPDATE projections SET generation=generation+1,state='invalidating',error_code='cross_vault_promotion_off' WHERE link_id=?",i.link);e!=nil{return e}
  if _,e=tx.ExecContext(ctx,"UPDATE jobs SET state='cancelled',attempt_token=NULL,lease_until=NULL WHERE subject=? AND kind IN ('bridge_project','bridge_invalidate') AND state IN ('pending','leased','blocked')",i.link);e!=nil{return e}
  var generation int64;if e=tx.QueryRowContext(ctx,"SELECT generation FROM projections WHERE link_id=?",i.link).Scan(&generation);e!=nil{return e}
  if _,e=m.svc.EnqueueJob(ctx,tx,memory.JobSpec{Kind:"bridge_invalidate",Scope:i.scope,Subject:i.link,Destination:i.scope,Priority:0,Versions:strconv.FormatInt(generation,10)});e!=nil{return e}
 };return nil
}
