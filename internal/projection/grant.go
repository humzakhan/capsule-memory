package projection

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

var ErrReadersExceedSources = errors.New("destination_readers_exceed_sources")
const bridgeDomain = "capsule-memory/bridge-grant/v1"
const aggregationDomain = "capsule-memory/aggregation-grant/v1"

func HasScope(c memory.Control, device, scope string) bool {
	for _, d := range c.Devices { if d.ID==device && !d.Revoked { for _, s := range d.Scopes { if s==scope { return true } } } }; return false
}
func CheckReaders(c memory.Control, sources []string, destination string) error {
	for _, d := range c.Devices { if HasScope(c,d.ID,destination) { for _, scope := range sources { if !HasScope(c,d.ID,scope) { return ErrReadersExceedSources } } } }; return nil
}
// SnapshotControl reads the signed control from the caller's snapshot/transaction.
func SnapshotControl(ctx context.Context, q memory.Queryer, svc *memory.Service) (memory.Control,error) {
	var raw []byte; var signed memory.Signed; var c memory.Control
	if err:=q.QueryRowContext(ctx,"SELECT value FROM meta WHERE key='current_control'").Scan(&raw); err!=nil { return c,err }
	if json.Unmarshal(raw,&signed)!=nil { return c,memory.ErrInvalid }
	// current_control is validated by memory before it enters this private DB.
	// Read its payload in this snapshot rather than opening a second snapshot.
	payload,err:=base64.StdEncoding.Strict().DecodeString(signed.Payload);if err!=nil||json.Unmarshal(payload,&c)!=nil{return c,memory.ErrInvalid};return c,nil
}
func recordHeads(ctx context.Context, q memory.Queryer, svc *memory.Service, kind string) ([]memory.Revision,error) {
	rows,err:=q.QueryContext(ctx,"SELECT DISTINCT scope,record FROM revisions WHERE kind=? ORDER BY record LIMIT 10001",kind);if err!=nil{return nil,err}
	type key struct{scope,record string}; keys:=[]key{}
	for rows.Next(){var k key;if err=rows.Scan(&k.scope,&k.record);err!=nil{rows.Close();return nil,err};keys=append(keys,k)}
	err=rows.Err();rows.Close();if err!=nil{return nil,err};if len(keys)>10000{return nil,memory.ErrInvalid}
	out:=[]memory.Revision{};for _,k:=range keys{heads,deleted,e:=svc.Heads(ctx,q,k.scope,k.record);if e!=nil{return nil,e};if !deleted{out=append(out,heads...)}};return out,nil
}
func Aggregation(ctx context.Context,q memory.Queryer,svc *memory.Service,id string,c memory.Control)(records.AggregationGrantBody,error){
	var body records.AggregationGrantBody;var scope string
	if err:=q.QueryRowContext(ctx,"SELECT scope FROM revisions WHERE record=? AND kind=? LIMIT 1",id,records.KindAggregationGrant).Scan(&scope);err!=nil{return body,err}
	heads,deleted,err:=svc.Heads(ctx,q,scope,id);if err!=nil{return body,err};if deleted||len(heads)!=1{return body,memory.ErrInvalid}
	var signed records.AggregationGrant;if json.Unmarshal(heads[0].Body,&signed)!=nil{return body,memory.ErrInvalid}
	if err=svc.VerifyOwner(aggregationDomain,memory.Signed{Payload:signed.Payload,Signature:signed.Signature},&body);err!=nil{return body,err}
	if body.Revoked||body.Version!=1||body.Vault!=c.Vault||body.Destination!=scope{return body,memory.ErrInvalid}
	if err=CheckReaders(c,body.Sources,body.Destination);err!=nil{return body,err};return body,nil
}
func CheckAggregation(ctx context.Context,q memory.Queryer,svc *memory.Service,ids []string,inputs []records.SourceRef,destination string,c memory.Control)error{
	for _,id:=range ids { b,err:=Aggregation(ctx,q,svc,id,c);if err!=nil||b.Destination!=destination{continue};allowed:=map[string]bool{};for _,s:=range b.Sources{allowed[s]=true};valid:=true
		for _,in:=range inputs{r,e:=svc.Revision(ctx,q,in.Revision);if e!=nil||r.Record!=in.Record||!allowed[r.Scope]{valid=false;break}}
		if valid{return nil}
	};return ErrReadersExceedSources
}
func GrantAggregation(ctx context.Context,owner *memory.Service,body records.AggregationGrantBody)(string,error){
	info,err:=owner.Info(ctx);if err!=nil{return "",err};c,err:=owner.Control(ctx);if err!=nil{return "",err}
	if body.Version!=1||body.Vault!=info.Vault||len(body.Sources)==0||len(body.Sources)>64||body.Name==""{return "",memory.ErrInvalid}
	if !body.Revoked{if err=CheckReaders(c,body.Sources,body.Destination);err!=nil{return "",err}}
	signed,err:=owner.OwnerSign(aggregationDomain,body);if err!=nil{return "",err}
	record:="";err=owner.Read(ctx,func(q memory.Queryer)error{all,e:=recordHeads(ctx,q,owner,records.KindAggregationGrant);if e!=nil{return e};for _,r:=range all{if r.Scope!=body.Destination{continue};var s records.AggregationGrant;var old records.AggregationGrantBody;if json.Unmarshal(r.Body,&s)!=nil{return memory.ErrInvalid};if e=owner.VerifyOwner(aggregationDomain,memory.Signed{Payload:s.Payload,Signature:s.Signature},&old);e!=nil{return e};if old.Name==body.Name{if record!=""&&record!=r.Record{return memory.ErrInvalid};record=r.Record}};return nil});if err!=nil{return "",err}
	p,err:=owner.Prepare(ctx,memory.PutInput{Scope:body.Destination,Kind:records.KindAggregationGrant,Record:record,Body:records.AggregationGrant{Payload:signed.Payload,Signature:signed.Signature},Provenance:memory.Provenance{SourceKind:"manual",SourceRef:"aggregation-grant"}});if err!=nil{return "",err}
	err=owner.Commit(ctx,[]memory.Prepared{p},func(ctx context.Context,tx *sql.Tx)error{if body.Revoked{return nil};current,e:=SnapshotControl(ctx,tx,owner);if e!=nil{return e};return CheckReaders(current,body.Sources,body.Destination)});return p.Revision.Record,err
}
func GrantBridge(ctx context.Context,owner *memory.Service,body records.BridgeGrantBody)(string,error){
	info,err:=owner.Info(ctx);if err!=nil{return "",err}
	if body.Version!=1||body.TargetVault!=info.Vault||len(body.TargetScopes)!=1||body.TargetScopes[0]!=body.LedgerScope{return "",memory.ErrInvalid}
	roleOK:=false;for _,s:=range info.Scopes{if s.ID==body.LedgerScope&&(s.Role=="project"||s.Role=="personal"){roleOK=true}};if !roleOK{return "",memory.ErrInvalid}
	signed,err:=owner.OwnerSign(bridgeDomain,body);if err!=nil{return "",err}
	record:="";err=owner.Read(ctx,func(q memory.Queryer)error{all,e:=recordHeads(ctx,q,owner,records.KindBridgeGrant);if e!=nil{return e};for _,r:=range all{if r.Scope!=body.LedgerScope{continue};var s records.BridgeGrant;var old records.BridgeGrantBody;if json.Unmarshal(r.Body,&s)!=nil{return memory.ErrInvalid};if e=owner.VerifyOwner(bridgeDomain,memory.Signed{Payload:s.Payload,Signature:s.Signature},&old);e!=nil{return e};if old.SourceVault==body.SourceVault&&old.Bridge==body.Bridge{if record!=""&&record!=r.Record{return memory.ErrInvalid};record=r.Record}};return nil});if err!=nil{return "",err}
	p,err:=owner.Prepare(ctx,memory.PutInput{Scope:body.LedgerScope,Kind:records.KindBridgeGrant,Record:record,Body:records.BridgeGrant{Payload:signed.Payload,Signature:signed.Signature},Provenance:memory.Provenance{SourceKind:"manual",SourceRef:"bridge-grant"}});if err!=nil{return "",err}
	err=owner.Commit(ctx,[]memory.Prepared{p},nil);return p.Revision.Record,err
}
func bridgeGrant(ctx context.Context,q memory.Queryer,svc *memory.Service,record,scope,author,sourceVault,targetVault string)error{
	heads,deleted,err:=svc.Heads(ctx,q,scope,record);if err!=nil{return err};if deleted||len(heads)!=1{return memory.ErrInvalid}
	var signed records.BridgeGrant;var body records.BridgeGrantBody
	if json.Unmarshal(heads[0].Body,&signed)!=nil{return memory.ErrInvalid}
	if err=svc.VerifyOwner(bridgeDomain,memory.Signed{Payload:signed.Payload,Signature:signed.Signature},&body);err!=nil{return err}
	if body.Revoked||body.Bridge!=author||body.SourceVault!=sourceVault||body.TargetVault!=targetVault||body.LedgerScope!=scope||len(body.TargetScopes)!=1||body.TargetScopes[0]!=scope{return memory.ErrInvalid};return nil
}
