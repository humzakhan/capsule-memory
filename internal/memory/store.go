package memory

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

 "github.com/humzakhan/capsule-memory/internal/records"
	_ "modernc.org/sqlite"
)

type Service struct { db *sql.DB; reader *sql.DB; config Config; configMu sync.RWMutex; home string; wmu sync.Mutex; syncGuard sync.Mutex; syncMu map[string]chan struct{}; appliers map[string]Applier; staleHooks []StaleHook; revisionHooks []RevisionHook; grantHooks []GrantCheck; publicationCheck func(context.Context,records.ScopedRef)error; schemaVersion int; profileLock *os.File }
const schema = `
CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY,value BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS controls(generation TEXT PRIMARY KEY,signed BLOB NOT NULL,receipt TEXT);
CREATE TABLE IF NOT EXISTS revisions(revision TEXT PRIMARY KEY,operation TEXT UNIQUE NOT NULL,scope TEXT NOT NULL,record TEXT NOT NULL,kind TEXT NOT NULL,body BLOB NOT NULL);
CREATE INDEX IF NOT EXISTS by_record ON revisions(scope,record);
CREATE TABLE IF NOT EXISTS parents(child TEXT NOT NULL REFERENCES revisions(revision),parent TEXT NOT NULL REFERENCES revisions(revision),PRIMARY KEY(child,parent));
CREATE TABLE IF NOT EXISTS restores(child TEXT NOT NULL REFERENCES revisions(revision),deletion TEXT NOT NULL REFERENCES revisions(revision),PRIMARY KEY(child,deletion));
CREATE TABLE IF NOT EXISTS outbox(seq INTEGER PRIMARY KEY AUTOINCREMENT,revision TEXT UNIQUE NOT NULL REFERENCES revisions(revision),scope TEXT NOT NULL,envelope BLOB NOT NULL,receipt TEXT);
CREATE VIRTUAL TABLE IF NOT EXISTS search USING fts5(scope UNINDEXED,record UNINDEXED,revision UNINDEXED,text);
CREATE TABLE IF NOT EXISTS replay(scope TEXT PRIMARY KEY,cursor TEXT NOT NULL,head TEXT NOT NULL,control BLOB);
CREATE TABLE IF NOT EXISTS approvals(revision TEXT PRIMARY KEY REFERENCES revisions(revision),release BLOB NOT NULL,active INTEGER NOT NULL DEFAULT 0);
PRAGMA user_version=1;
`
func Open(ctx context.Context,home string) (*Service,error) {return OpenWith(ctx,home,LockShared)}
func OpenWith(ctx context.Context,home string,mode LockMode) (*Service,error) {
	c,err:=Load(home);if err!=nil{return nil,err}
 lock,err:=lockProfile(home,mode);if err!=nil{return nil,err};ok:=false;defer func(){if !ok{unlockProfile(lock)}}()
	path:=filepath.Join(home,"memory.db")
	f,err:=os.OpenFile(path,os.O_CREATE|os.O_RDWR,0600);if err!=nil{return nil,err};f.Close();if err=safeFile(path);err!=nil{return nil,err}
	// DSN pragmas apply on every connection, including replacement connections.
	absPath,err:=filepath.Abs(path);if err!=nil{return nil,ErrStorage}
	dsn:=url.URL{Scheme:"file",Path:filepath.ToSlash(absPath)}
	pragmas:=url.Values{"_pragma":[]string{"journal_mode(WAL)","synchronous(FULL)","foreign_keys(1)","busy_timeout(5000)"}};dsn.RawQuery=pragmas.Encode()
	db,err:=sql.Open("sqlite",dsn.String());if err!=nil{return nil,ErrStorage}
	db.SetMaxOpenConns(1);db.SetMaxIdleConns(1)
	s:=&Service{db:db,config:c,home:home,profileLock:lock,syncMu:map[string]chan struct{}{},appliers:map[string]Applier{}}
	var version int
	if err=db.QueryRowContext(ctx,"PRAGMA user_version").Scan(&version);err!=nil{db.Close();return nil,ErrStorage}
	if version>2 {db.Close();return nil,errors.New("unsupported local schema version")}
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{db.Close();return nil,ErrStorage}
	if version==0 {if _,err=tx.ExecContext(ctx,schema);err==nil{_,err=tx.ExecContext(ctx,schemaV2)};version=2};if err!=nil{tx.Rollback();db.Close();return nil,ErrStorage};if err=tx.Commit();err!=nil{db.Close();return nil,ErrStorage};s.schemaVersion=version
	var existing Signed
	if err=s.getMeta(ctx,"current_control",&existing);err==sql.ErrNoRows {
		if len(c.Bootstrap)==0||len(c.Bootstrap)>256{db.Close();return nil,ErrInvalid}
		generation:="1";for _,signed:=range c.Bootstrap{control,e:=s.control(signed);if e!=nil||control.Generation!=generation{db.Close();return nil,ErrInvalid};generation=next(generation)}
		if err=s.putMeta(ctx,"current_control",c.Bootstrap[len(c.Bootstrap)-1]);err!=nil{db.Close();return nil,err}
	} else if err!=nil{db.Close();return nil,ErrStorage}
	pragmas.Add("_pragma","query_only(1)");dsn.RawQuery=pragmas.Encode();reader,err:=sql.Open("sqlite",dsn.String());if err!=nil{db.Close();return nil,ErrStorage};reader.SetMaxOpenConns(4);reader.SetMaxIdleConns(4);s.reader=reader
 if version==2 {if err=s.recoverPendingScope(ctx);err!=nil{s.Close();return nil,err}};ok=true;return s,nil
}
func (s *Service) Close() error {if s.reader!=nil{s.reader.Close()};e:=s.db.Close();l:=unlockProfile(s.profileLock);s.profileLock=nil;if e!=nil{return e};return l}
func (s *Service) putMeta(ctx context.Context,key string,v any) error {
	b,err:=json.Marshal(v);if err!=nil{return err};_,err=s.db.ExecContext(ctx,"INSERT INTO meta VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",key,b);if err!=nil{return ErrStorage};return nil
}
func (s *Service) getMeta(ctx context.Context,key string,v any) error {
	var b []byte;if err:=s.db.QueryRowContext(ctx,"SELECT value FROM meta WHERE key=?",key).Scan(&b);err!=nil{return err};return decode(b,v)
}
func (s *Service) control(signed Signed) (Control,error) {
	// Legacy v1 protocol domain is fixed for compatibility; see docs/renaming.md.
	var c Control;pub,err:=unbase(s.configuration().OwnerPublic,32);if err!=nil{return c,err};if err=verify("agent-memory/control/v1",signed,ed25519.PublicKey(pub),&c);err!=nil{return c,err};return c,validateControl(c,s.configuration().Vault)
}
func (s *Service) current(ctx context.Context) (Control,Signed,error) {
	var signed Signed;if err:=s.getMeta(ctx,"current_control",&signed);err!=nil{return Control{},signed,err};c,err:=s.control(signed);return c,signed,err
}
func (s *Service) scope(scope string) error {if _,ok:=s.configuration().Keys[scope];!ok||!idPattern.MatchString(scope){return ErrInvalid};return nil}

