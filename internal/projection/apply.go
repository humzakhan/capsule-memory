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
 svc.SetProjectionPublicationCheck(func(ctx context.Context,q memory.Queryer,r memory.Revision)(records.Projection,error){return publicationProjection(ctx,q,svc,r)})
 svc.OnRevision(func(ctx context.Context,tx *sql.Tx,r memory.Revision)error{
  if r.Kind!=records.KindInsight{return nil}
  rows,e:=tx.QueryContext(ctx,"SELECT link_id,source_scope FROM projections WHERE source_record=? AND source_scope=? AND owner_withdrawn=0 LIMIT 100001",r.Record,r.Scope);if e!=nil{return e};type item struct{link,scope string};var items []item;for rows.Next(){var i item;if rows.Scan(&i.link,&i.scope)!=nil{rows.Close();return memory.ErrStorage};items=append(items,i)};e=rows.Err();rows.Close();if e!=nil{return e};if len(items)>100000{return memory.ErrStaleOverflow}
  for _,i:=range items{if e=queueBridge(ctx,tx,svc,i.scope,i.link,"source_changed");e!=nil{return e}};return nil
 })
	for _,kind:=range []string{records.KindKnowledge,records.KindProjection,records.KindBridgeGrant,records.KindAggregationGrant}{svc.RegisterApplier(kind,&applier{svc:svc,kind:kind})}
	svc.OnGrantCheck(func(ctx context.Context,q memory.Queryer,c memory.Control)error{
		all,err:=recordHeads(ctx,q,svc,records.KindAggregationGrant);if err!=nil{return err};for _,r:=range all{var signed records.AggregationGrant;var body records.AggregationGrantBody;if json.Unmarshal(r.Body,&signed)!=nil{return memory.ErrInvalid};if err=svc.VerifyOwner(aggregationDomain,memory.Signed{Payload:signed.Payload,Signature:signed.Signature},&body);err!=nil{return err};if !body.Revoked{if err=CheckReaders(c,body.Sources,body.Destination);err!=nil{return err}}};return nil
	})
	svc.OnStale(func(ctx context.Context,tx *sql.Tx,affected []memory.StaleRef)error{
 seen:=map[string]bool{}
		for _,a:=range affected{
			rows,err:=tx.QueryContext(ctx,"SELECT link_id,source_record FROM projections WHERE owner_withdrawn=0 AND (source_record=? OR link_id IN (SELECT link_id FROM projection_records WHERE record=? AND role='source'))",a.Record,a.Record);if err!=nil{return err};type item struct{link,record string};list:=[]item{};for rows.Next(){var i item;if err=rows.Scan(&i.link,&i.record);err!=nil{rows.Close();return err};list=append(list,i)};err=rows.Err();rows.Close();if err!=nil{return err}
			for _,i:=range list{if seen[i.link]{continue};seen[i.link]=true;if err=queueBridge(ctx,tx,svc,a.Scope,i.link,a.Reason);err!=nil{return err}}
		};return nil
	})
}
func (a *applier) Apply(ctx context.Context,tx *sql.Tx,r memory.Revision,origin memory.ApplyOrigin)error{
	if r.Operation=="delete"{for _,table:=range []string{"session_search","knowledge","bridge_grants","aggregation_grants"}{if _,err:=tx.ExecContext(ctx,"DELETE FROM "+table+" WHERE record=?",r.Record);err!=nil{return err}};return nil}
	switch r.Kind{
	case records.KindKnowledge:
		var k records.Knowledge;if json.Unmarshal(r.Body,&k)!=nil{return memory.ErrInvalid}
		if _,err:=tx.ExecContext(ctx,"INSERT INTO knowledge(link_id,scope,record) VALUES(?,?,?) ON CONFLICT(link_id,scope,record) DO NOTHING",k.LinkID,r.Scope,r.Record);err!=nil{return err}
		if _,err:=tx.ExecContext(ctx,"DELETE FROM session_search WHERE revision=?",r.ID);err!=nil{return err}
		_,err:=tx.ExecContext(ctx,"INSERT INTO session_search(scope,record,revision,kind,session,text) VALUES(?,?,?,?,?,?)",r.Scope,r.Record,r.ID,r.Kind,"",k.Text);return err
	case records.KindProjection:
		var p records.Projection;if json.Unmarshal(r.Body,&p)!=nil||p.Validate()!=nil{return memory.ErrInvalid};if p.WithdrawalMarker{if e:=a.svc.VerifyProjectionMarker(ctx,tx,r,p,false);e!=nil{return e}}else if (p.State=="hidden"||p.State=="withdrawn")&&len(r.Parents)==0{return memory.ErrInvalid};if _,err:=tx.ExecContext(ctx,"INSERT INTO projection_records(record,scope,link_id,role,target_record) VALUES(?,?,?,?,?) ON CONFLICT(record,scope,link_id,role,target_record) DO NOTHING",r.Record,r.Scope,p.LinkID,p.Role,p.Target.Record);err!=nil{return err}
		// Terminal intent is monotonic across history, including concurrent heads.
		if p.Role=="source"{if p.Source.Vault!=r.Vault||p.Source.Scope!=r.Scope{return memory.ErrInvalid};if origin==memory.ApplyReplay{if _,e:=tx.ExecContext(ctx,"UPDATE projections SET generation=max(generation,?) WHERE link_id=? AND source_vault=? AND source_scope=? AND source_record=? AND target_vault=? AND target_scope=? AND target_record=?",p.Generation,p.LinkID,p.Source.Vault,p.Source.Scope,p.Source.Record,p.Target.Vault,p.Target.Scope,p.Target.Record);e!=nil{return e}};if p.OwnerWithdrawn{var prior bool;e:=tx.QueryRowContext(ctx,"SELECT owner_withdrawn FROM projections WHERE link_id=? AND source_vault=? AND source_scope=? AND source_record=? AND target_vault=? AND target_scope=? AND target_record=?",p.LinkID,p.Source.Vault,p.Source.Scope,p.Source.Record,p.Target.Vault,p.Target.Scope,p.Target.Record).Scan(&prior);if e!=nil&&e!=sql.ErrNoRows{return e};if _,err:=tx.ExecContext(ctx,"UPDATE projections SET owner_withdrawn=1 WHERE link_id=? AND source_vault=? AND source_scope=? AND source_record=? AND target_vault=? AND target_scope=? AND target_record=?",p.LinkID,p.Source.Vault,p.Source.Scope,p.Source.Record,p.Target.Vault,p.Target.Scope,p.Target.Record);err!=nil{return err};if e==nil&&!prior&&origin==memory.ApplyReplay{if e=queueBridge(ctx,tx,a.svc,r.Scope,p.LinkID,"owner_withdrawal");e!=nil{return e}}};if p.Source.Vault!=r.Vault||p.Source.Scope!=r.Scope{return memory.ErrInvalid};if p.WithdrawalMarker{return nil};deps:=append([]records.SourceRef{},p.Dependencies...);deps=append(deps,records.SourceRef{Record:p.Source.Record,Revision:p.Source.Revision,Kind:records.KindInsight});if e:=a.svc.SetDeps(ctx,tx,r,deps);e!=nil{return e};return nil}
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
	return memory.ScanRevisions(ctx,tx,a.kind,func(r memory.Revision)error{if r.Kind==records.KindProjection&&r.Operation!="delete"{return a.Apply(ctx,tx,r,memory.ApplyReplay)};hh,deleted,e:=a.svc.Heads(ctx,tx,r.Scope,r.Record);if e!=nil{return e};if deleted{return a.Apply(ctx,tx,memory.Revision{Record:r.Record,Operation:"delete"},memory.ApplyReplay)};for _,h:=range hh{if h.ID==r.ID{return a.Apply(ctx,tx,r,memory.ApplyReplay)}};return nil})
}
// SearchQuery keeps FTS operators out of a plain-text search.
func SearchQuery(query string)string{words:=strings.Fields(query);if len(words)>32{words=words[:32]};for i,w:=range words{words[i]="\""+strings.ReplaceAll(w,"\"","\"\"")+"\""};return strings.Join(words," AND ")}
