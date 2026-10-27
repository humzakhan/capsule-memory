package derive

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"sort"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/humzakhan/capsule-memory/internal/inference"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/projection"
	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/session"
)

type Config struct { Device,RollupDevice string; Idle time.Duration; PromptVersion,PolicyVersion string; WindowBytes int }
type Runner struct {svc *memory.Service;sess *session.Module;inf inference.Client;cfg Config;limits inference.Config;mu sync.RWMutex;resolver KnowledgeResolver;runMu sync.Mutex}
func NewRunner(svc *memory.Service,sess *session.Module,inf inference.Client,cfg Config)*Runner{
	limits:=inference.Limits(inf);if cfg.Idle<=0{cfg.Idle=15*time.Minute};if cfg.PromptVersion==""{cfg.PromptVersion=PromptVersion};maxWindow:=limits.MaxInputTokens*3-len(SystemPrompt)-len(outputSchema)-1024;if maxWindow<256{maxWindow=256};if cfg.WindowBytes<=0||cfg.WindowBytes>maxWindow{cfg.WindowBytes=maxWindow};return &Runner{svc:svc,sess:sess,inf:inf,cfg:cfg,limits:limits}
}
func (r *Runner) RunOnce(ctx context.Context)(bool,error){
	r.runMu.Lock();defer r.runMu.Unlock()
	policy,err:=r.sess.Policy(ctx);if err!=nil{return false,err};if !policy.Derive||r.inf==nil{return false,nil}
	if err=r.svc.Update(ctx,func(tx *sql.Tx)error{if _,e:=tx.ExecContext(ctx,"UPDATE jobs SET state='pending',error_code=NULL WHERE state='blocked' AND error_code='budget' AND not_before<=?",time.Now().UnixMilli());e!=nil{return e};return r.checkPersonalReady(ctx,tx,policy)});err!=nil{return false,err}
	job,err:=r.svc.LeaseJob(ctx,[]string{"session_digest","project_rollup","personal_rollup"},10*time.Minute);if err!=nil||job==nil{return false,err}
	if job.Spec.Kind!="session_digest"&&policy.RollupDevice!=""&&policy.RollupDevice!=r.cfg.Device{err=errors.New("rollup_device_mismatch")}else{err=r.run(ctx,job,policy)}
	if err == nil {
		return true, nil
	}
	code := "derive_failed"
	permanent := job.Attempts >= 3
	switch {
	case errors.Is(err, ErrOutput):
		code = "invalid_output"
	case errors.Is(err, inference.ErrBudget):
		// Transition directly from this leased token. The former two-step
		// pending-to-blocked update could park another worker's retry.
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
		blockErr := r.svc.BlockJob(ctx, job.Key, job.Token, "budget", next)
		return true, errors.Join(err, blockErr)
	case errors.Is(err, projection.ErrReadersExceedSources):
		code = "destination_readers_exceed_sources"
		permanent = true
	case errors.Is(err, memory.ErrHeadsChanged):
		code = "inputs_changed"
		permanent = false
	case errors.Is(err, memory.ErrStorage), errors.Is(err, memory.ErrMissingRevision):
		code = "inputs_unavailable"
		permanent = false
	case errors.Is(err, memory.ErrConsentWithdrawn):
		code = "consent_withdrawn"
		permanent = true
	case errors.Is(err, memory.ErrInvalid):
		code = "authorization_failed"
		permanent = true
	}
	failErr := r.svc.FailJob(ctx, job.Key, job.Token, code, time.Now().Add(time.Minute), permanent)
	return true, errors.Join(err, failErr)
}

