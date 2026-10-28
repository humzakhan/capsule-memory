package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/ops"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)

type Request struct {
	Version int             `json:"v"`
	ID      string          `json:"id"`
	Client  string          `json:"client"`
	Token   string          `json:"token"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type WireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Response struct {
	Version int             `json:"v"`
	ID      string          `json:"id"`
	OK      bool            `json:"ok"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *WireError      `json:"error,omitempty"`
}

func readLine(reader *bufio.Reader, max int) ([]byte, error) {
	var out []byte
	for {
		part, err := reader.ReadSlice('\n')
		if len(out)+len(part) > max {
			return nil, memory.ErrInvalid
		}
		out = append(out, part...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			return nil, err
		}
		return out, nil
	}
}

func serve(ctx context.Context, home string, host *app.Host) error {
	dir := filepath.Join(home, "run")
	if err := privateDir(dir); err != nil {
		return err
	}
	path := filepath.Join(dir, "daemon.sock")
	if len(path) >= socketLimit() {
		return errors.New("daemon socket path exceeds platform limit")
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return memory.ErrInvalid
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return ErrUnavailable
	}
	defer listener.Close()
	defer os.Remove(path)
	if err := os.Chmod(path, 0600); err != nil {
		return err
	}

	run, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(run, func() { listener.Close() })
	var handlers sync.WaitGroup
	defer func() {
		cancel()
		stop()
		handlers.Wait()
	}()
	slots := make(chan struct{}, 32)
	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			if run.Err() != nil {
				return nil
			}
			return ErrUnavailable
		}
		select {
		case slots <- struct{}{}:
			handlers.Add(1)
			go func() {
				defer handlers.Done()
				defer func() { <-slots }()
				handle(run, conn, home, host)
			}()
		default:
			conn.Close()
		}
	}
}

func handle(ctx context.Context, conn *net.UnixConn, home string, host *app.Host) {
	defer conn.Close()
	if !peerAllowed(conn) {
		return
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	reader := bufio.NewReaderSize(conn, 64<<10)
	for {
		if err := conn.SetReadDeadline(time.Now().Add(time.Minute)); err != nil {
			return
		}
		line, err := readLine(reader, 1<<20)
		if err != nil {
			return
		}
		var request Request
		if strictjson.Decode(line, 1<<20, &request) != nil || request.Version != 1 || !safeName.MatchString(request.ID) {
			return
		}
		response := callRequest(ctx, home, host, request)
		encoded, err := json.Marshal(response)
		if err != nil {
			return
		}
		if len(encoded) > 8<<20 {
			encoded, _ = json.Marshal(Response{
				Version: 1, ID: request.ID,
				Error: &WireError{Code: "invalid", Message: "response exceeds size bound"},
			})
		}
		if err := conn.SetWriteDeadline(time.Now().Add(time.Minute)); err != nil {
			return
		}
		if _, err := conn.Write(append(encoded, '\n')); err != nil {
			return
		}
	}
}

func callRequest(ctx context.Context, home string, host *app.Host, request Request) Response {
	response := Response{Version: 1, ID: request.ID}
	// Reload the private grant for each request so a persistent connection does
	// not retain a revoked capability or scope grant.
	grant, err := Authenticate(home, request.Client, request.Token)
	var value any
	if err == nil {
		value, err = host.Registry.Call(ctx, grant, request.Method, request.Params)
	}
	if err == nil {
		response.Result, err = json.Marshal(value)
	}
	if err != nil {
		response.Error = operationError(err)
		return response
	}
	response.OK = true
	return response
}

func operationError(err error) *WireError {
	code := "internal"
	switch {
	case errors.Is(err, ops.ErrForbidden):
		code = "forbidden"
	case errors.Is(err, ops.ErrUnknownMethod):
		code = "unknown_method"
	case errors.Is(err, memory.ErrInvalid):
		code = "invalid"
	case errors.Is(err, memory.ErrUnpublishedParent):
		code = "waiting_ancestry"
	case errors.Is(err, memory.ErrHeadsChanged):
		code = "heads_changed"
	case errors.Is(err, memory.ErrProfileBusy), errors.Is(err, memory.ErrSyncBusy):
		code = "busy"
	case errors.Is(err, ErrUnavailable):
		code = "unavailable"
	}
	return &WireError{Code: code, Message: "operation " + code}
}
