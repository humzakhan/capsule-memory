package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/capture/claude"
	"github.com/humzakhan/capsule-memory/internal/capture/codex"
	"github.com/humzakhan/capsule-memory/internal/capture/filetail"
	"github.com/humzakhan/capsule-memory/internal/capture/importv1"
	"github.com/humzakhan/capsule-memory/internal/capture/t3"
	"github.com/humzakhan/capsule-memory/internal/derive"
	"github.com/humzakhan/capsule-memory/internal/inference"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/projection"
	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/session"
)

type Profile struct {
	Name    string
	Svc     *memory.Service
	Session *session.Module
	Derive  *derive.Runner
	Bridge  *projection.Bridge
	wired   bool
}

type Host struct {
	Profiles    map[string]*Profile
	Registry    *ops.Registry
	Config      DaemonConfig
	mu          sync.RWMutex
	Blocked     map[string]string
	inf         inference.Client
	mode        memory.LockMode
	syncRetries map[string]map[string]SyncRetryStatus
}

func CaptureAdapters() map[string]capture.Adapter {
	adapters := capture.Registry()
	adapters["claude"] = claude.Adapter{Reader: filetail.NewReader()}
	adapters["codex"] = codex.Adapter{Reader: filetail.NewReader()}
	adapters["import"] = importv1.Adapter{Reader: filetail.NewReader()}
	adapters["t3"] = t3.Adapter{}
	return adapters
}

func OpenProfile(ctx context.Context, name, home string, mode memory.LockMode, inf inference.Client, cfg DaemonConfig) (*Profile, error) {
	svc, err := memory.OpenWith(ctx, home, mode)
	if err != nil {
		return nil, err
	}
	if err := svc.RequireV2(); err != nil {
		return nil, errors.Join(err, svc.Close())
	}
	sess := session.Install(svc)
	sess.SetAdapters(CaptureAdapters())
	sess.Home = home
	sess.FlushInterval = time.Duration(cfg.Schedule.CaptureFlushSeconds) * time.Second
	sess.HighWatermark = cfg.Storage.HighWatermarkBytes
	sess.LowWatermark = cfg.Storage.LowWatermarkBytes
	sess.FreeSpaceFloor = cfg.Storage.FreeSpaceFloorBytes
	derive.Install(svc)
	projection.Install(svc)
	info, err := svc.Info(ctx)
	if err != nil {
		return nil, errors.Join(err, svc.Close())
	}
	idle, err := time.ParseDuration(cfg.Derive.Idle)
	if err != nil {
		return nil, errors.Join(memory.ErrInvalid, svc.Close())
	}
	runner := derive.NewRunner(svc, sess, inf, derive.Config{
		Device: info.Device, Idle: idle, WindowBytes: cfg.Derive.WindowBytes,
		PromptVersion: derive.PromptVersion, PolicyVersion: "session/v1",
	})
	return &Profile{Name: name, Svc: svc, Session: sess, Derive: runner}, nil
}

func NewHost(ctx context.Context, home string, mode memory.LockMode) (*Host, error) {
	cfg, err := LoadDaemonConfig(home)
	if err != nil {
		return nil, err
	}
	var inf inference.Client
	if cfg.Inference.Enabled {
		inf, err = inference.NewHTTP(cfg.Inference)
		if err != nil {
			return nil, err
		}
	}
	host := &Host{
		Profiles: map[string]*Profile{}, Blocked: map[string]string{},
		Registry: ops.NewRegistry(), Config: cfg, inf: inf, mode: mode,
	}
	for _, ref := range cfg.Profiles {
		profile, err := OpenProfile(ctx, ref.Name, ref.Home, mode, inf, cfg)
		if errors.Is(err, memory.ErrProfileBusy) {
			host.Blocked[ref.Name] = "profile_busy"
			continue
		}
		if errors.Is(err, memory.ErrNeedsMigration) {
			host.Blocked[ref.Name] = "needs_migration"
			continue
		}
		if err != nil {
			return nil, errors.Join(err, host.Close())
		}
		host.Profiles[profile.Name] = profile
	}
	host.wire()
	host.register()
	return host, nil
}

func SingleHost(ctx context.Context, name, home string) (*Host, error) {
	cfg := Defaults(DaemonConfig{})
	profile, err := OpenProfile(ctx, name, home, memory.LockShared, nil, cfg)
	if err != nil {
		return nil, err
	}
	host := &Host{
		Profiles: map[string]*Profile{name: profile}, Registry: ops.NewRegistry(),
		Config: cfg, Blocked: map[string]string{},
	}
	host.wire()
	host.register()
	return host, nil
}

// wire runs before the host is shared or while its profile map is write-locked.
// Callback installation performs no network I/O or profile transaction.
func (h *Host) wire() {
	targets := map[string]*memory.Service{}
	for name, profile := range h.Profiles {
		targets[name] = profile.Svc
	}
	for _, profile := range h.Profiles {
		if profile.Bridge == nil {
			profile.Bridge = projection.NewBridge(profile.Svc, profile.Session, targets)
		}
	}
	for name, profile := range h.Profiles {
		if profile.wired {
			continue
		}
		profile.wired = true
		target := profile.Svc
		target.SetPublicationCheck(func(ctx context.Context, expected records.Projection) error {
			info, err := target.Info(ctx)
			if err != nil {
				return memory.ErrConsentUnavailable
			}
			if expected.Target.Vault != info.Vault {
				return memory.ErrInvalid
			}
			return h.checkPublicationSource(ctx, expected)
		})
		profile.Derive.SetKnowledgeResolver(h.knowledgeResolver(name, target))
	}
}