type Queryer interface {
	QueryContext(context.Context,string,...any)(*sql.Rows,error)
	QueryRowContext(context.Context,string,...any) *sql.Row
}
type queryer = Queryer
func recordRevisions(ctx context.Context,q queryer,scope,record string) ([]Revision,error) {
	rows,err:=q.QueryContext(ctx,"SELECT body FROM revisions WHERE scope=? AND record=? ORDER BY revision",scope,record);if err!=nil{return nil,ErrStorage};defer rows.Close()
	result:=[]Revision{};size:=0;for rows.Next(){var b []byte;var r Revision;if rows.Scan(&b)!=nil{return nil,ErrStorage};size+=len(b);if len(result)>=4096||size>32<<20{return nil,errors.New("record exceeds v1 local history working-set limit")};if decode(b,&r)!=nil{return nil,ErrStorage};result=append(result,r)};if rows.Err()!=nil{return nil,ErrStorage};return result,nil
}
func state(revs []Revision) (heads []Revision,deleted bool) {
	parents:=map[string]bool{};restored:=map[string]bool{}
	for _,r:=range revs {for _,p:=range r.Parents{parents[p]=true};for _,d:=range r.Restores{restored[d]=true}}
	heads=[]Revision{};for _,r:=range revs {if !parents[r.ID]{heads=append(heads,r)};if r.Operation=="delete"&&!restored[r.ID]{deleted=true}};return
}
func insertRevision(ctx context.Context,tx *sql.Tx,r Revision) error {
	if err:=validateRevision(r);err!=nil{return err};body,err:=json.Marshal(r);if err!=nil{return err}
 var count,total int;if err=tx.QueryRowContext(ctx,"SELECT count(*),COALESCE(sum(length(body)),0) FROM revisions WHERE scope=? AND record=?",r.Scope,r.Record).Scan(&count,&total);err!=nil{return ErrStorage}
	var previous []byte
	err=tx.QueryRowContext(ctx,"SELECT body FROM revisions WHERE revision=? OR operation=?",r.ID,r.OperationID).Scan(&previous)
	if err==nil {if string(previous)!=string(body){return errors.New("revision or operation identity conflict")};return nil};if err!=sql.ErrNoRows{return ErrStorage}
 if count>=4096||total+len(body)>32<<20{return errors.New("record exceeds local history working-set limit")}
	var cross int;if err=tx.QueryRowContext(ctx,"SELECT count(*) FROM revisions WHERE record=? AND scope!=? AND (?=2 OR json_extract(body,'$.schema_version')=2)",r.Record,r.Scope,r.Version).Scan(&cross);err!=nil{return ErrStorage};if cross>0{return ErrInvalid};revs,err:=recordRevisions(ctx,tx,r.Scope,r.Record);if err!=nil{return err};known:=map[string]Revision{};for _,old:=range revs{known[old.ID]=old;if old.Kind!=r.Kind{return ErrInvalid}}
	for _,p:=range r.Parents {if _,ok:=known[p];!ok{return errors.New("missing or cross-record parent; replay stopped")}}
	// Independent first writes to the same record may be concurrent roots.
	if len(r.Parents)==0&&r.Operation!="put"{return ErrInvalid}
	if r.Operation=="delete"&&len(revs)==0{return ErrInvalid}
	if r.Operation=="restore" {ancestors:=map[string]bool{};stack:=append([]string{},r.Parents...);for len(stack)>0{id:=stack[len(stack)-1];stack=stack[:len(stack)-1];if ancestors[id]{continue};ancestors[id]=true;stack=append(stack,known[id].Parents...)};for _,d:=range r.Restores{old,ok:=known[d];if !ok||old.Operation!="delete"||!ancestors[d]{return ErrInvalid}}}
	if _,err=tx.ExecContext(ctx,"INSERT INTO revisions VALUES(?,?,?,?,?,?)",r.ID,r.OperationID,r.Scope,r.Record,r.Kind,body);err!=nil{return ErrStorage}
	for _,p:=range r.Parents{if _,err=tx.ExecContext(ctx,"INSERT INTO parents VALUES(?,?)",r.ID,p);err!=nil{return ErrStorage}}
	for _,d:=range r.Restores{if _,err=tx.ExecContext(ctx,"INSERT INTO restores VALUES(?,?)",r.ID,d);err!=nil{return ErrStorage}}
	return indexRecord(ctx,tx,r.Scope,r.Record)
}
func indexRecord(ctx context.Context,tx *sql.Tx,scope,record string) error {
	if _,err:=tx.ExecContext(ctx,"DELETE FROM search WHERE scope=? AND record=?",scope,record);err!=nil{return ErrStorage}
	revs,err:=recordRevisions(ctx,tx,scope,record);if err!=nil{return err};heads,deleted:=state(revs);if deleted{return nil}
	for _,r:=range heads {if r.Payload!=nil {if _,err=tx.ExecContext(ctx,"INSERT INTO search VALUES(?,?,?,?)",scope,record,r.ID,r.Payload.Text);err!=nil{return ErrStorage}}};return nil
}

