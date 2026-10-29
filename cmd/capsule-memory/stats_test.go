package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestPrintStats(t *testing.T) {
	raw := json.RawMessage(`{"capture_status":"not reported","scopes":{"scope_b":{"memories":3,"pending_uploads":1,"coverage":{"cursor":"2","head":"3","complete":false}},"scope_a":{"memories":0,"pending_uploads":0,"coverage":{"cursor":"0","head":"0","complete":true}}}}`)
	var out bytes.Buffer
	if err := printStats(&out, raw); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"CAPSULE MEMORY", "scope_a", "scope_b", "Memories 3", "Incomplete", "Not reported", "Local snapshot"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q: %s", want, s)
		}
	}
	if strings.Index(s, "scope_a") > strings.Index(s, "scope_b") {
		t.Fatal("collections are not sorted")
	}
}
func TestPrintStatsCapture(t *testing.T) {
	raw := json.RawMessage(`{"capture_status":{"source_states":{"enabled":2},"saved_chunks":5,"pending_streams":1},"scopes":{}}`)
	var out bytes.Buffer
	if err := printStats(&out, raw); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"5 saved chunks", "1 pending streams", "enabled: 2"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestPrintStatsFleetAndUnknownConnection(t *testing.T) {
	raw := json.RawMessage(`{"memories":3,"skills":2,"pending_uploads":1,"capture_status":"not reported","devices":[{"device_id":"device_zzz","revoked":true},{"device_id":"device_aaa","this_device":true},{"device_id":"device_bbb"}],"scopes":{}}`)
	var out bytes.Buffer
	if err := printStats(&out, raw); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"CLOUDFLARE · ENCRYPTED ARCHIVE", "Unknown (no network check)", "device_aaa", "This device", "device_bbb", "Enrolled · sync unknown", "device_zzz", "Revoked", "Pending approvals  Unknown"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q: %s", want, s)
		}
	}
	if strings.Index(s, "device_aaa") > strings.Index(s, "device_zzz") {
		t.Fatal("devices are not sorted")
	}
}
