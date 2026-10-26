package session

import (
 "context"
 "encoding/json"
 "errors"
 "strings"
 "github.com/humzakhan/capsule-memory/internal/frame"
 "github.com/humzakhan/capsule-memory/internal/memory"
 "github.com/humzakhan/capsule-memory/internal/records"
)

var errChunkMetadataBound=errors.New("chunk_metadata_bound")

func chunkOmissions(events,all []records.Event,audit []records.Omission,first bool)[]records.Omission{
 matches:=func(id string,list []records.Event)bool{for _,e:=range list{if id==e.ID||id==e.NativeID||e.Part!=nil&&id==e.Part.Group{return true}};return false}
 var out []records.Omission;for _,o:=range audit{if o.Event!=""&&matches(o.Event,events)||first&&(o.Event==""||!matches(o.Event,all)){out=append(out,o)}};return out
}

// Measure a complete serialized revision with conservative maximum metadata
// before Prepare. Splitting retains each event's audit alongside that event.
func(m *Module) prepareChunk(ctx context.Context,info memory.ProfileInfo,srcKind,source,harness,scope string,c records.SessionChunk)([]memory.Prepared,error){
 body,e:=json.Marshal(c);if e!=nil{return nil,e};dummy:=strings.Repeat("x",128)
 revision:=memory.Revision{Version:2,Vault:info.Vault,Scope:scope,Record:dummy,Kind:records.KindSessionChunk,ID:dummy,Parents:[]string{},OperationID:dummy,Operation:"put",Author:info.Device,Created:"2000-01-01T00:00:00.000000000Z",Provenance:memory.Provenance{SourceKind:srcKind,SourceRef:source,Harness:harness},Tags:[]string{},Body:body}
 encoded,e:=json.Marshal(revision);if e!=nil{return nil,e}
 if len(encoded)<=frame.ChunkHardMax&&len(c.Omissions)<=4096&&len(c.Redactions)<=8192{
  p,e:=m.svc.Prepare(ctx,memory.PutInput{Scope:scope,Kind:records.KindSessionChunk,Body:c,Provenance:revision.Provenance});if e!=nil{return nil,e};return []memory.Prepared{p},nil
 }
 if len(c.Events)<2{return nil,errChunkMetadataBound}
 middle:=len(c.Events)/2;left,right:=c,c;left.Events=c.Events[:middle];right.Events=c.Events[middle:]
 left.Omissions=chunkOmissions(left.Events,c.Events,c.Omissions,true);right.Omissions=chunkOmissions(right.Events,c.Events,c.Omissions,false)
 left.Redactions=nil;right.Redactions=nil;for _,a:=range c.Redactions{for _,ev:=range left.Events{if a.Event==ev.ID{left.Redactions=append(left.Redactions,a)}};for _,ev:=range right.Events{if a.Event==ev.ID{right.Redactions=append(right.Redactions,a)}}}
 raw,_:=json.Marshal(left.Events);left.Digest=hash(raw);first,e:=m.prepareChunk(ctx,info,srcKind,source,harness,scope,left);if e!=nil{return nil,e}
 right.Seq=c.Seq+int64(len(first));right.Prev=first[len(first)-1].Revision.Record;raw,_=json.Marshal(right.Events);right.Digest=hash(raw)
 second,e:=m.prepareChunk(ctx,info,srcKind,source,harness,scope,right);if e!=nil{return nil,e};return append(first,second...),nil
}
