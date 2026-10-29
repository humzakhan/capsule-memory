package memory

import (
	"context"
	"path/filepath"
	"testing"
)

func TestStatsCountsVisibleMemoryRecords(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "owner")
	init, err := Init(ctx, home, "http://127.0.0.1:1", true)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := Open(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	scope := init["scope_id"].(string)
	write := func(in WriteInput) {
		t.Helper()
		if _, err := svc.Write(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	write(WriteInput{Scope: scope, Kind: "memory", Record: "visible_memory", Text: "synthetic"})
	write(WriteInput{Scope: scope, Kind: "memory", Record: "deleted_memory", Text: "synthetic"})
	write(WriteInput{Scope: scope, Record: "deleted_memory", Operation: "delete"})
	write(WriteInput{Scope: scope, Kind: "skill", Record: "skill_record", Text: "synthetic"})
	got, err := svc.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stats := got["scopes"].(map[string]any)[scope].(map[string]any)
	if stats["memories"] != 1 || stats["skills"] != 1 || stats["pending_uploads"] != 4 {
		t.Fatalf("unexpected stats: %v", stats)
	}
	if got["memories"] != 1 || got["skills"] != 1 || got["pending_uploads"] != 4 {
		t.Fatalf("unexpected totals: %v", got)
	}
	devices := got["devices"].([]map[string]any)
	if len(devices) != 1 || devices[0]["this_device"] != true {
		t.Fatalf("unexpected devices: %v", devices)
	}
	if _, exists := devices[0]["public_key_b64"]; exists {
		t.Fatal("stats should not expose keys")
	}
	write(WriteInput{Scope: scope, Record: "deleted_memory", Operation: "restore", Text: "synthetic", TextSet: true})
	got, err = svc.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stats = got["scopes"].(map[string]any)[scope].(map[string]any)
	if stats["memories"] != 2 {
		t.Fatalf("restored memory missing: %v", stats)
	}
}
