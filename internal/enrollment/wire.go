// Package enrollment defines the additive device-pairing protocol. It never
// receives an owner's content database or an enrolling device's signing secret.
package enrollment

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/humzakhan/capsule-memory/internal/strictjson"
)

const MaxBody = 2 << 20
const MaxPlaintext = 1 << 20
const InviteDomain = "capsule-memory/enrollment-invite/v1"
const RequestDomain = "capsule-memory/enrollment-request/v1"
const DecisionDomain = "capsule-memory/enrollment-decision/v1"
const HTTPDomain = "capsule-memory/enrollment-http/v1"

var ErrInvalid = errors.New("invalid enrollment identity, signature or context")
var ErrExpired = errors.New("enrollment invitation expired; create a new invitation")
var id = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var cursorPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,127})$`)

type Signed struct {
	Payload   string `json:"payload" cm:"required"`
	Signature string `json:"signature" cm:"required"`
}

type Invitation struct {
	signedDigest string
	Version      int      `json:"version" cm:"required"`
	Vault        string   `json:"vault_id" cm:"required"`
	ID           string   `json:"invite_id" cm:"required"`
	Relay        string   `json:"relay" cm:"required"`
	Owner        string   `json:"owner_public_key_b64" cm:"required"`
	Scopes       []string `json:"scope_ids" cm:"required"`
	Issued       string   `json:"issued_at" cm:"required"`
	Expires      string   `json:"expires_at" cm:"required"`
	Loopback     bool     `json:"allow_loopback_http" cm:"required"`
}

type Request struct {
	Version          int    `json:"version" cm:"required"`
	Vault            string `json:"vault_id" cm:"required"`
	Invite           string `json:"invite_id" cm:"required"`
	InvitationDigest string `json:"invitation_sha256" cm:"required"`
	Device           string `json:"device_id" cm:"required"`
	Public           string `json:"public_key_b64" cm:"required"`
	Encryption       string `json:"encryption_key_b64" cm:"required"`
}

type Submission struct {
	Invitation Signed `json:"invitation" cm:"required"`
	Request    Signed `json:"request" cm:"required"`
}

type Decision struct {
	Version          int      `json:"version" cm:"required"`
	Vault            string   `json:"vault_id" cm:"required"`
	Invite           string   `json:"invite_id" cm:"required"`
	RequestDigest    string   `json:"request_sha256" cm:"required"`
	Action           string   `json:"action" cm:"required"`
	Scopes           []string `json:"scope_ids" cm:"required"`
	Parent           string   `json:"parent_sha256" cm:"required"`
	ArchiveCursor    string   `json:"archive_cursor" cm:"required"`
	ActivationDigest string   `json:"activation_sha256" cm:"required"`
	Control          *Signed  `json:"control,omitempty"`
	Package          string   `json:"key_package_b64" cm:"required"`
}

type Entry struct {
	Invitation     Signed  `json:"invitation" cm:"required"`
	Request        *Signed `json:"request,omitempty"`
	State          string  `json:"state" cm:"required"`
	DecisionDigest string  `json:"decision_sha256" cm:"required"`
}

type Inbox struct {
	Entries []Entry `json:"entries" cm:"required"`
}

type Receipt struct {
	RequestClosed bool    `json:"request_closed,omitempty"`
	State         string  `json:"state" cm:"required"`
	Decision      *Signed `json:"decision,omitempty"`
	Cursor        string  `json:"cursor" cm:"required"`
	Manifest      *Signed `json:"manifest,omitempty"`
}

func RandomID() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func DecodeKey(s string, size int) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != size || base64.StdEncoding.EncodeToString(b) != s {
		return nil, ErrInvalid
	}
	return b, nil
}

func Sign(domain string, value any, key ed25519.PrivateKey) (Signed, error) {
	if len(key) != ed25519.PrivateKeySize {
		return Signed{}, ErrInvalid
	}
	b, err := json.Marshal(value)
	if err != nil || len(b) > MaxBody {
		return Signed{}, ErrInvalid
	}
	payload := base64.StdEncoding.EncodeToString(b)
	sig := ed25519.Sign(key, []byte(domain+"\n"+payload+"\n"))
	return Signed{payload, base64.StdEncoding.EncodeToString(sig)}, nil
}

func Verify(domain string, s Signed, public string, out any) error {
	key, err := DecodeKey(public, ed25519.PublicKeySize)
	if err != nil {
		return err
	}
	sig, err := DecodeKey(s.Signature, ed25519.SignatureSize)
	if err != nil || len(s.Payload) > MaxBody || !ed25519.Verify(key, []byte(domain+"\n"+s.Payload+"\n"), sig) {
		return ErrInvalid
	}
	b, err := base64.StdEncoding.DecodeString(s.Payload)
	if err != nil || base64.StdEncoding.EncodeToString(b) != s.Payload || strictjson.Decode(b, MaxBody, out) != nil {
		return ErrInvalid
	}
	return nil
}

func Digest(s Signed) string {
	return Hash([]byte(s.Payload + "\n" + s.Signature + "\n"))
}
func Hash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// Fingerprint displays 128 bits of the signed request digest. Confirmation must
// compare the whole value on the joining device, never just its chosen label.
func Fingerprint(s Signed) string {
	d := strings.ToUpper(Digest(s)[:32])
	parts := make([]string, 0, 8)
	for n := 0; n < len(d); n += 4 {
		parts = append(parts, d[n:n+4])
	}
	return strings.Join(parts, "-")
}

func ValidScopes(scopes []string) bool {
	if len(scopes) == 0 || len(scopes) > 64 {
		return false
	}
	seen := map[string]bool{}
	for _, s := range scopes {
		if !id.MatchString(s) || seen[s] {
			return false
		}
		seen[s] = true
	}
	return true
}

func Unix(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 || strconv.FormatInt(n, 10) != s {
		return 0, ErrInvalid
	}
	return n, nil
}

func ParseInvitation(s Signed) (Invitation, error) {
	var hint Invitation
	b, err := base64.StdEncoding.DecodeString(s.Payload)
	if err != nil || strictjson.Decode(b, 16<<10, &hint) != nil {
		return hint, ErrInvalid
	}
	if Verify(InviteDomain, s, hint.Owner, &hint) != nil || hint.Version != 1 || !id.MatchString(hint.Vault) || !id.MatchString(hint.ID) || !ValidScopes(hint.Scopes) {
		return hint, ErrInvalid
	}
	u, err := url.Parse(hint.Relay)
	if err != nil || u.User != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return hint, ErrInvalid
	}
	if u.Scheme != "https" && !(hint.Loopback && u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")) {
		return hint, ErrInvalid
	}
	issued, e1 := Unix(hint.Issued)
	expires, e2 := Unix(hint.Expires)
	if e1 != nil || e2 != nil || expires <= issued || expires-issued > 3600 {
		return hint, ErrInvalid
	}
	hint.signedDigest = Digest(s)
	return hint, nil
}

func CheckLive(i Invitation, now time.Time) error {
	issued, e1 := Unix(i.Issued)
	expires, e2 := Unix(i.Expires)
	if e1 != nil || e2 != nil || issued > now.Unix()+300 {
		return ErrInvalid
	}
	if expires <= now.Unix() {
		return ErrExpired
	}
	return nil
}

func ParseRequest(s Signed, i Invitation) (Request, error) {
	var r Request
	b, err := base64.StdEncoding.DecodeString(s.Payload)
	if err != nil || strictjson.Decode(b, 4096, &r) != nil || Verify(RequestDomain, s, r.Public, &r) != nil {
		return r, ErrInvalid
	}
	if r.Version != 1 || r.Vault != i.Vault || r.Invite != i.ID || r.InvitationDigest != i.signedDigest || i.signedDigest == "" || !id.MatchString(r.Device) {
		return r, ErrInvalid
	}
	if _, err = DecodeKey(r.Encryption, 32); err != nil {
		return r, err
	}
	return r, nil
}

func ParseDecision(s Signed, i Invitation, request Signed) (Decision, error) {
	var d Decision
	if Verify(DecisionDomain, s, i.Owner, &d) != nil || d.Version != 1 || d.Vault != i.Vault || d.Invite != i.ID || d.RequestDigest != Digest(request) {
		return d, ErrInvalid
	}
	if d.Action == "deny" {
		if len(d.Scopes) != 0 || d.Control != nil || d.Package != "" || d.Parent != "" || d.ArchiveCursor != "" || d.ActivationDigest != "" {
			return d, ErrInvalid
		}
		return d, nil
	}
	if d.Action != "approve" || !ValidScopes(d.Scopes) || d.Control == nil || !digestPattern.MatchString(d.Parent) || !cursorPattern.MatchString(d.ArchiveCursor) || !digestPattern.MatchString(d.ActivationDigest) {
		return d, ErrInvalid
	}
	ct, err := base64.StdEncoding.DecodeString(d.Package)
	if err != nil || len(ct) < 48 || len(ct) > MaxPlaintext+128 || base64.StdEncoding.EncodeToString(ct) != d.Package {
		return d, ErrInvalid
	}
	for _, scope := range d.Scopes {
		found := false
		for _, allowed := range i.Scopes {
			if scope == allowed {
				found = true
			}
		}
		if !found {
			return d, ErrInvalid
		}
	}
	return d, nil
}
