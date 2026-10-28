package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)

type Client struct {
	conn        net.Conn
	reader      *bufio.Reader
	mu          sync.Mutex
	name, token string
	seq         uint64
	Profile     string
	Grant       ops.Grant
}

func Dial(ctx context.Context, home, name, tokenPath string) (*Client, error) {
	if !safeName.MatchString(name) {
		return nil, memory.ErrInvalid
	}
	token, err := privateRead(tokenPath, 256)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", filepath.Join(home, "run", "daemon.sock"))
	if err != nil {
		return nil, ErrUnavailable
	}
	return &Client{
		conn: conn, reader: bufio.NewReaderSize(conn, 64<<10),
		name: name, token: strings.TrimSpace(string(token)),
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil {
		return ErrUnavailable
	}
	encoded, err := c.encodeRequest(method, params)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(60 * time.Second)
	if requested, ok := ctx.Deadline(); ok && requested.Before(deadline) {
		deadline = requested
	}
	if err := c.conn.SetDeadline(deadline); err != nil {
		c.conn.Close()
		return ErrUnavailable
	}
	cancelled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(cancelled)
		_ = c.conn.SetDeadline(time.Now())
	})
	defer func() {
		// A running callback must finish before another serialized Call resets
		// the deadline; otherwise cancellation can corrupt the next request.
		if !stop() {
			<-cancelled
		}
	}()

	if _, err := c.conn.Write(append(encoded, '\n')); err != nil {
		c.conn.Close()
		return ErrUnavailable
	}
	line, err := readLine(c.reader, 8<<20)
	if err != nil {
		// An unread response cannot be correlated safely with a later call.
		c.conn.Close()
		return ErrUnavailable
	}
	response, err := decodeResponse(line)
	if err != nil || response.Version != 1 || response.ID != "req" {
		c.conn.Close()
		return memory.ErrInvalid
	}
	if !response.OK {
		if response.Error == nil {
			c.conn.Close()
			return memory.ErrInvalid
		}
		return responseError(response.Error.Code)
	}
	if result != nil && json.Unmarshal(response.Result, result) != nil {
		return memory.ErrInvalid
	}
	return nil
}

// Result is an opaque operation payload, including JSON null for a successful
// command without a return value. Keep strict validation on the envelope and
// duplicate keys throughout the original frame without rejecting that payload.
func decodeResponse(line []byte) (Response, error) {
	var response Response
	var fields map[string]json.RawMessage
	if !utf8.Valid(line) || strictjson.Unique(line) != nil || json.Unmarshal(line, &fields) != nil || fields == nil {
		return response, memory.ErrInvalid
	}
	result, hasResult := fields["result"]
	delete(fields, "result")
	envelope, err := json.Marshal(fields)
	if err != nil || strictjson.Decode(envelope, 8<<20, &response) != nil {
		return response, memory.ErrInvalid
	}
	if response.OK {
		if !hasResult || response.Error != nil {
			return response, memory.ErrInvalid
		}
	} else if hasResult || response.Error == nil {
		return response, memory.ErrInvalid
	}
	response.Result = result
	return response, nil
}

func (c *Client) encodeRequest(method string, params any) ([]byte, error) {
	encoded, err := json.Marshal(params)
	if err != nil {
		return nil, memory.ErrInvalid
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(encoded, &object) != nil || object == nil {
		return nil, memory.ErrInvalid
	}
	if c.Profile != "" {
		object["profile"], err = json.Marshal(c.Profile)
		if err != nil {
			return nil, memory.ErrInvalid
		}
		encoded, err = json.Marshal(object)
		if err != nil {
			return nil, memory.ErrInvalid
		}
	}
	request := Request{
		Version: 1, ID: "req", Client: c.name, Token: c.token,
		Method: method, Params: encoded,
	}
	encoded, err = json.Marshal(request)
	if err != nil || len(encoded) > 1<<20 {
		return nil, memory.ErrInvalid
	}
	return encoded, nil
}

func responseError(code string) error {
	switch code {
	case "forbidden":
		return ops.ErrForbidden
	case "busy":
		return memory.ErrProfileBusy
	case "invalid":
		return memory.ErrInvalid
	case "unknown_method":
		return ops.ErrUnknownMethod
	case "waiting_ancestry":
		return memory.ErrUnpublishedParent
	case "heads_changed":
		return memory.ErrHeadsChanged
	default:
		return ErrUnavailable
	}
}
