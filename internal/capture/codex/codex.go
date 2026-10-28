package codex

import (
	"context"
 "crypto/sha256"
 "encoding/hex"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/capture/filetail"
	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)
type Adapter struct{Reader *filetail.Reader}
type userRef struct{Hash string `json:"hash" cm:"required"`;ID string `json:"id" cm:"required"`;Kind string `json:"kind" cm:"required"`}
type checkpointState struct {
	Version int `json:"version" cm:"required"`
	Turn string `json:"turn" cm:"required"`
	Users []userRef `json:"users" cm:"required"`
	// Optional for compatibility with existing v1 checkpoint state. Nil means
	// a pre-upgrade/future-only cursor has not observed the current turn's CWD.
	CWD *string `json:"cwd,omitempty"`
}

type Config struct{Root string `json:"root"`;ArchivedRoot string `json:"archived_root"`;PinnedVersions []string `json:"pinned_versions"`;AllowUnknownVersion bool `json:"allow_unknown_version"`}
func config(b json.RawMessage)(Config,error){var c Config;if strictjson.Decode(b,1<<20,&c)!=nil{return c,capture.ErrInvalid};if c.Root==""{c.Root=defaultRoot()};if !filepath.IsAbs(c.Root)||(c.ArchivedRoot!=""&&!filepath.IsAbs(c.ArchivedRoot)){return c,capture.ErrInvalid};for _,p:=range c.PinnedVersions{if p==""{return c,capture.ErrInvalid}};return c,nil}
func tail(c Config)filetail.Config{return filetail.Config{Root:c.Root,PinnedVersions:c.PinnedVersions,AllowUnknownVersion:c.AllowUnknownVersion}}
func (Adapter) Name()string{return "codex-rollout"};func (Adapter) Version()string{return "1"}
func (Adapter) ParseConfig(b json.RawMessage)(any,error){return config(b)}
func accept(k string)bool{return strings.HasPrefix(filepath.Base(k),"rollout-")&&strings.HasSuffix(k,".jsonl")}
func (Adapter) Preview(ctx context.Context,s capture.Source)(capture.Preview,error){c,e:=config(s.Config);if e!=nil{return capture.Preview{},e};p,e:=filetail.Preview(ctx,tail(c),accept);if c.ArchivedRoot!=""{p.Locations=append(p.Locations,c.ArchivedRoot)};p.Versions=previewVersions(ctx,c);p.Unsupported=append(p.Unsupported,"app_server: not_used (no automatic spawn)");return p,e}
func (Adapter) Discover(ctx context.Context,s capture.Source,page string,n int)([]capture.Stream,string,error){c,e:=config(s.Config);if e!=nil{return nil,"",e};archive:=strings.HasPrefix(page,"archive:");if archive{page=strings.TrimPrefix(page,"archive:");c.Root=c.ArchivedRoot};streams,next,e:=filetail.Discover(ctx,tail(c),page,n,accept);if archive{for i:=range streams{streams[i].Key="archive:"+streams[i].Key};if next!=""{next="archive:"+next}}else if next==""&&c.ArchivedRoot!=""{next="archive:"};return streams,next,e}
type row struct{Type string `json:"type"`;Timestamp string `json:"timestamp"`;Payload json.RawMessage `json:"payload"`}
type payload struct{ID string `json:"id"`;SessionID string `json:"session_id"`;CWD string `json:"cwd"`;Version string `json:"cli_version"`;Type string `json:"type"`;Role string `json:"role"`;Message string `json:"message"`;Name string `json:"name"`;CallID string `json:"call_id"`;Arguments string `json:"arguments"`;Output string `json:"output"`;TurnID string `json:"turn_id"`;ForkedFrom string `json:"forked_from_id"`;Content []struct{Type string `json:"type"`;Text string `json:"text"`} `json:"content"`;Metadata struct{Turn string `json:"turn_id"`} `json:"internal_chat_message_metadata_passthrough"`}
func decode(b []byte)(row,payload,error){var r row;var p payload;if strictjson.Unique(b)!=nil||json.Unmarshal(b,&r)!=nil||r.Type==""||json.Unmarshal(r.Payload,&p)!=nil{return r,p,capture.ErrInvalid};return r,p,nil}
func (a Adapter) ReadChanges(ctx context.Context, s capture.Source, st capture.Stream, cp capture.Checkpoint, lim capture.Limits) (capture.Changes, error) {
	var changes capture.Changes
	config, err := config(s.Config)
	if err != nil {
		return changes, err
	}
	originalKey := st.Key
	if strings.HasPrefix(st.Key, "archive:") {
		config.Root = config.ArchivedRoot
		st.Key = strings.TrimPrefix(st.Key, "archive:")
	}
	if st.Size > 4<<30 {
		changes.Next = cp
		changes.Omissions = []records.Omission{{Reason: "too_large", Bytes: st.Size}}
		changes.Quarantine = "too_large"
		return changes, nil
	}
	header, err := filetail.Header(config.Root, st.Key)
	if err != nil {
		return changes, err
	}
	headerRow, metadata, err := decode(header)
	if err != nil || headerRow.Type != "session_meta" {
		changes.Quarantine = "unknown_shape"
		return changes, nil
	}
	if !filetail.Pinned(tail(config), metadata.Version) {
		changes.Quarantine = "unpinned_version"
		return changes, nil
	}
	sessionID := metadata.ID
	if sessionID == "" {
		sessionID = metadata.SessionID
	}
	if sessionID == "" || len(metadata.CWD) > 4096 {
		changes.Quarantine = "unknown_shape"
		return changes, nil
	}
	origin := records.Origin{Harness: "codex", SourceStore: s.StoreIdentity, NativeSessionID: sessionID}
	observation := capture.Observation{Origin: origin, CWD: metadata.CWD, HarnessVersion: metadata.Version, State: "unknown"}
	if metadata.ForkedFrom != "" {
		parent := origin
		parent.NativeSessionID = metadata.ForkedFrom
		observation.Parent = &records.SessionLink{Origin: parent, Relation: "fork"}
	}
	read := filetail.Read
	if a.Reader != nil {
		read = a.Reader.Read
	}
	batch, err := read(ctx, tail(config), st, cp, lim)
	if err != nil {
		return changes, err
	}
	changes.ScannedBytes = batch.ScannedBytes
	changes.Reconcile = batch.Reconcile
	changes.Next = batch.Next
	changes.Next.Stream = originalKey
	changes.Quarantine = batch.Quarantine
	changes.Omissions = batch.Omissions
	changes.CaughtUp = batch.CaughtUp
	if changes.Quarantine != "" {
		return changes, nil
	}
	state := checkpointState{Version: 1, Users: []userRef{}}
	if len(changes.Next.AdapterState) > 65536 {
		return changes, capture.ErrInvalid
	}
	if changes.Next.AdapterState != "" {
		if strictjson.Decode([]byte(changes.Next.AdapterState), 65536, &state) != nil {
			return changes, capture.ErrInvalid
		}
	} else if cp.Position == "" || cp.Position == "0" || batch.Reconcile && cp.Generation != batch.Next.Generation {
		// Only a read beginning at the header can use its initial CWD. An
		// existing cursor may be in a later turn with a different directory.
		cwd := metadata.CWD
		state.CWD = &cwd
	}
	if state.Version != 1 || len(state.Turn) > 256 || state.Users == nil || len(state.Users) > 256 ||
		(state.CWD != nil && len(*state.CWD) > 4096) {
		return changes, capture.ErrInvalid
	}
	for _, ref := range state.Users {
		digest, err := hex.DecodeString(ref.Hash)
		if err != nil || len(digest) != 32 || len(ref.Hash) != 64 || ref.ID == "" || len(ref.ID) > 256 ||
			(ref.Kind != "event_msg" && ref.Kind != "response_item") {
			return changes, capture.ErrInvalid
		}
	}
	turn := state.Turn
	for _, line := range batch.Lines {
		row, payload, err := decode(line.Data)
		if err != nil {
			changes.Quarantine = "unknown_shape"
			return changes, nil
		}
		if row.Type == "session_meta" {
			continue
		}
		if row.Type == "turn_context" {
			if len(payload.CWD) > 4096 || len(payload.TurnID) > 256 {
				changes.Quarantine = "unknown_shape"
				return changes, nil
			}
			state.Users = []userRef{}
			turn = payload.TurnID
			if turn == "" {
				turn = "position:" + strconv.FormatInt(line.Offset, 10)
			}
			cwd := payload.CWD
			state.CWD = &cwd
			continue
		}
		event := records.Event{Role: "system", Type: "meta", Time: row.Timestamp, Turn: turn}
		body := ""
		position := strconv.FormatInt(line.Offset, 10)
		event.NativeID = payload.ID
		event.ID = payload.ID
		if event.ID == "" {
			event.ID = "p:" + changes.Next.Generation + ":" + position
		}
		switch row.Type {
		case "event_msg":
			if payload.Type != "user_message" {
				continue
			}
			event.Role = "user"
			event.Type = "message"
			event.Text = payload.Message
		case "response_item":
			switch payload.Type {
			case "message":
				if payload.Role != "user" && payload.Role != "assistant" {
					changes.Quarantine = "unknown_shape"
					return changes, nil
				}
				event.Role = payload.Role
				event.Type = "message"
				for _, content := range payload.Content {
					event.Text += content.Text
				}
				if payload.Metadata.Turn != "" {
					event.Turn = payload.Metadata.Turn
				}
			case "function_call":
				event.Role = "assistant"
				event.Type = "tool_call"
				event.ToolName = payload.Name
				event.ToolCallID = payload.CallID
				body = payload.Arguments
			case "function_call_output":
				event.Role = "tool"
				event.Type = "tool_result"
				event.ToolCallID = payload.CallID
				body = payload.Output
			case "reasoning":
				event.Type = "reasoning"
				body = string(row.Payload)
			default:
				changes.Omissions = append(changes.Omissions, records.Omission{Reason: "unsupported_type", Bytes: int64(len(line.Data))})
				continue
			}
		default:
			changes.Omissions = append(changes.Omissions, records.Omission{Reason: "unsupported_type", Bytes: int64(len(line.Data))})
			continue
		}
		if state.CWD == nil || *state.CWD == "" {
			// Existing checkpoints remain readable. Without turn metadata we
			// cannot safely attribute their new events to the header's project.
			changes.Omissions = append(changes.Omissions, records.Omission{Event: event.ID, Reason: "ambiguous_project"})
			continue
		}
		event.Text = strings.ToValidUTF8(event.Text, "�")
		// User deduplication persists hashes and references only. Directory
		// metadata is separate; no prompt plaintext enters checkpoint state.
		if event.Role == "user" && event.Type == "message" {
			sum := sha256.Sum256([]byte(event.Turn + "\n" + event.Text))
			digest := hex.EncodeToString(sum[:])
			matched := -1
			for i, prior := range state.Users {
				if prior.Hash == digest && prior.Kind != row.Type {
					matched = i
					event.ViewOf = prior.ID
					event.Text = ""
					changes.Omissions = append(changes.Omissions, records.Omission{Event: event.ID, Reason: "duplicate_view"})
					break
				}
			}
			if matched >= 0 {
				state.Users = append(state.Users[:matched], state.Users[matched+1:]...)
			} else {
				if len(state.Users) >= 256 {
					changes.Quarantine = "turn_identity_bound"
					return changes, nil
				}
				state.Users = append(state.Users, userRef{Hash: digest, ID: event.ID, Kind: row.Type})
			}
		}
		changes.Events = append(changes.Events, capture.Event{
			Origin: origin, CWD: *state.CWD, Event: event,
			ToolBody: strings.ToValidUTF8(body, "�"), PositionKey: position,
		})
	}
	observation.CWD = ""
	if state.CWD != nil {
		observation.CWD = *state.CWD
	}
	changes.Observations = []capture.Observation{observation}
	state.Turn = turn
	encoded, err := json.Marshal(state)
	if err != nil || len(encoded) > 65536 {
		return changes, capture.ErrInvalid
	}
	changes.Next.AdapterState = string(encoded)
	return changes, nil
}
