package app

import (
	"context"
	"encoding/json"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
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

func (h *Host) register() {
	groups := []struct {
		cap   ops.Capability
		names []string
	}{
		{ops.CapStatus, []string{"status.get"}},
		{ops.CapMemoryRead, []string{"memory.recall", "memory.get", "memory.skill_list", "memory.skill_fetch"}},
		{ops.CapMemoryWrite, []string{"memory.store", "memory.delete", "memory.restore"}},
		{ops.CapKnowledge, []string{"knowledge.search", "candidate.list"}},
		{ops.CapCandidateReview, []string{"candidate.accept", "candidate.reject"}},
		{ops.CapSessionRead, []string{"session.search", "session.read"}},
		{ops.CapHint, []string{"capture.hint"}},
		{ops.CapAdmin, []string{
			"admin.source.add", "admin.source.configure", "admin.source.list",
			"admin.source.enable", "admin.source.disable", "admin.source.pause",
			"admin.source.resume", "admin.source.backfill", "admin.project.map",
			"admin.project.unmap", "admin.project.exclude", "admin.project.include",
			"admin.project.suggest", "admin.project.list", "admin.policy.get",
			"admin.policy.set", "admin.session.delete", "admin.session.restore",
			"admin.session.export", "admin.session.import", "admin.derive.assign",
			"admin.derive.retry", "admin.derive.run", "admin.promote.request",
			"admin.promote.list", "admin.promote.withdraw", "admin.bridge.grant",
			"admin.aggregation.grant", "admin.sync.now", "admin.scope.add",
			"admin.device.create", "admin.device.grant", "admin.device.revoke",
			"admin.rebuild", "admin.skill.approve", "admin.skill.activate",
			"admin.legacy.status", "admin.profile.info",
		}},
	}
	for _, group := range groups {
		for _, name := range group.names {
			h.Registry.Add(ops.Method{Name: name, Cap: group.cap, Handler: h.handler(name)})
		}
	}
}

// Registry.Call checks the method capability before this adapter selects a
// profile. Scope-derived operations still enforce their own exact record scope.
func (h *Host) handler(name string) ops.Handler {
	return func(ctx context.Context, grant ops.Grant, raw json.RawMessage) (any, error) {
		var params Params
		if len(raw) == 0 {
			raw = []byte("{}")
		}
		if strictjson.Decode(raw, 1<<20, &params) != nil {
			return nil, memory.ErrInvalid
		}
		if params.Mode != "" && params.Mode != "strict" && params.Mode != "historical" {
			return nil, memory.ErrInvalid
		}
		profileName, err := authorizedProfile(grant, params.Profile)
		if err != nil {
			return nil, err
		}
		if name == "status.get" && profileName == "" {
			status, err := h.Status(ctx)
			if err != nil {
				return nil, err
			}
			status["grant"] = grant
			return status, nil
		}
		profile, err := h.Profile(profileName)
		if err != nil {
			return nil, err
		}
		grant, err = profileGrant(ctx, profile, grant)
		if err != nil {
			return nil, err
		}
		if params.Scope != "" && !grant.AllowsScope(params.Scope) {
			return nil, ops.ErrForbidden
		}
		return h.dispatch(ctx, profile, grant, name, params)
	}
}

func authorizedProfile(grant ops.Grant, requested string) (string, error) {
	if requested != "" && grant.Profile != "" && requested != grant.Profile {
		return "", ops.ErrForbidden
	}
	if grant.Profile != "" {
		return grant.Profile, nil
	}
	if !grant.Has(ops.CapAdmin) {
		return "", ops.ErrForbidden
	}
	return requested, nil
}

func profileGrant(ctx context.Context, profile *Profile, grant ops.Grant) (ops.Grant, error) {
	if !grant.Has(ops.CapAdmin) {
		return grant, nil
	}
	info, err := profile.Svc.Info(ctx)
	if err != nil {
		return grant, err
	}
	grant.Caps = append(grant.Caps, ops.CapStatus, ops.CapMemoryRead,
		ops.CapMemoryWrite, ops.CapKnowledge, ops.CapSessionRead,
		ops.CapCandidateReview, ops.CapHint)
	grant.Profile = profile.Name
	grant.Scopes = nil
	for _, scope := range info.Scopes {
		grant.Scopes = append(grant.Scopes, scope.ID)
	}
	return grant, nil
}

func (h *Host) dispatch(ctx context.Context, profile *Profile, grant ops.Grant, name string, in Params) (any, error) {
	svc := profile.Svc
	sess := profile.Session
	switch name {
	case "status.get":
		status, err := sess.Status(ctx)
		if err != nil {
			return nil, err
		}
		derived, err := profile.Derive.Status(ctx)
		if err != nil {
			return nil, err
		}
		retries := h.SyncRetries(profile.Name)
		for scope := range retries {
			if !grant.AllowsScope(scope) {
				delete(retries, scope)
			}
		}
		return map[string]any{
			"capture": status, "derive": derived, "grant": grant,
			"publication_retry": retries, "runtime_acceptance": "NOT RUN",
		}, nil
	case "memory.recall":
		return svc.Recall(ctx, in.Scope, in.Query)
	case "memory.get":
		return svc.Get(ctx, in.Scope, in.ID, in.History)
	case "memory.skill_list":
		v, err := svc.ListSkills(ctx, in.Scope)
		return memory.SkillListResult(v), err
	case "memory.skill_fetch":
		return svc.FetchSkill(ctx, in.Scope, in.ID, in.History)
	case "memory.store", "memory.delete", "memory.restore":
		operation := ""
		if name == "memory.delete" {
			operation = "delete"
		} else if name == "memory.restore" {
			operation = "restore"
		}
		authorSource := "harness"
		if grant.Has(ops.CapAdmin) {
			authorSource = ""
		}
		return svc.Write(ctx, memory.WriteInput{
			Scope: in.Scope, Kind: in.Kind, Text: in.Text, TextSet: in.TextSet,
			Record: in.ID, Parents: in.Parents, Operation: operation, Source: authorSource,
		})
	case "knowledge.search":
		if in.Mode == "" {
			in.Mode = "historical"
		}
		return profile.Derive.SearchKnowledgeMode(ctx, grant, in.Query, in.Limit, in.Mode)
	case "candidate.list":
		return profile.Derive.ListCandidates(ctx, grant, in.Scope, in.Review, in.Limit)
	case "candidate.accept":
		return profile.Derive.Accept(ctx, grant, in.ID, in.Revision, in.EditedText)
	case "candidate.reject":
		return profile.Derive.Reject(ctx, grant, in.ID, in.Revision)
	case "session.search":
		return sess.Search(ctx, grant, in.Query, in.Limit)
	case "session.read":
		return sess.ReadSession(ctx, grant, in.Session, in.Branch, in.From, in.MaxBytes)
	case "capture.hint":
		sess.Hint(in.Source, in.Stream)
		return map[string]any{"hinted": true}, nil
	default:
		return h.dispatchAdmin(ctx, profile, name, in)
	}
}
