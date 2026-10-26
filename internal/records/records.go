// Package records defines the encrypted schema-v2 logical bodies.
package records

const (
 KindProject = "project"
 KindSessionManifest = "session_manifest"
 KindSessionChunk = "session_chunk"
 KindSummary = "summary"
 KindInsight = "insight"
 KindProjection = "projection"
 KindBridgeGrant = "bridge_grant"
 KindJob = "job"
 KindSuppression = "suppression"
 KindKnowledge = "knowledge"
 KindAggregationGrant = "aggregation_grant"
)
var V2Kinds = map[string]bool{KindProject:true,KindSessionManifest:true,KindSessionChunk:true,KindSummary:true,KindInsight:true,KindProjection:true,KindBridgeGrant:true,KindJob:true,KindSuppression:true,KindKnowledge:true,KindAggregationGrant:true}
type Project struct {
	ProjectID string   `json:"project_id" cm:"required"`
	Name      string   `json:"name" cm:"required"`             // <=256, encrypted only
	Hints     []string `json:"hints,omitempty"`  // <=16 x <=512; e.g. remote URL; suggestion only
}

type Origin struct {
	SourceStore     string `json:"source_store" cm:"required"`      // opaque stable per harness profile, <=128 [A-Za-z0-9_.:-]
	Harness         string `json:"harness" cm:"required"`           // "claude"|"codex"|"t3"|"import"
	NativeSessionID string `json:"native_session_id" cm:"required"` // <=256
	Environment     string `json:"environment,omitempty"` // T3 environmentId
}
type Binding struct { // proven strong native binding (T3 → provider)
	Harness  string `json:"harness" cm:"required"`   // "claude"|"codex"
	NativeID string `json:"native_id" cm:"required"`
	Strength string `json:"strength" cm:"required"`  // only "strong" creates dedup
}
type SessionLink struct {
	Session  string `json:"session" cm:"required"`            // manifest record id ("" if unknown locally)
	Origin   Origin `json:"origin" cm:"required"`
	Relation string `json:"relation" cm:"required"`           // "fork"|"subagent"|"continuation"|"view_of"|"clear_successor"
}
type CapturePolicy struct {
	Version     int    `json:"version" cm:"required"`          // policy version number
	Redaction   string `json:"redaction" cm:"required"`        // "redact/v1"
	ToolDetail  string `json:"tool_detail" cm:"required"`      // "metadata"|"full"
	Attachments bool   `json:"attachments" cm:"required"`      // always false in this PR
}
type Gap struct {
	From   string `json:"from,omitempty"` // event id or position label
	To     string `json:"to,omitempty"`
	Reason string `json:"reason" cm:"required"`         // see OmissionReasons
	Count  int    `json:"count" cm:"required"`
}
type SessionManifest struct {
	SessionKey string        `json:"session_key" cm:"required"`   // lowercase hex sha256 origin key (§6.2)
	ProjectID  string        `json:"project_id" cm:"required"`
	Origin     Origin        `json:"origin" cm:"required"`
	Adapter    string        `json:"adapter" cm:"required"`       // e.g. "claude-jsonl/1"
	HarnessVersion string    `json:"harness_version,omitempty"`
	Title      string        `json:"title,omitempty"` // <=512
	Branch     string        `json:"branch,omitempty"` // git branch <=256
	Parent     *SessionLink  `json:"parent,omitempty"`
	Links      []SessionLink `json:"links,omitempty"`    // <=64
	Bindings   []Binding     `json:"bindings,omitempty"` // <=16
	Policy     CapturePolicy `json:"policy" cm:"required"`
	State      string        `json:"state" cm:"required"`         // "open"|"idle"|"closed"|"unknown"
	ChunkHigh  map[string]int64 `json:"chunk_high" cm:"required"` // branch -> highest committed seq (-1 none)
	Gaps       []Gap         `json:"gaps,omitempty"`  // <=256, merged
	Continues  string        `json:"continues,omitempty"` // previous manifest record when revision cap hit
}

