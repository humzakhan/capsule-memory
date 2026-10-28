package session

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/humzakhan/capsule-memory/internal/capture"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/records"
)

func TestRetrievalSkipsSupersededChunkBeforeLimit(t *testing.T) {
	m, adapter, source, stream := pollFixture(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mapped-project")
	if _, err := m.MapProject(ctx, path, "", "synthetic"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	adapter.read = func(cp capture.Checkpoint) (capture.Changes, error) {
		calls++
		text := "needle"
		if calls == 2 {
			text = "needle with additional synthetic context for the second chunk"
		}
		cp.Position = strconv.Itoa(calls)
		return capture.Changes{Next: cp, CaughtUp: true,
			Observations: []capture.Observation{{Origin: records.Origin{Harness: "codex", SourceStore: source.StoreIdentity, NativeSessionID: "synthetic-session"}, CWD: path, State: "open"}},
			Events: []capture.Event{{Origin: records.Origin{Harness: "codex", SourceStore: source.StoreIdentity, NativeSessionID: "synthetic-session"}, CWD: path, PositionKey: cp.Position,
				Event: records.Event{ID: "event-" + cp.Position, Role: "user", Type: "message", Text: text, Time: time.Now().UTC().Format(time.RFC3339Nano)}}}}, nil
	}
	for i := 0; i < 2; i++ {
		if _, err := m.Poll(ctx, source, stream); err != nil {
			t.Fatal(err)
		}
	}
	var old memory.Revision
	var chunk records.SessionChunk
	var session, branch, scope string
	if err := m.svc.Read(ctx, func(q memory.Queryer) error {
		var raw []byte
		if err := q.QueryRowContext(ctx, `SELECT r.body,c.session,c.branch,r.scope FROM chunks c
			JOIN revisions r ON r.revision=c.revision WHERE c.seq=0 LIMIT 1`).Scan(&raw, &session, &branch, &scope); err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &old); err != nil {
			return err
		}
		return json.Unmarshal(old.Body, &chunk)
	}); err != nil {
		t.Fatal(err)
	}
	chunk.Events[0].Text = "updated synthetic text"
	body, err := json.Marshal(chunk.Events)
	if err != nil {
		t.Fatal(err)
	}
	chunk.Digest = hash(body)
	updated, err := m.svc.Prepare(ctx, memory.PutInput{Scope: scope, Kind: records.KindSessionChunk, Record: old.Record, Body: chunk,
		Provenance: memory.Provenance{SourceKind: "manual", SourceRef: "synthetic-correction"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.svc.Commit(ctx, []memory.Prepared{updated}, nil); err != nil {
		t.Fatal(err)
	}
	grant := Grant{Scopes: []string{scope}, Caps: []ops.Capability{ops.CapSessionRead}}
	found, err := m.Search(ctx, grant, "needle", 1)
	if err != nil || len(found.Results) != 1 || found.Results[0].Chunk == old.Record {
		t.Fatalf("historical index row consumed the result limit: %+v %v", found, err)
	}
	read, err := m.ReadSession(ctx, grant, session, branch, 0, 4<<20)
	if err != nil || len(read.Chunks) != 2 || read.Chunks[0].Events[0].Text != "updated synthetic text" || read.Next != 2 {
		t.Fatalf("historical chunk leaked or disrupted pagination: %+v %v", read, err)
	}
}
