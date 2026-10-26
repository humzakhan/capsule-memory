package derive

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/projection"
	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/session"
)

type KnowledgeVisibility struct {Guard func(context.Context)bool;Revision string;Visible,Current bool;LastVerifiedAt string;LastVerifiedHeads []records.ScopeHead;Reason string}
type KnowledgeResolver func(context.Context,[]string,string)(map[string]KnowledgeVisibility,error)
func (r *Runner) SetKnowledgeResolver(fn KnowledgeResolver){r.mu.Lock();defer r.mu.Unlock();r.resolver=fn}
type KnowledgeResult struct {Results []KnowledgeHit `json:"results"`;Coverage session.Coverage `json:"coverage"`}
type KnowledgeHit struct {
	Kind,Record,Revision,Level,State string
	Text string
	Generated,Accepted,Stale,Conflict bool
	Contradictions int
	Evidence []EvidenceView
	Current bool `json:"current"`
	LastVerifiedAt string `json:"last_verified_at"`
	LastVerifiedHeads []records.ScopeHead `json:"last_verified_heads"`
	Untrusted bool `json:"untrusted"`
	Coverage session.Coverage `json:"coverage"`
}
func (r *Runner) SearchKnowledge(ctx context.Context,g ops.Grant,query string,limit int)(KnowledgeResult,error){return r.SearchKnowledgeMode(ctx,g,query,limit,"historical")}
func (r *Runner) SearchKnowledgeMode(ctx context.Context,g ops.Grant,query string,limit int,mode string)(KnowledgeResult,error){
	result:=KnowledgeResult{Results:[]KnowledgeHit{},Coverage:session.Coverage{Complete:false}}
	if !g.Has(ops.CapKnowledge){return result,ops.ErrForbidden};if mode!="historical"&&mode!="strict"{return result,memory.ErrInvalid};if limit<1||limit>100{limit=20};if len(query)>4096{return result,memory.ErrInvalid};fts:=projection.SearchQuery(query);if fts==""{return result,nil}
	knowledge:=[]string{};pending:=map[string]KnowledgeHit{};guards:=map[string]func(context.Context)bool{}
	err:=r.svc.Read(ctx,func(q memory.Queryer)error{
		for _,scope:=range g.Scopes{
			rows,err:=q.QueryContext(ctx,"SELECT record,revision,kind FROM session_search WHERE scope=? AND session_search MATCH ? AND kind IN ('summary','insight','knowledge') ORDER BY rank LIMIT 1000",scope,fts);if err!=nil{return err};type hit struct{record,revision,kind string};hits:=[]hit{};for rows.Next(){var h hit;if err=rows.Scan(&h.record,&h.revision,&h.kind);err!=nil{rows.Close();return err};hits=append(hits,h)};err=rows.Err();rows.Close();if err!=nil{return err}
			for _,h:=range hits{hh,del,e:=r.svc.Heads(ctx,q,scope,h.record);if e!=nil{return e};if del{continue};current:=false;for _,head:=range hh{if head.ID==h.revision{current=true}};if !current{continue};rev,e:=r.svc.Revision(ctx,q,h.revision);if e!=nil{return e};view:=KnowledgeHit{Kind:h.kind,Record:h.record,Revision:h.revision,Conflict:len(hh)>1,Untrusted:true,Evidence:[]EvidenceView{},LastVerifiedHeads:[]records.ScopeHead{},Coverage:result.Coverage}
				if h.kind==records.KindKnowledge{var k records.Knowledge;if json.Unmarshal(rev.Body,&k)!=nil{return memory.ErrInvalid};view.Text=k.Text;view.Accepted=true;view.State=k.Review;pending[h.record]=view;knowledge=append(knowledge,h.record);continue}
				visible,e:=memory.IsVisible(ctx,q,h.record);if e!=nil{return e};if !visible{continue}
				if h.kind==records.KindSummary{var s records.Summary;if json.Unmarshal(rev.Body,&s)!=nil{return memory.ErrInvalid};if s.Level=="personal_week"{c,e:=projection.SnapshotControl(ctx,q,r.svc);if e!=nil{return e};rows,e:=q.QueryContext(ctx,"SELECT record FROM aggregation_grants WHERE destination=? AND revoked=0",scope);if e!=nil{return e};var grants []string;for rows.Next(){var id string;if rows.Scan(&id)!=nil{rows.Close();return memory.ErrStorage};grants=append(grants,id)};e=rows.Err();rows.Close();if e!=nil{return e};if projection.CheckAggregation(ctx,q,r.svc,grants,s.SourceHeads,scope,c)!=nil{continue}};text:=[]string{};for _,f:=range s.Findings{text=append(text,f.Text)};view.Text=strings.Join(text,"\n");view.Level=s.Level;view.Generated=true;view.State="generated";view.Evidence=r.evidence(ctx,q,g,s.Primary);view.Coverage.Gaps=s.Gaps;view.Coverage.Complete=s.Complete
				}else{var in records.Insight;if json.Unmarshal(rev.Body,&in)!=nil{return memory.ErrInvalid};if in.Review!="accepted"&&in.Review!="edited"{continue};view.Text=in.Assertion;view.Accepted=true;view.Generated=in.Generator!=nil;view.State=in.Review;view.Contradictions=len(in.Contradictions);view.Evidence=r.evidence(ctx,q,g,in.Evidence)}
				result.Results=append(result.Results,view)
			}
		};return nil
	});if err!=nil{return result,err}
	r.mu.RLock();resolver:=r.resolver;r.mu.RUnlock()
	if resolver!=nil&&len(knowledge)>0{visibility,e:=resolver(ctx,knowledge,mode);if e==nil{for _,id:=range knowledge{v,ok:=visibility[id];if !ok||!v.Visible||mode=="strict"&&!v.Current{continue};view:=pending[id];if v.Revision!=view.Revision{continue};view.Current=v.Current;guards[id]=v.Guard;view.LastVerifiedAt=v.LastVerifiedAt;view.LastVerifiedHeads=v.LastVerifiedHeads;result.Results=append(result.Results,view)}}}
	// Network freshness work may overlap deletion or replacement. Recheck every
// returned revision and lifecycle in one final snapshot before exposing text.
filtered:=[]KnowledgeHit{};err=r.svc.Read(ctx,func(q memory.Queryer)error{for _,hit:=range result.Results{rev,e:=r.svc.Revision(ctx,q,hit.Revision);if e!=nil{continue};hh,deleted,e:=r.svc.Heads(ctx,q,rev.Scope,hit.Record);if e!=nil{return e};if deleted{continue};head:=false;for _,h:=range hh{head=head||h.ID==hit.Revision};if !head{continue};if hit.Kind==records.KindKnowledge{v,e:=projection.TargetVisible(ctx,q,r.svc,hit.Record);if e!=nil{return e};if !v.Visible||v.Revision!=hit.Revision{continue}}else{visible,e:=memory.IsVisible(ctx,q,hit.Record);if e!=nil{return e};if !visible{continue};if hit.Kind==records.KindSummary{var summary records.Summary;if json.Unmarshal(rev.Body,&summary)!=nil{return memory.ErrInvalid};if summary.Level=="personal_week"{c,e:=projection.SnapshotControl(ctx,q,r.svc);if e!=nil{return e};rows,e:=q.QueryContext(ctx,"SELECT record FROM aggregation_grants WHERE destination=? AND revoked=0",rev.Scope);if e!=nil{return e};var ids []string;for rows.Next(){var id string;if rows.Scan(&id)!=nil{rows.Close();return memory.ErrStorage};ids=append(ids,id)};e=rows.Err();rows.Close();if e!=nil{return e};if projection.CheckAggregation(ctx,q,r.svc,ids,summary.SourceHeads,rev.Scope,c)!=nil{continue}}}};filtered=append(filtered,hit)};return nil});if err!=nil{return result,err};result.Results=filtered;filtered=[]KnowledgeHit{};for _,hit:=range result.Results{if hit.Kind==records.KindKnowledge&&hit.Current{guard:=guards[hit.Record];if guard==nil||!guard(ctx){hit.Current=false;if mode=="strict"{continue}}};filtered=append(filtered,hit)};result.Results=filtered
if len(result.Results)>limit{result.Results=result.Results[:limit]};for _,hit:=range result.Results{result.Coverage.SourceHeads=append(result.Coverage.SourceHeads,hit.Revision)};return result,nil
}
