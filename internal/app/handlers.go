package app

import (

	"context"
	"encoding/json"
	"errors"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/projection"
	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/session"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)
type Params struct {
	Profile string `json:"profile,omitempty"`;Scope string `json:"scope,omitempty"`;Scopes []string `json:"scopes,omitempty"`;Query string `json:"query,omitempty"`;ID string `json:"id,omitempty"`;Kind string `json:"kind,omitempty"`;Text string `json:"text,omitempty"`;TextSet bool `json:"text_set,omitempty"`;Parents []string `json:"parents,omitempty"`;History bool `json:"history,omitempty"`;Revision string `json:"revision,omitempty"`
	Mode string `json:"mode,omitempty"`;Limit int `json:"limit,omitempty"`;Branch string `json:"branch,omitempty"`;From int64 `json:"from_seq,omitempty"`;MaxBytes int `json:"max_bytes,omitempty"`;Session string `json:"session,omitempty"`;Review string `json:"review,omitempty"`;EditedText *string `json:"edited_text,omitempty"`
	Snapshot string `json:"snapshot,omitempty"`;Source string `json:"source,omitempty"`;Stream string `json:"stream,omitempty"`;Harness string `json:"harness,omitempty"`;Config json.RawMessage `json:"config,omitempty"`;Resolution string `json:"resolution,omitempty"`;Withdraw *bool `json:"withdraw,omitempty"`;Backfill *session.Backfill `json:"backfill,omitempty"`
	Path string `json:"path,omitempty"`;Project string `json:"project,omitempty"`;Name string `json:"name,omitempty"`;Patch json.RawMessage `json:"patch,omitempty"`;AcknowledgeUnaccepted bool `json:"acknowledge_unaccepted,omitempty"`;KeepInsights []string `json:"keep_insights,omitempty"`
	Job string `json:"job,omitempty"`;Device string `json:"device,omitempty"`;Out string `json:"out,omitempty"`;Role string `json:"role,omitempty"`;Label string `json:"label,omitempty"`;Insight string `json:"insight,omitempty"`;TargetProfile string `json:"target_profile,omitempty"`;TargetScope string `json:"target_scope,omitempty"`;Link string `json:"link,omitempty"`;BridgeGrant *records.BridgeGrantBody `json:"bridge_grant,omitempty"`;Aggregation *records.AggregationGrantBody `json:"aggregation,omitempty"`
}
func(h *Host) register(){
	register:=func(name string,cap ops.Capability){h.Registry.Add(ops.Method{Name:name,Cap:cap,Handler:func(ctx context.Context,g ops.Grant,raw json.RawMessage)(any,error){var p Params;if len(raw)==0{raw=[]byte("{}")};if strictjson.Decode(raw,1<<20,&p)!=nil{return nil,memory.ErrInvalid};if p.Mode!=""&&p.Mode!="strict"&&p.Mode!="historical"{return nil,memory.ErrInvalid};if p.Profile!=""&&g.Profile!=""&&p.Profile!=g.Profile{return nil,ops.ErrForbidden};nameProfile:=g.Profile;if nameProfile==""{if !g.Has(ops.CapAdmin){return nil,ops.ErrForbidden};nameProfile=p.Profile};if name=="status.get"&&nameProfile==""{s,e:=h.Status(ctx);if e!=nil{return nil,e};s["grant"]=g;return s,nil};profile,e:=h.Profile(nameProfile);if e!=nil{return nil,e};if g.Has(ops.CapAdmin){g.Caps=append(g.Caps,ops.CapStatus,ops.CapMemoryRead,ops.CapMemoryWrite,ops.CapKnowledge,ops.CapSessionRead,ops.CapCandidateReview,ops.CapHint);info,e:=profile.Svc.Info(ctx);if e!=nil{return nil,e};g.Profile=nameProfile;g.Scopes=nil;for _,s:=range info.Scopes{g.Scopes=append(g.Scopes,s.ID)}};if p.Scope!=""&&!g.AllowsScope(p.Scope){return nil,ops.ErrForbidden};return h.dispatch(ctx,profile,g,name,p)}})}
	for _,v:=range []struct{cap ops.Capability;names []string}{
		{ops.CapStatus,[]string{"status.get"}},
		{ops.CapMemoryRead,[]string{"memory.recall","memory.get","memory.skill_list","memory.skill_fetch"}},
		{ops.CapMemoryWrite,[]string{"memory.store","memory.delete","memory.restore"}},
		{ops.CapKnowledge,[]string{"knowledge.search","candidate.list"}},
		{ops.CapCandidateReview,[]string{"candidate.accept","candidate.reject"}},
		{ops.CapSessionRead,[]string{"session.search","session.read"}},
		{ops.CapHint,[]string{"capture.hint"}},
		{ops.CapAdmin,[]string{"admin.source.add","admin.source.configure","admin.source.list","admin.source.enable","admin.source.disable","admin.source.pause","admin.source.resume","admin.source.backfill","admin.project.map","admin.project.unmap","admin.project.exclude","admin.project.include","admin.project.suggest","admin.project.list","admin.policy.get","admin.policy.set","admin.session.delete","admin.session.restore","admin.session.export","admin.session.import","admin.derive.assign","admin.derive.retry","admin.derive.run","admin.promote.request","admin.promote.list","admin.promote.withdraw","admin.bridge.grant","admin.aggregation.grant","admin.sync.now","admin.scope.add","admin.device.create","admin.device.grant","admin.device.revoke","admin.rebuild","admin.skill.approve","admin.skill.activate","admin.legacy.status","admin.profile.info"}},
	}{for _,name:=range v.names{register(name,v.cap)}}
}
func(h *Host) dispatch(ctx context.Context,p *Profile,g ops.Grant,name string,in Params)(any,error){
	svc:=p.Svc;sess:=p.Session
	switch name{
	case "status.get":s,e:=sess.Status(ctx);if e!=nil{return nil,e};d,e:=p.Derive.Status(ctx);if e!=nil{return nil,e};return map[string]any{"capture":s,"derive":d,"grant":g,"runtime_acceptance":"NOT RUN"},nil
	case "memory.recall":return svc.Recall(ctx,in.Scope,in.Query)
	case "memory.get":return svc.Get(ctx,in.Scope,in.ID,in.History)
	case "memory.skill_list":v,e:=svc.ListSkills(ctx,in.Scope);return memory.SkillListResult(v),e
	case "memory.skill_fetch":return svc.FetchSkill(ctx,in.Scope,in.ID,in.History)
	case "memory.store","memory.delete","memory.restore":op:="";if name=="memory.delete"{op="delete"};if name=="memory.restore"{op="restore"};authorSource:="harness";if g.Has(ops.CapAdmin){authorSource=""};return svc.Write(ctx,memory.WriteInput{Scope:in.Scope,Kind:in.Kind,Text:in.Text,TextSet:in.TextSet,Record:in.ID,Parents:in.Parents,Operation:op,Source:authorSource})
	case "knowledge.search":if in.Mode==""{in.Mode="historical"};return p.Derive.SearchKnowledgeMode(ctx,g,in.Query,in.Limit,in.Mode)
	case "candidate.list":return p.Derive.ListCandidates(ctx,g,in.Scope,in.Review,in.Limit)
	case "candidate.accept":return p.Derive.Accept(ctx,g,in.ID,in.Revision,in.EditedText)
	case "candidate.reject":return p.Derive.Reject(ctx,g,in.ID,in.Revision)
	case "session.search":return sess.Search(ctx,g,in.Query,in.Limit)
	case "session.read":return sess.ReadSession(ctx,g,in.Session,in.Branch,in.From,in.MaxBytes)
	case "capture.hint":sess.Hint(in.Source,in.Stream);return map[string]any{"hinted":true},nil
	case "admin.source.add":s,preview,e:=sess.AddSource(ctx,in.Harness,in.Config);return map[string]any{"source":s,"preview":preview,"enabled":false},e
	case "admin.source.list":return sess.Sources(ctx)
	case "admin.source.configure":return sess.ConfigureSource(ctx,in.Source,in.Config)
	case "admin.source.enable","admin.source.backfill":return nil,sess.EnableSource(ctx,in.Source,in.Backfill)
	case "admin.source.disable":withdraw:=true;if in.Withdraw!=nil{withdraw=*in.Withdraw};return nil,sess.DisableSourceWithConsent(ctx,in.Source,withdraw)
	case "admin.source.pause":return nil,sess.Pause(ctx,in.Source)
	case "admin.source.resume":return nil,sess.Resume(ctx,in.Source,in.Resolution)
	case "admin.project.map":return sess.MapProject(ctx,in.Path,in.Project,in.Name)
	case "admin.project.exclude":return nil,sess.ExcludeProject(ctx,in.Path,true)
 case "admin.project.include":return nil,sess.ExcludeProject(ctx,in.Path,false)
 case "admin.project.unmap":return nil,sess.UnmapProject(ctx,in.Path)
	case "admin.project.suggest":return sess.SuggestProjects(ctx,in.Source)
	case "admin.project.list":return sess.ListProjects(ctx)
	case "admin.policy.get":return sess.Policy(ctx)
	case "admin.policy.set":return sess.SetPolicy(ctx,func(p *session.Policy)error{var patch map[string]json.RawMessage;if strictjson.Unique(in.Patch)!=nil||json.Unmarshal(in.Patch,&patch)!=nil{return memory.ErrInvalid};if raw,ok:=patch["cross_vault"];ok&&string(raw)==`"enabled_unaccepted"`&&!in.AcknowledgeUnaccepted{return errors.New("cross-vault requires --acknowledge-unaccepted")};b,e:=json.Marshal(p);if e!=nil{return e};var values map[string]json.RawMessage;if json.Unmarshal(b,&values)!=nil{return memory.ErrInvalid};for k,v:=range patch{if _,ok:=values[k];!ok||k=="version"{return memory.ErrInvalid};values[k]=v};b,e=json.Marshal(values);if e!=nil{return e};return strictjson.Decode(b,1<<20,p)})
	case "admin.session.delete":return sess.DeleteSession(ctx,in.Session,in.KeepInsights)
	case "admin.session.restore":return sess.RestoreSession(ctx,in.Session)
	case "admin.session.export":return sess.ExportSnapshotPage(ctx,in.Session,in.From,in.MaxBytes,in.Snapshot)
	case "admin.session.import":s,e:=sess.Source(ctx,in.Source);if e!=nil||s.Harness!="import"{return nil,memory.ErrInvalid};return sess.ImportStream(ctx,s,in.Path)
	case "admin.derive.retry":return nil,svc.RetryJob(ctx,in.Job)
	case "admin.derive.assign":return nil,p.Derive.Assign(ctx,in.Job,in.Device)
	case "admin.derive.run":return p.Derive.RunOnce(ctx)
	case "admin.promote.request":return h.BridgeFor(p.Name).Request(ctx,in.Insight,in.TargetProfile,in.TargetScope)
	case "admin.promote.list":return h.BridgeFor(p.Name).List(ctx)
	case "admin.promote.withdraw":return h.BridgeFor(p.Name).Withdraw(ctx,in.Link)
	case "admin.bridge.grant":if in.BridgeGrant==nil{return nil,memory.ErrInvalid};info,e:=svc.Info(ctx);if e!=nil{return nil,e};in.BridgeGrant.TargetVault=info.Vault;if in.BridgeGrant.Bridge==""{in.BridgeGrant.Bridge=info.Device};return projection.GrantBridge(ctx,svc,*in.BridgeGrant)
	case "admin.aggregation.grant":if in.Aggregation==nil{return nil,memory.ErrInvalid};info,e:=svc.Info(ctx);if e!=nil{return nil,e};in.Aggregation.Vault=info.Vault;return projection.GrantAggregation(ctx,svc,*in.Aggregation)
	case "admin.sync.now":return svc.Sync(ctx,in.Scope)
	case "admin.scope.add":return svc.AddScope(ctx,in.Label,in.Role)
	case "admin.device.create":return svc.CreateDeviceScoped(ctx,in.Out,in.Scopes)
	case "admin.device.grant":return svc.GrantDevice(ctx,in.Device,in.Scopes,in.Out)
	case "admin.device.revoke":return svc.RevokeScopeGrant(ctx,in.Device,in.Scopes)
	case "admin.rebuild":if e:=svc.Rebuild(ctx);e!=nil{return nil,e};return map[string]any{"rebuilt":true,"pending_writes_preserved":true},nil
	case "admin.skill.approve":return svc.ApproveSkill(ctx,in.Scope,in.ID,in.Revision)
	case "admin.skill.activate":return svc.ActivateSkill(ctx,in.Scope,in.ID,in.Revision)
	case "admin.legacy.status":return svc.Status(ctx)
	case "admin.profile.info":return svc.Info(ctx)
	};return nil,ops.ErrUnknownMethod
}
