package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/humzakhan/capsule-memory/internal/memory"
)

func TestClientAcceptsNullAcknowledgementAndChecksEnvelope(t *testing.T) {
	connection, peer := net.Pipe()
	client := &Client{conn: connection, reader: bufio.NewReader(connection), name: "synthetic", token: "synthetic"}
	t.Cleanup(func() { _ = client.Close(); _ = peer.Close() })
	finished := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(peer)
		for _, frame := range []string{
			`{"v":1,"id":"req","ok":true,"result":null}`,
			`{"v":1,"id":"req","ok":true,"result":{"ready":true}}`,
			`{"v":1,"id":"req","ok":true,"result":null,"unexpected":true}`,
		} {
			if _, err := reader.ReadBytes('\n'); err != nil {
				finished <- err
				return
			}
			if _, err := peer.Write([]byte(frame + "\n")); err != nil {
				finished <- err
				return
			}
		}
		finished <- nil
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result json.RawMessage
	if err := client.Call(ctx, "admin.source.enable", map[string]any{}, &result); err != nil {
		t.Fatal("successful empty acknowledgement rejected:", err)
	}
	if string(result) != "null" {
		t.Fatal("empty acknowledgement payload changed")
	}
	var status struct{ Ready bool }
	if err := client.Call(ctx, "status.get", map[string]any{}, &status); err != nil || !status.Ready {
		t.Fatal("valid acknowledgement prevented the next operation:", err)
	}
	if err := client.Call(ctx, "admin.source.disable", map[string]any{}, nil); !errors.Is(err, memory.ErrInvalid) {
		t.Fatal("unknown response envelope field accepted")
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}
