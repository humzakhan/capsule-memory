package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)
type Request struct{Version int `json:"v"`;ID string `json:"id"`;Client string `json:"client"`;Token string `json:"token"`;Method string `json:"method"`;Params json.RawMessage `json:"params"`}
type WireError struct{Code string `json:"code"`;Message string `json:"message"`}
type Response struct{Version int `json:"v"`;ID string `json:"id"`;OK bool `json:"ok"`;Result json.RawMessage `json:"result,omitempty"`;Error *WireError `json:"error,omitempty"`}
func readLine(r *bufio.Reader,max int)([]byte,error){var out []byte;for{part,e:=r.ReadSlice('\n');if len(out)+len(part)>max{return nil,memory.ErrInvalid};out=append(out,part...);if e==bufio.ErrBufferFull{continue};if e!=nil{return nil,e};return out,nil}}
func serve(ctx context.Context,home string,h *app.Host)error{dir:=filepath.Join(home,"run");if e:=privateDir(dir);e!=nil{return e};path:=filepath.Join(dir,"daemon.sock");if len(path)>=socketLimit(){return errors.New("daemon socket path exceeds platform limit")};if fi,e:=os.Lstat(path);e==nil{if fi.Mode()&os.ModeSocket==0{return memory.ErrInvalid};if e=os.Remove(path);e!=nil{return e}};listener,e:=net.ListenUnix("unix",&net.UnixAddr{Name:path,Net:"unix"});if e!=nil{return ErrUnavailable};defer listener.Close();defer os.Remove(path);if e=os.Chmod(path,0600);e!=nil{return e};stop:=context.AfterFunc(ctx,func(){listener.Close()});defer stop();slots:=make(chan struct{},32);done:=make(chan struct{},32);active:=0
	defer func(){for active>0{select{case <-done:active--;case <-time.After(10*time.Second):return}}}()
	for{conn,e:=listener.AcceptUnix();if e!=nil{if ctx.Err()!=nil{return nil};return ErrUnavailable};select{case slots<-struct{}{}:active++;go func(){defer func(){<-slots;done<-struct{}{}}();handle(ctx,conn,home,h)}();default:conn.Close()};for{select{case <-done:active--;default:goto next}};next:}
}
func handle(ctx context.Context,c *net.UnixConn,home string,h *app.Host){defer c.Close();if !peerAllowed(c){return};stop:=context.AfterFunc(ctx,func(){c.Close()});defer stop();r:=bufio.NewReaderSize(c,64<<10);for{_ = c.SetReadDeadline(time.Now().Add(time.Minute));line,e:=readLine(r,1<<20);if e!=nil{return};var req Request;resp:=Response{Version:1};if strictjson.Decode(line,1<<20,&req)!=nil||req.Version!=1||!safeName.MatchString(req.ID){return};resp.ID=req.ID;g,e:=Authenticate(home,req.Client,req.Token);var value any;if e==nil{value,e=h.Registry.Call(ctx,g,req.Method,req.Params)};if e!=nil{code:="internal";switch{case errors.Is(e,ops.ErrForbidden):code="forbidden";case errors.Is(e,ops.ErrUnknownMethod):code="unknown_method";case errors.Is(e,memory.ErrInvalid):code="invalid";case errors.Is(e,memory.ErrProfileBusy),errors.Is(e,memory.ErrSyncBusy):code="busy";case errors.Is(e,ErrUnavailable):code="unavailable"};resp.Error=&WireError{Code:code,Message:"operation "+code}}else{resp.OK=true;resp.Result,e=json.Marshal(value);if e!=nil{return}};b,e:=json.Marshal(resp);if e!=nil{return};if len(b)>8<<20{b,_=json.Marshal(Response{Version:1,ID:req.ID,Error:&WireError{Code:"invalid",Message:"response exceeds size bound"}})};_ = c.SetWriteDeadline(time.Now().Add(time.Minute));if _,e=c.Write(append(b,'\n'));e!=nil||e==io.EOF{return}}}
