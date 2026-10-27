package session

import (
	"encoding/json"
	"errors"
	"strconv"
	"unicode/utf8"

	"github.com/humzakhan/capsule-memory/internal/records"
)
// Split escaped JSON, not source bytes: control characters can expand six-fold.
func splitEvent(e records.Event)([]records.Event,error){b,_:=json.Marshal(e);if len(b)<=384<<10{return []records.Event{e},nil};if e.Type=="correction"&&len(e.Retracts)>0{var parts []records.Event;for start:=0;start<len(e.Retracts);start+=512{end:=start+512;if end>len(e.Retracts){end=len(e.Retracts)};p:=e;p.Retracts=e.Retracts[start:end];p.ID="part:"+hash([]byte(e.ID+":"+strconv.Itoa(len(parts))));parts=append(parts,p)};for i:=range parts{parts[i].Part=&records.Part{Group:e.ID,Index:i,Count:len(parts)}};return parts,nil};text:=e.Text;if text==""{return nil,errors.New("event metadata exceeds bound")};var parts []records.Event;for len(text)>0{if len(parts)>=64{return nil,errors.New("event exceeds 64 parts")};n:=len(text);if n>60<<10{n=60<<10};for n>0&&n<len(text)&&!utf8.RuneStart(text[n]){n--};if n==0{return nil,errors.New("invalid event boundary")};p:=e;p.Text=text[:n];p.ID="part:"+hash([]byte(e.ID+":"+strconv.Itoa(len(parts))));parts=append(parts,p);text=text[n:]};for i:=range parts{parts[i].Part=&records.Part{Group:e.ID,Index:i,Count:len(parts)}};return parts,nil}
func chunks(events []records.Event)[][]records.Event{var out [][]records.Event;start:=0;size:=0;for i,e:=range events{b,_:=json.Marshal(e);if i>start&&(size+len(b)>256<<10||i-start>=4096){out=append(out,events[start:i]);start=i;size=0};size+=len(b)};if start<len(events){out=append(out,events[start:])};return out}

func chunkRedactions(events []records.Event,audit []records.Redaction,all []records.Event)[]records.Redaction{
 offsets:=map[string]int{};groupBytes:=map[string]int{};for _,e:=range all{if e.Part!=nil{offsets[e.ID]=groupBytes[e.Part.Group];groupBytes[e.Part.Group]+=len(e.Text)}}
 var out []records.Redaction;for _,e:=range events{group:=e.ID;start:=0;if e.Part!=nil{group=e.Part.Group;start=offsets[e.ID]};end:=start+len(e.Text);for _,a:=range audit{if a.Event!=group{continue};lo:=a.Offset;if lo<start{lo=start};hi:=a.Offset+a.Length;if hi>end{hi=end};if hi>lo{out=append(out,records.Redaction{Event:e.ID,Offset:lo-start,Length:hi-lo,Class:a.Class})}}};return out
}
