package claude

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/capture/filetail"
	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)

type Adapter struct{ Reader *filetail.Reader }

func (Adapter) Name() string                               { return "claude-jsonl" }
func (Adapter) Version() string                            { return "1" }
func (Adapter) ParseConfig(b json.RawMessage) (any, error) { return config(b) }
func accept(k string) bool {
	return strings.HasSuffix(k, ".jsonl") && !strings.Contains(k, ".orphaned-") && !strings.Contains(k, ".superseded-")
}
func (Adapter) Preview(ctx context.Context, s capture.Source) (capture.Preview, error) {
	c, e := config(s.Config)
	if e != nil {
		return capture.Preview{}, e
	}
	p, e := filetail.Preview(ctx, c, accept)
	p.Versions = previewVersions(ctx, c)
	return p, e
}
func (Adapter) Discover(ctx context.Context, s capture.Source, p string, n int) ([]capture.Stream, string, error) {
	c, e := config(s.Config)
	if e != nil {
		return nil, "", e
	}
	return filetail.Discover(ctx, c, p, n, accept)
}

type row struct {
	Type, SessionID, UUID, ParentUUID, Timestamp, CWD, Version, AITitle string
	Sidechain, Meta, Compact                                            bool
	Message                                                             struct {
		Role, Model string
		Content     json.RawMessage
	}
}