type WriteInput struct {Scope string;Kind string;Text string;Record string;Parents []string;Operation string;Source string;TextSet bool}
func (s *Service) Write(ctx context.Context,in WriteInput) (Revision,error) {
	s.wmu.Lock();defer s.wmu.Unlock();var r Revision
	if err:=s.scope(in.Scope);err!=nil{return r,err};if in.Operation==""{in.Operation="put"};if in.Kind==""{in.Kind="memory"};if in.Source==""{in.Source="manual"}
	if in.Kind!="memory"&&in.Kind!="skill"{return r,ErrInvalid}
	control,_,err:=s.current(ctx);if err!=nil{return r,err};_,epoch,err:=control.grant(s.configuration().Device,in.Scope);if err!=nil||s.configuration().Keys[in.Scope].Epoch!=epoch{return r,errors.New("device unauthorized or scope epoch unavailable")}
	if in.Record==""{in.Record,err=newID();if err!=nil{return r,err}};if !idPattern.MatchString(in.Record){return r,ErrInvalid}
	tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return r,ErrStorage};defer tx.Rollback()
	revs,err:=recordRevisions(ctx,tx,in.Scope,in.Record);if err!=nil{return r,err};heads,_:=state(revs)
	if len(revs)>0 {in.Kind=revs[0].Kind;if in.Kind!="memory"&&in.Kind!="skill"{return r,ErrInvalid}};if in.Parents==nil {in.Parents=[]string{};for _,h:=range heads{in.Parents=append(in.Parents,h.ID)}}
	if in.Operation=="put"&&len(in.Parents)>1{in.Operation="resolve"}
	id,err:=newID();if err!=nil{return r,err};op,err:=newID();if err!=nil{return r,err}
	r=Revision{Version:1,Vault:s.configuration().Vault,Scope:in.Scope,Record:in.Record,Kind:in.Kind,ID:id,Parents:in.Parents,OperationID:op,Operation:in.Operation,Author:s.configuration().Device,Created:time.Now().UTC().Format(time.RFC3339Nano),Provenance:Provenance{SourceKind:in.Source,SourceRef:"local"},Tags:[]string{},Payload:&Payload{in.Text}}
	if in.Operation=="delete" {r.Payload=nil}
	if in.Operation=="restore" {
		restored:=map[string]bool{};known:=map[string]Revision{};for _,old:=range revs{known[old.ID]=old;for _,d:=range old.Restores{restored[d]=true}}
		for _,old:=range revs{if old.Operation=="delete"&&!restored[old.ID]{r.Restores=append(r.Restores,old.ID)}}
		if len(r.Restores)==0{return r,errors.New("record has no unresolved deletion")}
		if !in.TextSet {texts:=map[string]bool{};visited:=map[string]bool{};stack:=append([]string{},r.Parents...);for len(stack)>0{id:=stack[len(stack)-1];stack=stack[:len(stack)-1];if visited[id]{continue};visited[id]=true;old,ok:=known[id];if !ok{return r,ErrInvalid};if old.Payload!=nil{texts[old.Payload.Text]=true}else{stack=append(stack,old.Parents...)}};if len(texts)!=1{return r,errors.New("restore has ambiguous content; provide --text with the reviewed replacement")};for text:=range texts{r.Payload=&Payload{text}}}
	}
	if err=validateRevision(r);err!=nil{return r,err};envelope,err:=s.encrypt(r);if err!=nil{return r,err}
	if err=insertRevision(ctx,tx,r);err!=nil{return r,err};if err=s.apply(ctx,tx,r,ApplyLocal);err!=nil{return r,err};if _,err=tx.ExecContext(ctx,"INSERT INTO outbox(revision,scope,envelope) VALUES(?,?,?)",r.ID,r.Scope,envelope);err!=nil{return r,ErrStorage}
	if err=tx.Commit();err!=nil{return r,ErrStorage};return r,nil
}

