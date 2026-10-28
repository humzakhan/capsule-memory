package memory

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUncertainEnrollmentFencesLegacyAndModuleWritesAfterRestart(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "synthetic-owner")
	initialized, err := Init(ctx, home, "https://example.invalid", false)
	if err != nil {
		t.Fatal(err)
	}
	s, err := OpenWith(ctx, home, LockExclusive)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the durable reservation left by a crashed approval. Even a
	// damaged intent must fail closed rather than release the writer fence.
	if err = s.Update(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, "INSERT INTO meta(key,value) VALUES('enrollment_intent','{}')")
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenWith(ctx, home, LockExclusive)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Write(ctx, WriteInput{Scope: initialized["scope_id"].(string), Text: "synthetic blocked write"}); !errors.Is(err, ErrEnrollmentPending) {
		t.Fatalf("legacy write bypassed durable fence: %v", err)
	}
	if err = s.Rebuild(ctx); !errors.Is(err, ErrEnrollmentPending) {
		t.Fatalf("rebuild bypassed durable fence: %v", err)
	}
	if err = s.Update(ctx, func(*sql.Tx) error { return nil }); !errors.Is(err, ErrEnrollmentPending) {
		t.Fatalf("module write bypassed durable fence: %v", err)
	}
	if _, err = s.AddScope(ctx, "synthetic denied scope", "project"); !errors.Is(err, ErrEnrollmentPending) {
		t.Fatalf("owner scope mutation bypassed fence: %v", err)
	}
	if _, err = os.Stat(filepath.Join(home, "pending-scope.json")); !os.IsNotExist(err) {
		t.Fatal("blocked scope operation left conflicting recovery state")
	}
}
