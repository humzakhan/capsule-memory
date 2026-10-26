package derive

import (
	"errors"
	"unicode/utf8"

	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)

var ErrOutput = errors.New("invalid derivation output")
type findingOutput struct { Kind string `json:"kind" cm:"required"`; Text string `json:"text" cm:"required"`; Evidence []string `json:"evidence" cm:"required"` }
type candidateOutput struct {
	Type string `json:"type" cm:"required"`; Category string `json:"category" cm:"required"`; Assertion string `json:"assertion" cm:"required"`; Basis string `json:"basis" cm:"required"`; DecisionState string `json:"decision_state" cm:"required"`; Verification string `json:"verification" cm:"required"`; Evidence []string `json:"evidence" cm:"required"`; Contradictions []string `json:"contradictions" cm:"required"`
}
type modelOutput struct { Findings []findingOutput `json:"findings" cm:"required"`; Candidates []candidateOutput `json:"candidates" cm:"required"`; Gaps []string `json:"gaps" cm:"required"` }
func oneOf(s string,options ...string)bool{for _,o:=range options{if s==o{return true}};return false}
func bounded(s string)bool{return s!=""&&len(s)<=4096&&utf8.ValidString(s)}
func refs(ids []string,known map[string]citation)([]records.SourceRef,error){
	if len(ids)>256{return nil,ErrOutput};out:=[]records.SourceRef{};seen:=map[string]bool{}
	for _,id:=range ids{c,ok:=known[id];if !ok{return nil,ErrOutput};if !seen[id]{out=append(out,c.Ref);seen[id]=true}};return out,nil
}
func validate(content string,known map[string]citation)(modelOutput,error){
	var out modelOutput;if strictjson.Decode([]byte(content),1<<20,&out)!=nil||len(out.Findings)>256||len(out.Candidates)>64||len(out.Gaps)>256{return out,ErrOutput}
	for i:=range out.Findings{f:=&out.Findings[i];if !oneOf(f.Kind,"goal","work","claimed_outcome","evidenced_outcome","decision","open_question","lesson","contradiction")||!bounded(f.Text)||len(f.Evidence)==0&&f.Kind!="open_question"{return out,ErrOutput};if _,err:=refs(f.Evidence,known);err!=nil{return out,err}
		if f.Kind=="evidenced_outcome"{evidenced:=false;for _,id:=range f.Evidence{if known[id].Type=="tool_result"{evidenced=true}};if !evidenced{f.Kind="claimed_outcome";out.Gaps=append(out.Gaps,"outcome downgraded: no tool-result evidence")}}
	}
	for i:=range out.Candidates{c:=&out.Candidates[i];if !oneOf(c.Type,"memory","decision")||!oneOf(c.Category,"fact","preference","constraint","lesson","choice","idea","rejected_alternative")||!bounded(c.Assertion)||!oneOf(c.Basis,"explicit","inferred")||!oneOf(c.Verification,"none","claimed","evidenced")||len(c.Evidence)==0{return out,ErrOutput}
		if c.Type=="decision"{if !oneOf(c.DecisionState,"proposed","accepted","superseded","rejected"){return out,ErrOutput}}else if c.DecisionState!=""{return out,ErrOutput}
		if _,err:=refs(c.Evidence,known);err!=nil{return out,err};if _,err:=refs(c.Contradictions,known);err!=nil{return out,err}
		user,tool:=false,false;for _,id:=range c.Evidence{if known[id].Role=="user"{user=true};if known[id].Type=="tool_result"{tool=true}}
		if c.DecisionState=="accepted"&&!user{c.DecisionState="proposed";out.Gaps=append(out.Gaps,"decision downgraded: no user evidence")};if c.Verification=="evidenced"&&!tool{c.Verification="claimed";out.Gaps=append(out.Gaps,"verification downgraded: no tool-result evidence")}
	}
	if len(out.Gaps)>256{out.Gaps=out.Gaps[:256]};for _,g:=range out.Gaps{if !bounded(g){return out,ErrOutput}};return out,nil
}