type RecordView struct {Record string `json:"record_id"`;Heads []Revision `json:"heads"`;Deleted bool `json:"deleted"`;Conflict bool `json:"conflict"`;History []Revision `json:"history,omitempty"`;*SkillState}
func (s *Service) Get(ctx context.Context,scope,record string,history bool) (RecordView,error) {
	if err:=s.scope(scope);err!=nil{return RecordView{},err}
	tx,err:=s.reader.BeginTx(ctx,&sql.TxOptions{ReadOnly:true});if err!=nil{return RecordView{},ErrStorage};defer tx.Rollback()
	v,err:=s.recordView(ctx,tx,scope,record,history);if err!=nil{return RecordView{},err}
	if err=tx.Commit();err!=nil{return RecordView{},ErrStorage};return v,nil
}
type Coverage struct {Cursor string `json:"cursor"`;Head string `json:"head"`;Complete bool `json:"complete"`;Basis string `json:"basis"`}
func (s *Service) coverage(ctx context.Context,scope string) (Coverage,error) {
	c:=Coverage{Cursor:"0",Head:"0",Basis:"last successful relay scan; offline freshness unknown"};err:=s.reader.QueryRowContext(ctx,"SELECT cursor,head FROM replay WHERE scope=?",scope).Scan(&c.Cursor,&c.Head);if err!=nil&&err!=sql.ErrNoRows{return c,ErrStorage};c.Complete=err==nil&&c.Cursor==c.Head;return c,nil
}
type SearchHit struct {Revision;Conflict bool `json:"conflict"`;Staged bool `json:"staged"`}
type RecallResult struct {Results []SearchHit `json:"results"`;Coverage Coverage `json:"coverage"`}
func (s *Service) Recall(ctx context.Context,scope,query string) (RecallResult,error) {
 out:=RecallResult{Results:[]SearchHit{}};if err:=s.scope(scope);err!=nil{return out,err};if len(query)>4096||query==""{return out,ErrInvalid}
 err:=s.Read(ctx,func(q Queryer)error{
  rows,err:=q.QueryContext(ctx,"SELECT r.body FROM search JOIN revisions r ON r.revision=search.revision WHERE search MATCH ? AND search.scope=? ORDER BY bm25(search) LIMIT 100",query,scope);if err!=nil{return errors.New("invalid search query or unavailable index")}
  found:=[]Revision{};for rows.Next(){var b []byte;var r Revision;if rows.Scan(&b)!=nil||decode(b,&r)!=nil{rows.Close();return ErrStorage};found=append(found,r)};err=rows.Err();rows.Close();if err!=nil{return ErrStorage}
  for _,r:=range found{if r.Kind!="memory"&&r.Kind!="skill"{continue};v,err:=s.recordView(ctx,q,scope,r.Record,false);if err!=nil{return err};if !v.Deleted{active:=v.SkillState!=nil&&v.Active&&len(v.Heads)==1&&v.Heads[0].ID==r.ID;out.Results=append(out.Results,SearchHit{r,v.Conflict,r.Kind=="skill"&&!active})}}
  out.Coverage,err=coverageQuery(ctx,q,scope);return err
 });return out,err
}
func (s *Service) Status(ctx context.Context) (map[string]any,error) {
	var pending int;if err:=s.reader.QueryRowContext(ctx,"SELECT count(*) FROM outbox WHERE receipt IS NULL").Scan(&pending);err!=nil{return nil,ErrStorage};coverage:=map[string]Coverage{};for scope:=range s.configuration().Keys{c,err:=s.coverage(ctx,scope);if err!=nil{return nil,err};coverage[scope]=c};var controlCount int;if s.reader.QueryRowContext(ctx,"SELECT count(*) FROM controls WHERE receipt IS NULL").Scan(&controlCount)!=nil{return nil,ErrStorage};controlPending:=controlCount>0
	return map[string]any{"device_id":s.configuration().Device,"vault_id":s.configuration().Vault,"pending":pending,"control_pending":controlPending,"coverage":coverage},nil
}
func (s *Service) Rebuild(ctx context.Context) error {
	s.wmu.Lock();defer s.wmu.Unlock();tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return ErrStorage};defer tx.Rollback()
	if _,err=tx.ExecContext(ctx,"DELETE FROM search");err!=nil{return ErrStorage}
	// SQLite streams the durable rows; the Go process does not materialize the
	// entire archive to rebuild derived state. This is the same head/tombstone
	// rule as state(): any unrestored deletion suppresses every head.
	if _,err=tx.ExecContext(ctx,`INSERT INTO search(scope,record,revision,text)
		SELECT r.scope,r.record,r.revision,json_extract(r.body,'$.payload.text')
		FROM revisions r
		WHERE json_type(r.body,'$.payload')='object'
		AND NOT EXISTS(SELECT 1 FROM parents p WHERE p.parent=r.revision)
		AND NOT EXISTS(
			SELECT 1 FROM revisions d WHERE d.scope=r.scope AND d.record=r.record
			AND json_extract(d.body,'$.operation')='delete'
			AND NOT EXISTS(SELECT 1 FROM restores x WHERE x.deletion=d.revision)
		)`);err!=nil{return ErrStorage}
	if s.schemaVersion==2 {if err=s.rebuildV2(ctx,tx);err!=nil{return err}}
	if err=tx.Commit();err!=nil{return ErrStorage};return nil
}
