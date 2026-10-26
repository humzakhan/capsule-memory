package memory

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"regexp"
	"time"
	"unicode/utf8"
)

const MaxEnvelope = 2 << 20
const MaxText = 1 << 20

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)
var decimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
var ErrInvalid = errors.New("invalid or unauthorized data")
var ErrStorage = errors.New("local storage operation failed")

type Signed struct {
	Payload string `json:"payload"`
	Signature string `json:"signature"`
}
type Scope struct {
	ID string `json:"scope_id"`
	Epoch string `json:"key_epoch"`
}
type Device struct {
	ID string `json:"device_id"`
	Public string `json:"public_key_b64"`
	Scopes []string `json:"scope_ids"`
	Revoked bool `json:"revoked"`
}
type Control struct {
	Version int `json:"version"`
	Vault string `json:"vault_id"`
	Generation string `json:"generation"`
	Scopes []Scope `json:"scopes"`
	Devices []Device `json:"devices"`
}
type Envelope struct {
	Version int `json:"version"`
	Vault string `json:"vault_id"`
	Scope string `json:"scope_id"`
	Device string `json:"device_id"`
	Epoch string `json:"key_epoch"`
	Transport string `json:"transport_id"`
	Nonce string `json:"nonce_b64"`
	Ciphertext string `json:"ciphertext_b64"`
	Digest string `json:"ciphertext_sha256"`
}
type Provenance struct {
	SourceKind string `json:"source_kind"`
	SourceRef string `json:"source_ref"`
	Harness string `json:"harness,omitempty"`
}
type Payload struct { Text string `json:"text"` }
type Revision struct {
	Version int `json:"schema_version"`
	Vault string `json:"vault_id"`
	Scope string `json:"scope_id"`
	Record string `json:"record_id"`
	Kind string `json:"kind"`
	ID string `json:"revision_id"`
	Parents []string `json:"parents"`
	OperationID string `json:"operation_id"`
	Operation string `json:"operation"`
	Restores []string `json:"restores,omitempty"`
	Author string `json:"author_device_id"`
	Created string `json:"created_at"`
	Provenance Provenance `json:"provenance"`
	Tags []string `json:"tags"`
	Payload *Payload `json:"payload,omitempty"`
}

