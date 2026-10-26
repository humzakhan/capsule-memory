package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/capture/claude"
	"github.com/humzakhan/capsule-memory/internal/capture/codex"
	"github.com/humzakhan/capsule-memory/internal/capture/importv1"
 "github.com/humzakhan/capsule-memory/internal/capture/filetail"
	"github.com/humzakhan/capsule-memory/internal/capture/t3"
	"github.com/humzakhan/capsule-memory/internal/derive"
	"github.com/humzakhan/capsule-memory/internal/inference"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/projection"
	"github.com/humzakhan/capsule-memory/internal/session"
)
type Profile struct{Name string;Svc *memory.Service;Session *session.Module;Derive *derive.Runner;Bridge *projection.Bridge;wired bool}
type Host struct{Profiles map[string]*Profile;Registry *ops.Registry;Config DaemonConfig;mu sync.RWMutex;Blocked map[string]string;inf inference.Client;mode memory.LockMode}
func CaptureAdapters()map[string]capture.Adapter{a:=capture.Registry();a["claude"]=claude.Adapter{Reader:filetail.NewReader()};a["codex"]=codex.Adapter{Reader:filetail.NewReader()};a["import"]=importv1.Adapter{Reader:filetail.NewReader()};a["t3"]=t3.Adapter{};return a}
func OpenProfile(ctx context.Context,name,home string,mode memory.LockMode,inf inference.Client,cfg DaemonConfig)(*Profile,error){svc,e:=memory.OpenWith(ctx,home,mode);if e!=nil{return nil,e};if e=svc.RequireV2();e!=nil{svc.Close();return nil,e};sess:=session.Install(svc);sess.SetAdapters(CaptureAdapters());sess.Home=home;sess.FlushInterval=time.Duration(cfg.Schedule.CaptureFlushSeconds)*time.Second;sess.HighWatermark=cfg.Storage.HighWatermarkBytes;sess.LowWatermark=cfg.Storage.LowWatermarkBytes;sess.FreeSpaceFloor=cfg.Storage.FreeSpaceFloorBytes;derive.Install(svc);projection.Install(svc);info,e:=svc.Info(ctx);if e!=nil{svc.Close();return nil,e};idle,e:=time.ParseDuration(cfg.Derive.Idle);if e!=nil{svc.Close();return nil,memory.ErrInvalid};runner:=derive.NewRunner(svc,sess,inf,derive.Config{Device:info.Device,Idle:idle,WindowBytes:cfg.Derive.WindowBytes,PromptVersion:"digest/v1",PolicyVersion:"session/v1"});return &Profile{Name:name,Svc:svc,Session:sess,Derive:runner},nil}
func NewHost(ctx context.Context,home string,mode memory.LockMode)(*Host,error){cfg,e:=LoadDaemonConfig(home);if e!=nil{return nil,e};var inf inference.Client;if cfg.Inference.Enabled{inf,e=inference.NewHTTP(cfg.Inference);if e!=nil{return nil,e}};h:=&Host{Profiles:map[string]*Profile{},Blocked:map[string]string{},Registry:ops.NewRegistry(),Config:cfg,inf:inf,mode:mode};for _,ref:=range cfg.Profiles{p,e:=OpenProfile(ctx,ref.Name,ref.Home,mode,inf,cfg);if e!=nil{if errors.Is(e,memory.ErrProfileBusy){h.Blocked[ref.Name]="profile_busy";continue};if errors.Is(e,memory.ErrNeedsMigration){h.Blocked[ref.Name]="needs_migration";continue};h.Close();return nil,e};h.Profiles[p.Name]=p};h.wire();h.register();return h,nil}
func SingleHost(ctx context.Context,name,home string)(*Host,error){cfg:=Defaults(DaemonConfig{});p,e:=OpenProfile(ctx,name,home,memory.LockShared,nil,cfg);if e!=nil{return nil,e};h:=&Host{Profiles:map[string]*Profile{name:p},Registry:ops.NewRegistry(),Config:cfg,Blocked:map[string]string{}};h.wire();h.register();return h,nil}
func(h *Host) wire(){targets:=map[string]*memory.Service{};for n,p:=range h.Profiles{targets[n]=p.Svc};for _,p:=range h.Profiles{if p.Bridge==nil{p.Bridge=projection.NewBridge(p.Svc,p.Session,targets)}};for n,p:=range h.Profiles{if p.wired{continue};p.wired=true;name:=n;target:=p.Svc;target.SetPublicationCheck(h.checkPublicationSource);p.Derive.SetKnowledgeResolver(func(ctx context.Context,ids []string,mode string)(map[string]derive.KnowledgeVisibility,error){m:=projection.Historical;if mode=="strict"{m=projection.Strict}else if mode!="historical"&&mode!=""{return nil,memory.ErrInvalid};v,e:=projection.Resolve(ctx,target,ids,m,h.Freshness(name));if e!=nil{return nil,e};out:=map[string]derive.KnowledgeVisibility{};for id,s:=range v{out[id]=derive.KnowledgeVisibility{Guard:s.Guard,Revision:s.Revision,Visible:s.Visible,Current:s.Current,LastVerifiedAt:s.LastVerifiedAt,LastVerifiedHeads:s.LastVerifiedHeads,Reason:s.Reason}};return out,nil})}}
func(h *Host) Freshness(profile string)projection.Freshness{h.mu.RLock();defer h.mu.RUnlock();target:=h.Profiles[profile];if target==nil{return nil};targets:=map[string]*memory.Service{};var sources []*Profile;for n,p:=range h.Profiles{targets[n]=p.Svc;sources=append(sources,p)};return routedFresh{target:target.Svc,sources:sources,targets:targets}}
func(h *Host) Profile(name string)(*Profile,error){h.mu.RLock();defer h.mu.RUnlock();p:=h.Profiles[name];if p==nil{if h.Blocked[name]=="needs_migration"{return nil,memory.ErrNeedsMigration};if h.Blocked[name]!=""{return nil,memory.ErrProfileBusy};return nil,memory.ErrInvalid};return p,nil}
func(h *Host) Snapshot()[]*Profile{h.mu.RLock();defer h.mu.RUnlock();out:=make([]*Profile,0,len(h.Profiles));for _,p:=range h.Profiles{out=append(out,p)};return out}
func(h *Host) RetryBusy(ctx context.Context)error{h.mu.Lock();defer h.mu.Unlock();changed:=false;for _,ref:=range h.Config.Profiles{if h.Blocked[ref.Name]!="profile_busy"&&h.Blocked[ref.Name]!="needs_migration"{continue};p,e:=OpenProfile(ctx,ref.Name,ref.Home,h.mode,h.inf,h.Config);if e!=nil{continue};h.Profiles[ref.Name]=p;delete(h.Blocked,ref.Name);changed=true};if changed{h.wire()};return nil}
func(h *Host) Close()error{var first error;for _,p:=range h.Snapshot(){if e:=p.Svc.Close();e!=nil&&first==nil{first=e}};return first}
func(h *Host) Status(ctx context.Context)(map[string]any,error){out:=map[string]any{"cache_eviction":"not_implemented","websocket":"not_used","runtime_acceptance":"NOT RUN"};profiles:=map[string]any{};queues:=map[string]map[string]int{};for _,p:=range h.Snapshot(){s,e:=p.Session.Status(ctx);if e!=nil{return nil,e};d,e:=p.Derive.Status(ctx);if e!=nil{return nil,e};legacy,e:=p.Svc.Status(ctx);if e!=nil{return nil,e};for kind,states:=range s.Queues{if queues[kind]==nil{queues[kind]=map[string]int{}};for state,n:=range states{queues[kind][state]+=n}};profiles[p.Name]=map[string]any{"capture":s,"derive":d,"sync":legacy}};h.mu.RLock();for name,reason:=range h.Blocked{profiles[name]=map[string]any{"blocked":reason}};h.mu.RUnlock();out["profiles"]=profiles;out["queues"]=queues;return out,nil}

