package enrollment

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublishedSyntheticSigningAndHPKEVector(t *testing.T) {
	var vector struct {
		Notice      string `json:"notice"`
		Invitation  Signed `json:"invitation"`
		Request     Signed `json:"request"`
		Plaintext   string `json:"plaintext"`
		Ciphertext  string `json:"ciphertext_b64"`
		Fingerprint string `json:"fingerprint"`
	}
	// This public fixture is intentionally not a private enrollment file.
	b, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", "device-enrollment-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &vector); err != nil {
		t.Fatal(err)
	}
	i, err := ParseInvitation(vector.Invitation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseRequest(vector.Request, i); err != nil {
		t.Fatal(err)
	}
	device := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32))
	identity := Identity{Signing: base64.StdEncoding.EncodeToString(device), Encryption: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))}
	plain, err := Open(i, vector.Request, identity, vector.Ciphertext)
	if err != nil || string(plain) != vector.Plaintext {
		t.Fatalf("published synthetic HPKE vector changed: %v", err)
	}
	if Fingerprint(vector.Request) != vector.Fingerprint {
		t.Fatal("signed-request fingerprint bytes changed")
	}
}

func syntheticInvite(t *testing.T) (Invitation, Signed, ed25519.PrivateKey) {
	t.Helper()
	owner := ed25519.NewKeyFromSeed(make([]byte, 32))
	i := Invitation{Version: 1, Vault: "synthetic_vault_01", ID: "synthetic_invite_01", Relay: "https://example.invalid", Owner: base64.StdEncoding.EncodeToString(owner.Public().(ed25519.PublicKey)), Scopes: []string{"synthetic_scope_01"}, Issued: "1000", Expires: "1600"}
	s, err := Sign(InviteDomain, i, owner)
	if err != nil {
		t.Fatal(err)
	}
	i, err = ParseInvitation(s)
	if err != nil {
		t.Fatal(err)
	}
	return i, s, owner
}

func TestKeyDeliveryIsBoundToRecipientAndEntireRequest(t *testing.T) {
	i, _, _ := syntheticInvite(t)
	identity, request, err := NewIdentity(i)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte(`{"synthetic_scope_key":"not a production secret"}`)
	sealed, err := Seal(i, request, plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Open(i, request, identity, sealed)
	if err != nil || string(got) != string(plain) {
		t.Fatalf("recipient could not recover synthetic package: %v", err)
	}
	other, otherRequest, err := NewIdentity(i)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Open(i, otherRequest, other, sealed); err == nil {
		t.Fatal("another requesting device decrypted package")
	}
	tampered := []byte(sealed)
	if tampered[8] == 'A' {
		tampered[8] = 'B'
	} else {
		tampered[8] = 'A'
	}
	if _, err = Open(i, request, identity, string(tampered)); err == nil {
		t.Fatal("modified ciphertext was accepted")
	}
	r, err := ParseRequest(request, i)
	if err != nil {
		t.Fatal(err)
	}
	r.Device = "another_device_01"
	private, _ := DecodeKey(identity.Signing, 64)
	changed, err := Sign(RequestDomain, r, private)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Open(i, changed, identity, sealed); err == nil {
		t.Fatal("same recipient accepted package bound to another request")
	}
}

func TestInvitationAndDecisionRejectSubstitutionAndWidening(t *testing.T) {
	i, invitation, owner := syntheticInvite(t)
	if _, err := ParseInvitation(invitation); err != nil {
		t.Fatal(err)
	}
	identity, request, err := NewIdentity(i)
	if err != nil {
		t.Fatal(err)
	}
	r, err := ParseRequest(request, i)
	if err != nil {
		t.Fatal(err)
	}
	attacker := ed25519.NewKeyFromSeed([]byte(strings.Repeat("x", 32)))
	r.Encryption = base64.StdEncoding.EncodeToString(make([]byte, 32))
	forged, err := Sign(RequestDomain, r, attacker)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseRequest(forged, i); err == nil {
		t.Fatal("replacement encryption key without matching device proof was accepted")
	}
	if _, err = identity.Validate(i, forged); err == nil {
		t.Fatal("substituted identity survived local recovery validation")
	}
	ciphertext, err := Seal(i, request, []byte("synthetic package"))
	if err != nil {
		t.Fatal(err)
	}
	control, err := Sign("agent-memory/control/v1", map[string]any{"version": 1, "vault_id": i.Vault, "generation": "2", "scopes": []map[string]string{{"scope_id": i.Scopes[0], "key_epoch": "1"}}, "devices": []map[string]any{{"device_id": r.Device, "public_key_b64": r.Public, "scope_ids": i.Scopes, "revoked": false}}}, owner)
	if err != nil {
		t.Fatal(err)
	}
	d := Decision{ActivationDigest: strings.Repeat("b", 64), Version: 1, Vault: i.Vault, Invite: i.ID, RequestDigest: Digest(request), Action: "approve", Scopes: i.Scopes, Control: &control, Parent: strings.Repeat("a", 64), ArchiveCursor: "1", Package: ciphertext}
	valid, err := Sign(DecisionDomain, d, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseDecision(valid, i, request); err != nil {
		t.Fatal("positive decision failed before scope substitution", err)
	}
	d.Scopes = []string{"unauthorized_scope_01"}
	signed, err := Sign(DecisionDomain, d, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseDecision(signed, i, request); err == nil {
		t.Fatal("owner-signed decision widened the invitation")
	}
	if err = CheckLive(i, time.Unix(1600, 0)); err != ErrExpired {
		t.Fatalf("expiry boundary accepted: %v", err)
	}
	i.Expires = "01600"
	bad, err := Sign(InviteDomain, i, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseInvitation(bad); err == nil {
		t.Fatal("noncanonical expiry accepted")
	}
}