func newID() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil { return "", err }
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func decode(b []byte, dst any) error {
	if len(b) > MaxEnvelope || !utf8.Valid(b) { return ErrInvalid }
	if err:=uniqueJSON(b);err!=nil{return err}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil { return ErrInvalid }
	if d.Decode(new(any)) != io.EOF { return ErrInvalid }
	return wireFields(b,dst)
}
// Reject duplicate keys before encoding/json can silently choose a value.
func uniqueJSON(b []byte) error {
	d:=json.NewDecoder(bytes.NewReader(b));d.UseNumber()
	var value func(int)error
	value=func(depth int)error{
		if depth>32{return ErrInvalid};token,err:=d.Token();if err!=nil{return ErrInvalid}
		delim,ok:=token.(json.Delim);if !ok{return nil}
		switch delim {
		case '{':seen:=map[string]bool{};for d.More(){key,err:=d.Token();if err!=nil{return ErrInvalid};name,ok:=key.(string);if !ok||seen[name]{return ErrInvalid};seen[name]=true;if err=value(depth+1);err!=nil{return err}};end,err:=d.Token();if err!=nil||end!=json.Delim('}'){return ErrInvalid}
		case '[':for d.More(){if err=value(depth+1);err!=nil{return err}};end,err:=d.Token();if err!=nil||end!=json.Delim(']'){return ErrInvalid}
		default:return ErrInvalid
		};return nil
	}
	if err:=value(0);err!=nil{return err};if _,err:=d.Token();err!=io.EOF{return ErrInvalid};return nil
}
func fields(b []byte,required,optional []string) (map[string]json.RawMessage,error) {
	var m map[string]json.RawMessage;if json.Unmarshal(b,&m)!=nil||m==nil{return nil,ErrInvalid};allowed:=map[string]bool{}
	for _,k:=range required{v,ok:=m[k];if !ok||bytes.Equal(bytes.TrimSpace(v),[]byte("null")){return nil,ErrInvalid};allowed[k]=true};for _,k:=range optional{allowed[k]=true}
	for k,v:=range m{if !allowed[k]||bytes.Equal(bytes.TrimSpace(v),[]byte("null")){return nil,ErrInvalid}};return m,nil
}
// Exact property names and required fields keep Go's case-insensitive struct
// decoder from accepting a different contract than the relay.
func wireFields(b []byte,dst any) error {
	var err error
	switch dst.(type) {
	case *Signed:_,err=fields(b,[]string{"payload","signature"},nil)
	case *Release:_,err=fields(b,[]string{"version","vault_id","scope_id","record_id","revision_id","sha256","runtime","capabilities"},nil)
	case *Envelope:_,err=fields(b,[]string{"version","vault_id","scope_id","device_id","key_epoch","transport_id","nonce_b64","ciphertext_b64","ciphertext_sha256"},nil)
	case *Control:
		var m map[string]json.RawMessage;m,err=fields(b,[]string{"version","vault_id","generation","scopes","devices"},nil);if err!=nil{return err}
		var scopes,devices []json.RawMessage;if json.Unmarshal(m["scopes"],&scopes)!=nil||json.Unmarshal(m["devices"],&devices)!=nil{return ErrInvalid}
		for _,v:=range scopes{if _,err=fields(v,[]string{"scope_id","key_epoch"},nil);err!=nil{return err}}
		for _,v:=range devices{if _,err=fields(v,[]string{"device_id","public_key_b64","scope_ids","revoked"},nil);err!=nil{return err}}
	case *Revision:
		var m map[string]json.RawMessage;m,err=fields(b,[]string{"schema_version","vault_id","scope_id","record_id","kind","revision_id","parents","operation_id","operation","author_device_id","created_at","provenance","tags"},[]string{"payload","restores"});if err!=nil{return err}
		if _,err=fields(m["provenance"],[]string{"source_kind","source_ref"},[]string{"harness"});err!=nil{return err}
		r:=dst.(*Revision);if r.Operation=="delete"{if _,ok:=m["payload"];ok{return ErrInvalid}}else{if _,err=fields(m["payload"],[]string{"text"},nil);err!=nil{return err}}
		_,hasRestores:=m["restores"];if hasRestores!=(r.Operation=="restore"){return ErrInvalid}
	}
	return err
}
func unbase(s string, size int) ([]byte, error) {
	b, err := base64.StdEncoding.Strict().DecodeString(s)
	if err != nil || base64.StdEncoding.EncodeToString(b) != s || (size >= 0 && len(b) != size) { return nil, ErrInvalid }
	return b, nil
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func compare(a, b string) (int, error) {
	if !decimalPattern.MatchString(a) || !decimalPattern.MatchString(b) || len(a)>128 || len(b)>128 { return 0, ErrInvalid }
	x, _ := new(big.Int).SetString(a,10); y, _ := new(big.Int).SetString(b,10)
	return x.Cmp(y), nil
}
func next(s string) string { x,_ := new(big.Int).SetString(s,10); return x.Add(x,big.NewInt(1)).String() }
func sign(prefix string, value any, key ed25519.PrivateKey) (Signed,error) {
	b,err := json.Marshal(value); if err != nil { return Signed{},err }
	p := base64.StdEncoding.EncodeToString(b)
	return Signed{p,base64.StdEncoding.EncodeToString(ed25519.Sign(key,[]byte(prefix+"\n"+p+"\n")))},nil
}
func verify(prefix string, s Signed, public ed25519.PublicKey, dst any) error {
	b,err := unbase(s.Payload,-1); if err != nil || len(b)>MaxEnvelope { return ErrInvalid }
	sig,err := unbase(s.Signature,ed25519.SignatureSize)
	if err != nil || !ed25519.Verify(public,[]byte(prefix+"\n"+s.Payload+"\n"),sig) { return ErrInvalid }
	return decode(b,dst)
}
func (c Control) grant(device, scope string) (Device,string,error) {
	var epoch string
	for _,s := range c.Scopes { if s.ID==scope { epoch=s.Epoch } }
	for _,d := range c.Devices { if d.ID==device && !d.Revoked && epoch!="" { for _,s := range d.Scopes { if s==scope { return d,epoch,nil } } } }
	return Device{},"",ErrInvalid
}
func validateControl(c Control, vault string) error {
	if c.Version!=1 || c.Vault!=vault || !idPattern.MatchString(vault) || c.Generation=="0" { return ErrInvalid }
	if _,err:=compare(c.Generation,"1");err!=nil{return err}
	if len(c.Scopes)==0 || len(c.Scopes)>64 || len(c.Devices)==0 || len(c.Devices)>256 {return ErrInvalid}
	scopes:=map[string]bool{}; devices:=map[string]bool{}
	for _,s:=range c.Scopes { if !idPattern.MatchString(s.ID)||scopes[s.ID] {return ErrInvalid}; if _,err:=compare(s.Epoch,"0");err!=nil{return err};scopes[s.ID]=true }
	for _,d:=range c.Devices { if !idPattern.MatchString(d.ID)||devices[d.ID] {return ErrInvalid};devices[d.ID]=true; if _,err:=unbase(d.Public,32);err!=nil{return err};seen:=map[string]bool{};for _,s:=range d.Scopes {if !scopes[s]||seen[s]{return ErrInvalid};seen[s]=true} }
	return nil
}
func validateRevision(r Revision) error {
	for _,s:=range []string{r.Vault,r.Scope,r.Record,r.ID,r.OperationID,r.Author} {if !idPattern.MatchString(s){return ErrInvalid}}
	if r.Version!=1 || (r.Kind!="memory"&&r.Kind!="skill") || len(r.Parents)>128 || len(r.Restores)>128 || len(r.Tags)>64 {return ErrInvalid}
	if _,err:=time.Parse(time.RFC3339Nano,r.Created);err!=nil{return ErrInvalid}
	if len(r.Created)>40 || len(r.Provenance.SourceRef)==0 || len(r.Provenance.SourceRef)>1024 || len(r.Provenance.Harness)>128 {return ErrInvalid}
	switch r.Provenance.SourceKind {case "manual","harness","git","import": default:return ErrInvalid}
	seen:=map[string]bool{};for _,p:=range r.Parents {if !idPattern.MatchString(p)||p==r.ID||seen[p]{return ErrInvalid};seen[p]=true}
	seen=map[string]bool{};for _,p:=range r.Restores {if !idPattern.MatchString(p)||seen[p]{return ErrInvalid};seen[p]=true}
	seen=map[string]bool{};for _,t:=range r.Tags {if t==""||len(t)>64||seen[t]{return ErrInvalid};seen[t]=true}
	if r.Operation=="delete" {if r.Payload!=nil{return ErrInvalid}} else if r.Payload==nil || len(r.Payload.Text)>MaxText || !utf8.ValidString(r.Payload.Text) {return ErrInvalid}
	switch r.Operation {case "put","delete":if len(r.Restores)!=0{return ErrInvalid};case "resolve":if len(r.Parents)<2||len(r.Restores)!=0{return ErrInvalid};case "restore":if len(r.Parents)==0||len(r.Restores)==0{return ErrInvalid};default:return ErrInvalid}
	return nil
}
