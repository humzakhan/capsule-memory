package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
 "strings"
	"time"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
)
func defaultPolicy()Policy{return Policy{Version:1,ToolDetail:"metadata",CrossVault:"off",Aggregations:[]string{},PublishScopes:[]string{}}}
func(m *Module) Policy(ctx context.Context)(Policy,error){p:=defaultPolicy();if e:=m.svc.RequireV2();e!=nil{return p,e};e:=m.svc.Read(ctx,func(q memory.Queryer)error{var b []byte;e:=q.QueryRowContext(ctx,"SELECT body FROM policy WHERE name='session'").Scan(&b);if errors.Is(e,sql.ErrNoRows){return nil};if e!=nil{return memory.ErrStorage};if json.Unmarshal(b,&p)!=nil{return memory.ErrStorage};return nil});return p,e}
func(m *Module) SetPolicy(ctx context.Context,change func(*Policy)error)(Policy,error){m.mu.Lock();defer m.mu.Unlock();p,e:=m.Policy(ctx);if e!=nil{return p,e};old:=p;if e=change(&p);e!=nil{return old,e};if p.ToolDetail!="metadata"&&p.ToolDetail!="full"{return old,memory.ErrInvalid};if p.CrossVault!="off"&&p.CrossVault!="enabled_unaccepted"{return old,memory.ErrInvalid};for _,s:=range append(append([]string{},p.RedactionExtra...),p.SessionExclusions...){if _,e:=regexp.Compile(s);e!=nil{return old,memory.ErrInvalid}};if len(p.RedactionExtra)>64||len(p.SessionExclusions)>256||len(p.Aggregations)>64{return old,memory.ErrInvalid};p.Version=old.Version+1;b,e:=json.Marshal(p);if e!=nil{return old,memory.ErrInvalid};e=m.svc.Update(ctx,func(tx *sql.Tx)error{if _,e:=tx.ExecContext(ctx,"INSERT INTO policy(name,version,body) VALUES('session',?,?) ON CONFLICT(name) DO UPDATE SET version=excluded.version,body=excluded.body",p.Version,b);e!=nil{return e};if old.Derive&&!p.Derive{if _,e:=m.svc.CancelJobs(ctx,tx,[]string{"session_digest","project_rollup","personal_rollup"},"");e!=nil{return e}};if old.CrossVault!="off"&&p.CrossVault=="off"{return m.withdrawCrossVault(ctx,tx)};return nil});if e==nil&&old.Capture&&!p.Capture{m.buffers=map[string]streamBuffer{}};return p,e}
func(m *Module) AddSource(ctx context.Context,harness string,cfg json.RawMessage)(capture.Source,capture.Preview,error){var s capture.Source;var p capture.Preview;if e:=m.svc.RequireV2();e!=nil{return s,p,e};if e:=m.sourceCapacity(ctx);e!=nil{return s,p,e};a:=m.adapters[harness];if a==nil{return s,p,capture.ErrInvalid};if _,e:=a.ParseConfig(cfg);e!=nil{return s,p,e};id,e:=memory.NewID();if e!=nil{return s,p,e};store,e:=memory.NewID();if e!=nil{return s,p,e};if stable,ok:=a.(interface{Identity(json.RawMessage)(string,error)});ok{store,e=stable.Identity(cfg);if e!=nil{return s,p,e}};s=capture.Source{ID:id,Harness:harness,Adapter:a.Name()+"/"+a.Version(),StoreIdentity:store,Config:cfg};p,e=a.Preview(ctx,s);if e!=nil{return s,p,e};e=m.svc.Update(ctx,func(tx *sql.Tx)error{_,e:=tx.ExecContext(ctx,"INSERT INTO sources(source_id,harness,adapter,store_identity,config) VALUES(?,?,?,?,?)",s.ID,s.Harness,s.Adapter,s.StoreIdentity,[]byte(s.Config));return e});return s,p,e}
func(m *Module) EnableSource(ctx context.Context,id string,bf *Backfill)error{
 m.mu.Lock();defer m.mu.Unlock();source,e:=m.Source(ctx,id);if e!=nil{return e};adapter:=m.adapters[source.Harness];if adapter==nil{return capture.ErrUnavailable}
 wanted:=map[string]bool{};if bf!=nil{if bf.MaxBytes<=0||bf.Since.IsZero()||len(bf.Streams)==0||len(bf.Streams)>64{return errors.New("backfill requires since, positive max bytes and 1..64 explicit streams")};for _,stream:=range bf.Streams{if stream==""||wanted[stream]{return memory.ErrInvalid};wanted[stream]=true}}
 var checkpoints []capture.Checkpoint;ends:=map[string]int64{};page:="";remaining:=int64(0);if bf!=nil{remaining=bf.MaxBytes}
 for pages:=0;pages<1563;pages++{streams,next,e:=adapter.Discover(ctx,source,page,64);if e!=nil{return e};for _,stream:=range streams{
  cp:=capture.Checkpoint{Stream:stream.Key,Generation:stream.Generation,ObservedModified:stream.Modified.UTC().Format(time.RFC3339Nano),ObservedSize:stream.Size}
  if wanted[stream.Key]{if stream.Size>=0{if stream.Size>remaining{return errors.New("backfill size exceeds explicit bound")};remaining-=stream.Size};ends[stream.Key]=stream.Size;delete(wanted,stream.Key)}else if stream.Size>=0{cp.Position=strconv.FormatInt(stream.Size,10)}else if baseline,ok:=adapter.(interface{Baseline(context.Context,capture.Source,capture.Stream)(capture.Checkpoint,error)});ok{cp,e=baseline.Baseline(ctx,source,stream);if e!=nil{return e}}else{return capture.ErrUnavailable}
  if _,selected:=ends[cp.Stream];!selected{cp.CaughtUp=true};checkpoints=append(checkpoints,cp)
 };if next==""{break};if next==page||pages==1562{return errors.New("source discovery bound exceeded")};page=next};if len(wanted)>0{return errors.New("backfill stream not found")}
 e = m.svc.Update(ctx,func(tx *sql.Tx)error{
  if _,e:=tx.ExecContext(ctx,"DELETE FROM backfill_streams WHERE source_id=?",id);e!=nil{return e};if _,e:=tx.ExecContext(ctx,"DELETE FROM backfills WHERE source_id=?",id);e!=nil{return e};if bf!=nil{if _,e:=tx.ExecContext(ctx,"INSERT INTO backfills VALUES(?,?,?)",id,bf.Since.UTC().Format(time.RFC3339Nano),bf.MaxBytes);e!=nil{return e}}
  for _,cp:=range checkpoints{if end,selected:=ends[cp.Stream];selected{if _,e:=tx.ExecContext(ctx,"INSERT INTO backfill_streams VALUES(?,?,?,1)",id,cp.Stream,end);e!=nil{return e}}else{if e:=auditGap(ctx,tx,id,cp.Stream,cp.Position,"future_baseline_excluded",1);e!=nil{return e}};if e:=checkpoint(ctx,tx,id,cp);e!=nil{return e};if _,e:=tx.ExecContext(ctx,"UPDATE checkpoints SET state='ok',reason=NULL WHERE source_id=? AND stream=?",id,cp.Stream);e!=nil{return e}}
  _,e:=tx.ExecContext(ctx,"UPDATE sources SET enabled=1,consent_at=?,state='enabled',reason=NULL WHERE source_id=?",time.Now().UTC().Format(time.RFC3339Nano),id);return e
 })
 if e == nil {
  for key := range m.buffers {
   if strings.HasPrefix(key, id+":") {
    delete(m.buffers, key)
   }
  }
 }
 return e
}

