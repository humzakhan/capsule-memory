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

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/daemon"
	"github.com/humzakhan/capsule-memory/internal/memory"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string) error {
	base, err := os.UserConfigDir()
	if err != nil {
		return errors.New("cannot locate default state directory; use --home")
	}
	home := filepath.Join(base, "capsule-memory")
	global := flag.NewFlagSet("capsule-memory", flag.ContinueOnError)
	global.SetOutput(io.Discard)
	global.StringVar(&home, "home", home, "private device state directory")
	daemonHome := filepath.Join(base, "capsule-memory-daemon")
	global.StringVar(&daemonHome, "daemon-home", daemonHome, "private optional daemon directory")
	if err = global.Parse(args); err != nil {
		return errors.New("invalid global flags")
	}
	args = global.Args()
	if len(args) == 0 {
		return errors.New("command required: devices, device, owner protect, init, store, recall, get, delete, restore, sync, status, stats, rebuild, skill, mcp")
	}
	command := args[0]
	args = args[1:]
	sub := ""
	if handled, err := runEnrollment(ctx, home, command, args); handled {
		return err
	}
	if handled, err := runOptional(ctx, home, daemonHome, command, args); handled {
		return err
	}
	if command == "device" || command == "skill" {
		if len(args) == 0 {
			return errors.New("subcommand required")
		}
		sub = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&home, "home", home, "private device state directory")
	var scope, kind, text, id, parents, query, relay, out, revision string
	var history, loopback bool
	switch command {
	case "init":
		fs.StringVar(&relay, "relay", "", "relay base URL")
		fs.BoolVar(&loopback, "allow-loopback-http", false, "explicit local development only")
	case "device":
		if sub != "create" {
			return errors.New("unknown device subcommand")
		}
		fs.StringVar(&out, "out", "", "new device state directory")
	case "store":
		fs.StringVar(&scope, "scope", "", "fixed scope ID")
		fs.StringVar(&kind, "kind", "memory", "memory or skill")
		fs.StringVar(&text, "text", "", "record text")
		fs.StringVar(&id, "id", "", "record ID")
		fs.StringVar(&parents, "parents", "", "comma-separated parents")
	case "recall":
		fs.StringVar(&scope, "scope", "", "scope ID")
		fs.StringVar(&query, "query", "", "FTS5 query")
	case "get", "delete", "restore":
		fs.StringVar(&scope, "scope", "", "scope ID")
		fs.StringVar(&id, "id", "", "record ID")
		if command == "get" {
			fs.BoolVar(&history, "history", false, "include immutable history")
		}
		if command == "restore" {
			fs.StringVar(&text, "text", "", "explicit reviewed replacement for ambiguous restore")
		}
	case "sync", "mcp":
		fs.StringVar(&scope, "scope", "", "scope ID")
	case "skill":
		if sub != "approve" && sub != "activate" && sub != "list" && sub != "fetch" {
			return errors.New("unknown skill subcommand")
		}
		fs.StringVar(&scope, "scope", "", "scope ID")
		fs.StringVar(&id, "id", "", "record ID")
		fs.StringVar(&revision, "revision", "", "exact revision ID")
		if sub == "fetch" {
			fs.BoolVar(&history, "history", false, "include immutable history")
		}
	case "status", "stats", "rebuild":
	default:
		return errors.New("unknown command")
	}
	if fs.Parse(args) != nil || len(fs.Args()) != 0 {
		return errors.New("invalid command flags")
	}
	print := func(v any) error { return json.NewEncoder(os.Stdout).Encode(v) }
	if command == "init" {
		if relay == "" {
			return errors.New("--relay is required")
		}
		v, err := memory.Init(ctx, home, relay, loopback)
		if err != nil {
			return err
		}
		return print(v)
	}
	service, err := memory.Open(ctx, home)
	if errors.Is(err, memory.ErrProfileBusy) {
		if command == "mcp" {
			return errors.New("profile is owned by the capsule-memory daemon; configure \"mcp --client <name> --token-file <path>\"")
		}
		return routeLegacy(ctx, home, daemonHome, command, sub, fs, scope, kind, text, id, parents, query, out, revision, history)
	}
	if err != nil {
		return err
	}
	defer service.Close()
	if (command == "sync" || command == "rebuild") && service.RequireV2() == nil {
		app.InstallModules(service)
	}
	switch command {
	case "device":
		if out == "" {
			return errors.New("--out is required")
		}
		v, err := service.CreateDevice(ctx, out)
		if err != nil {
			return err
		}
		return print(v)
	case "store":
		var p []string
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "parents" {
				p = []string{}
				if parents != "" {
					p = strings.Split(parents, ",")
				}
			}
		})
		r, err := service.Write(ctx, memory.WriteInput{Scope: scope, Kind: kind, Text: text, Record: id, Parents: p})
		if err != nil {
			return err
		}
		return print(map[string]any{"record_id": r.Record, "revision_id": r.ID, "operation_id": r.OperationID, "parents": r.Parents, "local_durable": true, "published": false})
	case "recall":
		v, err := service.Recall(ctx, scope, query)
		if err != nil {
			return err
		}
		return print(v)
	case "get":
		if id == "" {
			return errors.New("--id is required")
		}
		v, err := service.Get(ctx, scope, id, history)
		if err != nil {
			return err
		}
		return print(v)
	case "delete", "restore":
		if id == "" {
			return errors.New("--id is required")
		}
		textSet := false
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "text" {
				textSet = true
			}
		})
		r, err := service.Write(ctx, memory.WriteInput{Scope: scope, Record: id, Operation: command, Text: text, TextSet: textSet})
		if err != nil {
			return err
		}
		return print(map[string]any{"record_id": r.Record, "revision_id": r.ID, "local_durable": true, "published": false})
	case "sync":
		v, err := service.Sync(ctx, scope)
		if err != nil {
			return err
		}
		return print(v)
	case "status":
		v, err := service.Status(ctx)
		if err != nil {
			return err
		}
		return print(v)
	case "stats":
		v, err := service.Stats(ctx)
		if err != nil {
			return err
		}
		return print(v)
	case "rebuild":
		if err := service.Rebuild(ctx); err != nil {
			return err
		}
		return print(map[string]any{"rebuilt": true, "pending_writes_preserved": true})
	case "mcp":
		return service.MCP(ctx, scope)
	case "skill":
		switch sub {
		case "approve":
			v, err := service.ApproveSkill(ctx, scope, id, revision)
			if err != nil {
				return err
			}
			return print(v)
		case "activate":
			v, err := service.ActivateSkill(ctx, scope, id, revision)
			if err != nil {
				return err
			}
			return print(v)
		case "list":
			v, err := service.ListSkills(ctx, scope)
			if err != nil {
				return err
			}
			return print(memory.SkillListResult(v))
		case "fetch":
			v, err := service.FetchSkill(ctx, scope, id, history)
			if err != nil {
				return err
			}
			return print(v)
		}
	}
	return errors.New("unsupported command")
}