func parse(b []byte) (row, error) {
	var r row
	var v struct {
		Type       string `json:"type"`
		SessionID  string `json:"sessionId"`
		UUID       string `json:"uuid"`
		ParentUUID string `json:"parentUuid"`
		Timestamp  string `json:"timestamp"`
		CWD        string `json:"cwd"`
		Version    string `json:"version"`
		AITitle    string `json:"aiTitle"`
		Sidechain  bool   `json:"isSidechain"`
		Meta       bool   `json:"isMeta"`
		Compact    bool   `json:"isCompactSummary"`
		Message    struct {
			Role    string          `json:"role"`
			Model   string          `json:"model"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if strictjson.Unique(b) != nil || json.Unmarshal(b, &v) != nil || v.Type == "" {
		return r, capture.ErrInvalid
	}
	r.Type = v.Type
	r.SessionID = v.SessionID
	r.UUID = v.UUID
	r.ParentUUID = v.ParentUUID
	r.Timestamp = v.Timestamp
	r.CWD = v.CWD
	r.Version = v.Version
	r.AITitle = v.AITitle
	r.Sidechain = v.Sidechain
	r.Meta = v.Meta
	r.Compact = v.Compact
	r.Message.Role = v.Message.Role
	r.Message.Model = v.Message.Model
	r.Message.Content = v.Message.Content
	return r, nil
}
func (a Adapter) ReadChanges(ctx context.Context, s capture.Source, st capture.Stream, cp capture.Checkpoint, lim capture.Limits) (capture.Changes, error) {
	var ch capture.Changes
	c, e := config(s.Config)
	if e != nil {
		return ch, e
	}
	read := filetail.Read
	if a.Reader != nil {
		read = a.Reader.Read
	}
	b, e := read(ctx, c, st, cp, lim)
	if e != nil {
		return ch, e
	}
	ch.ScannedBytes = b.ScannedBytes
	ch.Reconcile = b.Reconcile
	ch.Next = b.Next
	ch.CaughtUp = b.CaughtUp
	ch.Quarantine = b.Quarantine
	ch.Omissions = b.Omissions
	if ch.Quarantine != "" {
		return ch, nil
	}
	h, e := filetail.Header(c.Root, st.Key)
	if e != nil {
		return ch, e
	}
	first, e := parse(h)
	if e != nil {
		ch.Quarantine = "unknown_shape"
		return ch, nil
	}
	version := first.Version
	if version == "" {
		version = "unknown"
	}
	if !filetail.Pinned(c, version) {
		ch.Quarantine = "unpinned_version"
		return ch, nil
	}
	sid := first.SessionID
	if sid == "" {
		sid = strings.TrimSuffix(filepath.Base(st.Key), ".jsonl")
	}
	origin := records.Origin{Harness: "claude", SourceStore: s.StoreIdentity, NativeSessionID: sid}
	obs := capture.Observation{Origin: origin, CWD: first.CWD, Title: first.AITitle, HarnessVersion: version, State: "open"}
	if time.Since(st.Modified) > 16*time.Minute {
		obs.State = "closed"
	}
	if strings.Contains(filepath.ToSlash(st.Key), "/subagents/") {
		parts := strings.Split(filepath.ToSlash(st.Key), "/")
		if len(parts) >= 4 {
			parent := origin
			parent.NativeSessionID = parts[len(parts)-3]
			obs.Parent = &records.SessionLink{Origin: parent, Relation: "subagent"}
			origin.NativeSessionID = sid + ":" + strings.TrimSuffix(filepath.Base(st.Key), ".jsonl")
			obs.Origin = origin
		}
	}
	ch.Observations = append(ch.Observations, obs)
	for _, line := range b.Lines {
		r, e := parse(line.Data)
		if e != nil {
			ch.Quarantine = "unknown_shape"
			return ch, nil
		}
		if r.Version != "" && !filetail.Pinned(c, r.Version) {
			ch.Quarantine = "unpinned_version"
			return ch, nil
		}
		if r.Type != "user" && r.Type != "assistant" {
			ch.Omissions = append(ch.Omissions, records.Omission{Reason: "unsupported_type", Bytes: int64(len(line.Data))})
			continue
		}
		if r.SessionID == "" || r.Message.Role == "" || len(r.Message.Content) == 0 {
			ch.Quarantine = "unknown_shape"
			return ch, nil
		}
		base := records.Event{NativeID: r.UUID, Role: r.Message.Role, Type: "message", Time: r.Timestamp, Parent: r.ParentUUID, Sidechain: r.Sidechain}
		if r.Compact {
			base.Type = "compaction"
		}
		if r.Meta || r.Message.Model == "<synthetic>" {
			base.Type = "meta"
		}
		var text string
		if json.Unmarshal(r.Message.Content, &text) == nil {
			base.Text = strings.ToValidUTF8(text, "�")
			ch.Events = append(ch.Events, capture.Event{Origin: origin, CWD: r.CWD, Event: base, PositionKey: strconv.FormatInt(line.Offset, 10)})
			continue
		}
		var blocks []struct {
			Type      string          `json:"type"`
			Text      string          `json:"text"`
			Thinking  string          `json:"thinking"`
			Name      string          `json:"name"`
			ID        string          `json:"id"`
			ToolUseID string          `json:"tool_use_id"`
			Input     json.RawMessage `json:"input"`
			Content   json.RawMessage `json:"content"`
			IsError   bool            `json:"is_error"`
		}
		if json.Unmarshal(r.Message.Content, &blocks) != nil {
			ch.Quarantine = "unknown_shape"
			return ch, nil
		}
		for i, block := range blocks {
			ev := base
			body := ""
			switch block.Type {
			case "text":
				ev.Text = block.Text
			case "tool_use":
				ev.Type = "tool_call"
				ev.ToolName = block.Name
				ev.ToolCallID = block.ID
				body = string(block.Input)
			case "tool_result":
				ev.Type = "tool_result"
				ev.Role = "tool"
				ev.ToolCallID = block.ToolUseID
				ev.ToolStatus = "completed"
				if block.IsError {
					ev.ToolStatus = "failed"
				}
				body = string(block.Content)
			case "thinking":
				ev.Type = "reasoning"
				body = block.Thinking
			default:
				ch.Omissions = append(ch.Omissions, records.Omission{Reason: "attachment_excluded", Bytes: int64(len(line.Data))})
				continue
			}
			if strings.HasPrefix(ev.ToolName, "mcp__capsule-memory__") || strings.HasPrefix(ev.ToolName, "mcp__capsule_memory__") {
				ev.Origin = "capsule_recall"
			}
			if ev.NativeID != "" {
				ev.NativeID += ":" + strconv.Itoa(i)
			}
			ev.Text = strings.ToValidUTF8(ev.Text, "�")
			ch.Events = append(ch.Events, capture.Event{Origin: origin, CWD: r.CWD, Event: ev, ToolBody: strings.ToValidUTF8(body, "�"), PositionKey: strconv.FormatInt(line.Offset, 10) + ":" + strconv.Itoa(i)})
		}
	}
	return ch, nil
}
