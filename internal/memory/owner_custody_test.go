package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type syntheticOwnerStore struct {
	secret                  []byte
	putError, errorAfterPut bool
}

func (s *syntheticOwnerStore) Put(_ string, key []byte) error {
	if s.putError {
		return errors.New("synthetic store unavailable")
	}
	s.secret = append([]byte{}, key...)
	return nil
}
func (s *syntheticOwnerStore) Get(string) ([]byte, error) {
	if s.errorAfterPut {
		return nil, errors.New("synthetic credential store locked")
	}
	return append([]byte{}, s.secret...), nil
}

// Validate the custody transaction without writing any real OS credential
// item or showing an unattended system prompt. Native OS behavior is separate.
func TestOwnerCustodyPreservesRecoveryOnStoreFailureAndNeverFallsBack(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "synthetic-owner")
	if _, err := Init(ctx, home, "https://example.invalid", false); err != nil {
		t.Fatal(err)
	}
	s, err := OpenWith(ctx, home, LockExclusive)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	store := &syntheticOwnerStore{putError: true}
	s.ownerSecretStore = store
	original, err := os.ReadFile(filepath.Join(home, "owner.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ProtectOwner(ctx); err == nil {
		t.Fatal("failed native write reported migration success")
	}
	if _, err = os.Stat(filepath.Join(home, "owner.json")); err != nil {
		t.Fatal("failed native write removed recovery authority")
	}
	store.putError = false
	store.errorAfterPut = true
	if err = s.ProtectOwner(ctx); err == nil {
		t.Fatal("failed native readback reported success")
	}
	if _, err = os.Stat(filepath.Join(home, "owner-key.json")); !os.IsNotExist(err) {
		t.Fatal("native reference published before verification")
	}
	store.errorAfterPut = false
	if err = s.ProtectOwner(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(home, "owner.json")); !os.IsNotExist(err) {
		t.Fatal("successful custody migration retained plaintext owner key file")
	}
	if err = os.WriteFile(filepath.Join(home, "owner.json"), original, 0600); err != nil {
		t.Fatal(err)
	}
	store.errorAfterPut = true
	if _, err = s.ownerKey(); err == nil {
		t.Fatal("locked native store silently fell back to old private file")
	}
	store.errorAfterPut = false
	if err = s.ProtectOwner(ctx); err != nil {
		t.Fatal("custody migration could not finish after a crash with both copies", err)
	}
}
