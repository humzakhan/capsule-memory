package memory

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"

	"github.com/humzakhan/capsule-memory/internal/enrollment"
	"github.com/humzakhan/capsule-memory/internal/ownerkey"
)

type ownerReference struct {
	Version  int    `json:"version" cm:"required"`
	Provider string `json:"provider" cm:"required"`
	Account  string `json:"account" cm:"required"`
}

type ownerSecretStore interface {
	Get(string) ([]byte, error)
	Put(string, []byte) error
}

type nativeOwnerSecrets struct{}

func (nativeOwnerSecrets) Get(account string) ([]byte, error)   { return ownerkey.Get(account) }
func (nativeOwnerSecrets) Put(account string, key []byte) error { return ownerkey.Put(account, key) }
func (s *Service) ownerSecrets() ownerSecretStore {
	if s.ownerSecretStore != nil {
		return s.ownerSecretStore
	}
	return nativeOwnerSecrets{}
}

// ProtectOwner changes custody only after the native store has been read back
// and matched to the existing pinned owner. A crash may leave both copies; a
// subsequent invocation completes the same migration. No new owner is created.
func (s *Service) ProtectOwner(ctx context.Context) error {
	if s.lockMode != LockExclusive {
		return ErrProfileBusy
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	key, err := s.ownerKey()
	if err != nil {
		return err
	}
	if err = s.ownerSecrets().Put(s.configuration().Vault, key); err != nil {
		return err
	}
	stored, err := s.ownerSecrets().Get(s.configuration().Vault)
	if err != nil || !ed25519.PrivateKey(key).Equal(ed25519.PrivateKey(stored)) {
		return errors.New("owner Keychain verification failed; private file preserved")
	}
	reference := filepath.Join(s.home, "owner-key.json")
	if _, err = os.Lstat(reference); os.IsNotExist(err) {
		err = enrollment.WritePrivate(reference, ownerReference{1, "macos-keychain", s.configuration().Vault})
	}
	if err != nil {
		return err
	}
	path := filepath.Join(s.home, "owner.json")
	if _, err = os.Lstat(path); os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = safeFile(path); err != nil {
		return err
	}
	var old struct {
		Private string `json:"private_key_b64"`
	}
	b, err := os.ReadFile(path)
	if err != nil || decode(b, &old) != nil || old.Private != base64.StdEncoding.EncodeToString(key) {
		return errors.New("owner private file differs; preserve it for manual custody review")
	}
	if err = os.Remove(path); err != nil {
		return errors.New("owner Keychain selected; old private file removal failed")
	}
	return syncDir(s.home)
}

func (s *Service) nativeOwnerKey() ([]byte, bool, error) {
	p := filepath.Join(s.home, "owner-key.json")
	if _, err := os.Lstat(p); os.IsNotExist(err) {
		return nil, false, nil
	}
	if err := safeFile(p); err != nil {
		return nil, true, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, true, err
	}
	var ref ownerReference
	if decode(b, &ref) != nil || ref.Version != 1 || ref.Provider != "macos-keychain" || ref.Account != s.configuration().Vault {
		return nil, true, ErrInvalid
	}
	key, err := s.ownerSecrets().Get(ref.Account)
	return key, true, err
}