func (h *Host) knowledgeResolver(name string, target *memory.Service) derive.KnowledgeResolver {
	return func(ctx context.Context, ids []string, mode string) (map[string]derive.KnowledgeVisibility, error) {
		resolvedMode := projection.Historical
		if mode == "strict" {
			resolvedMode = projection.Strict
		} else if mode != "historical" && mode != "" {
			return nil, memory.ErrInvalid
		}
		visibility, err := projection.Resolve(ctx, target, ids, resolvedMode, h.Freshness(name))
		if err != nil {
			return nil, err
		}
		out := map[string]derive.KnowledgeVisibility{}
		for id, status := range visibility {
			out[id] = derive.KnowledgeVisibility{
				Guard: status.Guard, Revision: status.Revision,
				Visible: status.Visible, Current: status.Current,
				LastVerifiedAt: status.LastVerifiedAt, LastVerifiedHeads: status.LastVerifiedHeads,
				Reason: status.Reason,
			}
		}
		return out, nil
	}
}

func (h *Host) Freshness(profile string) projection.Freshness {
	h.mu.RLock()
	defer h.mu.RUnlock()
	target := h.Profiles[profile]
	if target == nil {
		return nil
	}
	targets := map[string]*memory.Service{}
	var sources []*Profile
	for name, source := range h.Profiles {
		targets[name] = source.Svc
		sources = append(sources, source)
	}
	// A blocked profile's vault/scope cannot be established safely. Do not
	// authorize a different local copy while that configured copy is unknown.
	return routedFresh{
		target: target.Svc, sources: sources, targets: targets,
		unavailable: len(h.Blocked) != 0,
	}
}

func (h *Host) Profile(name string) (*Profile, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if profile := h.Profiles[name]; profile != nil {
		return profile, nil
	}
	if h.Blocked[name] == "needs_migration" {
		return nil, memory.ErrNeedsMigration
	}
	if h.Blocked[name] != "" {
		return nil, memory.ErrProfileBusy
	}
	return nil, memory.ErrInvalid
}

func (h *Host) Snapshot() []*Profile {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*Profile, 0, len(h.Profiles))
	for _, profile := range h.Profiles {
		out = append(out, profile)
	}
	return out
}

func (h *Host) RetryBusy(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	changed := false
	for _, ref := range h.Config.Profiles {
		if h.Blocked[ref.Name] != "profile_busy" && h.Blocked[ref.Name] != "needs_migration" {
			continue
		}
		profile, err := OpenProfile(ctx, ref.Name, ref.Home, h.mode, h.inf, h.Config)
		if err != nil {
			continue
		}
		h.Profiles[ref.Name] = profile
		delete(h.Blocked, ref.Name)
		changed = true
	}
	if changed {
		h.wire()
	}
	return nil
}

func (h *Host) Close() error {
	var result error
	for _, profile := range h.Snapshot() {
		result = errors.Join(result, profile.Svc.Close())
	}
	return result
}

func (h *Host) Status(ctx context.Context) (map[string]any, error) {
	out := map[string]any{
		"cache_eviction": "not_implemented", "websocket": "not_used", "runtime_acceptance": "NOT RUN",
	}
	profiles := map[string]any{}
	queues := map[string]map[string]int{}
	for _, profile := range h.Snapshot() {
		status, err := profile.Session.Status(ctx)
		if err != nil {
			return nil, err
		}
		derived, err := profile.Derive.Status(ctx)
		if err != nil {
			return nil, err
		}
		legacy, err := profile.Svc.Status(ctx)
		if err != nil {
			return nil, err
		}
		for kind, states := range status.Queues {
			if queues[kind] == nil {
				queues[kind] = map[string]int{}
			}
			for state, count := range states {
				queues[kind][state] += count
			}
		}
		profiles[profile.Name] = map[string]any{
			"capture": status, "derive": derived, "sync": legacy,
			"publication_retry": h.SyncRetries(profile.Name),
		}
	}
	h.mu.RLock()
	for name, reason := range h.Blocked {
		profiles[name] = map[string]any{"blocked": reason}
	}
	h.mu.RUnlock()
	out["profiles"] = profiles
	out["queues"] = queues
	return out, nil
}

// InstallModules equips direct sync/rebuild with v2 appliers without another profile.
func InstallModules(svc *memory.Service) {
	session.Install(svc)
	derive.Install(svc)
	projection.Install(svc)
}

func (h *Host) BridgeFor(name string) *projection.Bridge {
	h.mu.RLock()
	defer h.mu.RUnlock()
	profile := h.Profiles[name]
	if profile == nil {
		return nil
	}
	targets := map[string]*memory.Service{}
	for name, target := range h.Profiles {
		targets[name] = target.Svc
	}
	return projection.NewBridge(profile.Svc, profile.Session, targets)
}
