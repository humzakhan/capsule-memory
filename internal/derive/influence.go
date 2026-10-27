package derive

import (
 "encoding/json"
 "sort"
 "github.com/humzakhan/capsule-memory/internal/records"
)

func inputFingerprint(input preparedInput)string{
 raw,_:=json.Marshal(struct{Lines []inputLine;Heads []records.SourceRef;Gaps []records.Gap;Omissions []records.Omission}{input.lines,input.heads,input.gaps,input.omissions});return stableID(string(raw))
}

func windowFingerprint(input preparedInput,lines []inputLine,known map[string]citation)string{
 raw,_:=json.Marshal(struct{Input string;Lines []inputLine;Known map[string]citation}{inputFingerprint(input),lines,known});return stableID(string(raw))
}

// All prompt entries influence output, irrespective of model-selected citations.
// Intermediate summary edges carry their own exact consumed ancestry transitively.
func consumedRefs(known map[string]citation)[]records.SourceRef{
 entries:=make([]citation,0,len(known));for _,c:=range known{entries=append(entries,c)};sort.Slice(entries,func(i,j int)bool{a,b:=entries[i],entries[j];if a.Ref.Revision!=b.Ref.Revision{return a.Ref.Revision<b.Ref.Revision};return a.Ordinal<b.Ordinal})
 refs:=[]records.SourceRef{};previous:=-2;for _,c:=range entries{ref:=c.Ref;if len(refs)>0&&ref.Kind==records.KindSessionChunk&&refs[len(refs)-1].Revision==ref.Revision&&c.Ordinal==previous+1{refs[len(refs)-1].EventTo=ref.EventTo}else{refs=append(refs,ref)};previous=c.Ordinal};return uniqueRefs(refs)
}
