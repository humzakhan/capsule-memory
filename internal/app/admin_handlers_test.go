package app

import (
	"encoding/json"
	"testing"

	"github.com/humzakhan/capsule-memory/internal/session"
)

// JSON escapes must not bypass the independent cross-vault acknowledgement.
func TestCrossVaultPolicyRequiresAcknowledgementAfterDecoding(t *testing.T) {
	policy := session.Policy{Version: 1, CrossVault: "off", ToolDetail: "metadata"}
	patch := json.RawMessage(`{"cross_vault":"enabled_\u0075naccepted"}`)
	if err := applyPolicyPatch(&policy, patch, false); err == nil {
		t.Fatal("cross-vault policy accepted without acknowledgement")
	}
	if policy.CrossVault != "off" {
		t.Fatal("rejected patch changed policy")
	}
	if err := applyPolicyPatch(&policy, patch, true); err != nil {
		t.Fatal(err)
	}
	if policy.CrossVault != "enabled_unaccepted" {
		t.Fatal("acknowledged patch was not applied")
	}
	if err := applyPolicyPatch(&policy, json.RawMessage(`{"publish":null}`), true); err == nil {
		t.Fatal("null patch field accepted while preserving internal nil slices")
	}
	if policy.CrossVault != "enabled_unaccepted" || policy.Publish {
		t.Fatal("rejected null field changed policy")
	}
}
