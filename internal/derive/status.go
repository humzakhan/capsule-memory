package derive

import (
	"context"
	"database/sql"
	"time"

	"github.com/humzakhan/capsule-memory/internal/memory"
)

type BudgetStatus struct {InputTokens,OutputTokens,DailyInputBudget,DailyOutputBudget int}
type DeriveStatus struct {Enabled bool;Backlog map[string]int;Leased,Failed int;LastSummary string;Budget BudgetStatus;Blocked string;RejectedJobs,WithdrawalRequired int;CacheEviction string}
func (r *Runner) Status(ctx context.Context)(DeriveStatus,error){
	out:=DeriveStatus{Backlog:map[string]int{},CacheEviction:"not_implemented",Budget:BudgetStatus{DailyInputBudget:r.limits.DailyInputBudget,DailyOutputBudget:r.limits.DailyOutputBudget}}
	policy,err:=r.sess.Policy(ctx);if err!=nil{return out,err};out.Enabled=policy.Derive&&r.inf!=nil;if !policy.Derive{out.Blocked="derivation_disabled"}else if r.inf==nil{out.Blocked="inference_unconfigured"}
	err=r.svc.Read(ctx,func(q memory.Queryer)error{
		rows,err:=q.QueryContext(ctx,"SELECT kind,state,count(*) FROM jobs WHERE kind IN ('session_digest','project_rollup','personal_rollup') GROUP BY kind,state");if err!=nil{return err};for rows.Next(){var kind,state string;var count int;if err=rows.Scan(&kind,&state,&count);err!=nil{rows.Close();return err};if state=="pending"||state=="blocked"{out.Backlog[kind]+=count};if state=="leased"{out.Leased+=count};if state=="failed"{out.Failed+=count};if state=="blocked"{out.Blocked="budget"}};err=rows.Err();rows.Close();if err!=nil{return err}
		err=q.QueryRowContext(ctx,"SELECT input_tokens,output_tokens FROM budget WHERE day=?",time.Now().UTC().Format("2006-01-02")).Scan(&out.Budget.InputTokens,&out.Budget.OutputTokens);if err!=nil&&err!=sql.ErrNoRows{return err}
		err=q.QueryRowContext(ctx,"SELECT value FROM local_seq WHERE name='rejected_jobs'").Scan(&out.RejectedJobs);if err!=nil&&err!=sql.ErrNoRows{return err}
		if err=q.QueryRowContext(ctx,"SELECT count(DISTINCT r.record) FROM revisions r JOIN stale s ON s.record=r.record WHERE r.kind='insight' AND json_extract(r.body,'$.body.review') IN ('accepted','edited')").Scan(&out.WithdrawalRequired);err!=nil{return err}
		err=q.QueryRowContext(ctx,"SELECT json_extract(body,'$.created_at') FROM revisions WHERE kind='summary' ORDER BY revision DESC LIMIT 1").Scan(&out.LastSummary);if err!=nil&&err!=sql.ErrNoRows{return err};return nil
	});return out,err
}
