package memory

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"
)

type relayError struct { Status int }
func (e relayError) Error() string {return "relay rejected request (HTTP "+strconv.Itoa(e.Status)+")"}
func (s *Service) request(ctx context.Context,method,path,scope string,body []byte,out any) error {
	req,err:=http.NewRequestWithContext(ctx,method,s.config.Relay+path,bytes.NewReader(body));if err!=nil{return errors.New("construct relay request failed")}
	if len(body)>0{req.Header.Set("Content-Type","application/json")}
	if scope!="" {
		if err=s.scope(scope);err!=nil{return err}
		nonce:=make([]byte,24);if _,err=rand.Read(nonce);err!=nil{return err};n:=hex.EncodeToString(nonce);ts:=strconv.FormatInt(time.Now().Unix(),10)
		// Legacy v1 protocol domain is fixed for compatibility; see docs/renaming.md.
		input:="agent-memory/request/v1\n"+method+"\n"+req.URL.RequestURI()+"\n"+s.config.Vault+"\n"+scope+"\n"+s.config.Device+"\n"+ts+"\n"+n+"\n"+digest(body)+"\n"
		key,err:=unbase(s.config.DevicePrivate,64);if err!=nil{return err}
		req.Header.Set("X-AM-Device",s.config.Device);req.Header.Set("X-AM-Scope",scope);req.Header.Set("X-AM-Time",ts);req.Header.Set("X-AM-Nonce",n);req.Header.Set("X-AM-Signature",base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(key),[]byte(input))))
	}
	client:=&http.Client{Timeout:30*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
	res,err:=client.Do(req);if err!=nil{return errors.New("relay transport failed; committed local writes remain queued")};defer res.Body.Close()
	if res.StatusCode<200||res.StatusCode>=300{return relayError{res.StatusCode}}
	// limit=1 bounds a page to one maximum envelope plus small journal metadata.
	b,err:=io.ReadAll(io.LimitReader(res.Body,MaxEnvelope+(64<<10)+1));if err!=nil||len(b)>MaxEnvelope+(64<<10){return errors.New("relay response exceeds limit or cannot be read")}
	if !utf8.Valid(b)||uniqueJSON(b)!=nil{return ErrInvalid}
	d:=json.NewDecoder(bytes.NewReader(b));d.DisallowUnknownFields();if d.Decode(out)!=nil||d.Decode(new(any))!=io.EOF{return errors.New("malformed relay response")};return wireFields(b,out)
}
func (s *Service) basePath() string {return "/v1/vaults/"+s.config.Vault}
func (s *Service) rememberControl(ctx context.Context,signed Signed) error {
	c,err:=s.control(signed);if err!=nil{return err}
	old,oldSigned,err:=s.current(ctx)
	if err==nil {cmp,e:=compare(c.Generation,old.Generation);if e!=nil||cmp<0{return errors.New("owner control rollback rejected")};if cmp==0&&(signed!=oldSigned){return errors.New("owner control generation conflict")}} else if err!=sql.ErrNoRows{return err}
	return s.putMeta(ctx,"current_control",signed)
}
func (s *Service) publishControl(ctx context.Context) error {
	rows,err:=s.db.QueryContext(ctx,"SELECT signed FROM controls WHERE receipt IS NULL ORDER BY length(generation),generation LIMIT 256");if err!=nil{return ErrStorage}
	queue:=[][]byte{};for rows.Next(){var b []byte;if rows.Scan(&b)!=nil{rows.Close();return ErrStorage};queue=append(queue,b)};err=rows.Err();rows.Close();if err!=nil{return ErrStorage};if len(queue)==0{return nil}
	if _,err=s.ownerKey();err!=nil{return err}
	for _,b:=range queue {
		var signed Signed;if err=decode(b,&signed);err!=nil{return err};c,err:=s.control(signed);if err!=nil{return err}
		var receipt struct{Cursor string `json:"cursor"`;Generation string `json:"generation"`}
		if err=s.request(ctx,"PUT",s.basePath()+"/manifest","",b,&receipt);err!=nil{return err}
		cmp,err:=compare(receipt.Cursor,"0");if err!=nil||cmp<=0||receipt.Generation!=c.Generation{return ErrInvalid}
		if err=s.rememberControl(ctx,signed);err!=nil{return err}
		if _,err=s.db.ExecContext(ctx,"UPDATE controls SET receipt=? WHERE generation=?",receipt.Cursor,c.Generation);err!=nil{return ErrStorage}
	}
	return nil
}

