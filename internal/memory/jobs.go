package memory

import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "sort"
 "strings"
 "time"

 "github.com/humzakhan/capsule-memory/internal/records"
)
var ErrLeaseLost=errors.New("job lease lost; result discarded")
type JobSpec struct{Kind,Scope,Subject,Destination string;Inputs []records.SourceRef;Versions string;Priority int;NotBefore time.Time;AssignedDevice string}
type Job struct{Key string;Spec JobSpec;Token string;Attempts int}
func JobKey(spec JobSpec)string{a:=make([]string,0,len(spec.Inputs));for _,r:=range spec.Inputs{a=append(a,r.Revision)};sort.Strings(a);return digest([]byte("capsule-memory/job-key/v1\n"+spec.Kind+"\n"+spec.Scope+"\n"+spec.Subject+"\n"+strings.Join(a,",")+"\n"+spec.Versions+"\n"+spec.Destination+"\n"))}
func (s *Service) EnqueueJob(ctx context.Context,tx *sql.Tx,spec JobSpec)(string,error){
 if e:=s.RequireV2();e!=nil{return "",e};if s.scope(spec.Scope)!=nil||s.scope(spec.Destination)!=nil||!idPattern.MatchString(spec.Subject)||len(spec.Inputs)>4096||spec.Priority<0||spec.Priority>4{return "",ErrInvalid};if spec.AssignedDevice==""{spec.AssignedDevice=s.configuration().Device};if !idPattern.MatchString(spec.AssignedDevice){return "",ErrInvalid}
 key:=JobKey(spec);b,e:=json.Marshal(spec.Inputs);if e!=nil{return "",e};if _,e=tx.ExecContext(ctx,"INSERT INTO local_seq VALUES('job',1) ON CONFLICT(name) DO UPDATE SET value=value+1");e!=nil{return "",ErrStorage}
 n:=spec.NotBefore.UnixMilli();if spec.NotBefore.IsZero(){n=0}
 _,e=tx.ExecContext(ctx,`INSERT INTO jobs(job_key,kind,scope,subject,inputs,destination,priority,state,not_before,assigned_device,seq,versions) VALUES(?,?,?,?,?,?,?,'pending',?,?,(SELECT value FROM local_seq WHERE name='job'),?) ON CONFLICT(job_key) DO UPDATE SET not_before=MAX(jobs.not_before,excluded.not_before) WHERE jobs.state='pending'`,key,spec.Kind,spec.Scope,spec.Subject,b,spec.Destination,spec.Priority,n,spec.AssignedDevice,spec.Versions);if e!=nil{return "",ErrStorage};return key,nil
}
func (s *Service) SupersedePending(ctx context.Context,tx *sql.Tx,kind,subject,keep string)error{_,e:=tx.ExecContext(ctx,"UPDATE jobs SET state='cancelled',attempt_token=NULL,lease_until=NULL WHERE kind=? AND subject=? AND job_key!=? AND state='pending'",kind,subject,keep);if e!=nil{return ErrStorage};return nil}
func (s *Service) LeaseJob(ctx context.Context,kinds []string,lease time.Duration)(*Job,error){
 if e:=s.RequireV2();e!=nil{return nil,e};if len(kinds)==0||len(kinds)>32||lease<=0||lease>time.Hour{return nil,ErrInvalid};var out *Job
 e:=s.Update(ctx,func(tx *sql.Tx)error{now:=time.Now().UnixMilli();if _,e:=tx.ExecContext(ctx,"UPDATE jobs SET state='pending',attempt_token=NULL,lease_until=NULL WHERE state='leased' AND lease_until<=?",now);e!=nil{return ErrStorage}
  args:=[]any{s.configuration().Device,now};marks:=[]string{};for _,k:=range kinds{marks=append(marks,"?");args=append(args,k)}
  var j Job;var b []byte;var nb int64;e:=tx.QueryRowContext(ctx,"SELECT job_key,kind,scope,subject,inputs,destination,priority,not_before,assigned_device,attempts,versions FROM jobs WHERE assigned_device=? AND state='pending' AND not_before<=? AND kind IN("+strings.Join(marks,",")+") ORDER BY priority,not_before,seq LIMIT 1",args...).Scan(&j.Key,&j.Spec.Kind,&j.Spec.Scope,&j.Spec.Subject,&b,&j.Spec.Destination,&j.Spec.Priority,&nb,&j.Spec.AssignedDevice,&j.Attempts,&j.Spec.Versions);if e==sql.ErrNoRows{return nil};if e!=nil{return ErrStorage};if json.Unmarshal(b,&j.Spec.Inputs)!=nil{return ErrStorage};j.Spec.NotBefore=time.UnixMilli(nb);j.Token,e=newID();if e!=nil{return e};j.Attempts++
  if _,e=tx.ExecContext(ctx,"UPDATE jobs SET state='leased',attempt_token=?,lease_until=?,attempts=attempts+1 WHERE job_key=?",j.Token,now+lease.Milliseconds(),j.Key);e!=nil{return ErrStorage};out=&j;return nil
 });return out,e
}
func (s *Service) CompleteJob(ctx context.Context,tx *sql.Tx,key,token,result string)error{r,e:=tx.ExecContext(ctx,"UPDATE jobs SET state='done',result_record=?,attempt_token=NULL,lease_until=NULL,error_code=NULL WHERE job_key=? AND state='leased' AND attempt_token=? AND lease_until>?",result,key,token,time.Now().UnixMilli());if e!=nil{return ErrStorage};n,e:=r.RowsAffected();if e!=nil{return ErrStorage};if n!=1{return ErrLeaseLost};return nil}
func (s *Service) FailJob(ctx context.Context,key,token,code string,retry time.Time,permanent bool)error{return s.Update(ctx,func(tx *sql.Tx)error{state:="pending";if permanent{state="failed"};r,e:=tx.ExecContext(ctx,"UPDATE jobs SET state=?,not_before=?,error_code=?,attempt_token=NULL,lease_until=NULL WHERE job_key=? AND state='leased' AND attempt_token=? AND lease_until>?",state,retry.UnixMilli(),code,key,token,time.Now().UnixMilli());if e!=nil{return ErrStorage};n,e:=r.RowsAffected();if e!=nil{return ErrStorage};if n!=1{return ErrLeaseLost};return nil})}
func (s *Service) CancelJobs(ctx context.Context,tx *sql.Tx,kinds []string,scope string)(int,error){
 if len(kinds)==0{return 0,nil};marks:=[]string{};args:=[]any{};filter:="";if scope!=""{if s.scope(scope)!=nil{return 0,ErrInvalid};filter="scope=? AND ";args=append(args,scope)};for _,k:=range kinds{marks=append(marks,"?");args=append(args,k)}
 r,e:=tx.ExecContext(ctx,"UPDATE jobs SET state='cancelled',attempt_token=NULL,lease_until=NULL WHERE "+filter+"state IN('pending','leased','blocked') AND kind IN("+strings.Join(marks,",")+")",args...);if e!=nil{return 0,ErrStorage};n,e:=r.RowsAffected();if e!=nil{return 0,ErrStorage};return int(n),nil
}
