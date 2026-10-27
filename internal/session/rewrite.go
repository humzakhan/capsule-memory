package session

import (
 "context"
 "encoding/json"
 "sort"
 "strings"

 "github.com/humzakhan/capsule-memory/internal/memory"
 "github.com/humzakhan/capsule-memory/internal/records"
)

// Only a stable native id proves a rewrite. Original chunks remain immutable;
// replacements receive content-derived ids and an explicit correction event.
func reconcileEvent(ctx context.Context,q memory.Queryer,key,scope,native string,event records.Event)(records.Event,*records.Event,error){
 event.NativeID=native;event.ID=native
 rows,e:=q.QueryContext(ctx,`SELECT json_extract(r.body,'$.body.events['||ev.ordinal||']'),ev.session FROM event_lookup ev JOIN sessions s ON s.session=ev.session JOIN revisions r ON r.revision=ev.revision WHERE s.session_key=? AND ev.scope=? AND (ev.native_id=? OR ev.event_id=?) LIMIT 1001`,key,scope,native,native);if e!=nil{return event,nil,e}
 type observed struct{event records.Event;session string};var prior []observed
 for rows.Next(){var raw []byte;var old observed;if rows.Scan(&raw,&old.session)!=nil||json.Unmarshal(raw,&old.event)!=nil{rows.Close();return event,nil,memory.ErrStorage};prior=append(prior,old)};e=rows.Err();rows.Close();if e!=nil{return event,nil,e};if len(prior)>1000{return event,nil,memory.ErrInvalid}
 groups:=map[string][]records.Event{};seen:=map[string]bool{}
 for _,old:=range prior{ev:=old.event;group:=ev.ID;if ev.Part!=nil{group=ev.Part.Group};invalid,e:=memory.EventRetracted(ctx,q,old.session,ev.ID,group);if e!=nil{return event,nil,e};if invalid||seen[ev.ID]{continue};seen[ev.ID]=true;groups[group]=append(groups[group],ev)}
 raw,_:=json.Marshal(event);different:=false;var active []string
 for id,parts:=range groups{sort.Slice(parts,func(i,j int)bool{if parts[i].Part==nil||parts[j].Part==nil{return parts[i].ID<parts[j].ID};return parts[i].Part.Index<parts[j].Part.Index});normalized:=parts[0];if normalized.Part!=nil{if len(parts)!=normalized.Part.Count{return event,nil,memory.ErrHeadsChanged};normalized.Text="";if normalized.Type=="correction"{normalized.Retracts=nil};for i,p:=range parts{if p.Part==nil||p.Part.Index!=i{return event,nil,memory.ErrHeadsChanged};normalized.Text+=p.Text;if normalized.Type=="correction"{normalized.Retracts=append(normalized.Retracts,p.Retracts...)}};normalized.Part=nil};normalized.ID=native;normalized.NativeID=native;old,_:=json.Marshal(normalized);if string(old)!=string(raw){different=true};active=append(active,id)}
 sort.Strings(active);if !different{if len(active)>0{event.ID=active[0]};return event,nil,nil}
 event.ID="n:"+hash(append([]byte(native+"\n"+strings.Join(active,",")+"\n"),raw...));correction:=records.Event{ID:"correction:"+hash([]byte(key+"\n"+event.ID)),Role:"system",Type:"correction",Time:event.Time,Retracts:active};return event,&correction,nil
}
