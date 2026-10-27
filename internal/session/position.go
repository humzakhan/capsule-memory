package session

import (
 "context"
 "encoding/json"
 "database/sql"
 "errors"
 "sort"
 "strings"

 "github.com/humzakhan/capsule-memory/internal/capture"
 "github.com/humzakhan/capsule-memory/internal/memory"
 "github.com/humzakhan/capsule-memory/internal/records"
)

func archivedPosition(ctx context.Context,q memory.Queryer,src capture.Source,stream,position,generation string)(string,error){
 rows,e:=q.QueryContext(ctx,"SELECT DISTINCT event_id FROM event_ids WHERE source_id=? AND stream=? AND position_key=? AND generation!=? LIMIT 2",src.ID,stream,position,generation);if e!=nil{return "",e};defer rows.Close();var ids []string;for rows.Next(){var id string;if rows.Scan(&id)!=nil{return "",memory.ErrStorage};ids=append(ids,id)};if e=rows.Err();e!=nil{return "",e};if len(ids)>1{return "",ErrRewriteUnresolved};if len(ids)==1{return ids[0],nil};return "",nil
}
// A positional identity is reusable only when its admitted payload is identical
// at the same position. Any mutation or missing old position is ambiguous.
func reconcilePosition(ctx context.Context,q memory.Queryer,src capture.Source,stream,position,generation,scope string,event records.Event)(records.Event,error){
 old,e:=archivedPosition(ctx,q,src,stream,position,generation);if e!=nil||old==""{return event,e}
 rows,e:=q.QueryContext(ctx,`SELECT ev.value FROM chunks c JOIN sessions s ON s.session=c.session JOIN revisions r ON r.revision=c.revision JOIN json_each(r.body,'$.body.events') ev WHERE s.scope=? AND json_extract(r.body,'$.provenance.source_ref')=? AND (json_extract(ev.value,'$.id')=? OR json_extract(ev.value,'$.part.group')=?) LIMIT 65`,scope,src.ID,old,old);if e!=nil{return event,e};var parts []records.Event;seen:=map[string]bool{}
 for rows.Next(){var raw []byte;var previous records.Event;if rows.Scan(&raw)!=nil||json.Unmarshal(raw,&previous)!=nil{rows.Close();return event,memory.ErrStorage};if !seen[previous.ID]{seen[previous.ID]=true;parts=append(parts,previous)}};e=rows.Err();rows.Close();if e!=nil{return event,e};if len(parts)==0||len(parts)>64{return event,ErrRewriteUnresolved};previous:=parts[0]
 if previous.Part!=nil{if len(parts)!=previous.Part.Count{return event,ErrRewriteUnresolved};for _,p:=range parts{if p.Part==nil{return event,ErrRewriteUnresolved}};sort.Slice(parts,func(i,j int)bool{return parts[i].Part.Index<parts[j].Part.Index});previous.Text="";if previous.Type=="correction"{previous.Retracts=nil};for i,p:=range parts{if p.Part==nil||p.Part.Index!=i||p.Part.Group!=old{return event,ErrRewriteUnresolved};previous.Text+=p.Text;if previous.Type=="correction"{previous.Retracts=append(previous.Retracts,p.Retracts...)}};previous.ID=old;previous.Part=nil}else if len(parts)!=1{return event,ErrRewriteUnresolved}
 candidate:=event;candidate.ID=old;a,_:=json.Marshal(candidate);b,_:=json.Marshal(previous);if string(a)!=string(b){return event,ErrRewriteUnresolved};event.ID=old;return event,nil
}

func nativeIdentity(ce capture.Event,generation string)string{if ce.Event.NativeID!=""{return ce.Event.NativeID};if strings.HasPrefix(ce.Event.ID,"p:"+generation+":"){return ""};return ce.Event.ID}
func positionalView(ctx context.Context,q memory.Queryer,src capture.Source,stream,generation,view string,staged []identity)(string,error){
 prefix:="p:"+generation+":";if !strings.HasPrefix(view,prefix){return view,nil};position:=strings.TrimPrefix(view,prefix);for _,id:=range staged{if id.position==position{return id.id,nil}};var id string;e:=q.QueryRowContext(ctx,"SELECT event_id FROM event_ids WHERE source_id=? AND stream=? AND generation=? AND position_key=?",src.ID,stream,generation,position).Scan(&id);if e==nil{return id,nil};if !errors.Is(e,sql.ErrNoRows){return view,e};id,e=archivedPosition(ctx,q,src,stream,position,generation);if e!=nil{return view,e};if id!=""{return id,nil};return view,nil
}