type preparedInput struct{lines []inputLine;known map[string]citation;primary []records.SourceRef;heads []records.SourceRef;gaps []records.Gap;project string;omissions []records.Omission;period time.Time}
func (r *Runner) inputs(ctx context.Context,q memory.Queryer,job *memory.Job)(preparedInput,error){
	out:=preparedInput{known:map[string]citation{},primary:[]records.SourceRef{},heads:[]records.SourceRef{},gaps:[]records.Gap{},omissions:[]records.Omission{},project:job.Spec.Destination}
	_ = q.QueryRowContext(ctx,"SELECT project_id FROM projects WHERE scope=? LIMIT 1",job.Spec.Scope).Scan(&out.project)
	if len(job.Spec.Inputs)==0||len(job.Spec.Inputs)>4096{return out,memory.ErrInvalid}
	if job.Spec.Kind=="session_digest"{var deleted int;var view sql.NullString;if err:=q.QueryRowContext(ctx,"SELECT project_id,deleted,view_of FROM sessions WHERE session=?",job.Spec.Subject).Scan(&out.project,&deleted,&view);err!=nil{return out,err};if deleted!=0||view.Valid&&view.String!=""{return out,memory.ErrInvalid}}
	seen:=map[string]bool{};retracted:=map[string]bool{};inputBytes:=0
	for _,ref:=range job.Spec.Inputs{
		rev,err:=r.svc.Revision(ctx,q,ref.Revision);if err!=nil{return out,err};if rev.Record!=ref.Record{return out,memory.ErrInvalid};visible,err:=memory.IsVisible(ctx,q,ref.Record);if err!=nil||!visible{return out,memory.ErrHeadsChanged};hh,del,err:=r.svc.Heads(ctx,q,rev.Scope,rev.Record);if err!=nil||del||len(hh)!=1||hh[0].ID!=ref.Revision{return out,memory.ErrHeadsChanged}
		inputBytes+=len(rev.Body);if inputBytes>64<<20{return out,inference.ErrBudget}
		if job.Spec.Kind!="personal_rollup"&&rev.Scope!=job.Spec.Scope{return out,memory.ErrInvalid};out.heads=append(out.heads,ref)
		switch rev.Kind{
		case records.KindSessionManifest:
			var m records.SessionManifest;if json.Unmarshal(rev.Body,&m)!=nil{return out,memory.ErrInvalid};out.gaps=append(out.gaps,m.Gaps...)
			owner,e:=r.sess.ManifestRoot(ctx,q,rev.Scope,rev.Record);if e!=nil{return out,e};for branch,high:=range m.ChunkHigh{var count int;if err=q.QueryRowContext(ctx,"SELECT count(DISTINCT seq) FROM chunks WHERE session=? AND branch=? AND seq BETWEEN 0 AND ?",owner,branch,high).Scan(&count);err!=nil{return out,err};if int64(count)!=high+1{return out,errors.New("incomplete")}}
		case records.KindSessionChunk:
			var chunk records.SessionChunk;if json.Unmarshal(rev.Body,&chunk)!=nil{return out,memory.ErrInvalid};out.omissions=append(out.omissions,chunk.Omissions...)
			if chunk.Session!=job.Spec.Subject{var parent sql.NullString;if q.QueryRowContext(ctx,"SELECT view_of FROM sessions WHERE session=?",chunk.Session).Scan(&parent)!=nil||!parent.Valid||parent.String!=job.Spec.Subject{return out,memory.ErrInvalid}}
			var nativeView int;if err=q.QueryRowContext(ctx,"SELECT count(*) FROM sessions s JOIN sessions canonical ON canonical.session=s.view_of WHERE s.session=? AND s.harness!='t3' AND canonical.harness='t3'",chunk.Session).Scan(&nativeView);err!=nil{return out,err};if nativeView>0{continue};for _,event:=range chunk.Events{for _,id:=range event.Retracts{retracted[id]=true}}
			for ordinal,event:=range chunk.Events{group:="";if event.Part!=nil{group=event.Part.Group};invalid,e:=memory.EventRetracted(ctx,q,chunk.Session,event.ID,group);if e!=nil{return out,e};if invalid{continue};if event.ViewOf!=""||event.Origin=="capsule_recall"||event.Type=="correction"{continue};id:=event.ID;if seen[id]{continue};seen[id]=true;source:=records.SourceRef{Record:rev.Record,Revision:rev.ID,Kind:rev.Kind,EventFrom:id,EventTo:id};out.known[id]=citation{Ordinal:ordinal,Ref:source,Role:event.Role,Type:event.Type,Primary:[]records.SourceRef{source}};out.lines=append(out.lines,inputLine{ID:id,Role:event.Role,Type:event.Type,Text:event.Text});out.primary=append(out.primary,source)}
		case records.KindSummary:
			var summary records.Summary;if json.Unmarshal(rev.Body,&summary)!=nil{return out,memory.ErrInvalid};if job.Spec.Kind=="project_rollup"&&summary.Level!="session"||job.Spec.Kind=="personal_rollup"&&summary.Level!="project_day"{return out,memory.ErrInvalid}
			stamp:=rev.Created;if summary.PeriodStart!=""{stamp=summary.PeriodStart};when,e:=time.Parse(time.RFC3339Nano,stamp);if e!=nil{return out,e};if out.period.IsZero()||when.Before(out.period){out.period=when}
			for i,f:=range summary.Findings{id:=rev.Record+"#"+strconv.Itoa(i);source:=records.SourceRef{Record:rev.Record,Revision:rev.ID,Kind:rev.Kind};out.known[id]=citation{Ref:source,Type:"summary",Primary:summary.Primary};out.lines=append(out.lines,inputLine{ID:id,Role:"assistant",Type:f.Kind,Text:f.Text})};out.primary=append(out.primary,summary.Primary...);out.gaps=append(out.gaps,summary.Gaps...)
		default:return out,memory.ErrInvalid
		}
	};if len(retracted)>0{kept:=[]inputLine{};for _,line:=range out.lines{if retracted[line.ID]{delete(out.known,line.ID);continue};kept=append(kept,line)};out.lines=kept;out.primary=nil;for _,line:=range kept{out.primary=append(out.primary,out.known[line.ID].Primary...)}}
	if len(out.lines)==0||len(out.primary)==0{return out,memory.ErrInvalid};return out,nil
}
func (r *Runner) authorize(ctx context.Context,q memory.Queryer,job *memory.Job,policy session.Policy)error{
	if e:=memory.CheckConsent(ctx,q,job.Spec.Inputs);e!=nil{return e};c,err:=projection.SnapshotControl(ctx,q,r.svc);if err!=nil{return err}
	device:=r.cfg.Device;if device==""{device=job.Spec.AssignedDevice};if !projection.HasScope(c,device,job.Spec.Destination){return memory.ErrInvalid}
	for _,in:=range job.Spec.Inputs{rev,e:=r.svc.Revision(ctx,q,in.Revision);if e!=nil{return e};if !projection.HasScope(c,device,rev.Scope){return memory.ErrInvalid}}
	// Replayed assignment authority can be withdrawn after enqueueing. Recheck
	// its signed author at execution and commit, rather than treating the queue as authority.
	rows,err:=q.QueryContext(ctx,"SELECT body FROM revisions WHERE kind='job' AND scope=? AND json_extract(body,'$.body.job_key')=? AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=revisions.revision) LIMIT 1001",job.Spec.Scope,job.Key);if err!=nil{return err};assignments:=[]memory.Revision{}
	for rows.Next(){var raw []byte;var rev memory.Revision;if err=rows.Scan(&raw);err!=nil{rows.Close();return err};if json.Unmarshal(raw,&rev)!=nil{rows.Close();return memory.ErrInvalid};assignments=append(assignments,rev)};err=rows.Err();rows.Close();if err!=nil{return err};if len(assignments)>1000{return memory.ErrInvalid}
	for _,rev:=range assignments{var assigned records.Job;if json.Unmarshal(rev.Body,&assigned)!=nil{return memory.ErrInvalid};if assigned.State!="requested"||assigned.AssignedDevice!=job.Spec.AssignedDevice{return memory.ErrInvalid};if err=authorizeJob(ctx,q,r.svc,rev,assigned);err!=nil{return err}}
	if job.Spec.Kind=="personal_rollup"{return projection.CheckAggregation(ctx,q,r.svc,policy.Aggregations,job.Spec.Inputs,job.Spec.Destination,c)};if job.Spec.Destination!=job.Spec.Scope{return memory.ErrInvalid};return nil
}
func stableID(s string)string{sum:=sha256.Sum256([]byte(s));return hex.EncodeToString(sum[:])}
func (r *Runner) reserve(ctx context.Context,job *memory.Job,input,output int)error{
	return r.svc.Update(ctx,func(tx *sql.Tx)error{
		var token string;var until int64;if tx.QueryRowContext(ctx,"SELECT attempt_token,lease_until FROM jobs WHERE job_key=? AND state='leased'",job.Key).Scan(&token,&until)!=nil||token!=job.Token||until<=time.Now().UnixMilli(){return memory.ErrLeaseLost}
		day:=time.Now().UTC().Format("2006-01-02");if _,err:=tx.ExecContext(ctx,"INSERT OR IGNORE INTO budget(day,input_tokens,output_tokens,jobs) VALUES(?,0,0,0)",day);err!=nil{return err}
		var usedIn,usedOut int;if err:=tx.QueryRowContext(ctx,"SELECT input_tokens,output_tokens FROM budget WHERE day=?",day).Scan(&usedIn,&usedOut);err!=nil{return err};if usedIn+input>r.limits.DailyInputBudget||usedOut+output>r.limits.DailyOutputBudget{return inference.ErrBudget}
		if _,err:=tx.ExecContext(ctx,"UPDATE budget SET input_tokens=input_tokens+?,output_tokens=output_tokens+?,jobs=jobs+1 WHERE day=?",input,output,day);err!=nil{return err}
		_,err:=tx.ExecContext(ctx,"UPDATE jobs SET lease_until=? WHERE job_key=? AND attempt_token=? AND state='leased'",time.Now().Add(10*time.Minute).UnixMilli(),job.Key,job.Token);return err
	})
}
func (r *Runner) call(ctx context.Context,job *memory.Job,lines []inputLine,known map[string]citation,omissions []records.Omission,rollup bool)(modelOutput,error){
	prompt:=SystemPrompt;name:="digest_v1";if rollup{prompt=RollupPrompt;name="rollup_v1"};text:=render(lines,omissions);estimated:=inference.EstimateTokens(prompt)+inference.EstimateTokens(text)+inference.EstimateTokens(string(outputSchema))+32
	if estimated>r.limits.MaxInputTokens{return modelOutput{},inference.ErrBudget}
	// Reserve the configured maximum before inference. Failures/crashes keep the reservation;
	// this conservative accounting prevents retries from silently exceeding daily budgets.
	if err:=r.reserve(ctx,job,r.limits.MaxInputTokens,r.limits.MaxOutputTokens);err!=nil{return modelOutput{},err}
	bounded,cancel:=context.WithTimeout(ctx,r.limits.Timeout);defer cancel();response,err:=r.inf.Complete(bounded,inference.Request{System:prompt,Messages:[]inference.Message{{Role:"user",Content:text}},Schema:outputSchema,SchemaName:name,MaxOutputTokens:r.limits.MaxOutputTokens});if err!=nil{return modelOutput{},err};return validate(response.Content,known)
}
func (r *Runner) run(ctx context.Context,job *memory.Job,policy session.Policy)error{
	var input preparedInput;err:=r.svc.Read(ctx,func(q memory.Queryer)error{if e:=r.authorize(ctx,q,job,policy);e!=nil{return e};var e error;input,e=r.inputs(ctx,q,job);return e});if err!=nil{return err}
	generator:=records.Generator{Model:r.limits.Model,PromptVersion:PromptVersion,PolicyVersion:strconv.Itoa(policy.Version),Device:job.Spec.AssignedDevice,JobKey:job.Key};if generator.Model==""{generator.Model="configured-local-model"};if job.Spec.Kind!="session_digest"{generator.PromptVersion=RollupVersion}
	windows:=[][]inputLine{};current:=[]inputLine{};size:=0
	for _,original:=range input.lines{for _,line:=range splitLine(original,r.cfg.WindowBytes){raw,_:=json.Marshal(line);if len(raw)>r.cfg.WindowBytes{return inference.ErrBudget};if size+len(raw)>r.cfg.WindowBytes&&len(current)>0{windows=append(windows,current);current=[]inputLine{};size=0};current=append(current,line);size+=len(raw)}};if len(current)>0{windows=append(windows,current)};if len(windows)>4096{return inference.ErrBudget}
	set:=[]memory.Prepared{};finalLines:=input.lines;known:=input.known
	if len(windows)>1{
		finalLines=[]inputLine{};known=map[string]citation{}
		for index,window:=range windows{record:=stableID("window\n"+job.Spec.Scope+"\n"+job.Spec.Subject+"\n"+job.Key+"\n"+strconv.Itoa(index));summary,revision,e:=r.window(ctx,job,policy,input,window,restrictKnown(input.known,window),generator,record,job.Spec.Kind!="session_digest");if e!=nil{return e}
			for i,f:=range summary.Findings{id:=record+"#"+strconv.Itoa(i);known[id]=citation{Ref:records.SourceRef{Record:record,Revision:revision,Kind:records.KindSummary},Primary:summary.Primary};finalLines=append(finalLines,inputLine{ID:id,Role:"assistant",Type:f.Kind,Text:f.Text})}
		}
	}
	// Reduce large sets of window findings through a bounded tree. Each intermediate
	// remains a cited summary; no truncation silently discards evidence.
	for depth:=0;len(render(finalLines,nil))>r.cfg.WindowBytes;depth++{
		if depth>=8{return inference.ErrBudget};groups:=[][]inputLine{};group:=[]inputLine{};bytes:=0
		for _,line:=range finalLines{raw,_:=json.Marshal(line);if len(raw)>r.cfg.WindowBytes{return inference.ErrBudget};if bytes+len(raw)>r.cfg.WindowBytes&&len(group)>0{groups=append(groups,group);group=[]inputLine{};bytes=0};group=append(group,line);bytes+=len(raw)};if len(group)>0{groups=append(groups,group)}
		next:=[]inputLine{};nextKnown:=map[string]citation{}
		for i,lines:=range groups{record:=stableID("reduction\n"+job.Key+"\n"+strconv.Itoa(depth)+"\n"+strconv.Itoa(i));summary,revision,e:=r.window(ctx,job,policy,input,lines,restrictKnown(known,lines),generator,record,true);if e!=nil{return e}
			for n,f:=range summary.Findings{id:=record+"#"+strconv.Itoa(n);nextKnown[id]=citation{Ref:records.SourceRef{Record:record,Revision:revision,Kind:records.KindSummary},Primary:summary.Primary};next=append(next,inputLine{ID:id,Role:"assistant",Type:f.Kind,Text:f.Text})}
		};finalLines=next;known=nextKnown
	}
	output,err:=r.call(ctx,job,finalLines,known,input.omissions,job.Spec.Kind!="session_digest"||len(windows)>1);if err!=nil{return err}
	level:="session";if job.Spec.Kind=="project_rollup"{level="project_day"};if job.Spec.Kind=="personal_rollup"{level="personal_week"}
	summary:=makeSummary(output,input,known,generator,level,job.Spec.Subject)
	if job.Spec.Kind!="session_digest"{stamp:=input.period.UTC();start:=time.Date(stamp.Year(),stamp.Month(),stamp.Day(),0,0,0,0,time.UTC);end:=start.AddDate(0,0,1);if job.Spec.Kind=="personal_rollup"{start=start.AddDate(0,0,-(int(start.Weekday())+6)%7);end=start.AddDate(0,0,7)};summary.PeriodStart=start.Format(time.RFC3339Nano);summary.PeriodEnd=end.Format(time.RFC3339Nano)}
	summaryRecord:=stableID("summary\n"+job.Spec.Scope+"\n"+level+"\n"+job.Spec.Subject+"\n"+summary.PeriodStart)
	p,err:=r.svc.Prepare(ctx,memory.PutInput{Scope:job.Spec.Destination,Kind:records.KindSummary,Record:summaryRecord,Body:summary,Provenance:memory.Provenance{SourceKind:"derived",SourceRef:job.Key}});if err!=nil{return err};set=append(set,p)
	for _,candidate:=range output.Candidates{in:=makeInsight(candidate,known,input.project,generator,policy.AutoAcceptProjectFacts&&job.Spec.Kind=="session_digest");in.Influence=consumedRefs(known);in.CandidateKey=candidateKey(job.Spec.Destination,in.Type,in.Category,in.Assertion);var existing string
		err=r.svc.Read(ctx,func(q memory.Queryer)error{e:=q.QueryRowContext(ctx,"SELECT review FROM candidate_keys WHERE candidate_key=?",in.CandidateKey).Scan(&existing);if e==sql.ErrNoRows{return nil};return e});if err!=nil{return err};if oneOf(existing,"accepted","edited","rejected"){continue}
		ip,e:=r.svc.Prepare(ctx,memory.PutInput{Scope:job.Spec.Destination,Kind:records.KindInsight,Record:stableID("candidate-record\n"+in.CandidateKey),Body:in,Provenance:memory.Provenance{SourceKind:"derived",SourceRef:job.Key}});if e!=nil{return e};set=append(set,ip)
	}
	return r.svc.Commit(ctx,set,func(ctx context.Context,tx *sql.Tx)error{
		var raw []byte;if err:=tx.QueryRowContext(ctx,"SELECT body FROM policy WHERE name='session'").Scan(&raw);err!=nil{return err};var currentPolicy session.Policy;if json.Unmarshal(raw,&currentPolicy)!=nil||!currentPolicy.Derive||currentPolicy.Version!=policy.Version{return memory.ErrLeaseLost}
		if err:=r.authorize(ctx,tx,job,currentPolicy);err!=nil{return err};if now,e:=r.inputs(ctx,tx,job);e!=nil{return e}else if inputFingerprint(now)!=inputFingerprint(input){return memory.ErrHeadsChanged}
		for _,prepared:=range set{
			if prepared.Revision.Kind==records.KindInsight{for _,parent:=range prepared.Revision.Parents{old,e:=r.svc.Revision(ctx,tx,parent);if e!=nil{return e};var reviewed records.Insight;if json.Unmarshal(old.Body,&reviewed)!=nil{return memory.ErrInvalid};if oneOf(reviewed.Review,"accepted","edited","rejected"){return memory.ErrHeadsChanged}}}
			visible,e:=memory.IsVisible(ctx,tx,prepared.Revision.Record);if e!=nil{return e};if !visible{return memory.ErrHeadsChanged}
		}
		return r.svc.CompleteJob(ctx,tx,job.Key,job.Token,p.Revision.Record)
	})
}
func restrictKnown(known map[string]citation,lines []inputLine)map[string]citation{out:=map[string]citation{};for _,line:=range lines{out[line.ID]=known[line.ID]};return out}
func makeSummary(output modelOutput,input preparedInput,known map[string]citation,g records.Generator,level,subject string)records.Summary{
	findings:=[]records.Finding{};sources:=[]records.SourceRef{};primary:=[]records.SourceRef{}
	for _,f:=range output.Findings{rr,_:=refs(f.Evidence,known);findings=append(findings,records.Finding{Kind:f.Kind,Text:f.Text,Evidence:rr});sources=append(sources,rr...);for _,id:=range f.Evidence{primary=append(primary,known[id].Primary...)}}
	if len(sources)==0{sources=consumedRefs(known)};if len(primary)==0{ids:=make([]string,0,len(known));for id:=range known{ids=append(ids,id)};sort.Strings(ids);for _,id:=range ids{primary=append(primary,known[id].Primary...)}}
	gaps:=append([]records.Gap{},input.gaps...);for _,g:=range output.Gaps{if len(findings)<256{findings=append(findings,records.Finding{Kind:"open_question",Text:g,Evidence:[]records.SourceRef{}})}}
	if len(output.Gaps)>0{gaps=append(gaps,records.Gap{Reason:"unsupported_type",Count:len(output.Gaps)})}
	return records.Summary{Influence:consumedRefs(known),InputFingerprint:inputFingerprint(input),Level:level,Subject:subject,Findings:findings,Sources:uniqueRefs(sources),Primary:uniqueRefs(primary),SourceHeads:input.heads,Gaps:records.MergeGaps(gaps),Complete:len(gaps)==0,Generator:g}
}
func uniqueRefs(refs []records.SourceRef)[]records.SourceRef{out:=[]records.SourceRef{};seen:=map[records.SourceRef]bool{};for _,ref:=range refs{if !seen[ref]{seen[ref]=true;out=append(out,ref)}};return out}

