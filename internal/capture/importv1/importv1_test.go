package importv1

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/capture/filetail"
)

func TestPartialRowRewindPreservesRewriteDetection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.jsonl")
	header := `{"capsule_import":1,"kind":"session","harness":"codex","source_store_id":"synthetic","session_id":"session","cwd":"/old"}` + "\n"
	partial := `{"capsule_import":1,"kind":"event","session_id":"session","role":"assistant","type":"message","status":"partial","text":"draft"}` + "\n"
	content := header + partial
	past := time.Now().Add(-time.Hour)
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, past, past); err != nil {
			t.Fatal(err)
		}
	}
	write(content)
	raw, err := json.Marshal(Config{Files: []string{path}})
	if err != nil {
		t.Fatal(err)
	}
	source := capture.Source{Config: raw}
	stream := capture.Stream{Key: path}
	adapter := Adapter{}
	first, err := adapter.ReadChanges(context.Background(), source, stream, capture.Checkpoint{}, capture.Limits{})
	if err != nil || first.Quarantine != "" || first.Next.Position != strconv.Itoa(len(header)) || first.CaughtUp || len(first.Events) != 0 {
		t.Fatalf("partial acknowledged: %+v %v", first, err)
	}
	repeated, err := adapter.ReadChanges(context.Background(), source, stream, first.Next, capture.Limits{})
	if err != nil || repeated.Reconcile || repeated.Next.Generation != first.Next.Generation {
		t.Fatalf("unchanged partial falsely detected as rewrite: %+v %v", repeated, err)
	}
	// Same inode/mtime, but an earlier header change followed by an append:
	// size/mtime alone cannot establish an unchanged acknowledged prefix.
	write(strings.Replace(content, "/old", "/new", 1) + "\n")
	changed, err := adapter.ReadChanges(context.Background(), source, stream, first.Next, capture.Limits{})
	if err != nil || !changed.Reconcile {
		t.Fatalf("prefix rewrite missed after rewind: %+v %v", changed, err)
	}
	// A rewind using an obsolete batch snapshot must fail retryably.
	_, err = filetail.RewindCheckpoint(context.Background(), filetail.Config{Root: filepath.Dir(path)},
		capture.Stream{Key: filepath.Base(path)}, first.Next, 0)
	if err != capture.ErrUnavailable {
		t.Fatalf("obsolete snapshot accepted: %v", err)
	}
}
