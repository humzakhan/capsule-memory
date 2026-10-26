package memory

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type ScopeKey struct { Epoch string `json:"epoch"`; Key string `json:"key_b64"` }
type Config struct {
	Version int `json:"version"`
	Relay string `json:"relay"`
	Vault string `json:"vault_id"`
	Device string `json:"device_id"`
	OwnerPublic string `json:"owner_public_key_b64"`
	DevicePrivate string `json:"device_private_key_b64"`
	Keys map[string]ScopeKey `json:"scope_keys"`
	Loopback bool `json:"allow_loopback_http"`
	Bootstrap []Signed `json:"control_history"`
}
func safeFile(path string) error {
	fi,err:=os.Lstat(path);if err!=nil{return err}
	if !fi.Mode().IsRegular()||fi.Mode().Perm()&0077!=0{return errors.New("state file must be regular and private (0600)")};return nil
}
func privateDir(home string, create bool) error {
	if create {if err:=os.MkdirAll(home,0700);err!=nil{return err}}
	fi,err:=os.Lstat(home);if err!=nil{return err}
	if !fi.IsDir()||fi.Mode()&os.ModeSymlink!=0||fi.Mode().Perm()&0077!=0{return errors.New("state directory must be private (0700)")};return nil
}
func writeNew(path string, value any) error {
	b,err:=json.Marshal(value);if err!=nil{return err}
	f,err:=os.OpenFile(path,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);if err!=nil{return err}
	defer f.Close();if _,err=f.Write(b);err!=nil{return err};if err=f.Sync();err!=nil{return err}
	d,err:=os.Open(filepath.Dir(path));if err!=nil{return err};defer d.Close();return d.Sync()
}
func validRelay(raw string, loopback bool) error {
	u,err:=url.Parse(raw);if err!=nil||u.Host==""||u.User!=nil||u.RawQuery!=""||u.Fragment!=""||strings.Trim(u.Path,"/")!=""{return ErrInvalid}
	if u.Scheme=="https" {return nil}
	if loopback&&u.Scheme=="http"&&(u.Hostname()=="localhost"||u.Hostname()=="127.0.0.1"||u.Hostname()=="::1"){return nil};return errors.New("relay requires HTTPS; explicit loopback development flag is available")
}
func Load(home string) (Config,error) {
	var c Config
	if err:=privateDir(home,false);err!=nil{return c,err}
	path:=filepath.Join(home,"device.json");if err:=safeFile(path);err!=nil{return c,err}
	b,err:=os.ReadFile(path);if err!=nil{return c,err};if err=decode(b,&c);err!=nil{return c,err}
	if c.Version!=1||!idPattern.MatchString(c.Vault)||!idPattern.MatchString(c.Device)||len(c.Keys)==0||len(c.Keys)>128{return c,ErrInvalid}
	if err=validRelay(c.Relay,c.Loopback);err!=nil{return c,err}
	if _,err=unbase(c.OwnerPublic,32);err!=nil{return c,err};private,err:=unbase(c.DevicePrivate,64);if err!=nil{return c,err};if subtle.ConstantTimeCompare(private,ed25519.NewKeyFromSeed(private[:32]))!=1{return c,ErrInvalid}
	for s,k:=range c.Keys {if !idPattern.MatchString(s){return c,ErrInvalid};if _,err=compare(k.Epoch,"0");err!=nil{return c,err};if _,err=unbase(k.Key,32);err!=nil{return c,err}}
	return c,nil
}
func Init(ctx context.Context, home, relay string, loopback bool) (map[string]any,error) {
	if err:=validRelay(relay,loopback);err!=nil{return nil,err}
	if err:=privateDir(home,true);err!=nil{return nil,err}
	entries,err:=os.ReadDir(home);if err!=nil{return nil,err};if len(entries)!=0{return nil,errors.New("initialization requires an empty private state directory")}
	ownerPub,ownerPriv,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{return nil,err}
	devicePub,devicePriv,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{return nil,err}
	vault,err:=newID();if err!=nil{return nil,err};scope,err:=newID();if err!=nil{return nil,err};device,err:=newID();if err!=nil{return nil,err}
	key:=make([]byte,32);if _,err=rand.Read(key);err!=nil{return nil,err}
	control:=Control{1,vault,"1",[]Scope{{scope,"1"}},[]Device{{device,base64.StdEncoding.EncodeToString(devicePub),[]string{scope},false}}}
	// Legacy v1 protocol domain is fixed for compatibility; see docs/renaming.md.
	signed,err:=sign("agent-memory/control/v1",control,ownerPriv);if err!=nil{return nil,err}
	c:=Config{Version:1,Relay:strings.TrimRight(relay,"/"),Vault:vault,Device:device,OwnerPublic:base64.StdEncoding.EncodeToString(ownerPub),DevicePrivate:base64.StdEncoding.EncodeToString(devicePriv),Keys:map[string]ScopeKey{scope:{"1",base64.StdEncoding.EncodeToString(key)}},Loopback:loopback,Bootstrap:[]Signed{signed}}
	if err=writeNew(filepath.Join(home,"owner.json"),map[string]string{"private_key_b64":base64.StdEncoding.EncodeToString(ownerPriv)});err!=nil{return nil,err}
	if err=writeNew(filepath.Join(home,"device.json"),c);err!=nil{return nil,err}
	s,err:=Open(ctx,home);if err!=nil{return nil,err};defer s.Close()
	b,err:=json.Marshal(signed);if err!=nil{return nil,err}
	if _,err=s.db.ExecContext(ctx,"INSERT INTO controls(generation,signed) VALUES(?,?)",control.Generation,b);err!=nil{return nil,ErrStorage}
	if err=s.putMeta(ctx,"current_control",signed);err!=nil{return nil,err}
	return map[string]any{"vault_id":vault,"scope_id":scope,"device_id":device,"VAULT_ID":vault,"OWNER_PUBLIC_KEY_B64":c.OwnerPublic,"control_pending":true},nil
}
func (s *Service) ownerKey() (ed25519.PrivateKey,error) {
	p:=filepath.Join(s.home,"owner.json");if err:=safeFile(p);err!=nil{return nil,errors.New("owner authority required on this device")}
	var v struct{Private string `json:"private_key_b64"`};b,err:=os.ReadFile(p);if err!=nil{return nil,err};if err=decode(b,&v);err!=nil{return nil,err}
	key,err:=unbase(v.Private,64);if err!=nil{return nil,err}
	if subtle.ConstantTimeCompare(key,ed25519.NewKeyFromSeed(key[:32]))!=1{return nil,ErrInvalid}
	if base64.StdEncoding.EncodeToString(ed25519.PrivateKey(key).Public().(ed25519.PublicKey))!=s.config.OwnerPublic{return nil,ErrInvalid};return ed25519.PrivateKey(key),nil
}
func (s *Service) CreateDevice(ctx context.Context,out string) (map[string]any,error) {
	s.mu.Lock();defer s.mu.Unlock()
	owner,err:=s.ownerKey();if err!=nil{return nil,err}
	c,_,err:=s.current(ctx);if err!=nil{return nil,err}
	rows,err:=s.db.QueryContext(ctx,"SELECT signed FROM controls ORDER BY length(generation),generation");if err!=nil{return nil,ErrStorage}
	history:=[]Signed{};for rows.Next(){var b []byte;var signed Signed;if rows.Scan(&b)!=nil||decode(b,&signed)!=nil{rows.Close();return nil,ErrStorage};history=append(history,signed)};err=rows.Err();rows.Close();if err!=nil{return nil,ErrStorage}
	if len(history)>0 {latest,err:=s.control(history[len(history)-1]);if err!=nil{return nil,err};cmp,err:=compare(latest.Generation,c.Generation);if err!=nil||cmp<0{return nil,errors.New("owner control history incomplete")};c=latest}
	if len(history)==0||len(history)>=256||len(c.Devices)>=256{return nil,errors.New("device or control-history limit reached; no new profile created")}
	pub,priv,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{return nil,err};id,err:=newID();if err!=nil{return nil,err}
	scopes:=[]string{};for _,sc:=range c.Scopes {k,ok:=s.config.Keys[sc.ID];if !ok||k.Epoch!=sc.Epoch{return nil,errors.New("current scope key unavailable; manual rotation required")};scopes=append(scopes,sc.ID)}
	c.Generation=next(c.Generation);c.Devices=append(c.Devices,Device{id,base64.StdEncoding.EncodeToString(pub),scopes,false})
	if err=validateControl(c,s.config.Vault);err!=nil{return nil,err}
	signed,err:=sign("agent-memory/control/v1",c,owner);if err!=nil{return nil,err}
	child:=s.config;child.Device=id;child.DevicePrivate=base64.StdEncoding.EncodeToString(priv);child.Bootstrap=append(history,signed)
	profile,err:=json.Marshal(child);if err!=nil{return nil,err};if len(profile)>MaxEnvelope{return nil,errors.New("public bootstrap history exceeds v1 profile size limit")}
	if err=privateDir(out,true);err!=nil{return nil,err}
	if _,err=os.Lstat(filepath.Join(out,"owner.json"));!os.IsNotExist(err){return nil,errors.New("new device destination contains owner material or is inaccessible")}
	if _,err=os.Lstat(filepath.Join(out,"device.json"));!os.IsNotExist(err){return nil,errors.New("new device profile already exists or is inaccessible")}
	b,err:=json.Marshal(signed);if err!=nil{return nil,err};if _,err=s.db.ExecContext(ctx,"INSERT INTO controls(generation,signed) VALUES(?,?)",c.Generation,b);err!=nil{return nil,ErrStorage}
	if err=writeNew(filepath.Join(out,"device.json"),child);err!=nil{return nil,errors.New("owner control retained, but new profile write failed; create another profile in a new directory")}
	return map[string]any{"device_id":id,"public_key_b64":base64.StdEncoding.EncodeToString(pub),"control_pending":true,"profile_created":true},nil
}
