package projection

import (
 "context"
 "database/sql"
 "encoding/json"
 "strconv"
 "time"
 "github.com/humzakhan/capsule-memory/internal/memory"
 "github.com/humzakhan/capsule-memory/internal/records"
 "github.com/humzakhan/capsule-memory/internal/session"
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

// commitTarget separates source validation from the target transaction while
// retaining both local write owners. Source checks never call a target query.
func (b *Bridge) commitTarget(ctx context.Context, target *memory.Service, set []memory.Prepared, job *memory.Job, p records.Projection, extra func(context.Context, *sql.Tx) error) error {
	var deadline int64
	checkSource := func(ctx context.Context, q memory.Queryer) error {
		if err := b.checkTargetLease(ctx, q, job, p); err != nil {
			return err
		}
		if err := q.QueryRowContext(ctx, "SELECT lease_until FROM jobs WHERE job_key=? AND state='leased' AND attempt_token=?", job.Key, job.Token).Scan(&deadline); err != nil {
			return memory.ErrLeaseLost
		}
		return nil
	}
	targetExtra := func(ctx context.Context, tx *sql.Tx) error {
		if time.Now().UnixMilli() >= deadline {
			return memory.ErrLeaseLost
		}
		if err := extra(ctx, tx); err != nil {
			return err
		}
		// Target insertion, appliers and grant checks may themselves take time.
		if time.Now().UnixMilli() >= deadline {
			return memory.ErrLeaseLost
		}
		return ctx.Err()
	}
	return target.CommitWithSource(ctx, set, b.source, checkSource, targetExtra)
}

// The source query belongs to CommitWithSource's write exclusion window.
func (b *Bridge) checkTargetLease(ctx context.Context, q memory.Queryer, job *memory.Job, p records.Projection) error {
	if err := bridgeLease(ctx, q, job); err != nil {
		return err
	}
	if p.State != "projected" {
		return nil
	}
	if err := memory.CheckConsent(ctx, q, p.Dependencies); err != nil {
		return err
	}
	current, insight, err := b.insight(ctx, q, p.Source.Record)
	if err != nil {
		return err
	}
	if current.ID != p.Source.Revision || current.Vault != p.Source.Vault || current.Scope != p.Source.Scope || hashText(insight.Assertion) != p.TextSHA256 {
		return memory.ErrHeadsChanged
	}
	// Live replay happened before preparation. A control change in that gap
	// must also revoke access to any transitive input scope at commit.
	rows, err := q.QueryContext(ctx, `WITH RECURSIVE ancestry(revision) AS (
		SELECT ? UNION SELECT d.parent_revision FROM deps d JOIN ancestry a ON d.child_revision=a.revision
	) SELECT DISTINCT r.scope FROM ancestry a JOIN revisions r ON r.revision=a.revision LIMIT 65`, current.ID)
	if err != nil {
		return err
	}
	var scopes []string
	for rows.Next() {
		var scope string
		if err = rows.Scan(&scope); err != nil {
			rows.Close()
			return err
		}
		scopes = append(scopes, scope)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(scopes) > 64 {
		return memory.ErrInvalid
	}
	for _, scope := range scopes {
		if err = b.source.CheckLocalScope(ctx, q, scope); err != nil {
			return err
		}
	}
	// Re-read policy here: the earlier live check precedes target preparation.
	var raw []byte
	err = q.QueryRowContext(ctx, "SELECT body FROM policy WHERE name='session'").Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	policy := session.Policy{CrossVault: "off"}
	if err == nil && json.Unmarshal(raw, &policy) != nil {
		return memory.ErrInvalid
	}
	if p.Source.Vault != p.Target.Vault && policy.CrossVault != "enabled_unaccepted" {
		return memory.ErrConsentWithdrawn
	}
	return nil
}

func targetGeneration(ctx context.Context,q memory.Queryer,p records.Projection)error{
 var newest int64
 e:=q.QueryRowContext(ctx,`SELECT COALESCE(max(json_extract(r.body,'$.body.generation')),0) FROM projection_records i JOIN revisions r ON r.record=i.record AND r.scope=i.scope WHERE i.link_id=? AND i.role='target' AND i.scope=? AND i.target_record=? AND json_extract(r.body,'$.body.link_id')=? AND json_extract(r.body,'$.body.role')='target' AND json_extract(r.body,'$.body.target.vault')=? AND json_extract(r.body,'$.body.source.vault')=? AND json_extract(r.body,'$.body.source.scope')=? AND json_extract(r.body,'$.body.source.record')=?`,p.LinkID,p.Target.Scope,p.Target.Record,p.LinkID,p.Target.Vault,p.Source.Vault,p.Source.Scope,p.Source.Record).Scan(&newest);if e!=nil{return e};if newest>p.Generation{return memory.ErrLeaseLost};return nil
}
