// Package filetail reads only owner-selected transcript trees. It never opens a harness DB.
package filetail

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
 "sync"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)

type Config struct {
	Root string `json:"root"`
	PinnedVersions []string `json:"pinned_versions"`
	AllowUnknownVersion bool `json:"allow_unknown_version"`
	IncludeSubagents bool `json:"include_subagents"`
}
func Parse(raw json.RawMessage) (Config, error) {
	var c Config
	if strictjson.Decode(raw, 1<<20, &c) != nil || !filepath.IsAbs(c.Root) { return c, capture.ErrInvalid }
	for _, p := range c.PinnedVersions { if p == "" || len(p)>128 { return c, capture.ErrInvalid } }
	return c,nil
}
func Pinned(c Config, version string) bool {
	if version == "" || version == "unknown" { return c.AllowUnknownVersion }
	for _, p := range c.PinnedVersions { if strings.HasPrefix(version,p) { return true } }; return false
}
func Path(root,key string) (string,error) {
	if filepath.IsAbs(key) { return "", capture.ErrInvalid }
	p := filepath.Join(root,key); rel,err:=filepath.Rel(root,p)
	if err!=nil || rel==".." || strings.HasPrefix(rel,".."+string(filepath.Separator)) { return "", capture.ErrInvalid }
	// Refuse symlink escapes, including a symlink in any parent component.
	real,err:=filepath.EvalSymlinks(p); if err!=nil{return "",capture.ErrUnavailable}
	rr,err:=filepath.EvalSymlinks(root); if err!=nil{return "",capture.ErrUnavailable}
	rel,err=filepath.Rel(rr,real); if err!=nil||rel==".."||strings.HasPrefix(rel,".."+string(filepath.Separator)){return "",capture.ErrInvalid}
	return real,nil
}
func Discover(ctx context.Context,c Config,page string,limit int,accept func(string)bool)([]capture.Stream,string,error){
	if limit<=0||limit>64{limit=64}; var keys []string
	err:=filepath.WalkDir(c.Root,func(p string,d os.DirEntry,e error)error{
		if ctx.Err()!=nil{return ctx.Err()};if e!=nil{return capture.ErrUnavailable};if d.Type()&os.ModeSymlink!=0{return nil}
		rel,e:=filepath.Rel(c.Root,p);if e!=nil{return e}
		if d.IsDir(){if d.Name()=="tool-results"||d.Name()=="file-history"||(!c.IncludeSubagents&&d.Name()=="subagents"){return filepath.SkipDir};return nil}
		if accept(filepath.ToSlash(rel))&&rel>page{keys=append(keys,rel)}
		if len(keys)>100000{return errors.New("capture discovery bound exceeded")};return nil
	});if err!=nil{return nil,"",err};sort.Strings(keys);next:="";if len(keys)>limit{keys=keys[:limit];next=keys[len(keys)-1]}
	var out []capture.Stream;for _,k:=range keys{p,e:=Path(c.Root,k);if e!=nil{continue};fi,e:=os.Stat(p);if e!=nil||!fi.Mode().IsRegular(){continue};out=append(out,capture.Stream{Key:k,Generation:Identity(fi),Size:fi.Size(),Modified:fi.ModTime()})};return out,next,nil
}
func Preview(ctx context.Context,c Config,accept func(string)bool)(capture.Preview,error){
	p:=capture.Preview{Locations:[]string{c.Root},Unsupported:[]string{"format compatibility NOT RUN; owner version pins required"}}
	s,_,e:=Discover(ctx,c,"",64,accept);if e!=nil{return p,e};p.Streams=len(s);for _,v:=range s{p.Bytes+=v.Size};return p,nil
}
type Line struct { Offset int64; Data []byte }
type Batch struct { Lines []Line; Next capture.Checkpoint; CaughtUp bool; Omissions []records.Omission; Quarantine string; ScannedBytes int64; Reconcile bool }
func Header(root,key string)([]byte,error){p,e:=Path(root,key);if e!=nil{return nil,e};f,e:=os.Open(p);if e!=nil{return nil,capture.ErrUnavailable};defer f.Close();r:=bufio.NewReader(io.LimitReader(f,64<<10));b,e:=r.ReadBytes('\n');if e!=nil{return nil,capture.ErrInvalid};return b,nil}
type tailState struct{start int64;identity string;modified time.Time;partial []byte}
// Reader owns a bounded, ephemeral partial-line cache. It never acknowledges
// plaintext staging; a process crash re-reads from the durable checkpoint.
type Reader struct{mu sync.Mutex;pending map[string]*tailState}
func NewReader()*Reader{return &Reader{pending:map[string]*tailState{}}}
func(r *Reader) Read(ctx context.Context,c Config,st capture.Stream,cp capture.Checkpoint,lim capture.Limits)(Batch,error){
 r.mu.Lock();defer r.mu.Unlock();key:=filepath.Join(c.Root,st.Key);state:=r.pending[key];if state==nil{if len(r.pending)>=64{return Batch{},capture.ErrUnavailable};state=&tailState{}}
 b,e:=read(ctx,c,st,cp,lim,state);if len(state.partial)>0{total:=len(state.partial);for k,p:=range r.pending{if k!=key{total+=len(p.partial)}};if total>16<<20{delete(r.pending,key);return Batch{},capture.ErrUnavailable};r.pending[key]=state}else{delete(r.pending,key)};return b,e
}
func Read(ctx context.Context,c Config,st capture.Stream,cp capture.Checkpoint,lim capture.Limits)(Batch,error){return read(ctx,c,st,cp,lim,&tailState{})}
func read(ctx context.Context,c Config,st capture.Stream,cp capture.Checkpoint,lim capture.Limits,state *tailState)(Batch,error){
 lim=capture.DefaultLimits(lim);b:=Batch{Next:cp};b.Next.Stream=st.Key
 p,e:=Path(c.Root,st.Key);if e!=nil{return b,e};f,e:=os.Open(p);if e!=nil{return b,capture.ErrUnavailable};defer f.Close();fi,e:=f.Stat();if e!=nil||!fi.Mode().IsRegular(){return b,capture.ErrUnavailable}
 off:=int64(0);if cp.Position!=""{off,e=strconv.ParseInt(cp.Position,10,64);if e!=nil||off<0{return b,capture.ErrInvalid}}
 identity:=Identity(fi);prefixLen:=off;if prefixLen>64<<10{prefixLen=64<<10};prefix:=make([]byte,prefixLen);if _,e=f.ReadAt(prefix,0);e!=nil&&e!=io.EOF{return b,capture.ErrUnavailable};sum:=sha256.Sum256(prefix)
 tailChanged:=false
 if cp.TailDigest!=""{start:=off-(64<<10);if start<0{start=0};tail:=make([]byte,off-start);if _,e=f.ReadAt(tail,start);e!=nil&&e!=io.EOF{return b,capture.ErrUnavailable};digest:=sha256.Sum256(tail);tailChanged=cp.TailDigest!=hex.EncodeToString(digest[:])}
 if cp.Position!=""&&((cp.ObservedModified!=""&&cp.ObservedSize==fi.Size()&&cp.ObservedModified!=fi.ModTime().UTC().Format(time.RFC3339Nano))||fi.Size()<off||(cp.FileIdentity!=""&&cp.FileIdentity!=identity)||(cp.PrefixDigest!=""&&cp.PrefixDigest!=hex.EncodeToString(sum[:]))||tailChanged){b.Reconcile=true;b.Next.Reconciling=true;off=0;seed:=sha256.Sum256([]byte(identity+":"+hex.EncodeToString(sum[:])+":"+strconv.FormatInt(fi.Size(),10)+":"+fi.ModTime().UTC().Format(time.RFC3339Nano)));b.Next.Generation=hex.EncodeToString(seed[:]);b.Next.PrefixDigest="";b.Next.Discarding=false;b.Next.AdapterState="";state.partial=nil}
 b.Reconcile=b.Reconcile||cp.Reconciling;b.Next.ObservedModified=fi.ModTime().UTC().Format(time.RFC3339Nano);b.Next.ObservedSize=fi.Size();b.Next.FileIdentity=identity;if b.Next.Generation==""{b.Next.Generation=identity};b.Next.Position=strconv.FormatInt(off,10)
 if time.Since(fi.ModTime())<lim.Stability{return b,nil}
 if state.start!=off||state.identity!=identity||!state.modified.Equal(fi.ModTime()){state.partial=nil};partial:=state.partial;state.partial=nil
 scanOff:=off+int64(len(partial));stop:=fi.Size();if lim.EndBounded&&lim.StopAt<stop{stop=lim.StopAt};readLimit:=lim.MaxBytes;if stop-scanOff<readLimit{readLimit=stop-scanOff};if readLimit<0{return b,capture.ErrInvalid};if readLimit==0&&len(partial)>0{state.partial=partial;state.start=off;state.identity=identity;state.modified=fi.ModTime();return b,nil};if _,e=f.Seek(scanOff,io.SeekStart);e!=nil{return b,capture.ErrUnavailable};reader:=bufio.NewReaderSize(io.LimitReader(f,readLimit),64<<10);readBytes:=int64(0)
 for len(b.Lines)<lim.MaxEvents&&readBytes<readLimit{
  if ctx.Err()!=nil{return b,ctx.Err()};line:=partial;total:=len(partial);partial=nil;oversize:=b.Next.Discarding
  for{part,er:=reader.ReadSlice('\n');readBytes+=int64(len(part));total+=len(part);if total<=8<<20&&!oversize{line=append(line,part...)}else{oversize=true;line=nil};if er==bufio.ErrBufferFull{continue};if er==io.EOF{
   if oversize{off+=int64(total);b.Next.Discarding=true;b.Omissions=append(b.Omissions,records.Omission{Reason:"too_large",Bytes:int64(total)})}else if total>0{state.partial=line;state.start=off;state.identity=identity;state.modified=fi.ModTime()};goto done
  };if er!=nil{return b,capture.ErrUnavailable};break}
  if oversize{b.Omissions=append(b.Omissions,records.Omission{Reason:"too_large",Bytes:int64(total)})}else{b.Lines=append(b.Lines,Line{Offset:off,Data:line})};off+=int64(total);b.Next.Discarding=false
 }
 done:
 b.CaughtUp=off==stop&&!b.Next.Discarding&&len(state.partial)==0;b.Next.Position=strconv.FormatInt(off,10);b.ScannedBytes=readBytes
 prefixLen=off;if prefixLen>64<<10{prefixLen=64<<10};prefix=make([]byte,prefixLen);if _,e=f.ReadAt(prefix,0);e!=nil&&e!=io.EOF{return b,capture.ErrUnavailable};sum=sha256.Sum256(prefix);b.Next.PrefixDigest=hex.EncodeToString(sum[:]);tail:=make([]byte,prefixLen);if _,e=f.ReadAt(tail,off-prefixLen);e!=nil&&e!=io.EOF{return b,capture.ErrUnavailable};sum=sha256.Sum256(tail);b.Next.TailDigest=hex.EncodeToString(sum[:]);return b,nil
}
