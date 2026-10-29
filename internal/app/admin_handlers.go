package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/projection"
	"github.com/humzakhan/capsule-memory/internal/session"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)

// Only admin methods registered with CapAdmin reach these operations.
func (h *Host) dispatchAdmin(ctx context.Context, profile *Profile, name string, in Params) (any, error) {
	svc := profile.Svc
	sess := profile.Session
	switch name {
	case "admin.source.add":
		source, preview, err := sess.AddSource(ctx, in.Harness, in.Config)
		return map[string]any{"source": source, "preview": preview, "enabled": false}, err
	case "admin.source.list":
		return sess.Sources(ctx)
	case "admin.source.configure":
		return sess.ConfigureSource(ctx, in.Source, in.Config)
	case "admin.source.enable", "admin.source.backfill":
		return nil, sess.EnableSource(ctx, in.Source, in.Backfill)
	case "admin.source.disable":
		withdraw := true
		if in.Withdraw != nil {
			withdraw = *in.Withdraw
		}
		return nil, sess.DisableSourceWithConsent(ctx, in.Source, withdraw)
	case "admin.source.pause":
		return nil, sess.Pause(ctx, in.Source)
	case "admin.source.resume":
		return nil, sess.Resume(ctx, in.Source, in.Resolution)
	case "admin.project.map":
		return sess.MapProject(ctx, in.Path, in.Project, in.Name)
	case "admin.project.exclude":
		return nil, sess.ExcludeProject(ctx, in.Path, true)
	case "admin.project.include":
		return nil, sess.ExcludeProject(ctx, in.Path, false)
	case "admin.project.unmap":
		return nil, sess.UnmapProject(ctx, in.Path)
	case "admin.project.suggest":
		return sess.SuggestProjects(ctx, in.Source)
	case "admin.project.list":
		return sess.ListProjects(ctx)
	case "admin.policy.get":
		return sess.Policy(ctx)
	case "admin.policy.set":
		return sess.SetPolicy(ctx, func(policy *session.Policy) error {
			return applyPolicyPatch(policy, in.Patch, in.AcknowledgeUnaccepted)
		})
	case "admin.session.delete":
		return sess.DeleteSession(ctx, in.Session, in.KeepInsights)
	case "admin.session.restore":
		return sess.RestoreSession(ctx, in.Session)
	case "admin.session.export":
		return sess.ExportSnapshotPage(ctx, in.Session, in.From, in.MaxBytes, in.Snapshot)
	case "admin.session.import":
		source, err := sess.Source(ctx, in.Source)
		if err != nil || source.Harness != "import" {
			return nil, memory.ErrInvalid
		}
		return sess.ImportStream(ctx, source, in.Path)
	case "admin.derive.retry":
		return nil, svc.RetryJob(ctx, in.Job)
	case "admin.derive.assign":
		return nil, profile.Derive.Assign(ctx, in.Job, in.Device)
	case "admin.derive.run":
		return profile.Derive.RunOnce(ctx)
	case "admin.promote.request":
		return h.BridgeFor(profile.Name).Request(ctx, in.Insight, in.TargetProfile, in.TargetScope)
	case "admin.promote.list":
		return h.BridgeFor(profile.Name).List(ctx)
	case "admin.promote.withdraw":
		return h.BridgeFor(profile.Name).Withdraw(ctx, in.Link)
	case "admin.bridge.grant":
		if in.BridgeGrant == nil {
			return nil, memory.ErrInvalid
		}
		info, err := svc.Info(ctx)
		if err != nil {
			return nil, err
		}
		in.BridgeGrant.TargetVault = info.Vault
		if in.BridgeGrant.Bridge == "" {
			in.BridgeGrant.Bridge = info.Device
		}
		return projection.GrantBridge(ctx, svc, *in.BridgeGrant)
	case "admin.aggregation.grant":
		if in.Aggregation == nil {
			return nil, memory.ErrInvalid
		}
		info, err := svc.Info(ctx)
		if err != nil {
			return nil, err
		}
		in.Aggregation.Vault = info.Vault
		return projection.GrantAggregation(ctx, svc, *in.Aggregation)
	case "admin.sync.now":
		return svc.Sync(ctx, in.Scope)
	case "admin.scope.add":
		return svc.AddScope(ctx, in.Label, in.Role)
	case "admin.device.create":
		return svc.CreateDeviceScoped(ctx, in.Out, in.Scopes)
	case "admin.device.grant":
		return svc.GrantDevice(ctx, in.Device, in.Scopes, in.Out)
	case "admin.device.revoke":
		return svc.RevokeScopeGrant(ctx, in.Device, in.Scopes)
	case "admin.rebuild":
		if err := svc.Rebuild(ctx); err != nil {
			return nil, err
		}
		return map[string]any{"rebuilt": true, "pending_writes_preserved": true}, nil
	case "admin.skill.approve":
		return svc.ApproveSkill(ctx, in.Scope, in.ID, in.Revision)
	case "admin.skill.activate":
		return svc.ActivateSkill(ctx, in.Scope, in.ID, in.Revision)
	case "admin.legacy.status":
		return svc.Status(ctx)
	case "admin.legacy.stats":
		stats, err := svc.Stats(ctx)
		if err != nil {
			return nil, err
		}
		capture, err := sess.Status(ctx)
		if err != nil {
			return nil, err
		}
		states := map[string]int{}
		for _, source := range capture.Sources {
			states[source.State]++
		}
		stats["capture_status"] = map[string]any{"source_states": states, "saved_chunks": capture.Saved, "pending_streams": capture.PendingStreams}
		return stats, nil
	case "admin.profile.info":
		return svc.Info(ctx)
	default:
		return nil, ops.ErrUnknownMethod
	}
}

func applyPolicyPatch(policy *session.Policy, raw json.RawMessage, acknowledgeUnaccepted bool) error {
	// Validate untrusted patch fields, rather than rejecting nil optional slices
	// marshaled from the existing policy as null during the internal round trip.
	var validated session.Policy
	if strictjson.Decode(raw, 1<<20, &validated) != nil {
		return memory.ErrInvalid
	}
	var patch map[string]json.RawMessage
	if json.Unmarshal(raw, &patch) != nil {
		return memory.ErrInvalid
	}
	if rawValue, ok := patch["cross_vault"]; ok {
		var value string
		if json.Unmarshal(rawValue, &value) != nil {
			return memory.ErrInvalid
		}
		if value == "enabled_unaccepted" && !acknowledgeUnaccepted {
			return errors.New("cross-vault requires --acknowledge-unaccepted")
		}
	}
	encoded, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(encoded, &values) != nil {
		return memory.ErrInvalid
	}
	for key, value := range patch {
		if _, ok := values[key]; !ok || key == "version" {
			return memory.ErrInvalid
		}
		values[key] = value
	}
	encoded, err = json.Marshal(values)
	if err != nil {
		return err
	}
	var updated session.Policy
	if json.Unmarshal(encoded, &updated) != nil {
		return memory.ErrInvalid
	}
	*policy = updated
	return nil
}
