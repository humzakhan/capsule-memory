package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/humzakhan/capsule-memory/internal/capture"
)

func TestTurnCWDAndDedupSurvivePageBoundary(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "rollout-synthetic.jsonl")
	lines := []string{
		`{"type":"session_meta","payload":{"id":"synthetic","cwd":"/old","cli_version":"1.0"}}`,
		`{"type":"turn_context","payload":{"turn_id":"turn","cwd":"/new"}}`,
		`{"type":"event_msg","timestamp":"2026-10-27T00:00:00Z","payload":{"type":"user_message","message":"Synthetic prompt"}}`,
		`{"type":"response_item","timestamp":"2026-10-27T00:00:00Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Synthetic prompt"}]}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(Config{Root: root, PinnedVersions: []string{"1."}})
	if err != nil {
		t.Fatal(err)
	}
	source := capture.Source{StoreIdentity: "synthetic-store", Config: raw}
	stream := capture.Stream{Key: filepath.Base(path)}
	adapter := Adapter{}
	// Stop after the first native user representation. Its paired response
	// arrives in the next page and must retain the new directory and view link.
	first, err := adapter.ReadChanges(context.Background(), source, stream, capture.Checkpoint{}, capture.Limits{MaxEvents: 3})
	if err != nil || first.Quarantine != "" || len(first.Events) != 1 || first.Events[0].CWD != "/new" {
		t.Fatalf("first page: %+v %v", first, err)
	}
	if strings.Contains(first.Next.AdapterState, "Synthetic prompt") {
		t.Fatal("prompt plaintext persisted in checkpoint")
	}
	second, err := adapter.ReadChanges(context.Background(), source, stream, first.Next, capture.Limits{})
	if err != nil || second.Quarantine != "" || len(second.Events) != 1 {
		t.Fatalf("second page: %+v %v", second, err)
	}
	if second.Events[0].CWD != "/new" || second.Events[0].Event.Text != "" || second.Events[0].Event.ViewOf != first.Events[0].Event.ID {
		t.Fatalf("turn attribution or hash-only dedup lost: %+v", second.Events[0])
	}
	// Existing checkpoint JSON remains readable, but cannot assert the old
	// header directory for a turn whose directory was never checkpointed.
	legacy := first.Next
	legacy.AdapterState = `{"version":1,"turn":"turn","users":[]}`
	unknown, err := adapter.ReadChanges(context.Background(), source, stream, legacy, capture.Limits{})
	if err != nil || unknown.Quarantine != "" || len(unknown.Events) != 0 || len(unknown.Omissions) != 1 || unknown.Omissions[0].Reason != "ambiguous_project" {
		t.Fatalf("legacy checkpoint falsely attributed: %+v %v", unknown, err)
	}
}
