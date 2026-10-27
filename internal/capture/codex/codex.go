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
type checkpointState struct{Version int `json:"version" cm:"required"`;Turn string `json:"turn" cm:"required"`;Users []userRef `json:"users" cm:"required"`}

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
func (a Adapter) ReadChanges(ctx context.Context,s capture.Source,st capture.Stream,cp capture.Checkpoint,lim capture.Limits)(capture.Changes,error){
	var ch capture.Changes;c,e:=config(s.Config);if e!=nil{return ch,e};origKey:=st.Key;if strings.HasPrefix(st.Key,"archive:"){c.Root=c.ArchivedRoot;st.Key=strings.TrimPrefix(st.Key,"archive:")};if st.Size>4<<30{ch.Next=cp;ch.Omissions=[]records.Omission{{Reason:"too_large",Bytes:st.Size}};ch.Quarantine="too_large";return ch,nil}
	h,e:=filetail.Header(c.Root,st.Key);if e!=nil{return ch,e};r,p,e:=decode(h);if e!=nil||r.Type!="session_meta"{ch.Quarantine="unknown_shape";return ch,nil};if !filetail.Pinned(tail(c),p.Version){ch.Quarantine="unpinned_version";return ch,nil};sid:=p.ID;if sid==""{sid=p.SessionID};if sid==""{ch.Quarantine="unknown_shape";return ch,nil};origin:=records.Origin{Harness:"codex",SourceStore:s.StoreIdentity,NativeSessionID:sid};obs:=capture.Observation{Origin:origin,CWD:p.CWD,HarnessVersion:p.Version,State:"unknown"};if p.ForkedFrom!=""{parent:=origin;parent.NativeSessionID=p.ForkedFrom;obs.Parent=&records.SessionLink{Origin:parent,Relation:"fork"}}
	read:=filetail.Read;if a.Reader!=nil{read=a.Reader.Read};b,e:=read(ctx,tail(c),st,cp,lim);if e!=nil{return ch,e};ch.ScannedBytes=b.ScannedBytes;ch.Reconcile=b.Reconcile;ch.Next=b.Next;ch.Next.Stream=origKey;ch.Quarantine=b.Quarantine;ch.Omissions=b.Omissions;ch.CaughtUp=b.CaughtUp;ch.Observations=[]capture.Observation{obs};if ch.Quarantine!=""{return ch,nil}
 state:=checkpointState{Version:1,Users:[]userRef{}}
 if len(ch.Next.AdapterState)>65536{return ch,capture.ErrInvalid};if ch.Next.AdapterState!=""&&strictjson.Decode([]byte(ch.Next.AdapterState),65536,&state)!=nil{return ch,capture.ErrInvalid}
 if state.Version!=1||len(state.Turn)>256||state.Users==nil||len(state.Users)>256{return ch,capture.ErrInvalid};for _,ref:=range state.Users{digest,err:=hex.DecodeString(ref.Hash);if err!=nil||len(digest)!=32||len(ref.Hash)!=64||ref.ID==""||len(ref.ID)>256||(ref.Kind!="event_msg"&&ref.Kind!="response_item"){return ch,capture.ErrInvalid}}
 turn:=state.Turn

	for _,line:=range b.Lines{r,p,e:=decode(line.Data);if e!=nil{ch.Quarantine="unknown_shape";return ch,nil}
		if r.Type=="session_meta"{continue};if r.Type=="turn_context"{state.Users=[]userRef{};turn=p.TurnID;if turn==""{turn="position:"+strconv.FormatInt(line.Offset,10)};continue}
		ev:=records.Event{Role:"system",Type:"meta",Time:r.Timestamp,Turn:turn};body:="";position:=strconv.FormatInt(line.Offset,10);ev.NativeID=p.ID;ev.ID=p.ID;if ev.ID==""{ev.ID="p:"+ch.Next.Generation+":"+position}
		switch r.Type{case "event_msg":if p.Type!="user_message"{continue};ev.Role="user";ev.Type="message";ev.Text=p.Message
		case "response_item":switch p.Type{case "message":if p.Role!="user"&&p.Role!="assistant"{ch.Quarantine="unknown_shape";return ch,nil};ev.Role=p.Role;ev.Type="message";for _,v:=range p.Content{ev.Text+=v.Text};if p.Metadata.Turn!=""{ev.Turn=p.Metadata.Turn}
		case "function_call":ev.Role="assistant";ev.Type="tool_call";ev.ToolName=p.Name;ev.ToolCallID=p.CallID;body=p.Arguments
		case "function_call_output":ev.Role="tool";ev.Type="tool_result";ev.ToolCallID=p.CallID;body=p.Output
		case "reasoning":ev.Type="reasoning";body=string(r.Payload)
		default:ch.Omissions=append(ch.Omissions,records.Omission{Reason:"unsupported_type",Bytes:int64(len(line.Data))});continue}
		default:ch.Omissions=append(ch.Omissions,records.Omission{Reason:"unsupported_type",Bytes:int64(len(line.Data))});continue}
  ev.Text=strings.ToValidUTF8(ev.Text,"�")
  // Deduplicate the two native user representations even across poll boundaries.
  // Only hashes and native references enter the checkpoint; no prompt plaintext.
  if ev.Role=="user"&&ev.Type=="message"{sum:=sha256.Sum256([]byte(ev.Turn+"\n"+ev.Text));digest:=hex.EncodeToString(sum[:]);matched:=-1;for i,prior:=range state.Users{if prior.Hash==digest&&prior.Kind!=r.Type{matched=i;ev.ViewOf=prior.ID;ev.Text="";ch.Omissions=append(ch.Omissions,records.Omission{Event:ev.ID,Reason:"duplicate_view"});break}};if matched>=0{state.Users=append(state.Users[:matched],state.Users[matched+1:]...)}else{if len(state.Users)>=256{ch.Quarantine="turn_identity_bound";return ch,nil};state.Users=append(state.Users,userRef{Hash:digest,ID:ev.ID,Kind:r.Type})}}
  ch.Events=append(ch.Events,capture.Event{Origin:origin,Event:ev,ToolBody:strings.ToValidUTF8(body,"�"),PositionKey:position})
 }
 state.Turn=turn;encoded,er:=json.Marshal(state);if er!=nil||len(encoded)>65536{return ch,capture.ErrInvalid};ch.Next.AdapterState=string(encoded);return ch,nil
}