func daemonClient(ctx context.Context, home, daemonHome string) (*daemon.Client, error) {
	config, err := app.LoadDaemonConfig(daemonHome)
	if err != nil {
		return nil, memory.ErrProfileBusy
	}
	absolute, err := filepath.Abs(home)
	if err != nil {
		return nil, memory.ErrInvalid
	}
	name := ""
	for _, profile := range config.Profiles {
		if filepath.Clean(profile.Home) == filepath.Clean(absolute) {
			name = profile.Name
		}
	}
	if name == "" {
		return nil, memory.ErrProfileBusy
	}
	client, err := daemon.Dial(ctx, daemonHome, "admin", filepath.Join(daemonHome, "clients", "admin.token"))
	if err != nil {
		return nil, memory.ErrProfileBusy
	}
	client.Profile = name
	return client, nil
}

func routeLegacy(ctx context.Context, home, daemonHome, command, sub string, fs *flag.FlagSet, scope, kind, text, id, parents, query, out, revision string, history bool) error {
	client, err := daemonClient(ctx, home, daemonHome)
	if err != nil {
		return err
	}
	defer client.Close()
	params := app.Params{
		Scope: scope, Kind: kind, Text: text, ID: id, Query: query,
		Out: out, Revision: revision, History: history,
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "text" {
			params.TextSet = true
		}
		if f.Name == "parents" {
			params.Parents = []string{}
			if parents != "" {
				params.Parents = strings.Split(parents, ",")
			}
		}
	})
	method := legacyMethod(command, sub)
	if method == "" {
		return memory.ErrInvalid
	}
	var result json.RawMessage
	if err := client.Call(ctx, method, params, &result); err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	if command != "store" && command != "delete" && command != "restore" {
		return encoder.Encode(result)
	}
	var written memory.Revision
	if json.Unmarshal(result, &written) != nil {
		return memory.ErrInvalid
	}
	value := map[string]any{
		"record_id": written.Record, "revision_id": written.ID,
		"local_durable": true, "published": false,
	}
	if command == "store" {
		value["operation_id"] = written.OperationID
		value["parents"] = written.Parents
	}
	return encoder.Encode(value)
}

func legacyMethod(command, sub string) string {
	switch command {
	case "store", "delete", "restore", "get", "recall":
		return "memory." + command
	case "sync":
		return "admin.sync.now"
	case "status":
		return "admin.legacy.status"
	case "stats":
		return "admin.legacy.stats"
	case "rebuild":
		return "admin.rebuild"
	case "device":
		return "admin.device.create"
	case "skill":
		switch sub {
		case "list":
			return "memory.skill_list"
		case "fetch":
			return "memory.skill_fetch"
		case "approve":
			return "admin.skill.approve"
		case "activate":
			return "admin.skill.activate"
		}
	}
	return ""
}
