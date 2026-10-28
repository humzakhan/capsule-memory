package enrollment

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/humzakhan/capsule-memory/internal/strictjson"
)

type Client struct{ Invitation Invitation }

// Call signs fresh HTTP authentication while preserving the exact persisted
// logical body on retries. Authentication never follows redirects.
func (c Client) Call(ctx context.Context, method, path, subject string, key ed25519.PrivateKey, value, out any) error {
	var body []byte
	var err error
	if value != nil {
		body, err = json.Marshal(value)
		if err != nil || len(body) > MaxBody {
			return ErrInvalid
		}
	}
	base := "/v1/vaults/" + c.Invitation.Vault + "/enrollment"
	req, err := http.NewRequestWithContext(ctx, method, c.Invitation.Relay+base+path, bytes.NewReader(body))
	if err != nil {
		return errors.New("construct enrollment request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	if subject != "" {
		if len(key) != ed25519.PrivateKeySize {
			return ErrInvalid
		}
		nonce := make([]byte, 24)
		if _, err = rand.Read(nonce); err != nil {
			return err
		}
		n := hex.EncodeToString(nonce)
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		h := sha256.Sum256(body)
		input := HTTPDomain + "\n" + method + "\n" + req.URL.RequestURI() + "\n" + c.Invitation.Vault + "\n" + subject + "\n" + ts + "\n" + n + "\n" + hex.EncodeToString(h[:]) + "\n"
		req.Header.Set("X-CM-Subject", subject)
		req.Header.Set("X-CM-Time", ts)
		req.Header.Set("X-CM-Nonce", n)
		req.Header.Set("X-CM-Signature", base64.StdEncoding.EncodeToString(ed25519.Sign(key, []byte(input))))
	}
	client := http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return errors.New("enrollment transport failed; preserve local state and resume the same operation")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("enrollment relay rejected request (HTTP %d); preserve local state", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, MaxBody+1))
	if err != nil || strictjson.Decode(b, MaxBody, out) != nil {
		return ErrInvalid
	}
	return nil
}
