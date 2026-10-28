// Package importv1 accepts owner exports, never URLs or executable references.
package importv1

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/capture/filetail"
	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)
type Adapter struct{Reader *filetail.Reader}
type Config struct{Files []string `json:"files"`;Dir string `json:"dir"`;Glob string `json:"glob"`}
type Line struct {
	Version int `json:"capsule_import"`;Kind string `json:"kind"`
	Harness string `json:"harness,omitempty"`;HarnessVersion string `json:"harness_version,omitempty"`;Store string `json:"source_store_id,omitempty"`;SessionID string `json:"session_id"`;Environment string `json:"environment,omitempty"`
	ParentSessionID string `json:"parent_session_id,omitempty"`;Lineage string `json:"lineage,omitempty"`;CWD string `json:"cwd,omitempty"`;Title string `json:"title,omitempty"`;CapturedAtBasis string `json:"captured_at_basis,omitempty"`;Generation string `json:"generation,omitempty"`
	EventID string `json:"event_id,omitempty"`;Seq int64 `json:"seq,omitempty"`;ParentEventID string `json:"parent_event_id,omitempty"`;Role string `json:"role,omitempty"`;Type string `json:"type,omitempty"`;Text string `json:"text,omitempty"`;Tool json.RawMessage `json:"tool,omitempty"`;Status string `json:"status,omitempty"`;Time string `json:"time,omitempty"`;Native json.RawMessage `json:"native,omitempty"`;Event *records.Event `json:"event,omitempty"`
}
func config(b json.RawMessage)(Config,error){var c Config;if strictjson.Decode(b,1<<20,&c)!=nil||(len(c.Files)==0)==(c.Dir==""){return c,capture.ErrInvalid};if len(c.Files)>1024{return c,capture.ErrInvalid};for _,p:=range c.Files{if !filepath.IsAbs(p){return c,capture.ErrInvalid}};if c.Dir!=""{if !filepath.IsAbs(c.Dir)||strings.ContainsAny(c.Glob,"/\\"){return c,capture.ErrInvalid};if c.Glob==""{c.Glob="*.jsonl"};if _,e:=filepath.Match(c.Glob,"");e!=nil{return c,capture.ErrInvalid}};return c,nil}
func (Adapter) Name()string{return "capsule-import"};func (Adapter) Version()string{return "1"}
func (Adapter) ParseConfig(b json.RawMessage)(any,error){return config(b)}
func (a Adapter) Preview(ctx context.Context,s capture.Source)(capture.Preview,error){ss,_,e:=a.Discover(ctx,s,"",64);p:=capture.Preview{Versions:[]string{"capsule-transcript/v1"}};for _,st:=range ss{p.Locations=append(p.Locations,st.Key);p.Bytes+=st.Size};p.Streams=len(ss);return p,e}
func (Adapter) Discover(ctx context.Context,s capture.Source,page string,n int)([]capture.Stream,string,error){c,e:=config(s.Config);if e!=nil{return nil,"",e};paths:=append([]string{},c.Files...);if c.Dir!=""{paths,e=filepath.Glob(filepath.Join(c.Dir,c.Glob));if e!=nil{return nil,"",capture.ErrInvalid}};sort.Strings(paths);if n<=0||n>64{n=64};var out []capture.Stream;next:="";for _,p:=range paths{if ctx.Err()!=nil{return nil,"",ctx.Err()};if p<=page{continue};if len(out)==n{next=out[len(out)-1].Key;break};fi,e:=os.Lstat(p);if e!=nil||!fi.Mode().IsRegular(){continue};out=append(out,capture.Stream{Key:p,Size:fi.Size(),Generation:filetail.Identity(fi),Modified:fi.ModTime()})};return out,next,nil}
func decode(b []byte)(Line,error){var l Line;if strictjson.Decode(b,8<<20,&l)!=nil||l.Version!=1||l.SessionID==""{return l,capture.ErrInvalid};return l,nil}
func (a Adapter) ReadChanges(ctx context.Context,s capture.Source,st capture.Stream,cp capture.Checkpoint,lim capture.Limits)(capture.Changes,error){
	var ch capture.Changes;c,e:=config(s.Config);if e!=nil{return ch,e};allowed:=false;for _,p:=range c.Files{if p==st.Key{allowed=true}};if c.Dir!=""{rel,e:=filepath.Rel(c.Dir,st.Key);if e==nil&&!strings.Contains(rel,string(filepath.Separator))&&rel!=".."{allowed,_=filepath.Match(c.Glob,rel)}};if !allowed{return ch,capture.ErrInvalid};if st.Size>4<<30{ch.Quarantine="too_large";return ch,nil}
	tc:=filetail.Config{Root:filepath.Dir(st.Key)};local:=st;local.Key=filepath.Base(st.Key);h,e:=filetail.Header(tc.Root,local.Key);if e!=nil{return ch,e};head,e:=decode(h);if e!=nil||head.Kind!="session"||head.Harness==""||head.Store==""{ch.Quarantine="unknown_shape";return ch,nil}
	origin:=records.Origin{Harness:head.Harness,SourceStore:head.Store,NativeSessionID:head.SessionID,Environment:head.Environment};obs:=capture.Observation{Origin:origin,CWD:head.CWD,Title:head.Title,HarnessVersion:head.HarnessVersion,State:"unknown"};if head.ParentSessionID!=""{parent:=origin;parent.NativeSessionID=head.ParentSessionID;obs.Parent=&records.SessionLink{Origin:parent,Relation:head.Lineage}};ch.Observations=[]capture.Observation{obs}
	read:=filetail.Read;if a.Reader!=nil{read=a.Reader.Read};b,e:=read(ctx,tc,local,cp,lim);if e!=nil{return ch,e};ch.ScannedBytes=b.ScannedBytes;ch.Reconcile=b.Reconcile;ch.Next=b.Next;ch.Next.Stream=st.Key;ch.Quarantine=b.Quarantine;ch.Omissions=b.Omissions;ch.CaughtUp=b.CaughtUp
	for _,line:=range b.Lines{l,e:=decode(line.Data);if e!=nil{ch.Quarantine="unknown_shape";return ch,nil};if l.Kind=="session"{continue};if l.Kind!="event"||l.SessionID!=head.SessionID||l.Role==""||l.Type==""||(l.Status!="final"&&l.Status!="partial"){ch.Quarantine="unknown_shape";return ch,nil};if l.Status == "partial" {
			rewound, err := filetail.RewindCheckpoint(ctx, tc, local, b.Next, line.Offset)
			if err != nil {
				return capture.Changes{}, err
			}
			rewound.Stream = st.Key
			ch.Next = rewound
			ch.CaughtUp = false
			break
		};ev:=records.Event{NativeID:l.EventID,Role:l.Role,Type:l.Type,Text:strings.ToValidUTF8(l.Text,"�"),Parent:l.ParentEventID,Time:l.Time};if l.Event!=nil{ev=*l.Event;ev.Text=strings.ToValidUTF8(ev.Text,"�")};body:="";if len(l.Tool)>0||len(l.Native)>0{bb,_:=json.Marshal(map[string]json.RawMessage{"tool":l.Tool,"native":l.Native});body=string(bb)};ch.Events=append(ch.Events,capture.Event{Origin:origin,CWD:l.CWD,Event:ev,ToolBody:body,PositionKey:strconv.FormatInt(line.Offset,10)})};return ch,nil
}
