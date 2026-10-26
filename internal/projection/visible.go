package projection

import (
	"context"
	"encoding/json"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/records"
)

type Mode int
const (Historical Mode = iota; Strict)
type ScopeHead = records.ScopeHead
type Freshness interface { VerifyFresh(context.Context, []string) (map[string]FreshResult,error) }
type FreshResult struct { Current bool; VerifiedHeads []ScopeHead; VerifiedAt, Reason string; source,target records.ScopedRef;link string;guard func(context.Context)bool }
type Visibility struct { Guard func(context.Context)bool `json:"-"`;Revision string;Visible, Current bool; LastVerifiedAt string; LastVerifiedHeads []ScopeHead; Reason string }

// TargetVisible checks lifecycle in the caller's snapshot. It never infers freshness.
func TargetVisible(ctx context.Context,q memory.Queryer,target *memory.Service,record string)(Visibility,error){
	v:=Visibility{Reason:"hidden"};var scope string
	err:=q.QueryRowContext(ctx,"SELECT scope FROM revisions WHERE record=? AND kind=? LIMIT 1",record,records.KindKnowledge).Scan(&scope);if err!=nil{return v,nil}
	var role string;if q.QueryRowContext(ctx,"SELECT role FROM scope_labels WHERE scope=?",scope).Scan(&role)!=nil||role!="project"&&role!="personal"{return v,nil}
	heads,deleted,err:=target.Heads(ctx,q,scope,record);if err!=nil{return v,err};if deleted{v.Reason="deleted";return v,nil};if len(heads)!=1{return v,nil}
	var stale int;if err=q.QueryRowContext(ctx,"SELECT count(*) FROM stale WHERE record=?",record).Scan(&stale);err!=nil{return v,err};if stale>0{return v,nil}
	knowledge:=heads[0];v.Revision=knowledge.ID;var k records.Knowledge;if json.Unmarshal(knowledge.Body,&k)!=nil{return v,memory.ErrInvalid}
	projections,err:=recordHeads(ctx,q,target,records.KindProjection);if err!=nil{return v,err};c,err:=SnapshotControl(ctx,q,target);if err!=nil{return v,err}
	found:=false
	for _,r:=range projections {
		if r.Scope!=scope{continue};var p records.Projection;if json.Unmarshal(r.Body,&p)!=nil{return v,memory.ErrInvalid};if p.Role!="target"||p.LinkID!=k.LinkID{continue};found=true
		if !HasScope(c,r.Author,scope){v.Reason="grant_invalid";return v,nil};if p.State!="projected"{v.Reason=p.State;return v,nil}
		if p.Target.Record!=record||p.Target.Revision!=knowledge.ID||p.Target.Scope!=scope||p.Target.Vault!=knowledge.Vault||p.Source!=k.Source||p.TextSHA256!=k.TextSHA256||hashText(k.Text)!=k.TextSHA256{return v,nil}
		if p.Source.Vault==knowledge.Vault {
			if !HasScope(c,r.Author,p.Source.Scope){v.Reason="grant_invalid";return v,nil}
		} else if bridgeGrant(ctx,q,target,p.Grant,scope,r.Author,p.Source.Vault,knowledge.Vault)!=nil{v.Reason="grant_invalid";return v,nil}
		if p.Attestation==nil||p.Attestation.Bridge!=r.Author||p.Attestation.Coverage!="complete"||len(p.Attestation.VerifiedHeads)==0{return v,nil}
		if v.LastVerifiedAt==""||p.Attestation.VerifiedAt<v.LastVerifiedAt{v.LastVerifiedAt=p.Attestation.VerifiedAt;v.LastVerifiedHeads=p.Attestation.VerifiedHeads}
	}
	if !found{v.Reason="pending";return v,nil};v.Visible=true;v.Reason="";return v,nil
}
