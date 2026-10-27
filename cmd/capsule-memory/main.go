package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/daemon"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/mcpserver"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/session"
)

func main() {
	ctx,cancel:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM);defer cancel()
	if err:=run(ctx,os.Args[1:]);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
}
func run(ctx context.Context,args []string) error {
	base,err:=os.UserConfigDir();if err!=nil{return errors.New("cannot locate default state directory; use --home")};home:=filepath.Join(base,"capsule-memory")
	global:=flag.NewFlagSet("capsule-memory",flag.ContinueOnError);global.SetOutput(io.Discard);global.StringVar(&home,"home",home,"private device state directory")
	daemonHome:=filepath.Join(base,"capsule-memory-daemon");global.StringVar(&daemonHome,"daemon-home",daemonHome,"private optional daemon directory")
	if err=global.Parse(args);err!=nil{return errors.New("invalid global flags")};args=global.Args();if len(args)==0{return errors.New("command required: init, device create, store, recall, get, delete, restore, sync, status, rebuild, skill, mcp")}
	command:=args[0];args=args[1:];sub:=""
	if handled,err:=runOptional(ctx,home,daemonHome,command,args);handled{return err}
	if command=="device"||command=="skill" {if len(args)==0{return errors.New("subcommand required")};sub=args[0];args=args[1:]}
	fs:=flag.NewFlagSet(command,flag.ContinueOnError);fs.SetOutput(io.Discard);fs.StringVar(&home,"home",home,"private device state directory")
	var scope,kind,text,id,parents,query,relay,out,revision string;var history,loopback bool
	switch command {
	case "init":fs.StringVar(&relay,"relay","","relay base URL");fs.BoolVar(&loopback,"allow-loopback-http",false,"explicit local development only")
	case "device":if sub!="create"{return errors.New("unknown device subcommand")};fs.StringVar(&out,"out","","new device state directory")
	case "store":fs.StringVar(&scope,"scope","","fixed scope ID");fs.StringVar(&kind,"kind","memory","memory or skill");fs.StringVar(&text,"text","","record text");fs.StringVar(&id,"id","","record ID");fs.StringVar(&parents,"parents","","comma-separated parents")
	case "recall":fs.StringVar(&scope,"scope","","scope ID");fs.StringVar(&query,"query","","FTS5 query")
	case "get","delete","restore":fs.StringVar(&scope,"scope","","scope ID");fs.StringVar(&id,"id","","record ID");if command=="get"{fs.BoolVar(&history,"history",false,"include immutable history")};if command=="restore"{fs.StringVar(&text,"text","","explicit reviewed replacement for ambiguous restore")}
	case "sync","mcp":fs.StringVar(&scope,"scope","","scope ID")
	case "skill":if sub!="approve"&&sub!="activate"&&sub!="list"&&sub!="fetch"{return errors.New("unknown skill subcommand")};fs.StringVar(&scope,"scope","","scope ID");fs.StringVar(&id,"id","","record ID");fs.StringVar(&revision,"revision","","exact revision ID");if sub=="fetch"{fs.BoolVar(&history,"history",false,"include immutable history")}
	case "status","rebuild":
	default:return errors.New("unknown command")
	}
	if fs.Parse(args)!=nil||len(fs.Args())!=0{return errors.New("invalid command flags")}
	print:=func(v any)error{return json.NewEncoder(os.Stdout).Encode(v)}
	if command=="init" {if relay==""{return errors.New("--relay is required")};v,err:=memory.Init(ctx,home,relay,loopback);if err!=nil{return err};return print(v)}
	service,err:=memory.Open(ctx,home);if errors.Is(err,memory.ErrProfileBusy){if command=="mcp"{return errors.New("profile is owned by the capsule-memory daemon; configure \"mcp --client <name> --token-file <path>\"")};return routeLegacy(ctx,home,daemonHome,command,sub,fs,scope,kind,text,id,parents,query,out,revision,history)};if err!=nil{return err};defer service.Close()
	if (command=="sync"||command=="rebuild")&&service.RequireV2()==nil{app.InstallModules(service)}
	switch command {
	case "device":if out==""{return errors.New("--out is required")};v,err:=service.CreateDevice(ctx,out);if err!=nil{return err};return print(v)
	case "store":var p []string;fs.Visit(func(f *flag.Flag){if f.Name=="parents"{p=[]string{};if parents!=""{p=strings.Split(parents,",")}}});r,err:=service.Write(ctx,memory.WriteInput{Scope:scope,Kind:kind,Text:text,Record:id,Parents:p});if err!=nil{return err};return print(map[string]any{"record_id":r.Record,"revision_id":r.ID,"operation_id":r.OperationID,"parents":r.Parents,"local_durable":true,"published":false})
	case "recall":v,err:=service.Recall(ctx,scope,query);if err!=nil{return err};return print(v)
	case "get":if id==""{return errors.New("--id is required")};v,err:=service.Get(ctx,scope,id,history);if err!=nil{return err};return print(v)
	case "delete","restore":if id==""{return errors.New("--id is required")};textSet:=false;fs.Visit(func(f *flag.Flag){if f.Name=="text"{textSet=true}});r,err:=service.Write(ctx,memory.WriteInput{Scope:scope,Record:id,Operation:command,Text:text,TextSet:textSet});if err!=nil{return err};return print(map[string]any{"record_id":r.Record,"revision_id":r.ID,"local_durable":true,"published":false})
	case "sync":v,err:=service.Sync(ctx,scope);if err!=nil{return err};return print(v)
	case "status":v,err:=service.Status(ctx);if err!=nil{return err};return print(v)
	case "rebuild":if err:=service.Rebuild(ctx);err!=nil{return err};return print(map[string]any{"rebuilt":true,"pending_writes_preserved":true})
	case "mcp":return service.MCP(ctx,scope)
	case "skill":switch sub {
		case "approve":v,err:=service.ApproveSkill(ctx,scope,id,revision);if err!=nil{return err};return print(v)
		case "activate":v,err:=service.ActivateSkill(ctx,scope,id,revision);if err!=nil{return err};return print(v)
		case "list":v,err:=service.ListSkills(ctx,scope);if err!=nil{return err};return print(memory.SkillListResult(v))
		case "fetch":v,err:=service.FetchSkill(ctx,scope,id,history);if err!=nil{return err};return print(v)
		}
	}
	return errors.New("unsupported command")
}

