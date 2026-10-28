package memory_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/projection"
	"github.com/humzakhan/capsule-memory/internal/records"
)

// Grant/enrollment must reject before disclosing the existing target key, even
// after the projected assertion is deleted and the profile opens without hooks.
func TestSameVaultArchiveConstrainsNewReaders(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "owner")
	initialized, err := memory.Init(ctx, home, "http://127.0.0.1:1", true)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := memory.Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if svc != nil {
			_ = svc.Close()
		}
	})
	source, err := svc.AddScope(ctx, "synthetic source", "project")
	if err != nil {
		t.Fatal(err)
	}
	target, err := svc.AddScope(ctx, "synthetic target", "personal")
	if err != nil {
		t.Fatal(err)
	}
	existing, err := svc.CreateDeviceScoped(ctx, filepath.Join(t.TempDir(), "existing"), []string{initialized["scope_id"].(string)})
	if err != nil {
		t.Fatal(err)
	}
	projection.Install(svc)
	id := func() string {
		value, err := memory.NewID()
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	text := "synthetic archived assertion"
	digest := sha256.Sum256([]byte(text))
	knowledge := records.Knowledge{
		LinkID: id(), Type: "memory", Category: "fact", Text: text,
		TextSHA256: hex.EncodeToString(digest[:]), Review: "accepted",
		Source: records.ScopedRef{Vault: initialized["vault_id"].(string), Scope: source.ID, Record: id(), Revision: id()},
	}
	prepared, err := svc.Prepare(ctx, memory.PutInput{
		Scope: target.ID, Kind: records.KindKnowledge, Body: knowledge,
		Provenance: memory.Provenance{SourceKind: "manual", SourceRef: "synthetic archived projection"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Commit(ctx, []memory.Prepared{prepared}, nil); err != nil {
		t.Fatal(err)
	}
	// This receipt is a synthetic fixture, not a real relay publication.
	if err = svc.Update(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE outbox SET receipt='1' WHERE revision=?", prepared.Revision.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	deleted, err := svc.Prepare(ctx, memory.PutInput{
		Scope: target.ID, Kind: records.KindKnowledge, Record: prepared.Revision.Record,
		Operation: "delete", Provenance: memory.Provenance{SourceKind: "manual", SourceRef: "synthetic deletion"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Commit(ctx, []memory.Prepared{deleted}, nil); err != nil {
		t.Fatal(err)
	}
	if err = svc.Close(); err != nil {
		t.Fatal(err)
	}
	svc, err = memory.Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately do not install projection/aggregation application hooks.
	before, err := svc.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	deniedPackage := filepath.Join(t.TempDir(), "denied-keys.json")
	if _, err = svc.GrantDevice(ctx, existing["device_id"].(string), []string{target.ID}, deniedPackage); !errors.Is(err, memory.ErrProjectionReaders) {
		t.Fatalf("target-only grant did not reject archived disclosure: %v", err)
	}
	if _, err := os.Stat(deniedPackage); !os.IsNotExist(err) {
		t.Fatal("rejected grant created a key package")
	}
	deniedProfile := filepath.Join(t.TempDir(), "denied-reader")
	if _, err = svc.CreateDeviceScoped(ctx, deniedProfile, []string{target.ID}); !errors.Is(err, memory.ErrProjectionReaders) {
		t.Fatalf("target-only enrollment did not reject archived disclosure: %v", err)
	}
	if _, err := os.Stat(deniedProfile); !os.IsNotExist(err) {
		t.Fatal("rejected enrollment created a profile")
	}
	after, err := svc.Control(ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("rejected disclosure changed owner control")
	}
	if _, err = svc.GrantDevice(ctx, existing["device_id"].(string), []string{source.ID, target.ID}, filepath.Join(t.TempDir(), "allowed-keys.json")); err != nil {
		t.Fatalf("matching source and target grant rejected: %v", err)
	}
	if _, err = svc.CreateDeviceScoped(ctx, filepath.Join(t.TempDir(), "allowed-reader"), []string{source.ID, target.ID}); err != nil {
		t.Fatalf("matching source and target enrollment rejected: %v", err)
	}
}
