package t3

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// The same explicitly configured T3 environment has one store identity across devices.
func (Adapter) Identity(raw json.RawMessage) (string, error) {
	c, e := config(raw)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256([]byte("capsule-memory/t3-store/v1\n" + c.Environment + "\n"))
	return hex.EncodeToString(h[:]), nil
}
