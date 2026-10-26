package derive

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/records"
)

type InsightView struct {Record,Revision string;Insight records.Insight;Stale,Conflict bool;Evidence []EvidenceView;Untrusted bool `json:"untrusted"`}
type EvidenceView struct {Ref records.SourceRef;Accessible bool;Snippet string}
func candidateKey(scope,kind,category,assertion string)string{
	// Stdlib has no NFC. Collapse ASCII whitespace only, then lowercase.
	normal:=strings.Join(strings.FieldsFunc(assertion,func(r rune)bool{return r==' '||r=='\t'||r=='\n'||r=='\r'||r=='\v'||r=='\f'})," ")
	return stableID("capsule-memory/candidate/v1\n"+scope+"\n"+kind+"\n"+category+"\n"+strings.ToLower(normal)+"\n")
}
func makeInsight(c candidateOutput,known map[string]citation,project string,g records.Generator,auto bool)records.Insight{
	evidence,_:=refs(c.Evidence,known);contradictions,_:=refs(c.Contradictions,known)
	in:=records.Insight{Type:c.Type,Category:c.Category,Assertion:c.Assertion,ProjectID:project,Evidence:evidence,Contradictions:contradictions,Basis:c.Basis,DecisionState:c.DecisionState,Verification:c.Verification,Review:"candidate",Generator:&g}
	user:=false;for _,id:=range c.Evidence{if known[id].Role=="user"{user=true}}
	if auto&&project!=""&&c.Type=="memory"&&c.Category=="fact"&&c.Basis=="explicit"&&user&&len(c.Contradictions)==0{in.Review="accepted";in.AutoAccepted=true}
	return in
}
func (r *Runner) ListCandidates(ctx context.Context,g ops.Grant,scope,review string,limit int)([]InsightView,error){
	if !g.Has(ops.CapKnowledge)&&!g.Has(ops.CapCandidateReview){return nil,ops.ErrForbidden};if !g.AllowsScope(scope){return nil,ops.ErrForbidden};if review!=""&&!oneOf(review,"candidate","accepted","edited","rejected"){return nil,memory.ErrInvalid};if limit<1||limit>100{limit=50}
	out:=[]InsightView{};err:=r.svc.Read(ctx,func(q memory.Queryer)error{
		rows,err:=q.QueryContext(ctx,"SELECT DISTINCT record FROM revisions WHERE scope=? AND kind=? ORDER BY record LIMIT 10001",scope,records.KindInsight);if err!=nil{return err};ids:=[]string{};for rows.Next(){var id string;if err=rows.Scan(&id);err!=nil{rows.Close();return err};ids=append(ids,id)};err=rows.Err();rows.Close();if err!=nil{return err};if len(ids)>10000{return memory.ErrInvalid}
		for _,id:=range ids{hh,del,e:=r.svc.Heads(ctx,q,scope,id);if e!=nil{return e};if del{continue};visible,e:=memory.IsVisible(ctx,q,id);if e!=nil{return e};for _,h:=range hh{var in records.Insight;if json.Unmarshal(h.Body,&in)!=nil{return memory.ErrInvalid};if review!=""&&in.Review!=review{continue};view:=InsightView{Record:id,Revision:h.ID,Insight:in,Stale:!visible,Conflict:len(hh)>1,Untrusted:true};view.Evidence=r.evidence(ctx,q,g,in.Evidence);out=append(out,view);if len(out)>=limit{return nil}}};return nil
	});return out,err
}
func (r *Runner) Accept(ctx context.Context,g ops.Grant,record string,editedAssertion *string)(InsightView,error){return r.review(ctx,g,record,editedAssertion,false)}
func (r *Runner) Reject(ctx context.Context,g ops.Grant,record string)(InsightView,error){return r.review(ctx,g,record,nil,true)}
func (r *Runner) review(ctx context.Context,g ops.Grant,record string,edited *string,reject bool)(InsightView,error){
	var result InsightView;if !g.Has(ops.CapCandidateReview){return result,ops.ErrForbidden}
	var rev memory.Revision;var in records.Insight
	err:=r.svc.Read(ctx,func(q memory.Queryer)error{var scope string;if err:=q.QueryRowContext(ctx,"SELECT scope FROM revisions WHERE record=? AND kind=? LIMIT 1",record,records.KindInsight).Scan(&scope);err!=nil{return err};if !g.AllowsScope(scope){return ops.ErrForbidden};hh,del,err:=r.svc.Heads(ctx,q,scope,record);if err!=nil{return err};if del||len(hh)!=1{return memory.ErrInvalid};if !reject{visible,e:=memory.IsVisible(ctx,q,record);if e!=nil||!visible{return memory.ErrInvalid}};rev=hh[0];if json.Unmarshal(rev.Body,&in)!=nil{return memory.ErrInvalid};return nil});if err!=nil{return result,err}
	info,err:=r.svc.Info(ctx);if err!=nil{return result,err};in.Review="accepted";if reject{in.Review="rejected"};in.Reviewer=info.Device;in.AutoAccepted=false
	if edited!=nil{if !bounded(*edited){return result,memory.ErrInvalid};in.Assertion=*edited;in.Review="edited"}
	p,err:=r.svc.Prepare(ctx,memory.PutInput{Scope:rev.Scope,Kind:records.KindInsight,Record:record,Body:in,Provenance:memory.Provenance{SourceKind:"manual",SourceRef:"candidate-review"}});if err!=nil{return result,err}
	err=r.svc.Commit(ctx,[]memory.Prepared{p},func(ctx context.Context,tx *sql.Tx)error{if !reject{visible,e:=memory.IsVisible(ctx,tx,record);if e!=nil{return e};if !visible{return memory.ErrHeadsChanged};for _,ref:=range in.Evidence{visible,e:=memory.IsVisible(ctx,tx,ref.Record);if e!=nil{return e};if !visible{return memory.ErrHeadsChanged}}};return nil});if err!=nil{return result,err};result=InsightView{Record:record,Revision:p.Revision.ID,Insight:in,Untrusted:true};return result,nil
}
func (r *Runner) evidence(ctx context.Context,q memory.Queryer,g ops.Grant,refs []records.SourceRef)[]EvidenceView{
	out:=[]EvidenceView{};for _,ref:=range refs{view:=EvidenceView{Ref:ref};rev,err:=r.svc.Revision(ctx,q,ref.Revision);if err==nil&&g.Has(ops.CapSessionRead)&&g.AllowsScope(rev.Scope){visible,e:=memory.RefVisible(ctx,q,ref,true);if e==nil&&visible{view.Accessible=true;if rev.Kind==records.KindSessionChunk{var chunk records.SessionChunk;if json.Unmarshal(rev.Body,&chunk)==nil{for _,event:=range chunk.Events{if event.ID==ref.EventFrom{view.Snippet=event.Text;if len(view.Snippet)>512{view.Snippet=view.Snippet[:512]};break}}}}}};out=append(out,view)};return out
}
