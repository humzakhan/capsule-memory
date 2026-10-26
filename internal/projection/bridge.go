package projection

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/session"
)

type Bridge struct { source *memory.Service; sess *session.Module; targets map[string]*memory.Service }
type LinkView struct { LinkID,State,SourceRecord,TargetScope,TargetRecord,VerifiedAt,Error string }
func NewBridge(source *memory.Service,sess *session.Module,targets map[string]*memory.Service)*Bridge{return &Bridge{source:source,sess:sess,targets:targets}}
func hashText(s string)string{h:=sha256.Sum256([]byte(s));return hex.EncodeToString(h[:])}
func linkRecord(role,link string)string{return hashText("capsule-memory/projection-record/v1\n"+role+"\n"+link)}
func (b *Bridge) insight(ctx context.Context,q memory.Queryer,record string)(memory.Revision,records.Insight,error){
	var scope string;var in records.Insight
	if err:=q.QueryRowContext(ctx,"SELECT scope FROM revisions WHERE record=? AND kind=? LIMIT 1",record,records.KindInsight).Scan(&scope);err!=nil{return memory.Revision{},in,err}
	heads,deleted,err:=b.source.Heads(ctx,q,scope,record);if err!=nil{return memory.Revision{},in,err};if deleted||len(heads)!=1{return memory.Revision{},in,memory.ErrInvalid}
	visible,err:=memory.IsVisible(ctx,q,record);if err!=nil||!visible{return memory.Revision{},in,memory.ErrInvalid}
	if json.Unmarshal(heads[0].Body,&in)!=nil||in.AutoAccepted||(in.Review!="accepted"&&in.Review!="edited")||in.Reviewer==""{return memory.Revision{},in,memory.ErrInvalid};return heads[0],in,nil
}
func (b *Bridge) targetGrant(ctx context.Context,target *memory.Service,p records.Projection)(string,error){
	info,err:=target.Info(ctx);if err!=nil{return "",err};var id string
	err=target.Read(ctx,func(q memory.Queryer)error{
		if p.Source.Vault==p.Target.Vault{c,e:=SnapshotControl(ctx,q,target);if e!=nil{return e};if !HasScope(c,info.Device,p.Source.Scope){return memory.ErrInvalid};return CheckReaders(c,[]string{p.Source.Scope},p.Target.Scope)}
		all,e:=recordHeads(ctx,q,target,records.KindBridgeGrant);if e!=nil{return e};for _,r:=range all{if r.Scope==p.Target.Scope&&bridgeGrant(ctx,q,target,r.Record,r.Scope,info.Device,p.Source.Vault,p.Target.Vault)==nil{id=r.Record;return nil}};return memory.ErrInvalid
	});return id,err
}
func (b *Bridge) Request(ctx context.Context,insightRecord,targetProfile,targetScope string)(LinkView,error){
	var result LinkView;target:=b.targets[targetProfile];if target==nil{return result,memory.ErrInvalid}
	policy,err:=b.sess.Policy(ctx);if err!=nil{return result,err};srcInfo,err:=b.source.Info(ctx);if err!=nil{return result,err};targetInfo,err:=target.Info(ctx);if err!=nil{return result,err}
	if srcInfo.Vault!=targetInfo.Vault&&policy.CrossVault!="enabled_unaccepted"{return result,errors.New("cross_vault_promotion_off")}
	validScope:=false;for _,s:=range targetInfo.Scopes{if s.ID==targetScope&&(s.Role=="project"||s.Role=="personal"){validScope=true}};if !validScope{return result,memory.ErrInvalid}
	var rev memory.Revision;var in records.Insight
	err=b.source.Read(ctx,func(q memory.Queryer)error{var e error;rev,in,e=b.insight(ctx,q,insightRecord);return e});if err!=nil{return result,err}
	link,err:=memory.NewID();if err!=nil{return result,err}
	deps:=append([]records.SourceRef{},in.Evidence...);deps=append(deps,records.SourceRef{Record:rev.Record,Revision:rev.ID,Kind:records.KindInsight})
	p:=records.Projection{LinkID:link,Role:"source",Source:records.ScopedRef{Vault:rev.Vault,Scope:rev.Scope,Record:rev.Record,Revision:rev.ID},Dependencies:deps,Target:records.ScopedRef{Vault:targetInfo.Vault,Scope:targetScope,Record:linkRecord("knowledge",link)},TextSHA256:hashText(in.Assertion),State:"pending"}
	grant,err:=b.targetGrant(ctx,target,p);if err!=nil{return result,err};p.Grant=grant
	prepared,err:=b.source.Prepare(ctx,memory.PutInput{Scope:rev.Scope,Kind:records.KindProjection,Record:linkRecord("source",link),Body:p,Provenance:memory.Provenance{SourceKind:"projection",SourceRef:link}});if err!=nil{return result,err}
	err=b.source.Commit(ctx,[]memory.Prepared{prepared},func(ctx context.Context,tx *sql.Tx)error{
		current,_,e:=b.insight(ctx,tx,insightRecord);if e!=nil{return e};if current.ID!=rev.ID{return memory.ErrHeadsChanged}
		if _,e=tx.ExecContext(ctx,"INSERT INTO projections(link_id,source_profile,source_record,source_revision,target_profile,target_scope,target_record,state) VALUES(?,?,?,?,?,?,?,?)",link,srcInfo.Home,rev.Record,rev.ID,targetProfile,targetScope,p.Target.Record,"pending");e!=nil{return e}
		_,e=b.source.EnqueueJob(ctx,tx,memory.JobSpec{Kind:"bridge_project",Scope:rev.Scope,Subject:link,Destination:rev.Scope,Inputs:[]records.SourceRef{{Record:rev.Record,Revision:rev.ID,Kind:records.KindInsight}},Priority:1,Versions:"projection/v1"});return e
	});result=LinkView{LinkID:link,State:"pending",SourceRecord:rev.Record,TargetScope:targetScope,TargetRecord:p.Target.Record};return result,err
}
func (b *Bridge) List(ctx context.Context)([]LinkView,error){
	out:=[]LinkView{};err:=b.source.Read(ctx,func(q memory.Queryer)error{rows,e:=q.QueryContext(ctx,"SELECT link_id,state,source_record,target_scope,coalesce(target_record,''),coalesce(verified_at,''),coalesce(error_code,'') FROM projections ORDER BY link_id LIMIT 10000");if e!=nil{return e};defer rows.Close();for rows.Next(){var v LinkView;if e=rows.Scan(&v.LinkID,&v.State,&v.SourceRecord,&v.TargetScope,&v.TargetRecord,&v.VerifiedAt,&v.Error);e!=nil{return e};out=append(out,v)};return rows.Err()});return out,err
}
func (b *Bridge) Withdraw(ctx context.Context,link string)(LinkView,error){
	var p records.Projection;err:=b.source.Read(ctx,func(q memory.Queryer)error{var e error;p,e=b.sourceProjection(ctx,q,link);return e});if err!=nil{return LinkView{},err}
	p.State="withdrawn";prep,err:=b.source.Prepare(ctx,memory.PutInput{Scope:p.Source.Scope,Kind:records.KindProjection,Record:linkRecord("source",link),Body:p,Provenance:memory.Provenance{SourceKind:"manual",SourceRef:link}});if err!=nil{return LinkView{},err}
	err=b.source.Commit(ctx,[]memory.Prepared{prep},func(ctx context.Context,tx *sql.Tx)error{if _,e:=tx.ExecContext(ctx,"UPDATE projections SET state='invalidating',error_code='owner_withdrawal' WHERE link_id=?",link);e!=nil{return e};_,e:=b.source.EnqueueJob(ctx,tx,memory.JobSpec{Kind:"bridge_invalidate",Scope:p.Source.Scope,Subject:link,Destination:p.Source.Scope,Priority:0,Versions:"owner-withdrawal"});return e});return LinkView{LinkID:link,State:"invalidating",SourceRecord:p.Source.Record,TargetScope:p.Target.Scope,TargetRecord:p.Target.Record},err
}
func (b *Bridge) refresh(ctx context.Context)error{
	return b.source.Update(ctx,func(tx *sql.Tx)error{
		rows,err:=tx.QueryContext(ctx,"SELECT link_id,source_revision,state FROM projections WHERE state='hidden' OR (state='projected' AND (verified_at IS NULL OR verified_at<?)) LIMIT 16",time.Now().Add(-24*time.Hour).UTC().Format(time.RFC3339Nano));if err!=nil{return err};type row struct{link,revision,state string};list:=[]row{};for rows.Next(){var v row;if err=rows.Scan(&v.link,&v.revision,&v.state);err!=nil{rows.Close();return err};list=append(list,v)};err=rows.Err();rows.Close();if err!=nil{return err}
		for _,v:=range list{p,e:=b.sourceProjection(ctx,tx,v.link);if e!=nil{continue};version:="attest/"+time.Now().UTC().Format("2006-01-02");if v.state=="hidden"{version="recover/"+time.Now().UTC().Format("2006-01-02T15:04")};_,e=b.source.EnqueueJob(ctx,tx,memory.JobSpec{Kind:"bridge_project",Scope:p.Source.Scope,Subject:v.link,Destination:p.Source.Scope,Priority:1,Versions:version,Inputs:[]records.SourceRef{{Record:p.Source.Record,Revision:v.revision,Kind:records.KindInsight}}});if e!=nil{return e}};return nil
	})
}
func (b *Bridge) RunOnce(ctx context.Context)(bool,error){
	if err:=b.refresh(ctx);err!=nil{return false,err};job,err:=b.source.LeaseJob(ctx,[]string{"bridge_invalidate","bridge_project"},2*time.Minute);if err!=nil||job==nil{return false,err}
	err=b.run(ctx,job);if err!=nil{_ = b.source.FailJob(ctx,job.Key,job.Token,"projection_unavailable",time.Now().Add(time.Minute),false)};return true,err
}
func (b *Bridge) run(ctx context.Context,job *memory.Job)error{
	var p records.Projection;var profile string;var state string;var code sql.NullString;var targetScope,targetRecord string
	err:=b.source.Read(ctx,func(q memory.Queryer)error{var e error;p,e=b.sourceProjection(ctx,q,job.Spec.Subject);if e!=nil{return e};return q.QueryRowContext(ctx,"SELECT target_profile,state,error_code,target_scope,target_record FROM projections WHERE link_id=?",p.LinkID).Scan(&profile,&state,&code,&targetScope,&targetRecord)});if err!=nil{return err};target:=b.targets[profile];if target==nil||p.Source.Scope!=job.Spec.Scope||job.Spec.Destination!=job.Spec.Scope||p.Target.Scope!=targetScope||p.Target.Record!=targetRecord{return memory.ErrInvalid}
	targetInfo,err:=target.Info(ctx);if err!=nil{return err};if targetInfo.Vault!=p.Target.Vault{return memory.ErrInvalid};roleOK:=false;for _,scope:=range targetInfo.Scopes{if scope.ID==targetScope&&(scope.Role=="project"||scope.Role=="personal"){roleOK=true}};if !roleOK{return memory.ErrInvalid}
	policy,err:=b.sess.Policy(ctx);if err!=nil{return err};if job.Spec.Kind=="bridge_project"&&p.Source.Vault!=p.Target.Vault&&policy.CrossVault!="enabled_unaccepted"{return errors.New("cross_vault_promotion_off")}
	invalidate:=job.Spec.Kind=="bridge_invalidate";withdraw:=invalidate;hidden:=false
	var in records.Insight;var rev memory.Revision
	if code.String!="owner_withdrawal"{
		err=b.source.Read(ctx,func(q memory.Queryer)error{var e error;rev,in,e=b.insight(ctx,q,p.Source.Record);return e})
		if err==nil{p.Source.Revision=rev.ID;p.Dependencies=append([]records.SourceRef{},in.Evidence...);p.Dependencies=append(p.Dependencies,records.SourceRef{Record:rev.Record,Revision:rev.ID,Kind:records.KindInsight});p.TextSHA256=hashText(in.Assertion);withdraw=false}else if !invalidate{return err}
	}
	var fresh FreshResult
	if !withdraw{
		// Persist the newer source before live verification so VerifyFresh checks exact bytes.
		if rev.ID!=""&&(rev.ID!=jobInputRevision(job)||p.State!="pending"){p.State="pending";pre,e:=b.source.Prepare(ctx,memory.PutInput{Scope:p.Source.Scope,Kind:records.KindProjection,Record:linkRecord("source",p.LinkID),Body:p,Provenance:memory.Provenance{SourceKind:"projection",SourceRef:p.LinkID}});if e!=nil{return e};if e=b.source.Commit(ctx,[]memory.Prepared{pre},nil);e!=nil{return e}}
		checks,e:=b.VerifyFresh(ctx,[]string{p.LinkID});fresh=checks[p.LinkID];if e!=nil||!fresh.Current{
			if invalidate||state=="projected"{hidden=true;withdraw=true}else{return errors.New("fresh_check_unavailable")}
		}
	}
	grant,e:=b.targetGrant(ctx,target,p);if e!=nil{if !invalidate{return e};hidden=true;withdraw=true};p.Grant=grant
	var targetRevision string
	if withdraw{
		p.State="withdrawn";if hidden{p.State="hidden"};p.Role="target"
		set:=[]memory.Prepared{};var existing []memory.Revision;var deleted bool
		err=target.Read(ctx,func(q memory.Queryer)error{var e error;existing,deleted,e=target.Heads(ctx,q,p.Target.Scope,p.Target.Record);return e});if err!=nil{return err}
		if len(existing)>0&&!deleted{d,e:=target.Prepare(ctx,memory.PutInput{Scope:p.Target.Scope,Kind:records.KindKnowledge,Record:p.Target.Record,Operation:"delete",Provenance:memory.Provenance{SourceKind:"projection",SourceRef:p.LinkID}});if e!=nil{return e};set=append(set,d);targetRevision=d.Revision.ID}
		l,e:=target.Prepare(ctx,memory.PutInput{Scope:p.Target.Scope,Kind:records.KindProjection,Record:linkRecord("target",p.LinkID),Body:p,Provenance:memory.Provenance{SourceKind:"projection",SourceRef:p.LinkID}});if e!=nil{return e};set=append(set,l);if err=target.Commit(ctx,set,nil);err!=nil{return err}
	}else{
		var already bool
		err=target.Read(ctx,func(q memory.Queryer)error{hh,del,e:=target.Heads(ctx,q,p.Target.Scope,p.Target.Record);if e!=nil{return e};if !del&&len(hh)==1{var k records.Knowledge;if json.Unmarshal(hh[0].Body,&k)==nil&&k.Source==p.Source&&k.TextSHA256==p.TextSHA256{v,e:=TargetVisible(ctx,q,target,p.Target.Record);if e!=nil{return e};if v.Visible{targetRevision=hh[0].ID;already=true}}};return nil});if err!=nil{return err}
		set:=[]memory.Prepared{}
		if !already{
			k:=records.Knowledge{LinkID:p.LinkID,Type:in.Type,Category:in.Category,Text:in.Assertion,TextSHA256:p.TextSHA256,ProjectID:in.ProjectID,Source:p.Source,Review:in.Review}
			input:=memory.PutInput{Scope:p.Target.Scope,Kind:records.KindKnowledge,Record:p.Target.Record,Body:k,Provenance:memory.Provenance{SourceKind:"projection",SourceRef:p.LinkID}}
			// An explicit new accepted source may restore a previously withdrawn knowledge record.
			err=target.Read(ctx,func(q memory.Queryer)error{_,deleted,e:=target.Heads(ctx,q,p.Target.Scope,p.Target.Record);if e!=nil{return e};if deleted{input.Operation="restore";input.Restores,e=target.UnrestoredDeletions(ctx,q,p.Target.Record);if e!=nil{return e}};return nil});if err!=nil{return err}
			kp,e:=target.Prepare(ctx,input);if e!=nil{return e};targetRevision=kp.Revision.ID;set=append(set,kp)
		}
		p.Role="target";p.State="projected";p.Target.Revision=targetRevision;p.Attestation=&records.Attestation{Bridge:rev.Author,VerifiedHeads:fresh.VerifiedHeads,VerifiedAt:fresh.VerifiedAt,Coverage:"complete"}
		info,e:=target.Info(ctx);if e!=nil{return e};p.Attestation.Bridge=info.Device
		lp,e:=target.Prepare(ctx,memory.PutInput{Scope:p.Target.Scope,Kind:records.KindProjection,Record:linkRecord("target",p.LinkID),Body:p,Provenance:memory.Provenance{SourceKind:"projection",SourceRef:p.LinkID}});if e!=nil{return e};set=append(set,lp)
		if err=target.Commit(ctx,set,func(ctx context.Context,tx *sql.Tx)error{if p.Source.Vault==p.Target.Vault{c,e:=SnapshotControl(ctx,tx,target);if e!=nil{return e};if !HasScope(c,info.Device,p.Source.Scope){return memory.ErrInvalid};return CheckReaders(c,[]string{p.Source.Scope},p.Target.Scope)};return bridgeGrant(ctx,tx,target,p.Grant,p.Target.Scope,info.Device,p.Source.Vault,p.Target.Vault)});err!=nil{return err}
	}
	p.Role="source";p.Target.Revision=targetRevision
	sp,err:=b.source.Prepare(ctx,memory.PutInput{Scope:p.Source.Scope,Kind:records.KindProjection,Record:linkRecord("source",p.LinkID),Body:p,Provenance:memory.Provenance{SourceKind:"projection",SourceRef:p.LinkID}});if err!=nil{return err}
	return b.source.Commit(ctx,[]memory.Prepared{sp},func(ctx context.Context,tx *sql.Tx)error{
		if _,e:=tx.ExecContext(ctx,"UPDATE projections SET source_revision=?,target_revision=?,state=?,verified_head=?,verified_at=?,error_code=NULL WHERE link_id=?",p.Source.Revision,p.Target.Revision,p.State,encodeHeads(fresh.VerifiedHeads),fresh.VerifiedAt,p.LinkID);e!=nil{return e};return b.source.CompleteJob(ctx,tx,job.Key,job.Token,p.Target.Record)
	})
}
func jobInputRevision(job *memory.Job)string{if len(job.Spec.Inputs)>0{return job.Spec.Inputs[0].Revision};return ""}
func encodeHeads(heads []ScopeHead)string{b,_:=json.Marshal(heads);return string(b)}