type Part struct{ Group string `json:"group" cm:"required"`; Index int `json:"index" cm:"required"`; Count int `json:"count" cm:"required"` }
type Event struct {
	ID         string `json:"id" cm:"required"`                    // stable event id <=256 (§6.2)
	NativeID   string `json:"native_id,omitempty"`
	Role       string `json:"role" cm:"required"`                  // "user"|"assistant"|"system"|"tool"
	Type       string `json:"type" cm:"required"`                  // "message"|"tool_call"|"tool_result"|"reasoning"|"compaction"|"correction"|"meta"
	Time       string `json:"time,omitempty"`
	Parent     string `json:"parent,omitempty"`
	Turn       string `json:"turn,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`   // <=256
	ToolStatus string `json:"tool_status,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	Text       string `json:"text,omitempty"`        // allowed, redacted content, <=512 KiB
	Part       *Part  `json:"part,omitempty"`
	ViewOf     string `json:"view_of,omitempty"`     // duplicate-view event id (Codex prompt pairing, T3 inherited)
	Retracts   []string `json:"retracts,omitempty"`  // correction events: retracted event ids (rollback/rewrite)
	Origin     string `json:"origin,omitempty"`      // "capsule_recall" for Capsule tool results (excluded from evidence)
	Sidechain  bool   `json:"sidechain,omitempty"`
}
type Omission struct { Event string `json:"event,omitempty"`; Reason string `json:"reason" cm:"required"`; Bytes int64 `json:"bytes" cm:"required"` }
type Redaction struct { Event string `json:"event" cm:"required"`; Offset int `json:"offset" cm:"required"`; Length int `json:"length" cm:"required"`; Class string `json:"class" cm:"required"` }
var OmissionReasons = []string{"tool_body_excluded","attachment_excluded","spilled_tool_output","ambiguous_project",
	"excluded_project","too_large","unsupported_type","paused_skipped","source_lost","duplicate_view","inherited_row",
	"session_excluded","transport_truncated","backfill_time_excluded","future_baseline_excluded","rewrite_unresolved","derive_inputs_incomplete"}
type SessionChunk struct {
	Session    string      `json:"session" cm:"required"`      // manifest record id
	Branch     string      `json:"branch" cm:"required"`       // "main" or branch id
	Seq        int64       `json:"seq" cm:"required"`          // 0.., contiguous per branch
	Prev       string      `json:"prev,omitempty"` // previous chunk record id in branch
	Generation string      `json:"generation" cm:"required"`   // observed source generation
	Events     []Event     `json:"events" cm:"required"`       // 1..4096
	Omissions  []Omission  `json:"omissions,omitempty"`
	Redactions []Redaction `json:"redactions,omitempty"`
	Digest     string      `json:"digest" cm:"required"`       // sha256 hex of json.Marshal(Events) (inside encryption)
}

type SourceRef struct {
	Record    string `json:"record" cm:"required"`
	Revision  string `json:"revision" cm:"required"`
	Kind      string `json:"kind" cm:"required"`
	EventFrom string `json:"event_from,omitempty"`
	EventTo   string `json:"event_to,omitempty"`
}
type Generator struct {
	Model string `json:"model" cm:"required"`; PromptVersion string `json:"prompt_version" cm:"required"`; PolicyVersion string `json:"policy_version" cm:"required"`
	Device string `json:"device" cm:"required"`; JobKey string `json:"job_key" cm:"required"`
}
type Finding struct {
	Kind     string      `json:"kind" cm:"required"`     // "goal"|"work"|"claimed_outcome"|"evidenced_outcome"|"decision"|"open_question"|"lesson"|"contradiction"
	Text     string      `json:"text" cm:"required"`     // <=4096
	Evidence []SourceRef `json:"evidence" cm:"required"` // >=1 except open_question
}
type Summary struct {
	Influence []SourceRef `json:"influence" cm:"required"`
	InputFingerprint string `json:"input_fingerprint" cm:"required"`
	Level       string      `json:"level" cm:"required"`   // "window"|"session"|"project_day"|"personal_week"
	Subject     string      `json:"subject" cm:"required"` // session record id | project id | personal scope id
	PeriodStart string      `json:"period_start,omitempty"`
	PeriodEnd   string      `json:"period_end,omitempty"`
	Findings    []Finding   `json:"findings" cm:"required"`  // <=256
	Sources     []SourceRef `json:"sources" cm:"required"`   // cited sources (also intermediate summaries)
	Primary     []SourceRef `json:"primary" cm:"required"`   // route to primary evidence (chunks), never empty for rollups
	SourceHeads []SourceRef `json:"source_heads" cm:"required"` // exact head set used
	Gaps        []Gap       `json:"gaps,omitempty"`
	Complete    bool        `json:"complete" cm:"required"`
	Generator   Generator   `json:"generator" cm:"required"`
}
type Insight struct {

	Influence []SourceRef `json:"influence,omitempty"`
	Type           string      `json:"type" cm:"required"`      // "memory"|"decision"
	Category       string      `json:"category" cm:"required"`  // "fact"|"preference"|"constraint"|"lesson"|"choice"|"idea"|"rejected_alternative"
	Assertion      string      `json:"assertion" cm:"required"` // <=4096
	ProjectID      string      `json:"project_id" cm:"required"`
	Evidence       []SourceRef `json:"evidence" cm:"required"`  // >=1
	Contradictions []SourceRef `json:"contradictions,omitempty"`
	Basis          string      `json:"basis" cm:"required"`     // "explicit"|"inferred"
	DecisionState  string      `json:"decision_state,omitempty"` // decisions only: "proposed"|"accepted"|"superseded"|"rejected"
	Verification   string      `json:"verification" cm:"required"` // "none"|"claimed"|"evidenced"
	Review         string      `json:"review" cm:"required"`    // "candidate"|"accepted"|"rejected"|"edited"
	AutoAccepted   bool        `json:"auto_accepted,omitempty"` // narrow policy label (§8.6)
	Reviewer       string      `json:"reviewer,omitempty"`      // device id of human review
	CandidateKey   string      `json:"candidate_key" cm:"required"`           // §8.5
	Supersedes     []string    `json:"supersedes,omitempty"`
	Generator      *Generator  `json:"generator,omitempty"`     // nil for human-authored
	OwnerProof *OwnerProof `json:"owner_proof,omitempty"`
}
type ScopedRef struct { Vault string `json:"vault" cm:"required"`; Scope string `json:"scope" cm:"required"`; Record string `json:"record" cm:"required"`; Revision string `json:"revision,omitempty"` }
type Attestation struct {
	Bridge       string `json:"bridge" cm:"required"`        // bridge device id
	VerifiedHeads []ScopeHead `json:"verified_heads" cm:"required"` // source replay head cursor (decimal)
	VerifiedAt   string `json:"verified_at" cm:"required"`
	Coverage     string `json:"coverage" cm:"required"`      // "complete"|"partial"
}
type Projection struct {
	Generation int64 `json:"generation" cm:"required"`
	OwnerWithdrawn bool `json:"owner_withdrawn,omitempty"`
 WithdrawalMarker bool `json:"withdrawal_marker,omitempty"`
	LinkID       string      `json:"link_id" cm:"required"`
	Role         string      `json:"role" cm:"required"`     // "source"|"target"
	Source       ScopedRef   `json:"source" cm:"required"`
	Dependencies []SourceRef `json:"dependencies" cm:"required"`
	Target       ScopedRef   `json:"target" cm:"required"`
	TextSHA256   string      `json:"text_sha256" cm:"required"`
	State        string      `json:"state" cm:"required"`    // "pending"|"projected"|"withdrawn"|"hidden"
	Attestation  *Attestation `json:"attestation,omitempty"`
	Grant        string      `json:"grant,omitempty"` // bridge_grant record id (target role)
}
type BridgeGrantBody struct { // signed payload
	Version      int      `json:"version" cm:"required"` // 1
	SourceVault  string   `json:"source_vault" cm:"required"`
	TargetVault  string   `json:"target_vault" cm:"required"`
	TargetScopes []string `json:"target_scopes" cm:"required"`
	LedgerScope  string   `json:"ledger_scope" cm:"required"`
	Bridge       string   `json:"bridge_device" cm:"required"`
	Revoked      bool     `json:"revoked" cm:"required"`
}
type BridgeGrant struct {
	Payload   string `json:"payload" cm:"required"`   // base64 json BridgeGrantBody
	Signature string `json:"signature" cm:"required"` // owner Ed25519 over "capsule-memory/bridge-grant/v1\n"+payload+"\n"
}
type Job struct {
	JobKey         string      `json:"job_key" cm:"required"`
	Kind           string      `json:"kind" cm:"required"`  // "session_digest"|"project_rollup"|"personal_rollup"
	Subject        string      `json:"subject" cm:"required"`
	Inputs         []SourceRef `json:"inputs" cm:"required"`
	AssignedDevice string      `json:"assigned_device" cm:"required"`
	Destination    string      `json:"destination" cm:"required"` // scope id
	State          string      `json:"state" cm:"required"` // "requested"|"cancelled"
}
type Suppression struct {
	SessionKey string `json:"session_key" cm:"required"`
	Origin     Origin `json:"origin" cm:"required"`
	Session    string `json:"session" cm:"required"`
	Reason     string `json:"reason" cm:"required"` // "deleted"|"excluded"
}
type Knowledge struct {
 LinkID string `json:"link_id" cm:"required"`
 Type string `json:"type" cm:"required"`
 Category string `json:"category" cm:"required"`
 Text string `json:"text" cm:"required"`
 TextSHA256 string `json:"text_sha256" cm:"required"`
 ProjectID string `json:"project_id,omitempty"`
 Source ScopedRef `json:"source" cm:"required"`
 Review string `json:"review" cm:"required"`
}
type ScopeHead struct { Scope string `json:"scope" cm:"required"`; Head string `json:"head" cm:"required"` }
type AggregationGrantBody struct { Version int `json:"version" cm:"required"`; Vault string `json:"vault_id" cm:"required"`; Name string `json:"name" cm:"required"`; Sources []string `json:"sources" cm:"required"`; Destination string `json:"destination" cm:"required"`; Revoked bool `json:"revoked" cm:"required"` }
type AggregationGrant struct { Payload string `json:"payload" cm:"required"`; Signature string `json:"signature" cm:"required"` }