type journalEntry struct {
	Cursor string `json:"cursor"`
	Type string `json:"type"`
	Envelope json.RawMessage `json:"envelope,omitempty"`
	Manifest *Signed `json:"manifest,omitempty"`
}
type changePage struct {
	Cursor string `json:"cursor"`
	Head string `json:"head"`
	Entries []journalEntry `json:"entries"`
	HasMore bool `json:"has_more"`
}
func (s *Service) replayPage(ctx context.Context,scope string,current Control,currentSigned Signed) (bool,error) {
	cursor:="0";head:="0";var controlBytes []byte
	err:=s.db.QueryRowContext(ctx,"SELECT cursor,head,control FROM replay WHERE scope=?",scope).Scan(&cursor,&head,&controlBytes)
	if err!=nil&&err!=sql.ErrNoRows{return false,ErrStorage}
	var previous Signed;var historical Control
	if len(controlBytes)>0 {if err=decode(controlBytes,&previous);err!=nil{return false,err};historical,err=s.control(previous);if err!=nil{return false,err}}
	var page changePage
	if err=s.request(ctx,"GET",s.basePath()+"/changes?after="+cursor+"&limit=1",scope,nil,&page);err!=nil{return false,err}
	cmp,err:=compare(page.Cursor,cursor);if err!=nil||cmp<0{return false,errors.New("relay cursor rollback rejected")};if page.HasMore&&cmp==0{return false,errors.New("relay replay made no progress")}
	cmp,err=compare(page.Head,head);if err!=nil||cmp<0{return false,errors.New("relay head rollback rejected")};cmp,err=compare(page.Cursor,page.Head);if err!=nil||cmp>0||page.HasMore!=(cmp<0)||len(page.Entries)>1{return false,ErrInvalid}
	tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return false,ErrStorage};defer tx.Rollback();last:=cursor
	for _,entry:=range page.Entries {
		cmp,err=compare(entry.Cursor,last);if err!=nil||cmp<=0{return false,ErrInvalid};cmp,err=compare(entry.Cursor,page.Cursor);if err!=nil||cmp>0{return false,ErrInvalid};last=entry.Cursor
		switch entry.Type {
		case "control":
			if entry.Manifest==nil||len(entry.Envelope)>0{return false,ErrInvalid};c,err:=s.control(*entry.Manifest);if err!=nil{return false,err}
			expected:="1";if historical.Generation!=""{expected=next(historical.Generation)};if c.Generation!=expected{return false,errors.New("control history is incomplete or reordered")}
			cmp,err=compare(c.Generation,current.Generation);if err!=nil||cmp>0{return false,errors.New("refresh owner control before replaying newer history")};if cmp==0&&*entry.Manifest!=currentSigned{return false,errors.New("control history differs from pinned current manifest")}
			historical=c;previous=*entry.Manifest;controlBytes,err=json.Marshal(previous);if err!=nil{return false,err}
			var old []byte
			err=tx.QueryRowContext(ctx,"SELECT signed FROM controls WHERE generation=?",c.Generation).Scan(&old)
			if err==nil {var existing Signed;if decode(old,&existing)!=nil||existing!=previous{return false,ErrInvalid};if _,err=tx.ExecContext(ctx,"UPDATE controls SET receipt=? WHERE generation=?",entry.Cursor,c.Generation);err!=nil{return false,ErrStorage}} else if err==sql.ErrNoRows {if _,err=tx.ExecContext(ctx,"INSERT INTO controls VALUES(?,?,?)",c.Generation,controlBytes,entry.Cursor);err!=nil{return false,ErrStorage}} else{return false,ErrStorage}
		case "record":
			if entry.Manifest!=nil||len(entry.Envelope)==0||historical.Generation==""{return false,ErrInvalid}
			r,err:=s.decrypt(entry.Envelope,scope,historical);if err!=nil{return false,err};if err=insertRevision(ctx,tx,r);err!=nil{return false,err}
			// Replay can prove publication after an ambiguous POST response.
			if _,err=tx.ExecContext(ctx,"UPDATE outbox SET receipt=? WHERE revision=? AND receipt IS NULL",entry.Cursor,r.ID);err!=nil{return false,ErrStorage}
		default:return false,ErrInvalid
		}
	}
	if _,err=tx.ExecContext(ctx,"INSERT INTO replay VALUES(?,?,?,?) ON CONFLICT(scope) DO UPDATE SET cursor=excluded.cursor,head=excluded.head,control=excluded.control",scope,page.Cursor,page.Head,controlBytes);err!=nil{return false,ErrStorage}
	if err=tx.Commit();err!=nil{return false,ErrStorage};return page.HasMore,nil
}
func (s *Service) replayAll(ctx context.Context,scope string,c Control,signed Signed) (bool,error) {
	for i:=0;i<100;i++ {more,err:=s.replayPage(ctx,scope,c,signed);if err!=nil{return false,err};if !more{return true,nil}};return false,nil
}
type SyncResult struct {Published int `json:"published"`;CaughtUp bool `json:"caught_up"`;Coverage Coverage `json:"coverage"`;Pending int `json:"pending"`}
func (s *Service) Sync(ctx context.Context,scope string) (SyncResult,error) {
	s.mu.Lock();defer s.mu.Unlock();var out SyncResult
	if err:=s.scope(scope);err!=nil{return out,err};ctx,cancel:=context.WithTimeout(ctx,2*time.Minute);defer cancel()
	if err:=s.publishControl(ctx);err!=nil{return out,err}
	var signed Signed;if err:=s.request(ctx,"GET",s.basePath()+"/manifest","",nil,&signed);err!=nil{return out,err}
	if err:=s.rememberControl(ctx,signed);err!=nil{return out,err};c,err:=s.control(signed);if err!=nil{return out,err}
	d,epoch,err:=c.grant(s.config.Device,scope);if err!=nil{return out,errors.New("device revoked or scope not granted")};key,err:=unbase(s.config.DevicePrivate,64);if err!=nil{return out,err}
	if base64.StdEncoding.EncodeToString(ed25519.PrivateKey(key).Public().(ed25519.PublicKey))!=d.Public||s.config.Keys[scope].Epoch!=epoch{return out,errors.New("identity or epoch unavailable; manual owner key distribution required")}
	caught,err:=s.replayAll(ctx,scope,c,signed);if err!=nil{return out,err}
	if caught {
		rows,err:=s.db.QueryContext(ctx,"SELECT revision,envelope FROM outbox WHERE scope=? AND receipt IS NULL ORDER BY seq LIMIT 100",scope);if err!=nil{return out,ErrStorage}
		type queued struct{id string;body []byte};queue:=[]queued{};for rows.Next(){var q queued;if rows.Scan(&q.id,&q.body)!=nil{rows.Close();return out,ErrStorage};queue=append(queue,q)};err=rows.Err();rows.Close();if err!=nil{return out,ErrStorage}
		for _,q:=range queue {
			// Fail closed on stale epochs. Never regenerate exact ambiguous retry bytes.
			if _,err=s.decrypt(q.body,scope,c);err!=nil{return out,errors.New("pending envelope no longer authorized; retained locally")}
			var receipt struct{Cursor string `json:"cursor"`;Duplicate bool `json:"duplicate"`}
			if err=s.request(ctx,"POST",s.basePath()+"/records",scope,q.body,&receipt);err!=nil{return out,err};cmp,err:=compare(receipt.Cursor,"0");if err!=nil||cmp<=0{return out,ErrInvalid}
			if _,err=s.db.ExecContext(ctx,"UPDATE outbox SET receipt=? WHERE revision=?",receipt.Cursor,q.id);err!=nil{return out,ErrStorage};out.Published++
		}
		out.CaughtUp,err=s.replayAll(ctx,scope,c,signed);if err!=nil{return out,err}
	}
	out.Coverage,err=s.coverage(ctx,scope);if err!=nil{return out,err};if err=s.db.QueryRowContext(ctx,"SELECT count(*) FROM outbox WHERE scope=? AND receipt IS NULL",scope).Scan(&out.Pending);err!=nil{return out,ErrStorage};return out,nil
}
