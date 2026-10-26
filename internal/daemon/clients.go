package daemon

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)
var safeName=regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
type ClientConfig struct{Version int `json:"version"`;Name string `json:"name"`;TokenSHA256 string `json:"token_sha256"`;Grant ops.Grant `json:"grant"`}
func privateDir(path string)error{if e:=os.MkdirAll(path,0700);e!=nil{return e};fi,e:=os.Lstat(path);if e!=nil||!fi.IsDir()||fi.Mode().Perm()!=0700{return memory.ErrInvalid};return nil}
func privateRead(path string,max int64)([]byte,error){fi,e:=os.Lstat(path);if e!=nil||!fi.Mode().IsRegular()||fi.Mode().Perm()!=0600||fi.Size()>max{return nil,memory.ErrInvalid};b,e:=os.ReadFile(path);if e!=nil{return nil,memory.ErrInvalid};return b,nil}
func writeNew(path string,b []byte)error{f,e:=os.OpenFile(path,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if e!=nil{return e};defer f.Close();if _,e=f.Write(b);e!=nil{return e};if e=f.Sync();e!=nil{return e};d,e:=os.Open(filepath.Dir(path));if e!=nil{return e};defer d.Close();return d.Sync()}
func Init(ctx context.Context,home string,profiles []app.ProfileRef)(map[string]any,error){if len(profiles)==0||len(profiles)>32{return nil,memory.ErrInvalid};if e:=privateDir(home);e!=nil{return nil,e};for _,p:=range profiles{if !safeName.MatchString(p.Name)||!filepath.IsAbs(p.Home){return nil,memory.ErrInvalid}};b,e:=app.EncodeConfig(app.DaemonConfig{Version:1,Profiles:profiles});if e!=nil{return nil,e};if e=writeNew(filepath.Join(home,"daemon.json"),b);e!=nil{return nil,e};caps:=[]ops.Capability{ops.CapAdmin,ops.CapStatus,ops.CapMemoryRead,ops.CapMemoryWrite,ops.CapKnowledge,ops.CapSessionRead,ops.CapCandidateReview,ops.CapHint};path,e:=addClient(home,"admin",ops.Grant{Client:"admin",Caps:caps});return map[string]any{"config":filepath.Join(home,"daemon.json"),"admin_token_file":path,"installed":false},e}
func AddClient(ctx context.Context,home,name string,g ops.Grant)(string,error){if name=="admin"||g.Has(ops.CapAdmin)||!g.Has(ops.CapStatus){return "",ops.ErrForbidden};cfg,e:=app.LoadDaemonConfig(home);if e!=nil{return "",e};var ref *app.ProfileRef;for _,p:=range cfg.Profiles{if p.Name==g.Profile{copy:=p;ref=&copy}};if ref==nil{return "",memory.ErrInvalid};info,e:=profileInfo(ctx,home,ref.Name,ref.Home);if e!=nil{return "",e};for _,scope:=range g.Scopes{valid:=false;for _,sc:=range info.Scopes{if sc.ID==scope{valid=true}};if !valid{return "",ops.ErrForbidden}};for _,cap:=range g.Caps{switch cap{case ops.CapStatus,ops.CapMemoryRead,ops.CapMemoryWrite,ops.CapKnowledge,ops.CapSessionRead,ops.CapCandidateReview,ops.CapHint:default:return "",ops.ErrForbidden}};return addClient(home,name,g)}
func addClient(home,name string,g ops.Grant)(string,error){if !safeName.MatchString(name){return "",memory.ErrInvalid};dir:=filepath.Join(home,"clients");if e:=privateDir(dir);e!=nil{return "",e};token:=make([]byte,32);if _,e:=rand.Read(token);e!=nil{return "",e};encoded:=base64.StdEncoding.EncodeToString(token);sum:=sha256.Sum256([]byte(encoded));g.Client=name;cfg:=ClientConfig{Version:1,Name:name,TokenSHA256:hex.EncodeToString(sum[:]),Grant:g};b,e:=json.Marshal(cfg);if e!=nil{return "",e};path:=filepath.Join(dir,name+".token");if e=writeNew(path,[]byte(encoded+"\n"));e!=nil{return "",e};if e=writeNew(filepath.Join(dir,name+".json"),b);e!=nil{return "",e};return path,nil}
func Authenticate(home,name,token string)(ops.Grant,error){if !safeName.MatchString(name)||len(token)>128{return ops.Grant{},ops.ErrForbidden};b,e:=privateRead(filepath.Join(home,"clients",name+".json"),1<<20);if e!=nil{return ops.Grant{},ops.ErrForbidden};var c ClientConfig;if strictjson.Decode(b,1<<20,&c)!=nil||c.Version!=1||c.Name!=name{return ops.Grant{},ops.ErrForbidden};raw,e:=base64.StdEncoding.DecodeString(token);if e!=nil||len(raw)!=32{return ops.Grant{},ops.ErrForbidden};sum:=sha256.Sum256([]byte(token));want,e:=hex.DecodeString(c.TokenSHA256);if e!=nil||subtle.ConstantTimeCompare(sum[:],want)!=1{return ops.Grant{},ops.ErrForbidden};c.Grant.Client=name;return c.Grant,nil}
var ErrUnavailable=errors.New("capsule-memory daemon unavailable")