// window checkpoints completed inference so a budget boundary or crash does not
// force the next lease to pay again for already-derived exact inputs.
func (r *Runner) window(ctx context.Context,job *memory.Job,policy session.Policy,input preparedInput,lines []inputLine,known map[string]citation,g records.Generator,record string,rollup bool)(records.Summary,string,error){
	var cached records.Summary;var revision string
	err:=r.svc.Read(ctx,func(q memory.Queryer)error{hh,deleted,e:=r.svc.Heads(ctx,q,job.Spec.Destination,record);if e!=nil{return e};if deleted||len(hh)!=1{return nil};visible,e:=memory.IsVisible(ctx,q,record);if e!=nil{return e};if visible&&json.Unmarshal(hh[0].Body,&cached)==nil&&cached.Generator.JobKey==job.Key&&cached.InputFingerprint==windowFingerprint(input,lines,known){revision=hh[0].ID};return nil});if err!=nil{return cached,"",err};if revision!=""{return cached,revision,nil}
	out,err:=r.call(ctx,job,lines,known,input.omissions,rollup);if err!=nil{return records.Summary{},"",err};summary:=makeSummary(out,input,known,g,"window",job.Spec.Subject);summary.InputFingerprint=windowFingerprint(input,lines,known)
	p,err:=r.svc.Prepare(ctx,memory.PutInput{Scope:job.Spec.Destination,Kind:records.KindSummary,Record:record,Body:summary,Provenance:memory.Provenance{SourceKind:"derived",SourceRef:job.Key}});if err!=nil{return summary,"",err}
	err=r.svc.Commit(ctx,[]memory.Prepared{p},func(ctx context.Context,tx *sql.Tx)error{
		var raw []byte;if e:=tx.QueryRowContext(ctx,"SELECT body FROM policy WHERE name='session'").Scan(&raw);e!=nil{return e};var current session.Policy;if json.Unmarshal(raw,&current)!=nil||!current.Derive||current.Version!=policy.Version{return memory.ErrLeaseLost};if e:=r.authorize(ctx,tx,job,current);e!=nil{return e};if now,e:=r.inputs(ctx,tx,job);e!=nil{return e}else if inputFingerprint(now)!=inputFingerprint(input){return memory.ErrHeadsChanged}
		result,e:=tx.ExecContext(ctx,"UPDATE jobs SET lease_until=? WHERE job_key=? AND state='leased' AND attempt_token=? AND lease_until>?",time.Now().Add(10*time.Minute).UnixMilli(),job.Key,job.Token,time.Now().UnixMilli());if e!=nil{return e};n,e:=result.RowsAffected();if e!=nil{return e};if n!=1{return memory.ErrLeaseLost};visible,e:=memory.IsVisible(ctx,tx,record);if e!=nil{return e};if !visible{return memory.ErrHeadsChanged};return nil
	});return summary,p.Revision.ID,err
}