func daemonClient(ctx context.Context,home,daemonHome string)(*daemon.Client,error){cfg,e:=app.LoadDaemonConfig(daemonHome);if e!=nil{return nil,memory.ErrProfileBusy};absolute,e:=filepath.Abs(home);if e!=nil{return nil,memory.ErrInvalid};name:="";for _,p:=range cfg.Profiles{if filepath.Clean(p.Home)==filepath.Clean(absolute){name=p.Name}};if name==""{return nil,memory.ErrProfileBusy};c,e:=daemon.Dial(ctx,daemonHome,"admin",filepath.Join(daemonHome,"clients","admin.token"));if e!=nil{return nil,memory.ErrProfileBusy};c.Profile=name;return c,nil}
func routeLegacy(ctx context.Context,home,daemonHome,command,sub string,fs *flag.FlagSet,scope,kind,text,id,parents,query,out,revision string,history bool)error{c,e:=daemonClient(ctx,home,daemonHome);if e!=nil{return e};defer c.Close();p:=app.Params{Scope:scope,Kind:kind,Text:text,ID:id,Query:query,Out:out,Revision:revision,History:history};fs.Visit(func(f *flag.Flag){if f.Name=="text"{p.TextSet=true};if f.Name=="parents"{p.Parents=[]string{};if parents!=""{p.Parents=strings.Split(parents,",")}}});method:="";switch command{case "store":method="memory.store";case "delete":method="memory.delete";case "restore":method="memory.restore";case "get":method="memory.get";case "recall":method="memory.recall";case "sync":method="admin.sync.now";case "status":method="admin.legacy.status";case "rebuild":method="admin.rebuild";case "device":method="admin.device.create";case "skill":switch sub{case "list":method="memory.skill_list";case "fetch":method="memory.skill_fetch";case "approve":method="admin.skill.approve";case "activate":method="admin.skill.activate"}};if method==""{return memory.ErrInvalid};var raw json.RawMessage;if e=c.Call(ctx,method,p,&raw);e!=nil{return e};enc:=json.NewEncoder(os.Stdout);if command=="store"||command=="delete"||command=="restore"{var r memory.Revision;if json.Unmarshal(raw,&r)!=nil{return memory.ErrInvalid};v:=map[string]any{"record_id":r.Record,"revision_id":r.ID,"local_durable":true,"published":false};if command=="store"{v["operation_id"]=r.OperationID;v["parents"]=r.Parents};return enc.Encode(v)};return enc.Encode(raw)}
func flagPresent(args []string,name string)bool{for _,a:=range args{if a=="--"+name||strings.HasPrefix(a,"--"+name+"="){return true}};return false}
func csv(s string)[]string{if s==""{return nil};return strings.Split(s,",")}
func runOptional(ctx context.Context,home,daemonHome,command string,args []string)(bool,error){
	newCommand:=false;switch command{case "profile","daemon","source","project","policy","session","summary","candidate","derive","promote","bridge","aggregation","scope","hint":newCommand=true;case "device":newCommand=len(args)>0&&(args[0]!="create"||flagPresent(args,"scopes"));case "mcp":newCommand=flagPresent(args,"client")||flagPresent(args,"token-file");case "init":newCommand=flagPresent(args,"role")};if !newCommand{return false,nil}
	sub:="";if command!="hint"&&command!="mcp"&&command!="init"{if len(args)==0{return true,errors.New("subcommand required")};sub=args[0];args=args[1:]};if command=="daemon"&&sub=="client"{if len(args)==0||args[0]!="add"{return true,memory.ErrInvalid};sub="client.add";args=args[1:]}
	fs:=flag.NewFlagSet(command,flag.ContinueOnError);fs.SetOutput(io.Discard);p:=app.Params{};var config,patch,scopes,caps,streams,keep,profileList,client,tokenFile,file,since,role,relay,sourceVault string;var maxBackfill int64;var loopback bool
	fs.StringVar(&home,"home",home,"");fs.StringVar(&daemonHome,"daemon-home",daemonHome,"");fs.StringVar(&p.Scope,"scope","","");fs.StringVar(&p.ID,"id","","");fs.StringVar(&p.Query,"query","","");fs.StringVar(&p.Text,"text","","");fs.StringVar(&p.Kind,"kind","memory","");fs.StringVar(&p.Mode,"mode","historical","");fs.IntVar(&p.Limit,"limit",20,"");fs.StringVar(&p.Source,"source","","");fs.StringVar(&p.Stream,"stream","","");fs.StringVar(&p.Session,"session","","");fs.StringVar(&p.Branch,"branch","main","");fs.Int64Var(&p.From,"from-seq",0,"");fs.IntVar(&p.MaxBytes,"max-bytes",4<<20,"");fs.StringVar(&p.Harness,"harness","","");fs.StringVar(&config,"config","","");fs.StringVar(&patch,"json","","");fs.StringVar(&p.Resolution,"resolution","","");fs.StringVar(&p.Path,"path","","");fs.StringVar(&p.Project,"project","","");fs.StringVar(&p.Name,"name","","");fs.StringVar(&p.Label,"label","","");fs.StringVar(&role,"role","memory","");fs.StringVar(&p.Device,"device","","");fs.StringVar(&p.Out,"out","","");fs.StringVar(&scopes,"scopes","","");fs.StringVar(&caps,"caps","","");fs.StringVar(&p.Profile,"profile","","");fs.StringVar(&profileList,"profiles","","");fs.StringVar(&client,"client","","");fs.StringVar(&tokenFile,"token-file","","");fs.StringVar(&p.Review,"review","candidate","");fs.StringVar(&p.Job,"job","","");fs.StringVar(&p.Insight,"insight","","");fs.StringVar(&p.TargetProfile,"target-profile","","");fs.StringVar(&p.TargetScope,"target-scope","","");fs.StringVar(&p.Link,"link","","");fs.StringVar(&file,"file","","");fs.StringVar(&since,"since","","");fs.StringVar(&since,"backfill-since","","");fs.Int64Var(&maxBackfill,"backfill-max-bytes",0,"");fs.StringVar(&streams,"streams","","");fs.StringVar(&keep,"keep-insight","","");fs.BoolVar(&p.AcknowledgeUnaccepted,"acknowledge-unaccepted",false,"");fs.StringVar(&relay,"relay","","");fs.BoolVar(&loopback,"allow-loopback-http",false,"");fs.StringVar(&sourceVault,"source-vault","","");fs.StringVar(&p.Revision,"revision","","")
	var bridgeDevice,targetScopes,ledgerScope,aggregationSources,destination,format,keyInput string;fs.StringVar(&bridgeDevice,"bridge-device","","");fs.StringVar(&targetScopes,"target-scopes","","");fs.StringVar(&ledgerScope,"ledger-scope","","");fs.StringVar(&aggregationSources,"sources","","");fs.StringVar(&destination,"destination","","");fs.StringVar(&format,"format","capsule-transcript-v1","");fs.StringVar(&keyInput,"in","","")
	if fs.Parse(args)!=nil||len(fs.Args())!=0{return true,errors.New("invalid command flags")};p.Config=json.RawMessage(config);if command=="device"&&sub=="create"&&flagPresent(args,"scopes")&&scopes==""{return true,memory.ErrInvalid};p.Patch=json.RawMessage(patch);p.Scopes=csv(scopes);if p.Source==""{p.Source=p.ID};p.KeepInsights=csv(keep);p.Role=role;fs.Visit(func(f *flag.Flag){if f.Name=="text"{p.EditedText=&p.Text;p.TextSet=true}})
	print:=func(v any,e error)error{if e!=nil{return e};return json.NewEncoder(os.Stdout).Encode(v)}
	if command=="mcp"{return true,mcpserver.Run(ctx,daemonHome,client,tokenFile)}
	if command=="hint"{bounded,cancel:=context.WithTimeout(ctx,500*time.Millisecond);defer cancel();if client==""{client="admin"};if tokenFile==""{tokenFile=filepath.Join(daemonHome,"clients",client+".token")};c,e:=daemon.Dial(bounded,daemonHome,client,tokenFile);if e==nil{defer c.Close();c.Profile=p.Profile;if c.Profile==""{cfg,er:=app.LoadDaemonConfig(daemonHome);if er==nil{for _,ref:=range cfg.Profiles{if filepath.Clean(ref.Home)==filepath.Clean(home){c.Profile=ref.Name}}}};e=c.Call(bounded,"capture.hint",p,nil)};if e!=nil{fmt.Fprintln(os.Stderr,"daemon unavailable")};return true,nil}
	if command=="profile"{if sub!="migrate"{return true,memory.ErrInvalid};v,e:=memory.Migrate(ctx,home);return true,print(v,e)}
	if command=="init"{if role!="memory"&&role!="session"{return true,memory.ErrInvalid};v,e:=memory.Init(ctx,home,relay,loopback);if e!=nil{return true,e};if role=="session"{svc,e:=memory.Open(ctx,home);if e!=nil{return true,e};_,e=svc.AddScope(ctx,"Session lifecycle","ledger");svc.Close();if e!=nil{return true,e}};return true,print(v,nil)}
	if command=="daemon"{switch sub{case "run":return true,daemon.Run(ctx,daemonHome);case "init":var refs []app.ProfileRef;for _,part:=range csv(profileList){name,path,ok:=strings.Cut(part,"=");if !ok{return true,memory.ErrInvalid};refs=append(refs,app.ProfileRef{Name:name,Home:path})};v,e:=daemon.Init(ctx,daemonHome,refs);return true,print(v,e);case "client.add":g:=ops.Grant{Profile:p.Profile,Scopes:p.Scopes};for _,cap:=range csv(caps){g.Caps=append(g.Caps,ops.Capability(cap))};path,e:=daemon.AddClient(ctx,daemonHome,p.Name,g);return true,print(map[string]any{"token_file":path},e)};return true,memory.ErrInvalid}
	if command=="device"&&sub=="import-keys"{v,e:=memory.ImportKeys(ctx,home,keyInput);return true,print(v,e)}
	if maxBackfill>0||since!=""||streams!=""{bf:=&session.Backfill{MaxBytes:maxBackfill,Streams:csv(streams)};if since!=""{t,e:=time.Parse(time.RFC3339,since);if e!=nil{return true,memory.ErrInvalid};bf.Since=t};p.Backfill=bf}
	if command=="session"{if p.Session==""{p.Session=p.ID};if sub=="import"{p.Path=file};if sub=="export"&&format!="capsule-transcript-v1"{return true,memory.ErrInvalid}}
	method:="";switch command{case "scope":if sub=="add"{method="admin.scope.add"};case "source","project","policy":method="admin."+command+"."+sub;case "session":if sub=="search"||sub=="read"{method="session."+sub}else{method="admin.session."+sub};case "summary":if sub=="search"{method="knowledge.search"};case "candidate":method="candidate."+sub;case "derive","promote":method="admin."+command+"."+sub;case "device":method="admin.device."+sub;if sub=="revoke-scope"{method="admin.device.revoke"};case "bridge":if sub=="grant"{method="admin.bridge.grant";p.BridgeGrant=&records.BridgeGrantBody{Version:1,SourceVault:sourceVault,TargetScopes:csv(targetScopes),LedgerScope:ledgerScope,Bridge:bridgeDevice}};case "aggregation":if sub=="grant"{method="admin.aggregation.grant";p.Aggregation=&records.AggregationGrantBody{Version:1,Name:p.Name,Sources:csv(aggregationSources),Destination:destination}}};if method==""{return true,memory.ErrInvalid}
	var result json.RawMessage
	// Taking a shared lock determines daemon ownership; no socket-existence guess authorizes a fallback writer.
	h,e:=app.SingleHost(ctx,"direct",home);if errors.Is(e,memory.ErrProfileBusy){c,e:=daemonClient(ctx,home,daemonHome);if e!=nil{return true,e};defer c.Close();if command=="session"&&sub=="export"{return true,exportIPC(ctx,c,p)};e=c.Call(ctx,method,p,&result);return true,print(result,e)};if e!=nil{return true,e};defer h.Close();if command=="session"&&sub=="export"{w,close,e:=exportWriter(p.Out);if e!=nil{return true,e};defer close();return true,h.Profiles["direct"].Session.Export(ctx,p.Session,w)};g:=ops.Grant{Client:"admin",Profile:"direct",Caps:[]ops.Capability{ops.CapAdmin,ops.CapStatus,ops.CapMemoryRead,ops.CapMemoryWrite,ops.CapKnowledge,ops.CapSessionRead,ops.CapCandidateReview,ops.CapHint}};p.Profile="direct";info,e:=h.Profiles["direct"].Svc.Info(ctx);if e!=nil{return true,e};if p.BridgeGrant!=nil{p.BridgeGrant.TargetVault=info.Vault;if p.BridgeGrant.Bridge==""{p.BridgeGrant.Bridge=info.Device}};if p.Aggregation!=nil{p.Aggregation.Vault=info.Vault};raw,e:=json.Marshal(p);if e!=nil{return true,e};v,e:=h.Registry.Call(ctx,g,method,raw);return true,print(v,e)
}

func exportWriter(out string)(io.Writer,func(),error){if out==""{return os.Stdout,func(){},nil};f,e:=os.OpenFile(out,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);if e!=nil{return nil,nil,e};return f,func(){_ = f.Sync();_ = f.Close()},nil}
func exportIPC(ctx context.Context,c *daemon.Client,p app.Params)error{w,close,e:=exportWriter(p.Out);if e!=nil{return e};defer close();p.From=0;p.MaxBytes=256<<10;for{var page session.ExportPage;if e=c.Call(ctx,"admin.session.export",p,&page);e!=nil{return e};if _,e=io.WriteString(w,page.JSONL);e!=nil{return e};if page.Done{return nil};if page.Next<=p.From{return memory.ErrInvalid};p.From=page.Next;p.Snapshot=page.Snapshot}}
