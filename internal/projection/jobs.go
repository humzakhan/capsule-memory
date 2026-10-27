package projection

import (
 "context"
 "database/sql"
 "strconv"
 "time"
 "github.com/humzakhan/capsule-memory/internal/memory"
 "github.com/humzakhan/capsule-memory/internal/records"
)

// Each invalidation is a new generation. Generic completed-job dedup is unchanged.
func queueBridge(ctx context.Context,tx *sql.Tx,svc *memory.Service,scope,link,reason string)error{
 if _,e:=tx.ExecContext(ctx,"UPDATE projections SET generation=generation+1,state='invalidating',error_code=? WHERE link_id=?",reason,link);e!=nil{return e}
 if _,e:=tx.ExecContext(ctx,"UPDATE jobs SET state='cancelled',attempt_token=NULL,lease_until=NULL WHERE subject=? AND kind IN ('bridge_project','bridge_invalidate') AND state IN ('pending','leased','blocked')",link);e!=nil{return e}
 var generation int64;if e:=tx.QueryRowContext(ctx,"SELECT generation FROM projections WHERE link_id=?",link).Scan(&generation);e!=nil{return e}
 _,e:=svc.EnqueueJob(ctx,tx,memory.JobSpec{Kind:"bridge_invalidate",Scope:scope,Subject:link,Destination:scope,Priority:0,Versions:strconv.FormatInt(generation,10)});return e
}

func bridgeLease(ctx context.Context,q memory.Queryer,job *memory.Job)error{
 var generation int64;var terminal bool;var token string;var until int64
 if e:=q.QueryRowContext(ctx,"SELECT p.generation,p.owner_withdrawn,j.attempt_token,j.lease_until FROM projections p JOIN jobs j ON j.subject=p.link_id WHERE p.link_id=? AND j.job_key=? AND j.state='leased'",job.Spec.Subject,job.Key).Scan(&generation,&terminal,&token,&until);e!=nil{return memory.ErrLeaseLost}
 expected,e:=strconv.ParseInt(job.Spec.Versions,10,64);if e!=nil||generation!=expected||token!=job.Token||until<=time.Now().UnixMilli()||terminal&&job.Spec.Kind!="bridge_invalidate"{return memory.ErrLeaseLost};return nil
}

func(b *Bridge) checkTargetLease(ctx context.Context,target *memory.Service,tx *sql.Tx,job *memory.Job,p records.Projection)error{
 check:=func(q memory.Queryer)error{if e:=bridgeLease(ctx,q,job);e!=nil{return e};if p.State=="projected"{return memory.CheckConsent(ctx,q,p.Dependencies)};return nil}
 if target==b.source{return check(tx)}
 return b.source.Read(ctx,check)
}

func targetGeneration(ctx context.Context,q memory.Queryer,p records.Projection)error{
 var newest int64
 e:=q.QueryRowContext(ctx,`SELECT COALESCE(max(json_extract(r.body,'$.body.generation')),0) FROM projection_records i JOIN revisions r ON r.record=i.record AND r.scope=i.scope WHERE i.link_id=? AND i.role='target' AND i.scope=? AND i.target_record=? AND json_extract(r.body,'$.body.link_id')=? AND json_extract(r.body,'$.body.role')='target' AND json_extract(r.body,'$.body.target.vault')=? AND json_extract(r.body,'$.body.source.vault')=? AND json_extract(r.body,'$.body.source.scope')=? AND json_extract(r.body,'$.body.source.record')=?`,p.LinkID,p.Target.Scope,p.Target.Record,p.LinkID,p.Target.Vault,p.Source.Vault,p.Source.Scope,p.Source.Record).Scan(&newest);if e!=nil{return e};if newest>p.Generation{return memory.ErrLeaseLost};return nil
}