func splitLine(line inputLine,max int)[]inputLine{
	raw,_:=json.Marshal(line);if len(raw)<=max{return []inputLine{line}}
	out:=[]inputLine{};text:=line.Text;step:=(max-1024)/6;if step<1{step=1}
	for len(text)>0{n:=step;if n>len(text){n=len(text)};for n>0&&!utf8.ValidString(text[:n]){n--};if n==0{_,n=utf8.DecodeRuneInString(text)};part:=line;part.Text=text[:n];out=append(out,part);text=text[n:]};return out
}
func (r *Runner) checkPersonalReady(ctx context.Context,tx *sql.Tx,policy session.Policy)error{
	rows,err:=tx.QueryContext(ctx,"SELECT job_key,scope,subject,inputs,destination,assigned_device FROM jobs WHERE kind='personal_rollup' AND state='pending' AND not_before<=? LIMIT 1000",time.Now().UnixMilli());if err!=nil{return err};jobs:=[]memory.Job{}
	for rows.Next(){var j memory.Job;var raw []byte;j.Spec.Kind="personal_rollup";if err=rows.Scan(&j.Key,&j.Spec.Scope,&j.Spec.Subject,&raw,&j.Spec.Destination,&j.Spec.AssignedDevice);err!=nil{rows.Close();return err};if json.Unmarshal(raw,&j.Spec.Inputs)!=nil{rows.Close();return memory.ErrInvalid};jobs=append(jobs,j)};err=rows.Err();rows.Close();if err!=nil{return err}
	for _,j:=range jobs{if err=r.authorize(ctx,tx,&j,policy);errors.Is(err,projection.ErrReadersExceedSources)||errors.Is(err,memory.ErrConsentWithdrawn)||errors.Is(err,memory.ErrInvalid){code:="authorization_failed";if errors.Is(err,projection.ErrReadersExceedSources){code="destination_readers_exceed_sources"};if errors.Is(err,memory.ErrConsentWithdrawn){code="consent_withdrawn"};if _,e:=tx.ExecContext(ctx,"UPDATE jobs SET state='failed',error_code=?,attempt_token=NULL,lease_until=NULL WHERE job_key=? AND state='pending'",code,j.Key);e!=nil{return e}}};return nil
}