func(m *Module) DisableSource(ctx context.Context,id string)error{return m.DisableSourceWithConsent(ctx,id,true)}
func (m *Module) DisableSourceWithConsent(ctx context.Context, id string, withdraw bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.svc.Update(ctx, func(tx *sql.Tx) error {
		state := "disabled"
		if withdraw {
			state = "withdrawn"
		}
		if _, err := tx.ExecContext(ctx, "UPDATE sources SET enabled=0,state=?,reason=NULL WHERE source_id=?", state, id); err != nil {
			return err
		}
		if withdraw {
			return m.svc.HoldSource(ctx, tx, id)
		}
		return nil
	})
	if err == nil {
		for key := range m.buffers {
			if strings.HasPrefix(key, id+":") {
				delete(m.buffers, key)
			}
		}
	}
	return err
}
func(m *Module) Pause(ctx context.Context,id string)error{m.mu.Lock();defer m.mu.Unlock();return m.svc.Update(ctx,func(tx *sql.Tx)error{_,e:=tx.ExecContext(ctx,"INSERT INTO pauses(source_id,started) VALUES(?,?)",id,time.Now().UTC().Format(time.RFC3339Nano));if e!=nil{return e};_,e=tx.ExecContext(ctx,"UPDATE sources SET state='paused',reason='pause_unresolved' WHERE source_id=?",id);return e})}
func(m *Module) Resume(ctx context.Context,id,resolution string)error{if resolution!="skip"&&resolution!="include"{return memory.ErrInvalid};if resolution=="skip"{if e:=m.EnableSource(ctx,id,nil);e!=nil{return e}};m.mu.Lock();defer m.mu.Unlock();return m.svc.Update(ctx,func(tx *sql.Tx)error{if _,e:=tx.ExecContext(ctx,"UPDATE pauses SET ended=?,resolution=? WHERE source_id=? AND resolution IS NULL",time.Now().UTC().Format(time.RFC3339Nano),resolution,id);e!=nil{return e};reason:="";if resolution=="skip"{reason="paused_skipped";if e:=m.pausedGap(ctx,tx,id);e!=nil{return e}};_,e:=tx.ExecContext(ctx,"UPDATE sources SET state='enabled',reason=? WHERE source_id=?",reason,id);return e})}
