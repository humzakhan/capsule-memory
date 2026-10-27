package session

type Policy struct {
	Capture bool `json:"capture"`
	Publish bool `json:"publish"`
	Derive bool `json:"derive"`
	ToolDetail string `json:"tool_detail"`
	RedactionExtra []string `json:"redaction_extra"`
	SessionExclusions []string `json:"session_exclusions"`
	AutoAcceptProjectFacts bool `json:"auto_accept_project_facts"`
	CrossVault string `json:"cross_vault"`
	Aggregations []string `json:"aggregations"`
	RollupDevice string `json:"rollup_device"`
	PublishScopes []string `json:"publish_scopes"`
	Version int `json:"version"`
}
