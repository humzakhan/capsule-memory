package t3

import (
	"context"
	"encoding/json"
	"time"

	"github.com/humzakhan/capsule-memory/internal/capture"
)

// A future-only baseline fetches metadata and the bounded tail, never walks old
// history. The time boundary persists across subsequent overlapping tail polls.
type position struct {
	Sequence int64  `json:"seq"`
	Walk     string `json:"walk"`
	WalkDone bool   `json:"walk_done"`
	Since    string `json:"since,omitempty"`
}

func (Adapter) Baseline(ctx context.Context, s capture.Source, st capture.Stream) (capture.Checkpoint, error) {
	c, e := config(s.Config)
	if e != nil {
		return capture.Checkpoint{}, e
	}
	tid := allIndex(st.Key)
	if !id.MatchString(tid) {
		return capture.Checkpoint{}, capture.ErrInvalid
	}
	budget := int64(4 << 20)
	if e = discoveryBudget(ctx, c, &budget); e != nil {
		return capture.Checkpoint{}, e
	}
	var snap snapshot
	if e = getBudget(ctx, c, "/api/orchestration/threads/"+tid+"/bounded", "", &snap, true, &budget); e != nil {
		return capture.Checkpoint{}, e
	}
	b, _ := json.Marshal(position{Sequence: snap.Sequence, WalkDone: true, Since: time.Now().UTC().Format(time.RFC3339Nano)})
	return capture.Checkpoint{Stream: st.Key, Generation: c.Environment, Position: string(b)}, nil
}
