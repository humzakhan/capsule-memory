package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)
type Client struct{conn net.Conn;reader *bufio.Reader;mu sync.Mutex;name,token string;seq uint64;Profile string;Grant ops.Grant}
func Dial(ctx context.Context,home,name,tokenPath string)(*Client,error){if !safeName.MatchString(name){return nil,memory.ErrInvalid};b,e:=privateRead(tokenPath,256);if e!=nil{return nil,e};d:=net.Dialer{Timeout:5*time.Second};conn,e:=d.DialContext(ctx,"unix",filepath.Join(home,"run","daemon.sock"));if e!=nil{return nil,ErrUnavailable};return &Client{conn:conn,reader:bufio.NewReaderSize(conn,64<<10),name:name,token:strings.TrimSpace(string(b))},nil}
func(c *Client) Close()error{return c.conn.Close()}
func(c *Client) Call(ctx context.Context,method string,params,result any)error{c.mu.Lock();defer c.mu.Unlock();b,e:=json.Marshal(params);if e!=nil{return memory.ErrInvalid};var object map[string]json.RawMessage;if json.Unmarshal(b,&object)!=nil{return memory.ErrInvalid};if c.Profile!=""{object["profile"],_=json.Marshal(c.Profile);b,_=json.Marshal(object)};request:=Request{Version:1,ID:"req",Client:c.name,Token:c.token,Method:method,Params:b};b,e=json.Marshal(request);if e!=nil||len(b)>1<<20{return memory.ErrInvalid};deadline:=time.Now().Add(60*time.Second);if d,ok:=ctx.Deadline();ok&&d.Before(deadline){deadline=d};if e=c.conn.SetDeadline(deadline);e!=nil{return ErrUnavailable};stop:=context.AfterFunc(ctx,func(){_ = c.conn.SetDeadline(time.Now())});defer stop();if _,e=c.conn.Write(append(b,'\n'));e!=nil{return ErrUnavailable};line,e:=readLine(c.reader,8<<20);if e!=nil{return ErrUnavailable};var response Response;if strictjson.Decode(line,8<<20,&response)!=nil||response.Version!=1||response.ID!=request.ID{return memory.ErrInvalid};if !response.OK{if response.Error==nil{return memory.ErrInvalid};switch response.Error.Code{case "forbidden":return ops.ErrForbidden;case "busy":return memory.ErrProfileBusy;case "invalid":return memory.ErrInvalid;case "unknown_method":return ops.ErrUnknownMethod;default:return ErrUnavailable}};if result!=nil&&json.Unmarshal(response.Result,result)!=nil{return memory.ErrInvalid};return nil}
