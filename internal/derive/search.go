package derive

import (
	"context"
	"encoding/json"
	"strings"
	"sort"

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
 Reason string `json:"reason,omitempty"`
	LastVerifiedAt string `json:"last_verified_at"`
	LastVerifiedHeads []records.ScopeHead `json:"last_verified_heads"`
	Untrusted bool `json:"untrusted"`
	Coverage session.Coverage `json:"coverage"`
}
func (r *Runner) SearchKnowledge(ctx context.Context,g ops.Grant,query string,limit int)(KnowledgeResult,error){return r.SearchKnowledgeMode(ctx,g,query,limit,"historical")}
func (r *Runner) SearchKnowledgeMode(ctx context.Context,g ops.Grant,query string,limit int,mode string)(KnowledgeResult,error){
	result:=KnowledgeResult{Results:[]KnowledgeHit{},Coverage:session.Coverage{Complete:false}}
	if !g.Has(ops.CapKnowledge){return result,ops.ErrForbidden};if mode!="historical"&&mode!="strict"{return result,memory.ErrInvalid};if limit<1||limit>100{limit=20};if len(query)>4096{return result,memory.ErrInvalid};fts:=projection.SearchQuery(query);if fts==""{return result,nil}
	ranks:=map[string]float64{};boundedScan:=false;knowledge:=[]string{};pending:=map[string]KnowledgeHit{};guards:=map[string]func(context.Context)bool{}
	err:=r.svc.Read(ctx,func(q memory.Queryer)error{
		for _,scope:=range g.Scopes{
			status,e:=r.svc.ScopeStatusQuery(ctx,q,scope);if e!=nil{return e};if !status.Coverage.Complete{result.Coverage.Issues=append(result.Coverage.Issues,"scope_not_caught_up")};for _,searchKind:=range []string{records.KindSummary,records.KindInsight,records.KindKnowledge}{rows,err:=q.QueryContext(ctx,"SELECT record,revision,kind,rank FROM session_search WHERE scope=? AND session_search MATCH ? AND kind=? AND NOT EXISTS(SELECT 1 FROM stale z WHERE z.record=session_search.record) AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=session_search.revision) AND NOT EXISTS(SELECT 1 FROM revisions v WHERE v.revision=session_search.revision AND v.kind='summary' AND json_extract(v.body,'$.body.level')='window') ORDER BY rank LIMIT 1001",scope,fts,searchKind);if err!=nil{return err};type hit struct{record,revision,kind string;rank float64};hits:=[]hit{};for rows.Next(){var h hit;if err=rows.Scan(&h.record,&h.revision,&h.kind,&h.rank);err!=nil{rows.Close();return err};hits=append(hits,h)};err=rows.Err();rows.Close();if err!=nil{return err}
			if len(hits)>1000{boundedScan=true;hits=hits[:1000]};for _,h:=range hits{ranks[h.revision]=h.rank;hh,del,e:=r.svc.Heads(ctx,q,scope,h.record);if e!=nil{return e};if del{continue};current:=false;for _,head:=range hh{if head.ID==h.revision{current=true}};if !current{continue};rev,e:=r.svc.Revision(ctx,q,h.revision);if e!=nil{return e};view:=KnowledgeHit{Kind:h.kind,Record:h.record,Revision:h.revision,Conflict:len(hh)>1,Untrusted:true,Evidence:[]EvidenceView{},LastVerifiedHeads:[]records.ScopeHead{},Coverage:result.Coverage}
				if h.kind==records.KindKnowledge{var k records.Knowledge;if json.Unmarshal(rev.Body,&k)!=nil{return memory.ErrInvalid};view.Text=k.Text;view.Accepted=true;view.State=k.Review;pending[h.record]=view;knowledge=append(knowledge,h.record);continue}
				visible,e:=memory.IsVisible(ctx,q,h.record);if e!=nil{return e};if !visible{continue}
				if h.kind==records.KindSummary{var s records.Summary;if json.Unmarshal(rev.Body,&s)!=nil{return memory.ErrInvalid};if s.Level=="window"{continue};if s.Level=="personal_week"{c,e:=projection.SnapshotControl(ctx,q,r.svc);if e!=nil{return e};rows,e:=q.QueryContext(ctx,"SELECT record FROM aggregation_grants WHERE destination=? AND revoked=0",scope);if e!=nil{return e};var grants []string;for rows.Next(){var id string;if rows.Scan(&id)!=nil{rows.Close();return memory.ErrStorage};grants=append(grants,id)};e=rows.Err();rows.Close();if e!=nil{return e};if projection.CheckAggregation(ctx,q,r.svc,grants,s.SourceHeads,scope,c)!=nil{continue}};text:=[]string{};for _,f:=range s.Findings{text=append(text,f.Text)};view.Text=strings.Join(text,"\n");view.Level=s.Level;view.Generated=true;view.State="generated";view.Evidence=r.evidence(ctx,q,g,s.Primary);view.Coverage.Gaps=s.Gaps;view.Coverage.Complete=s.Complete
				}else{var in records.Insight;if json.Unmarshal(rev.Body,&in)!=nil{return memory.ErrInvalid};if in.Review!="accepted"&&in.Review!="edited"{continue};view.Text=in.Assertion;view.Accepted=true;view.Generated=in.Generator!=nil;view.State=in.Review;view.Contradictions=len(in.Contradictions);view.Evidence=r.evidence(ctx,q,g,in.Evidence)}
				result.Results=append(result.Results,view)
			}
		}};return nil
	});if err!=nil{return result,err}
	sort.SliceStable(knowledge,func(i,j int)bool{return ranks[pending[knowledge[i]].Revision]<ranks[pending[knowledge[j]].Revision]});if len(knowledge)>1000{boundedScan=true;knowledge=knowledge[:1000]};r.mu.RLock();resolver:=r.resolver;r.mu.RUnlock();resolverMissing:=resolver==nil
	if resolver==nil{resolver=func(context.Context,[]string,string)(map[string]KnowledgeVisibility,error){return nil,memory.ErrConsentUnavailable}};if len(knowledge)>0&&resolverMissing{result.Coverage.Issues=append(result.Coverage.Issues,"knowledge_resolver_unavailable")};if resolver!=nil&&len(knowledge)>0{for start:=0;start<len(knowledge);start+=1000{end:=start+1000;if end>len(knowledge){end=len(knowledge)};batch:=knowledge[start:end];visibility,e:=resolver(ctx,batch,mode);if e!=nil{result.Coverage.Issues=append(result.Coverage.Issues,"knowledge_resolution_failed");if mode=="historical"{if e=r.svc.Read(ctx,func(q memory.Queryer)error{for _,id:=range batch{v,e:=projection.TargetVisible(ctx,q,r.svc,id);if e!=nil{return e};view:=pending[id];if v.Visible&&v.Revision==view.Revision{view.Reason="fresh_check_unavailable";view.LastVerifiedAt=v.LastVerifiedAt;view.LastVerifiedHeads=v.LastVerifiedHeads;result.Results=append(result.Results,view)}};return nil});e!=nil{return result,e}};continue};for _,id:=range batch{v,ok:=visibility[id];if !ok||!v.Visible||mode=="strict"&&!v.Current{continue};view:=pending[id];if v.Revision!=view.Revision{continue};view.Current=v.Current;view.Reason=v.Reason;guards[id]=v.Guard;view.LastVerifiedAt=v.LastVerifiedAt;view.LastVerifiedHeads=v.LastVerifiedHeads;result.Results=append(result.Results,view)}}}
	// Network freshness work may overlap deletion or replacement. Recheck every
// returned revision and lifecycle in one final snapshot before exposing text.
filtered:=[]KnowledgeHit{};err=r.svc.Read(ctx,func(q memory.Queryer)error{for _,hit:=range result.Results{rev,e:=r.svc.Revision(ctx,q,hit.Revision);if e!=nil{continue};hh,deleted,e:=r.svc.Heads(ctx,q,rev.Scope,hit.Record);if e!=nil{return e};if deleted{continue};head:=false;for _,h:=range hh{head=head||h.ID==hit.Revision};if !head{continue};if hit.Kind==records.KindKnowledge{v,e:=projection.TargetVisible(ctx,q,r.svc,hit.Record);if e!=nil{return e};if !v.Visible||v.Revision!=hit.Revision{continue}}else{visible,e:=memory.IsVisible(ctx,q,hit.Record);if e!=nil{return e};if !visible{continue};if hit.Kind==records.KindSummary{var summary records.Summary;if json.Unmarshal(rev.Body,&summary)!=nil{return memory.ErrInvalid};if summary.Level=="personal_week"{c,e:=projection.SnapshotControl(ctx,q,r.svc);if e!=nil{return e};rows,e:=q.QueryContext(ctx,"SELECT record FROM aggregation_grants WHERE destination=? AND revoked=0",rev.Scope);if e!=nil{return e};var ids []string;for rows.Next(){var id string;if rows.Scan(&id)!=nil{rows.Close();return memory.ErrStorage};ids=append(ids,id)};e=rows.Err();rows.Close();if e!=nil{return e};if projection.CheckAggregation(ctx,q,r.svc,ids,summary.SourceHeads,rev.Scope,c)!=nil{continue}}}};filtered=append(filtered,hit)};return nil});if err!=nil{return result,err};result.Results=filtered;filtered=[]KnowledgeHit{};for _,hit:=range result.Results{if hit.Kind==records.KindKnowledge&&hit.Current{guard:=guards[hit.Record];if guard==nil||!guard(ctx){hit.Current=false;if mode=="strict"{continue}}};filtered=append(filtered,hit)};result.Results=filtered
sort.SliceStable(result.Results,func(i,j int)bool{a,b:=result.Results[i],result.Results[j];if ranks[a.Revision]!=ranks[b.Revision]{return ranks[a.Revision]<ranks[b.Revision]};return a.Revision<b.Revision});if boundedScan{result.Coverage.Issues=append(result.Coverage.Issues,"query_candidate_bound")};result.Coverage.Complete=!boundedScan&&len(result.Coverage.Issues)==0;for _,hit:=range result.Results{if hit.Kind==records.KindKnowledge&&!hit.Current||hit.Kind==records.KindSummary&&!hit.Coverage.Complete{result.Coverage.Complete=false}};if len(result.Results)>limit{result.Results=result.Results[:limit]};for _,hit:=range result.Results{result.Coverage.SourceHeads=append(result.Coverage.SourceHeads,hit.Revision)};return result,nil
}
