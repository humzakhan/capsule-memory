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

	"agent-memory/internal/memory"
)

func main() {
	ctx,cancel:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM);defer cancel()
	if err:=run(ctx,os.Args[1:]);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
}
func run(ctx context.Context,args []string) error {
	base,err:=os.UserConfigDir();if err!=nil{return errors.New("cannot locate default state directory; use --home")};home:=filepath.Join(base,"agent-memory")
	global:=flag.NewFlagSet("agent-memory",flag.ContinueOnError);global.SetOutput(io.Discard);global.StringVar(&home,"home",home,"private device state directory")
	if err=global.Parse(args);err!=nil{return errors.New("invalid global flags")};args=global.Args();if len(args)==0{return errors.New("command required: init, device create, store, recall, get, delete, restore, sync, status, rebuild, skill, mcp")}
	command:=args[0];args=args[1:];sub:=""
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
	case "skill":if sub!="approve"&&sub!="activate"&&sub!="list"&&sub!="fetch"{return errors.New("unknown skill subcommand")};fs.StringVar(&scope,"scope","","scope ID");fs.StringVar(&id,"id","","record ID");fs.StringVar(&revision,"revision","","exact revision ID")
	case "status","rebuild":
	default:return errors.New("unknown command")
	}
	if fs.Parse(args)!=nil||len(fs.Args())!=0{return errors.New("invalid command flags")}
	print:=func(v any)error{return json.NewEncoder(os.Stdout).Encode(v)}
	if command=="init" {if relay==""{return errors.New("--relay is required")};v,err:=memory.Init(ctx,home,relay,loopback);if err!=nil{return err};return print(v)}
	service,err:=memory.Open(ctx,home);if err!=nil{return err};defer service.Close()
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
		case "activate":return service.ActivateSkill(ctx,scope,id,revision)
		case "list":v,err:=service.ListSkills(ctx,scope);if err!=nil{return err};return print(map[string]any{"skills":v,"staged":true,"activation_supported":false})
		case "fetch":v,err:=service.Get(ctx,scope,id,false);if err!=nil{return err};if v.Deleted||len(v.Heads)==0||v.Heads[0].Kind!="skill"{return memory.ErrInvalid};return print(map[string]any{"skill":v,"staged":true,"active":false})
		}
	}
	return errors.New("unsupported command")
}