// InstallModules equips a legacy direct sync/rebuild with every v2 applier without opening another profile.
func InstallModules(svc *memory.Service){session.Install(svc);derive.Install(svc);projection.Install(svc)}

func(h *Host) BridgeFor(name string)*projection.Bridge{h.mu.RLock();defer h.mu.RUnlock();p:=h.Profiles[name];if p==nil{return nil};targets:=map[string]*memory.Service{};for n,target:=range h.Profiles{targets[n]=target.Svc};return projection.NewBridge(p.Svc,p.Session,targets)}

func(h *Host) checkPublicationSource(ctx context.Context,ref records.ScopedRef)error{
 matched:=false;var unavailable error
 for _,p:=range h.Snapshot(){info,e:=p.Svc.Info(ctx);if e!=nil{return e};if info.Vault!=ref.Vault{continue};hasScope:=false;for _,scope:=range info.Scopes{hasScope=hasScope||scope.ID==ref.Scope};if !hasScope{continue};matched=true
  e=p.Svc.Read(ctx,func(q memory.Queryer)error{r,e:=p.Svc.Revision(ctx,q,ref.Revision);if e!=nil{return e};if r.Vault!=ref.Vault||r.Scope!=ref.Scope||r.Record!=ref.Record||r.Kind!=records.KindInsight{return memory.ErrInvalid};if e=p.Svc.CheckLocalScope(ctx,q,ref.Scope);e!=nil{return e};return memory.CheckConsent(ctx,q,[]records.SourceRef{{Record:r.Record,Revision:r.ID,Kind:r.Kind}})})
  if errors.Is(e,memory.ErrConsentWithdrawn){return e};if e!=nil{unavailable=e}
 };if !matched{return memory.ErrConsentUnavailable};return unavailable
}
