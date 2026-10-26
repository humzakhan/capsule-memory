package projection

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

type applier struct {svc *memory.Service;kind string}
func Install(svc *memory.Service){
	for _,kind:=range []string{records.KindKnowledge,records.KindProjection,records.KindBridgeGrant,records.KindAggregationGrant}{svc.RegisterApplier(kind,&applier{svc:svc,kind:kind})}
	svc.OnGrantCheck(func(ctx context.Context,q memory.Queryer,c memory.Control)error{
		all,err:=recordHeads(ctx,q,svc,records.KindAggregationGrant);if err!=nil{return err};for _,r:=range all{var signed records.AggregationGrant;var body records.AggregationGrantBody;if json.Unmarshal(r.Body,&signed)!=nil{return memory.ErrInvalid};if err=svc.VerifyOwner(aggregationDomain,memory.Signed{Payload:signed.Payload,Signature:signed.Signature},&body);err!=nil{return err};if !body.Revoked{if err=CheckReaders(c,body.Sources,body.Destination);err!=nil{return err}}};return nil
	})
	svc.OnStale(func(ctx context.Context,tx *sql.Tx,affected []memory.StaleRef)error{
		for _,a:=range affected{
			rows,err:=tx.QueryContext(ctx,"SELECT link_id,source_record FROM projections WHERE source_record=? AND state NOT IN ('withdrawn','hidden')",a.Record);if err!=nil{return err};type item struct{link,record string};list:=[]item{};for rows.Next(){var i item;if err=rows.Scan(&i.link,&i.record);err!=nil{rows.Close();return err};list=append(list,i)};err=rows.Err();rows.Close();if err!=nil{return err}
			for _,i:=range list{if _,err=tx.ExecContext(ctx,"UPDATE projections SET state='invalidating' WHERE link_id=?",i.link);err!=nil{return err};if _,err=svc.EnqueueJob(ctx,tx,memory.JobSpec{Kind:"bridge_invalidate",Scope:a.Scope,Subject:i.link,Destination:a.Scope,Priority:0,Versions:a.Reason});err!=nil{return err}}
		};return nil
	})
}
func (a *applier) Apply(ctx context.Context,tx *sql.Tx,r memory.Revision,origin memory.ApplyOrigin)error{
	if r.Operation=="delete"{_,err:=tx.ExecContext(ctx,"DELETE FROM session_search WHERE record=?",r.Record);return err}
	switch r.Kind{
	case records.KindKnowledge:
		var k records.Knowledge;if json.Unmarshal(r.Body,&k)!=nil{return memory.ErrInvalid}
		if _,err:=tx.ExecContext(ctx,"INSERT INTO knowledge(link_id,scope,record) VALUES(?,?,?) ON CONFLICT(link_id) DO UPDATE SET scope=excluded.scope,record=excluded.record",k.LinkID,r.Scope,r.Record);err!=nil{return err}
		if _,err:=tx.ExecContext(ctx,"DELETE FROM session_search WHERE record=?",r.Record);err!=nil{return err}
		_,err:=tx.ExecContext(ctx,"INSERT INTO session_search(scope,record,revision,kind,session,text) VALUES(?,?,?,?,?,?)",r.Scope,r.Record,r.ID,r.Kind,"",k.Text);return err
	case records.KindProjection:
		var p records.Projection;if json.Unmarshal(r.Body,&p)!=nil{return memory.ErrInvalid}
		if p.Role=="source"{if p.Source.Vault!=r.Vault||p.Source.Scope!=r.Scope{return memory.ErrInvalid};deps:=append([]records.SourceRef{},p.Dependencies...);deps=append(deps,records.SourceRef{Record:p.Source.Record,Revision:p.Source.Revision,Kind:records.KindInsight});if e:=a.svc.SetDeps(ctx,tx,r,deps);e!=nil{return e};return a.svc.RecomputeStale(ctx,tx)}
		var at string;var heads []byte;bridge:="";if p.Attestation!=nil{at=p.Attestation.VerifiedAt;heads,_=json.Marshal(p.Attestation.VerifiedHeads);bridge=p.Attestation.Bridge}
		_,err:=tx.ExecContext(ctx,"INSERT INTO ledger(link_id,target_record,state,bridge,grant_record,verified_head,verified_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(link_id) DO UPDATE SET target_record=excluded.target_record,state=excluded.state,bridge=excluded.bridge,grant_record=excluded.grant_record,verified_head=excluded.verified_head,verified_at=excluded.verified_at",p.LinkID,p.Target.Record,p.State,bridge,p.Grant,string(heads),at);return err
	case records.KindBridgeGrant:
		var signed records.BridgeGrant;var body records.BridgeGrantBody;if json.Unmarshal(r.Body,&signed)!=nil{return memory.ErrInvalid};if err:=a.svc.VerifyOwner(bridgeDomain,memory.Signed{Payload:signed.Payload,Signature:signed.Signature},&body);err!=nil{return err};if body.LedgerScope!=r.Scope||len(body.TargetScopes)!=1||body.TargetScopes[0]!=r.Scope||body.TargetVault!=r.Vault{return memory.ErrInvalid};scopes,_:=json.Marshal(body.TargetScopes)
		_,err:=tx.ExecContext(ctx,"INSERT INTO bridge_grants(record,bridge,ledger_scope,target_scopes,revoked) VALUES(?,?,?,?,?) ON CONFLICT(record) DO UPDATE SET bridge=excluded.bridge,ledger_scope=excluded.ledger_scope,target_scopes=excluded.target_scopes,revoked=excluded.revoked",r.Record,body.Bridge,body.LedgerScope,string(scopes),body.Revoked);return err
	case records.KindAggregationGrant:
		var signed records.AggregationGrant;var body records.AggregationGrantBody;if json.Unmarshal(r.Body,&signed)!=nil{return memory.ErrInvalid};if err:=a.svc.VerifyOwner(aggregationDomain,memory.Signed{Payload:signed.Payload,Signature:signed.Signature},&body);err!=nil{return err};if body.Destination!=r.Scope||body.Vault!=r.Vault{return memory.ErrInvalid};sources,_:=json.Marshal(body.Sources)
		_,err:=tx.ExecContext(ctx,"INSERT INTO aggregation_grants(record,destination,sources,revoked) VALUES(?,?,?,?) ON CONFLICT(record) DO UPDATE SET destination=excluded.destination,sources=excluded.sources,revoked=excluded.revoked",r.Record,body.Destination,string(sources),body.Revoked);return err
	};return memory.ErrInvalid
}
func (a *applier) Rebuild(ctx context.Context,tx *sql.Tx)error{
	all,err:=recordHeads(ctx,tx,a.svc,a.kind);if err!=nil{return err};for _,r:=range all{if err=a.Apply(ctx,tx,r,memory.ApplyReplay);err!=nil{return err}};return nil
}
// SearchQuery keeps FTS operators out of a plain-text search.
func SearchQuery(query string)string{words:=strings.Fields(query);if len(words)>32{words=words[:32]};for i,w:=range words{words[i]="\""+strings.ReplaceAll(w,"\"","\"\"")+"\""};return strings.Join(words," AND ")}
