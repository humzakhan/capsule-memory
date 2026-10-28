package t3

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/humzakhan/capsule-memory/internal/capture"
)

func TestDiscoveryFailureClassification(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     int
		maxBytes   int64
		shortBody  bool
		quarantine string
	}{
		{name: "confirmed protocol mismatch", status: http.StatusOK, quarantine: "protocol_or_version_mismatch"},
		{name: "temporary unavailable", status: http.StatusServiceUnavailable},
		{name: "budget exhausted", status: http.StatusOK, maxBytes: 1},
		{name: "interrupted response body", status: http.StatusOK, shortBody: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/.well-known/t3/environment" {
					t.Errorf("unexpected route: %s", r.URL.Path)
				}
				if test.shortBody {
					w.Header().Set("Content-Length", "4096")
				}
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(`{"environmentId":"synthetic","orchestrationProtocolVersion":3,"serverVersion":"1.0"}`))
			}))
			defer server.Close()
			raw, err := json.Marshal(Config{BaseURL: server.URL, Environment: "synthetic", Protocol: 2, Pinned: []string{"1."}})
			if err != nil {
				t.Fatal(err)
			}
			source := capture.Source{Config: raw}
			changes, err := (Adapter{}).ReadChanges(context.Background(), source,
				capture.Stream{Key: "env:synthetic/thread:thread", Generation: "synthetic"},
				capture.Checkpoint{}, capture.Limits{MaxBytes: test.maxBytes})
			if err == nil || changes.Quarantine != test.quarantine {
				t.Fatalf("classification: quarantine=%q err=%v", changes.Quarantine, err)
			}
		})
	}
}
