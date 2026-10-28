package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/daemon"
	"github.com/humzakhan/capsule-memory/internal/mcpserver"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/session"
)

func flagPresent(args []string, name string) bool {
	for _, arg := range args {
		if arg == "--"+name || strings.HasPrefix(arg, "--"+name+"=") {
			return true
		}
	}
	return false
}

func csv(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

func optionalCommand(command string, args []string) bool {
	switch command {
	case "profile", "daemon", "source", "project", "policy", "session", "summary",
		"candidate", "derive", "promote", "bridge", "aggregation", "scope", "hint":
		return true
	case "device":
		return len(args) > 0 && (args[0] != "create" || flagPresent(args, "scopes"))
	case "mcp":
		return flagPresent(args, "client") || flagPresent(args, "token-file")
	case "init":
		return flagPresent(args, "role")
	default:
		return false
	}
}

type sessionCommand struct {
	command, sub              string
	home, daemonHome          string
	params                    app.Params
	config, patch             string
	scopes, caps, streams     string
	keep, profileList         string
	client, tokenFile, file   string
	since, role, relay        string
	sourceVault, bridgeDevice string
	targetScopes, ledgerScope string
	aggregationSources        string
	destination, format       string
	keyInput                  string
	maxBackfill               int64
	loopback                  bool
}

func runOptional(ctx context.Context, home, daemonHome, command string, args []string) (bool, error) {
	if !optionalCommand(command, args) {
		return false, nil
	}
	parsed, err := parseSessionCommand(home, daemonHome, command, args)
	if err != nil {
		return true, err
	}
	return true, parsed.run(ctx)
}

func parseSessionCommand(home, daemonHome, command string, args []string) (sessionCommand, error) {
	c := sessionCommand{command: command, home: home, daemonHome: daemonHome}
	if command != "hint" && command != "mcp" && command != "init" {
		if len(args) == 0 {
			return c, errors.New("subcommand required")
		}
		c.sub = args[0]
		args = args[1:]
	}
	if command == "daemon" && c.sub == "client" {
		if len(args) == 0 || args[0] != "add" {
			return c, memory.ErrInvalid
		}
		c.sub = "client.add"
		args = args[1:]
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	c.flags(fs)
	if fs.Parse(args) != nil || len(fs.Args()) != 0 {
		return c, errors.New("invalid command flags")
	}
	if command == "device" && c.sub == "create" && flagPresent(args, "scopes") && c.scopes == "" {
		return c, memory.ErrInvalid
	}
	c.params.Config = json.RawMessage(c.config)
	c.params.Patch = json.RawMessage(c.patch)
	c.params.Scopes = csv(c.scopes)
	c.params.KeepInsights = csv(c.keep)
	c.params.Role = c.role
	if c.params.Source == "" {
		c.params.Source = c.params.ID
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "text" {
			// Keep edited text independent of the parser's local struct address.
			text := c.params.Text
			c.params.EditedText = &text
			c.params.TextSet = true
		}
	})
	return c, nil
}

func (c *sessionCommand) flags(fs *flag.FlagSet) {
	p := &c.params
	fs.StringVar(&c.home, "home", c.home, "")
	fs.StringVar(&c.daemonHome, "daemon-home", c.daemonHome, "")
	fs.StringVar(&p.Scope, "scope", "", "")
	fs.StringVar(&p.ID, "id", "", "")
	fs.StringVar(&p.Query, "query", "", "")
	fs.StringVar(&p.Text, "text", "", "")
	fs.StringVar(&p.Kind, "kind", "memory", "")
	fs.StringVar(&p.Mode, "mode", "historical", "")
	fs.IntVar(&p.Limit, "limit", 20, "")
	fs.StringVar(&p.Source, "source", "", "")
	fs.StringVar(&p.Stream, "stream", "", "")
	fs.StringVar(&p.Session, "session", "", "")
	fs.StringVar(&p.Branch, "branch", "main", "")
	fs.Int64Var(&p.From, "from-seq", 0, "")
	fs.IntVar(&p.MaxBytes, "max-bytes", 4<<20, "")
	fs.StringVar(&p.Harness, "harness", "", "")
	fs.StringVar(&c.config, "config", "", "")
	fs.StringVar(&c.patch, "json", "", "")
	fs.StringVar(&p.Resolution, "resolution", "", "")
	fs.StringVar(&p.Path, "path", "", "")
	fs.StringVar(&p.Project, "project", "", "")
	fs.StringVar(&p.Name, "name", "", "")
	fs.StringVar(&p.Label, "label", "", "")
	fs.StringVar(&c.role, "role", "memory", "")
	fs.StringVar(&p.Device, "device", "", "")
	fs.StringVar(&p.Out, "out", "", "")
	fs.StringVar(&c.scopes, "scopes", "", "")
	fs.StringVar(&c.caps, "caps", "", "")
	fs.StringVar(&p.Profile, "profile", "", "")
	fs.StringVar(&c.profileList, "profiles", "", "")
	fs.StringVar(&c.client, "client", "", "")
	fs.StringVar(&c.tokenFile, "token-file", "", "")
	fs.StringVar(&p.Review, "review", "candidate", "")
	fs.StringVar(&p.Job, "job", "", "")
	fs.StringVar(&p.Insight, "insight", "", "")
	fs.StringVar(&p.TargetProfile, "target-profile", "", "")
	fs.StringVar(&p.TargetScope, "target-scope", "", "")
	fs.StringVar(&p.Link, "link", "", "")
	fs.StringVar(&c.file, "file", "", "")
	fs.StringVar(&c.since, "since", "", "")
	fs.StringVar(&c.since, "backfill-since", "", "")
	fs.Int64Var(&c.maxBackfill, "backfill-max-bytes", 0, "")
	fs.StringVar(&c.streams, "streams", "", "")
	fs.StringVar(&c.keep, "keep-insight", "", "")
	fs.BoolVar(&p.AcknowledgeUnaccepted, "acknowledge-unaccepted", false, "")
	fs.StringVar(&c.relay, "relay", "", "")
	fs.BoolVar(&c.loopback, "allow-loopback-http", false, "")
	fs.StringVar(&c.sourceVault, "source-vault", "", "")
	fs.StringVar(&p.Revision, "revision", "", "")
	fs.StringVar(&c.bridgeDevice, "bridge-device", "", "")
	fs.StringVar(&c.targetScopes, "target-scopes", "", "")
	fs.StringVar(&c.ledgerScope, "ledger-scope", "", "")
	fs.StringVar(&c.aggregationSources, "sources", "", "")
	fs.StringVar(&c.destination, "destination", "", "")
	fs.StringVar(&c.format, "format", "capsule-transcript-v1", "")
	fs.StringVar(&c.keyInput, "in", "", "")
}

func printCommand(value any, err error) error {
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(value)
}

func (c sessionCommand) run(ctx context.Context) error {
	switch c.command {
	case "mcp":
		return mcpserver.Run(ctx, c.daemonHome, c.client, c.tokenFile)
	case "hint":
		return c.hint(ctx)
	case "profile":
		if c.sub != "migrate" {
			return memory.ErrInvalid
		}
		value, err := memory.Migrate(ctx, c.home)
		return printCommand(value, err)
	case "init":
		return c.initProfile(ctx)
	case "daemon":
		return c.daemonCommand(ctx)
	case "device":
		if c.sub == "import-keys" {
			value, err := memory.ImportKeys(ctx, c.home, c.keyInput)
			return printCommand(value, err)
		}
	}
	if err := c.prepareOperation(); err != nil {
		return err
	}
	method := c.method()
	if method == "" {
		return memory.ErrInvalid
	}
	return executeSessionCommand(ctx, c.home, c.daemonHome, method, c.params)
}

func (c *sessionCommand) prepareOperation() error {
	if c.maxBackfill > 0 || c.since != "" || c.streams != "" {
		backfill := &session.Backfill{MaxBytes: c.maxBackfill, Streams: csv(c.streams)}
		if c.since != "" {
			parsed, err := time.Parse(time.RFC3339, c.since)
			if err != nil {
				return memory.ErrInvalid
			}
			backfill.Since = parsed
		}
		c.params.Backfill = backfill
	}
	if c.command == "session" {
		if c.params.Session == "" {
			c.params.Session = c.params.ID
		}
		if c.sub == "import" {
			c.params.Path = c.file
		}
		if c.sub == "export" && c.format != "capsule-transcript-v1" {
			return memory.ErrInvalid
		}
	}
	return nil
}

func (c *sessionCommand) method() string {
	switch c.command {
	case "scope":
		if c.sub == "add" {
			return "admin.scope.add"
		}
	case "source", "project", "policy":
		return "admin." + c.command + "." + c.sub
	case "session":
		if c.sub == "search" || c.sub == "read" {
			return "session." + c.sub
		}
		return "admin.session." + c.sub
	case "summary":
		if c.sub == "search" {
			return "knowledge.search"
		}
	case "candidate":
		return "candidate." + c.sub
	case "derive", "promote":
		return "admin." + c.command + "." + c.sub
	case "device":
		if c.sub == "revoke-scope" {
			return "admin.device.revoke"
		}
		return "admin.device." + c.sub
	case "bridge":
		if c.sub == "grant" {
			c.params.BridgeGrant = &records.BridgeGrantBody{
				Version: 1, SourceVault: c.sourceVault, TargetScopes: csv(c.targetScopes),
				LedgerScope: c.ledgerScope, Bridge: c.bridgeDevice,
			}
			return "admin.bridge.grant"
		}
	case "aggregation":
		if c.sub == "grant" {
			c.params.Aggregation = &records.AggregationGrantBody{
				Version: 1, Name: c.params.Name, Sources: csv(c.aggregationSources), Destination: c.destination,
			}
			return "admin.aggregation.grant"
		}
	}
	return ""
}

func executeSessionCommand(ctx context.Context, home, daemonHome, method string, params app.Params) error {
	// A shared lock determines daemon ownership. A missing socket never grants
	// authority to open a fallback writer while another process owns the profile.
	host, err := app.SingleHost(ctx, "direct", home)
	if errors.Is(err, memory.ErrProfileBusy) {
		client, err := daemonClient(ctx, home, daemonHome)
		if err != nil {
			return err
		}
		defer client.Close()
		if method == "admin.session.export" {
			return exportIPC(ctx, client, params)
		}
		var result json.RawMessage
		err = client.Call(ctx, method, params, &result)
		return printCommand(result, err)
	}
	if err != nil {
		return err
	}
	defer host.Close()
	if method == "admin.session.export" {
		return exportTo(params.Out, func(w io.Writer) error {
			return host.Profiles["direct"].Session.Export(ctx, params.Session, w)
		})
	}
	// The same registry performs profile, capability and scope admission for
	// direct and IPC calls. Admin grant expansion lives in the application layer.
	grant := ops.Grant{Client: "admin", Profile: "direct", Caps: []ops.Capability{ops.CapAdmin}}
	// Registry admission checks the requested method capability before expanding
	// admin profile scopes, so direct CLI carries each explicit capability.
	grant.Caps = append(grant.Caps, ops.CapStatus, ops.CapMemoryRead, ops.CapMemoryWrite,
		ops.CapKnowledge, ops.CapSessionRead, ops.CapCandidateReview, ops.CapHint)
	params.Profile = "direct"
	encoded, err := json.Marshal(params)
	if err != nil {
		return err
	}
	value, err := host.Registry.Call(ctx, grant, method, encoded)
	return printCommand(value, err)
}

func (c sessionCommand) initProfile(ctx context.Context) error {
	if c.role != "memory" && c.role != "session" {
		return memory.ErrInvalid
	}
	value, err := memory.Init(ctx, c.home, c.relay, c.loopback)
	if err != nil {
		return err
	}
	if c.role == "session" {
		svc, err := memory.Open(ctx, c.home)
		if err != nil {
			return err
		}
		_, scopeErr := svc.AddScope(ctx, "Session lifecycle", "ledger")
		if err := errors.Join(scopeErr, svc.Close()); err != nil {
			return err
		}
	}
	return printCommand(value, nil)
}

func (c sessionCommand) daemonCommand(ctx context.Context) error {
	switch c.sub {
	case "run":
		return daemon.Run(ctx, c.daemonHome)
	case "init":
		var refs []app.ProfileRef
		for _, part := range csv(c.profileList) {
			name, path, ok := strings.Cut(part, "=")
			if !ok {
				return memory.ErrInvalid
			}
			refs = append(refs, app.ProfileRef{Name: name, Home: path})
		}
		value, err := daemon.Init(ctx, c.daemonHome, refs)
		return printCommand(value, err)
	case "client.add":
		grant := ops.Grant{Profile: c.params.Profile, Scopes: c.params.Scopes}
		for _, cap := range csv(c.caps) {
			grant.Caps = append(grant.Caps, ops.Capability(cap))
		}
		path, err := daemon.AddClient(ctx, c.daemonHome, c.params.Name, grant)
		return printCommand(map[string]any{"token_file": path}, err)
	default:
		return memory.ErrInvalid
	}
}

func (c sessionCommand) hint(ctx context.Context) error {
	bounded, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	if c.client == "" {
		c.client = "admin"
	}
	if c.tokenFile == "" {
		c.tokenFile = filepath.Join(c.daemonHome, "clients", c.client+".token")
	}
	client, err := daemon.Dial(bounded, c.daemonHome, c.client, c.tokenFile)
	if err == nil {
		defer client.Close()
		client.Profile = c.params.Profile
		if client.Profile == "" {
			config, configErr := app.LoadDaemonConfig(c.daemonHome)
			if configErr == nil {
				for _, ref := range config.Profiles {
					if filepath.Clean(ref.Home) == filepath.Clean(c.home) {
						client.Profile = ref.Name
					}
				}
			}
		}
		err = client.Call(bounded, "capture.hint", c.params, nil)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "daemon unavailable")
	}
	return nil
}
