package records

import (
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
 "errors"
 "regexp"
 "time"
 "unicode/utf8"

 "github.com/humzakhan/capsule-memory/internal/frame"
 "github.com/humzakhan/capsule-memory/internal/strictjson"
)
var ErrInvalid = errors.New("invalid session record")
var ids=regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)
var stores=regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
var hashes=regexp.MustCompile(`^[a-f0-9]{64}$`)
var decimals=regexp.MustCompile(`^(0|[1-9][0-9]{0,127})$`)
func one(v string, values ...string)bool{for _,s:=range values{if v==s{return true}};return false}
func text(s string,n int)bool{return len(s)<=n&&utf8.ValidString(s)}
func required(s string,n int)bool{return s!=""&&text(s,n)}
func stamp(s string)bool{_,e:=time.Parse(time.RFC3339Nano,s);return len(s)<=40&&e==nil}
func refs(a []SourceRef, min int)bool{if len(a)<min||len(a)>16384{return false};for _,s:=range a{if !ids.MatchString(s.Record)||!ids.MatchString(s.Revision)||!V2Kinds[s.Kind]||!text(s.EventFrom,256)||!text(s.EventTo,256)||(s.EventFrom=="")!=(s.EventTo=="")||s.EventFrom!=""&&s.Kind!=KindSessionChunk{return false}};return true}
func scoped(s ScopedRef)bool{return ids.MatchString(s.Vault)&&ids.MatchString(s.Scope)&&ids.MatchString(s.Record)&&(s.Revision==""||ids.MatchString(s.Revision))}
func generator(g Generator)bool{return required(g.Model,256)&&required(g.PromptVersion,128)&&required(g.PolicyVersion,128)&&ids.MatchString(g.Device)&&hashes.MatchString(g.JobKey)}
func gaps(a []Gap)bool{if len(a)>256{return false};for _,g:=range a{if !one(g.Reason,OmissionReasons...)||g.Count<0||!text(g.From,256)||!text(g.To,256){return false}};return true}
var harnesses=regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,127}$`)
func (v Origin) Validate()error{if !stores.MatchString(v.SourceStore)||!harnesses.MatchString(v.Harness)||!required(v.NativeSessionID,256)||!text(v.Environment,256){return ErrInvalid};return nil}
func (v Project) Validate()error{if !ids.MatchString(v.ProjectID)||!required(v.Name,256)||len(v.Hints)>16{return ErrInvalid};for _,h:=range v.Hints{if !text(h,512){return ErrInvalid}};return nil}
func link(v SessionLink)bool{return (v.Session==""||ids.MatchString(v.Session))&&v.Origin.Validate()==nil&&one(v.Relation,"fork","subagent","continuation","view_of","clear_successor")}
func (v SessionManifest) Validate()error{
 if !hashes.MatchString(v.SessionKey)||!ids.MatchString(v.ProjectID)||v.Origin.Validate()!=nil||!required(v.Adapter,128)||!text(v.HarnessVersion,128)||!text(v.Title,512)||!text(v.Branch,256)||!one(v.State,"open","idle","closed","unknown")||len(v.Links)>64||len(v.Bindings)>16||len(v.ChunkHigh)>256||v.ChunkHigh==nil||!gaps(v.Gaps)||(v.Continues!=""&&!ids.MatchString(v.Continues)){return ErrInvalid}
 if v.Parent!=nil&&!link(*v.Parent){return ErrInvalid};for _,l:=range v.Links{if !link(l){return ErrInvalid}};for _,b:=range v.Bindings{if !one(b.Harness,"claude","codex")||!required(b.NativeID,256)||b.Strength!="strong"{return ErrInvalid}}
 if v.Policy.Version<1||v.Policy.Redaction!="redact/v1"||!one(v.Policy.ToolDetail,"metadata","full")||v.Policy.Attachments{return ErrInvalid};for b,n:=range v.ChunkHigh{if !required(b,256)||n< -1{return ErrInvalid}};return nil
}
func (v Event) Validate()error{
 if !required(v.ID,256)||!text(v.NativeID,256)||!one(v.Role,"user","assistant","system","tool")||!one(v.Type,"message","tool_call","tool_result","reasoning","compaction","correction","meta")||!text(v.Text,512<<10)||!text(v.Parent,256)||!text(v.Turn,256)||!text(v.ToolName,256)||!text(v.ToolStatus,128)||!text(v.ToolCallID,256)||!text(v.ViewOf,256)||!one(v.Origin,"","capsule_recall")||len(v.Retracts)>4096{return ErrInvalid};if v.Time!=""&&!stamp(v.Time){return ErrInvalid};for _,r:=range v.Retracts{if !required(r,256){return ErrInvalid}};if v.Part!=nil&&(!required(v.Part.Group,256)||v.Part.Count<1||v.Part.Count>64||v.Part.Index<0||v.Part.Index>=v.Part.Count){return ErrInvalid};return nil
}
func (v SessionChunk) Validate()error{
 if !ids.MatchString(v.Session)||!required(v.Branch,256)||v.Seq<0||(v.Prev!=""&&!ids.MatchString(v.Prev))||!required(v.Generation,256)||len(v.Events)<1||len(v.Events)>4096||len(v.Omissions)>4096||len(v.Redactions)>8192{return ErrInvalid}
 seen:=map[string]bool{};for _,e:=range v.Events{if e.Validate()!=nil||seen[e.ID]{return ErrInvalid};seen[e.ID]=true};for _,o:=range v.Omissions{if !one(o.Reason,OmissionReasons...)||o.Bytes<0||!text(o.Event,256){return ErrInvalid}};for _,r:=range v.Redactions{if !required(r.Event,256)||r.Offset<0||r.Length<0||!required(r.Class,128){return ErrInvalid}}
 b,e:=json.Marshal(v.Events);if e!=nil{return ErrInvalid};h:=sha256.Sum256(b);if v.Digest!=hex.EncodeToString(h[:]){return ErrInvalid};b,e=json.Marshal(v);if e!=nil||len(b)>frame.ChunkHardMax{return ErrInvalid};return nil
}
func (v Summary) Validate()error{
 if !one(v.Level,"window","session","project_day","personal_week")||!ids.MatchString(v.Subject)||len(v.Findings)>256||!refs(v.Sources,1)||!refs(v.Primary,1)||!refs(v.SourceHeads,1)||!gaps(v.Gaps)||!refs(v.Influence,1)||!hashes.MatchString(v.InputFingerprint)||!generator(v.Generator){return ErrInvalid};if v.PeriodStart!=""&&!stamp(v.PeriodStart)||v.PeriodEnd!=""&&!stamp(v.PeriodEnd){return ErrInvalid};for _,f:=range v.Findings{n:=1;if f.Kind=="open_question"{n=0};if !one(f.Kind,"goal","work","claimed_outcome","evidenced_outcome","decision","open_question","lesson","contradiction")||!required(f.Text,4096)||!refs(f.Evidence,n){return ErrInvalid}};return nil
}
func (v Insight) Validate()error{
 if !one(v.Type,"memory","decision")||!one(v.Category,"fact","preference","constraint","lesson","choice","idea","rejected_alternative")||!required(v.Assertion,4096)||!ids.MatchString(v.ProjectID)||!one(v.Basis,"explicit","inferred")||!one(v.Verification,"none","claimed","evidenced")||!one(v.Review,"candidate","accepted","rejected","edited")||!hashes.MatchString(v.CandidateKey)||!refs(v.Contradictions,0)||!refs(v.Influence,0)||len(v.Supersedes)>128{return ErrInvalid}
 if v.Type=="decision"{if !one(v.DecisionState,"proposed","accepted","superseded","rejected"){return ErrInvalid}}else if v.DecisionState!=""{return ErrInvalid}
 claim:=v.Generator==nil&&one(v.Review,"accepted","edited")&&v.Basis=="explicit"&&len(v.Evidence)==0
 if len(v.Evidence)==0&&v.OwnerProof==nil{return ErrInvalid};if v.OwnerProof!=nil&&(!claim||len(v.Contradictions)!=0||len(v.Influence)!=0||v.AutoAccepted||!ids.MatchString(v.Reviewer)||v.OwnerProof.Validate()!=nil){return ErrInvalid}
 n:=1;if v.OwnerProof!=nil{n=0};if !refs(v.Evidence,n){return ErrInvalid};if v.Reviewer!=""&&!ids.MatchString(v.Reviewer){return ErrInvalid};if v.Generator!=nil&&!generator(*v.Generator){return ErrInvalid};for _,s:=range v.Supersedes{if !ids.MatchString(s){return ErrInvalid}};return nil
}
func (v Knowledge) Validate()error{h:=sha256.Sum256([]byte(v.Text));if !ids.MatchString(v.LinkID)||!one(v.Type,"memory","decision")||!one(v.Category,"fact","preference","constraint","lesson","choice","idea","rejected_alternative")||!required(v.Text,4096)||v.TextSHA256!=hex.EncodeToString(h[:])||!scoped(v.Source)||v.Source.Revision==""||!one(v.Review,"accepted","edited")||(v.ProjectID!=""&&!ids.MatchString(v.ProjectID)){return ErrInvalid};return nil}
func (v Attestation) Validate()error{if !ids.MatchString(v.Bridge)||!stamp(v.VerifiedAt)||!one(v.Coverage,"complete","partial")||len(v.VerifiedHeads)<1||len(v.VerifiedHeads)>64{return ErrInvalid};seen:=map[string]bool{};for _,s:=range v.VerifiedHeads{if !ids.MatchString(s.Scope)||!decimals.MatchString(s.Head)||seen[s.Scope]{return ErrInvalid};seen[s.Scope]=true};return nil}
func (v Projection) Validate()error{if v.WithdrawalMarker{if !one(v.State,"withdrawn","hidden")||v.Source.Revision!=""||v.Target.Revision!=""||len(v.Dependencies)!=0||v.Attestation!=nil||v.TextSHA256!="0000000000000000000000000000000000000000000000000000000000000000"||v.Role=="source"&&(!v.OwnerWithdrawn||v.State!="withdrawn"){return ErrInvalid}}else if v.Source.Revision==""{return ErrInvalid};if v.Generation<1{return ErrInvalid};if v.OwnerWithdrawn&&v.State!="withdrawn"{return ErrInvalid};if !ids.MatchString(v.LinkID)||!one(v.Role,"source","target")||!scoped(v.Source)||!scoped(v.Target)||!refs(v.Dependencies,0)||!hashes.MatchString(v.TextSHA256)||!one(v.State,"pending","projected","withdrawn","hidden")||(v.Grant!=""&&!ids.MatchString(v.Grant)){return ErrInvalid};if v.Attestation!=nil&&v.Attestation.Validate()!=nil{return ErrInvalid};return nil}
func (v BridgeGrantBody) Validate()error{if v.Version!=1||!ids.MatchString(v.SourceVault)||!ids.MatchString(v.TargetVault)||len(v.TargetScopes)!=1||v.LedgerScope!=v.TargetScopes[0]||!ids.MatchString(v.LedgerScope)||!ids.MatchString(v.Bridge){return ErrInvalid};return nil}
func (v AggregationGrantBody) Validate()error{if v.Version!=1||!ids.MatchString(v.Vault)||!required(v.Name,256)||!ids.MatchString(v.Destination)||len(v.Sources)<1||len(v.Sources)>64{return ErrInvalid};seen:=map[string]bool{};for _,s:=range v.Sources{if !ids.MatchString(s)||seen[s]{return ErrInvalid};seen[s]=true};return nil}
func signed(payload,signature string,dst any)error{b,e:=base64.StdEncoding.Strict().DecodeString(payload);if e!=nil||base64.StdEncoding.EncodeToString(b)!=payload{return ErrInvalid};sig,e:=base64.StdEncoding.Strict().DecodeString(signature);if e!=nil||len(sig)!=64||base64.StdEncoding.EncodeToString(sig)!=signature{return ErrInvalid};return strictjson.Decode(b,frame.MaxDecoded,dst)}
func (v BridgeGrant) Validate()error{var b BridgeGrantBody;if signed(v.Payload,v.Signature,&b)!=nil{return ErrInvalid};return b.Validate()}
func (v AggregationGrant) Validate()error{var b AggregationGrantBody;if signed(v.Payload,v.Signature,&b)!=nil{return ErrInvalid};return b.Validate()}
func (v Job) Validate()error{if !hashes.MatchString(v.JobKey)||!one(v.Kind,"session_digest","project_rollup","personal_rollup")||!ids.MatchString(v.Subject)||!refs(v.Inputs,1)||len(v.Inputs)>4096||!ids.MatchString(v.AssignedDevice)||!ids.MatchString(v.Destination)||!one(v.State,"requested","cancelled"){return ErrInvalid};return nil}
func (v Suppression) Validate()error{if !hashes.MatchString(v.SessionKey)||v.Origin.Validate()!=nil||!ids.MatchString(v.Session)||!one(v.Reason,"deleted","excluded"){return ErrInvalid};return nil}
func Validate(kind string,b []byte)error{
 var v interface{Validate()error};switch kind{case KindProject:v=&Project{};case KindSessionManifest:v=&SessionManifest{};case KindSessionChunk:v=&SessionChunk{};case KindSummary:v=&Summary{};case KindInsight:v=&Insight{};case KindProjection:v=&Projection{};case KindKnowledge:v=&Knowledge{};case KindBridgeGrant:v=&BridgeGrant{};case KindAggregationGrant:v=&AggregationGrant{};case KindJob:v=&Job{};case KindSuppression:v=&Suppression{};default:return ErrInvalid}
 if strictjson.Decode(b,frame.MaxDecoded,v)!=nil{return ErrInvalid};return v.Validate()
}
