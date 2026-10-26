package memory

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
 "github.com/humzakhan/capsule-memory/internal/frame"
 "github.com/humzakhan/capsule-memory/internal/strictjson"
)
// Legacy v1 protocol domain is fixed for compatibility; see docs/renaming.md.
func aad(e Envelope) []byte {return []byte("agent-memory/aead/v1\n"+e.Vault+"\n"+e.Scope+"\n"+e.Device+"\n"+e.Epoch+"\n"+e.Transport+"\n")}
func (s *Service) encrypt(r Revision) ([]byte,error) {plain,err:=json.Marshal(r);if err!=nil{return nil,err};return s.encryptPlain(r,plain)}
func (s *Service) encryptPlain(r Revision,plain []byte) ([]byte,error) {
	k,ok:=s.configuration().Keys[r.Scope];if !ok{return nil,ErrInvalid}
	key,err:=unbase(k.Key,32);if err!=nil{return nil,err};block,err:=aes.NewCipher(key);if err!=nil{return nil,err};gcm,err:=cipher.NewGCM(block);if err!=nil{return nil,err}
	transport,err:=newID();if err!=nil{return nil,err};nonce:=make([]byte,12);if _,err=rand.Read(nonce);err!=nil{return nil,err}
	e:=Envelope{Version:1,Vault:r.Vault,Scope:r.Scope,Device:r.Author,Epoch:k.Epoch,Transport:transport,Nonce:base64.StdEncoding.EncodeToString(nonce)}
	if r.Version==2 {plain,err=frame.Encode(frame.ContentRevisionJSON,plain);if err!=nil{return nil,err}};ct:=gcm.Seal(nil,nonce,plain,aad(e));e.Ciphertext=base64.StdEncoding.EncodeToString(ct);e.Digest=digest(ct)
	private,err:=unbase(s.configuration().DevicePrivate,64);if err!=nil{return nil,err};signed,err:=sign("agent-memory/envelope/v1",e,ed25519.PrivateKey(private));if err!=nil{return nil,err}
	b,err:=json.Marshal(signed);if err!=nil{return nil,err};if len(b)>MaxEnvelope{return nil,ErrInvalid};return b,nil
}
func (s *Service) decrypt(raw []byte, scope string, control Control) (Revision,error) {
	var r Revision;var signed Signed;var e Envelope
	if err:=decode(raw,&signed);err!=nil{return r,err}
	payload,err:=unbase(signed.Payload,-1);if err!=nil{return r,err};if err=decode(payload,&e);err!=nil{return r,err}
	if e.Version!=1||e.Vault!=s.configuration().Vault||e.Scope!=scope||!idPattern.MatchString(e.Transport){return r,ErrInvalid}
	d,epoch,err:=control.grant(e.Device,scope);if err!=nil||epoch!=e.Epoch{return r,ErrInvalid}
	pub,err:=unbase(d.Public,32);if err!=nil{return r,err};if err=verify("agent-memory/envelope/v1",signed,ed25519.PublicKey(pub),&e);err!=nil{return r,err}
	k,ok:=s.configuration().Keys[scope];if !ok||k.Epoch!=e.Epoch{return r,ErrInvalid}
	ct,err:=unbase(e.Ciphertext,-1);if err!=nil||digest(ct)!=e.Digest{return r,ErrInvalid};nonce,err:=unbase(e.Nonce,12);if err!=nil{return r,err};key,err:=unbase(k.Key,32);if err!=nil{return r,err}
	block,err:=aes.NewCipher(key);if err!=nil{return r,err};gcm,err:=cipher.NewGCM(block);if err!=nil{return r,err};plain,err:=gcm.Open(nil,nonce,ct,aad(e));if err!=nil{return r,ErrInvalid}
	if frame.IsFramed(plain) {_,plain,err=frame.Decode(plain);if err!=nil{return r,err};if err=strictjson.Decode(plain,frame.MaxDecoded,&r);err!=nil{return r,err};if r.Version!=2{return r,ErrInvalid};if err=wireFields(plain,&r);err!=nil{return r,err}}else{if err=decode(plain,&r);err!=nil{return r,err};if r.Version!=1{return r,ErrUnsupportedRecord}};if r.Vault!=e.Vault||r.Scope!=e.Scope||r.Author!=e.Device{return r,ErrInvalid};if err=validateRevision(r);err!=nil{return r,err};return r,s.verifyOwnerInsight(r)
}
