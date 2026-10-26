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
 "github.com/humzakhan/capsule-memory/internal/frame"
)

type relayError struct { Status int }
func (e relayError) Error() string {return "relay rejected request (HTTP "+strconv.Itoa(e.Status)+")"}
func (s *Service) request(ctx context.Context,method,path,scope string,body []byte,out any) error {
	req,err:=http.NewRequestWithContext(ctx,method,s.configuration().Relay+path,bytes.NewReader(body));if err!=nil{return errors.New("construct relay request failed")}
	if len(body)>0{req.Header.Set("Content-Type","application/json")}
	if scope!="" {
		if err=s.scope(scope);err!=nil{return err}
		nonce:=make([]byte,24);if _,err=rand.Read(nonce);err!=nil{return err};n:=hex.EncodeToString(nonce);ts:=strconv.FormatInt(time.Now().Unix(),10)
		// Legacy v1 protocol domain is fixed for compatibility; see docs/renaming.md.
		input:="agent-memory/request/v1\n"+method+"\n"+req.URL.RequestURI()+"\n"+s.configuration().Vault+"\n"+scope+"\n"+s.configuration().Device+"\n"+ts+"\n"+n+"\n"+digest(body)+"\n"
		key,err:=unbase(s.configuration().DevicePrivate,64);if err!=nil{return err}
		req.Header.Set("X-AM-Device",s.configuration().Device);req.Header.Set("X-AM-Scope",scope);req.Header.Set("X-AM-Time",ts);req.Header.Set("X-AM-Nonce",n);req.Header.Set("X-AM-Signature",base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(key),[]byte(input))))
	}
	client:=&http.Client{Timeout:30*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
	res,err:=client.Do(req);if err!=nil{return errors.New("relay transport failed; committed local writes remain queued")};defer res.Body.Close()
	if res.StatusCode<200||res.StatusCode>=300{return relayError{res.StatusCode}}
	// limit=1 bounds a page to one maximum envelope plus small journal metadata.
	b,err:=io.ReadAll(io.LimitReader(res.Body,MaxEnvelope+(64<<10)+1));if err!=nil||len(b)>MaxEnvelope+(64<<10){return errors.New("relay response exceeds limit or cannot be read")}
	if !utf8.Valid(b)||uniqueJSON(b)!=nil{return ErrInvalid}
	d:=json.NewDecoder(bytes.NewReader(b));d.DisallowUnknownFields();if d.Decode(out)!=nil||d.Decode(new(any))!=io.EOF{return errors.New("malformed relay response")};return wireFields(b,out)
}
func (s *Service) basePath() string {return "/v1/vaults/"+s.configuration().Vault}
func (s *Service) rememberControl(ctx context.Context,signed Signed) error {
 c,e:=s.control(signed);if e!=nil{return e};return s.Update(ctx,func(tx *sql.Tx)error{old,previous,e:=s.currentTx(ctx,tx);if e!=nil{return e};cmp,e:=compare(c.Generation,old.Generation);if e!=nil||cmp<0{return errors.New("owner control rollback rejected")};if cmp==0&&signed!=previous{return errors.New("owner control generation conflict")};b,e:=json.Marshal(signed);if e!=nil{return e};_,e=tx.ExecContext(ctx,"UPDATE meta SET value=? WHERE key='current_control'",b);if e!=nil{return ErrStorage};return nil})
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
		if err=s.Update(ctx,func(tx *sql.Tx)error{_,e:=tx.ExecContext(ctx,"UPDATE controls SET receipt=? WHERE generation=?",receipt.Cursor,c.Generation);return e});err!=nil{return err}
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
	s.wmu.Lock();defer s.wmu.Unlock();tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return false,ErrStorage};defer tx.Rollback();last:=cursor
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
			r,err:=s.decrypt(entry.Envelope,scope,historical);if err!=nil{return false,err};if err=insertRevision(ctx,tx,r);err!=nil{return false,err};if err=s.apply(ctx,tx,r,ApplyReplay);err!=nil{return false,err}
			// Replay can prove publication after an ambiguous POST response.
			if _,err=tx.ExecContext(ctx,"UPDATE outbox SET receipt=? WHERE revision=? AND receipt IS NULL",entry.Cursor,r.ID);err!=nil{return false,ErrStorage}
		default:return false,ErrInvalid
		}
	}
	if _,err=tx.ExecContext(ctx,"INSERT INTO replay VALUES(?,?,?,?) ON CONFLICT(scope) DO UPDATE SET cursor=excluded.cursor,head=excluded.head,control=excluded.control",scope,page.Cursor,page.Head,controlBytes);err!=nil{return false,ErrStorage}
	if s.schemaVersion==2{if _,err=tx.ExecContext(ctx,"DELETE FROM replay_block WHERE scope=?",scope);err!=nil{return false,ErrStorage}};if err=tx.Commit();err!=nil{return false,ErrStorage};return page.HasMore,nil
}
func (s *Service) replayAll(ctx context.Context,scope string,c Control,signed Signed) (bool,error) {
	for i:=0;i<100;i++ {more,err:=s.replayPage(ctx,scope,c,signed);if err!=nil{return false,err};if !more{return true,nil}};return false,nil
}
type SyncResult struct {Published int `json:"published"`;CaughtUp bool `json:"caught_up"`;Coverage Coverage `json:"coverage"`;Pending int `json:"pending"`}
func (s *Service) Sync(ctx context.Context,scope string) (SyncResult,error) {return s.SyncScope(ctx,scope,SyncOptions{})}
func (s *Service) SyncScope(ctx context.Context,scope string,opt SyncOptions) (out SyncResult,resultErr error) {
 if err:=s.scope(scope);err!=nil{return out,err};if opt.PublishOnly&&opt.ReplayOnly{return out,ErrInvalid}
 s.syncGuard.Lock();sem:=s.syncMu[scope];if sem==nil{sem=make(chan struct{},1);s.syncMu[scope]=sem};s.syncGuard.Unlock();select{case sem<-struct{}{}:defer func(){<-sem}();case <-ctx.Done():return out,ErrSyncBusy}
 defer func(){if resultErr!=nil&&s.schemaVersion==2&&(errors.Is(resultErr,ErrUnsupportedRecord)||errors.Is(resultErr,frame.ErrUnsupported)||errors.Is(resultErr,ErrStaleOverflow)){code:="unsupported_record";if errors.Is(resultErr,ErrStaleOverflow){code="stale_overflow"};_ctx,_cancel:=context.WithTimeout(context.WithoutCancel(ctx),5*time.Second);defer _cancel();_ = s.Update(_ctx,func(tx *sql.Tx)error{_,e:=tx.ExecContext(_ctx,"INSERT INTO replay_block VALUES(?,?,?) ON CONFLICT(scope) DO UPDATE SET reason=excluded.reason,at=excluded.at",scope,code,time.Now().UTC().Format(time.RFC3339Nano));return e})}}()
 if opt.MaxPages<=0{opt.MaxPages=100};if opt.MaxPages>10000{opt.MaxPages=10000};if opt.MaxPublish<=0{opt.MaxPublish=100};if opt.MaxPublish>1000{opt.MaxPublish=1000}
	if err:=s.scope(scope);err!=nil{return out,err};ctx,cancel:=context.WithTimeout(ctx,2*time.Minute);defer cancel()
	if !opt.ReplayOnly {if err:=s.publishControl(ctx);err!=nil{return out,err}}
	var signed Signed;if err:=s.request(ctx,"GET",s.basePath()+"/manifest","",nil,&signed);err!=nil{return out,err}
	if err:=s.rememberControl(ctx,signed);err!=nil{return out,err};c,err:=s.control(signed);if err!=nil{return out,err}
	d,epoch,err:=c.grant(s.configuration().Device,scope);if err!=nil{return out,errors.New("device revoked or scope not granted")};key,err:=unbase(s.configuration().DevicePrivate,64);if err!=nil{return out,err}
	if base64.StdEncoding.EncodeToString(ed25519.PrivateKey(key).Public().(ed25519.PublicKey))!=d.Public||s.configuration().Keys[scope].Epoch!=epoch{return out,errors.New("identity or epoch unavailable; manual owner key distribution required")}
	caught,err:=s.replayLimited(ctx,scope,c,signed,opt.MaxPages);if err!=nil{return out,err}
	out.CaughtUp=caught;if caught&&!opt.ReplayOnly {
		query:="SELECT revision,envelope FROM outbox WHERE scope=? AND receipt IS NULL";if s.schemaVersion==2{query+=" AND NOT EXISTS(SELECT 1 FROM outbox_hold h WHERE h.revision=outbox.revision)"};query+=" ORDER BY seq LIMIT ?";rows,err:=s.reader.QueryContext(ctx,query,scope,opt.MaxPublish);if err!=nil{return out,ErrStorage}
		type queued struct{id string;body []byte};queue:=[]queued{};for rows.Next(){var q queued;if rows.Scan(&q.id,&q.body)!=nil{rows.Close();return out,ErrStorage};queue=append(queue,q)};err=rows.Err();rows.Close();if err!=nil{return out,ErrStorage}
		for _,q:=range queue {
			// Fail closed on stale epochs. Never regenerate exact ambiguous retry bytes.
			if _,err=s.decrypt(q.body,scope,c);err!=nil{return out,errors.New("pending envelope no longer authorized; retained locally")}
   if s.schemaVersion==2{var held int;if err=s.reader.QueryRowContext(ctx,"SELECT count(*) FROM outbox_hold WHERE revision=?",q.id).Scan(&held);err!=nil{return out,ErrStorage};if held>0{continue}}
			var receipt struct{Cursor string `json:"cursor"`;Duplicate bool `json:"duplicate"`}
			if err=s.request(ctx,"POST",s.basePath()+"/records",scope,q.body,&receipt);err!=nil{return out,err};cmp,err:=compare(receipt.Cursor,"0");if err!=nil||cmp<=0{return out,ErrInvalid}
			if err=s.Update(ctx,func(tx *sql.Tx)error{_,e:=tx.ExecContext(ctx,"UPDATE outbox SET receipt=? WHERE revision=?",receipt.Cursor,q.id);return e});err!=nil{return out,err};out.Published++
		}
		if !opt.PublishOnly{out.CaughtUp,err=s.replayLimited(ctx,scope,c,signed,opt.MaxPages);if err!=nil{return out,err}}else if out.Published>0{out.CaughtUp=false}
	}
	out.Coverage,err=s.coverage(ctx,scope);if err!=nil{return out,err};if err=s.db.QueryRowContext(ctx,"SELECT count(*) FROM outbox WHERE scope=? AND receipt IS NULL",scope).Scan(&out.Pending);err!=nil{return out,ErrStorage};return out,nil
}
type SyncOptions struct{MaxPublish,MaxPages int;PublishOnly,ReplayOnly bool}
func (s *Service) replayLimited(ctx context.Context,scope string,c Control,signed Signed,max int)(bool,error){for i:=0;i<max;i++{more,e:=s.replayPage(ctx,scope,c,signed);if e!=nil{return false,e};if !more{return true,nil}};return false,nil}
type ScopeStatus struct{Coverage Coverage;Pending int;PendingBytes int64;Blocked string;LastReceipt string}
func (s *Service) ScopeStatus(ctx context.Context,scope string)(ScopeStatus,error){
 var out ScopeStatus;e:=s.Read(ctx,func(q Queryer)error{var err error;out,err=s.ScopeStatusQuery(ctx,q,scope);return err});return out,e
}
func (s *Service) ScopeStatusQuery(ctx context.Context,q Queryer,scope string)(ScopeStatus,error){
 var out ScopeStatus;if e:=s.scope(scope);e!=nil{return out,e};e:=func()error{
  out.Coverage=Coverage{Cursor:"0",Head:"0",Basis:"last successful relay scan; offline freshness unknown"};e:=q.QueryRowContext(ctx,"SELECT cursor,head FROM replay WHERE scope=?",scope).Scan(&out.Coverage.Cursor,&out.Coverage.Head);if e!=nil&&e!=sql.ErrNoRows{return ErrStorage};out.Coverage.Complete=e==nil&&out.Coverage.Cursor==out.Coverage.Head
  if e=q.QueryRowContext(ctx,"SELECT count(*),COALESCE(sum(length(envelope)),0) FROM outbox WHERE scope=? AND receipt IS NULL",scope).Scan(&out.Pending,&out.PendingBytes);e!=nil{return ErrStorage}
  if e=q.QueryRowContext(ctx,"SELECT COALESCE((SELECT receipt FROM outbox WHERE scope=? AND receipt IS NOT NULL ORDER BY length(receipt) DESC,receipt DESC LIMIT 1),'')",scope).Scan(&out.LastReceipt);e!=nil{return ErrStorage};
  if s.schemaVersion==2{e=q.QueryRowContext(ctx,"SELECT reason FROM replay_block WHERE scope=?",scope).Scan(&out.Blocked);if e!=nil&&e!=sql.ErrNoRows{return ErrStorage}};return nil
 }();return out,e
}
