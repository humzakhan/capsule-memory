// Package inference talks only to an explicitly configured local model endpoint.
package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var ErrDisabled = errors.New("local inference not configured")
var ErrBudget = errors.New("local inference budget exceeded")
var ErrResponse = errors.New("invalid local inference response")

type Config struct {
	Enabled           bool          `json:"enabled"`
	Endpoint          string        `json:"endpoint"`
	Model             string        `json:"model"`
	ResponseFormat    string        `json:"response_format,omitempty"`
	MaxInputTokens    int           `json:"max_input_tokens"`
	MaxOutputTokens   int           `json:"max_output_tokens"`
	DailyInputBudget  int           `json:"daily_input_budget"`
	DailyOutputBudget int           `json:"daily_output_budget"`
	Timeout           time.Duration `json:"timeout"`
	MaxResponseBytes  int           `json:"max_response_bytes"`
	CPUHint           string        `json:"cpu_hint"`
	RAMHint           string        `json:"ram_hint"`
}
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type Request struct {
	System          string
	Messages        []Message
	Schema          json.RawMessage
	SchemaName      string
	MaxOutputTokens int
}
type Response struct {
	Content                   string
	InputTokens, OutputTokens int
}
type Client interface {
	Complete(context.Context, Request) (Response, error)
}
type httpClient struct {
	cfg    Config
	client *http.Client
}

// EstimateTokens is a byte/4 estimate, not a tokenizer or a context guarantee.
func EstimateTokens(s string) int { return (len(s) + 3) / 4 }
func Defaults(cfg Config) Config {
	if cfg.ResponseFormat == "" {
		cfg.ResponseFormat = "json_schema"
	}
	if cfg.MaxInputTokens <= 0 {
		cfg.MaxInputTokens = 8192
	}
	if cfg.MaxOutputTokens <= 0 {
		cfg.MaxOutputTokens = 2048
	}
	if cfg.DailyInputBudget <= 0 {
		cfg.DailyInputBudget = 100000
	}
	if cfg.DailyOutputBudget <= 0 {
		cfg.DailyOutputBudget = 20000
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Minute
	}
	if cfg.MaxResponseBytes <= 0 {
		cfg.MaxResponseBytes = 1 << 20
	}
	return cfg
}
func Limits(c Client) Config {
	if h, ok := c.(*httpClient); ok {
		return h.cfg
	}
	return Defaults(Config{Enabled: c != nil})
}
func NewHTTP(cfg Config) (Client, error) {
	cfg = Defaults(cfg)
	if !cfg.Enabled {
		return nil, ErrDisabled
	}
	if cfg.ResponseFormat != "json_schema" && cfg.ResponseFormat != "prompt_json" {
		return nil, ErrResponse
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() == "" || cfg.Model == "" || len(cfg.Model) > 256 {
		return nil, ErrResponse
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, ErrResponse
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, ErrResponse
	}
	if cfg.MaxInputTokens > 262144 || cfg.MaxOutputTokens > 32768 || cfg.MaxResponseBytes > 4<<20 || cfg.Timeout > 10*time.Minute {
		return nil, ErrBudget
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: func(network, address string, conn syscall.RawConn) error {
		h, _, e := net.SplitHostPort(address)
		if e != nil {
			return ErrResponse
		}
		resolved := net.ParseIP(h)
		if resolved == nil || !resolved.IsLoopback() {
			return ErrResponse
		}
		return nil
	}}
	transport := &http.Transport{Proxy: nil, DialContext: dialer.DialContext, MaxConnsPerHost: 1, MaxIdleConns: 1, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: cfg.Timeout, DisableCompression: true}
	return &httpClient{cfg: cfg, client: &http.Client{Transport: transport, Timeout: cfg.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("local inference redirects forbidden") }}}, nil
}
func (h *httpClient) Complete(ctx context.Context, r Request) (Response, error) {
	if !h.cfg.Enabled {
		return Response{}, ErrDisabled
	}
	if r.System == "" || len(r.Messages) == 0 || len(r.Messages) > 64 || !json.Valid(r.Schema) || len(r.Schema) > 32768 || r.SchemaName == "" {
		return Response{}, ErrResponse
	}
	input := EstimateTokens(r.System) + EstimateTokens(string(r.Schema))
	system := r.System
	var format any
	if h.cfg.ResponseFormat == "prompt_json" {
		// Some local backends cannot constrain the answer channel correctly.
		// The owner may explicitly request a schema in the system prompt instead;
		// the derivation layer still validates every field and exact citation.
		system += "\nRequired output JSON schema:\n" + string(r.Schema)
		input = EstimateTokens(system)
	} else {
		format = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": r.SchemaName, "strict": true, "schema": r.Schema}}
	}
	messages := []Message{{Role: "system", Content: system}}
	for _, m := range r.Messages {
		if m.Role != "user" && m.Role != "assistant" {
			return Response{}, ErrResponse
		}
		input += EstimateTokens(m.Content) + 8
		messages = append(messages, m)
	}
	out := r.MaxOutputTokens
	if out <= 0 {
		out = h.cfg.MaxOutputTokens
	}
	if input > h.cfg.MaxInputTokens || out > h.cfg.MaxOutputTokens {
		return Response{}, ErrBudget
	}
	body := struct {
		Model       string    `json:"model"`
		Messages    []Message `json:"messages"`
		MaxTokens   int       `json:"max_tokens"`
		Temperature int       `json:"temperature"`
		Stream      bool      `json:"stream"`
		Format      any       `json:"response_format,omitempty"`
	}{h.cfg.Model, messages, out, 0, false, format}
	b, err := json.Marshal(body)
	if err != nil {
		return Response{}, ErrResponse
	}
	ctx, cancel := context.WithTimeout(ctx, h.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.Endpoint, bytes.NewReader(b))
	if err != nil {
		return Response{}, ErrResponse
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return Response{}, errors.New("local inference request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Response{}, errors.New("local inference endpoint refused request")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(h.cfg.MaxResponseBytes)+1))
	if err != nil || len(data) > h.cfg.MaxResponseBytes {
		return Response{}, ErrResponse
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Role         string          `json:"role"`
				Content      string          `json:"content"`
				ToolCalls    json.RawMessage `json:"tool_calls"`
				FunctionCall json.RawMessage `json:"function_call"`
			} `json:"message"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			Input  int `json:"prompt_tokens"`
			Output int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &envelope) != nil || len(envelope.Choices) != 1 {
		return Response{}, ErrResponse
	}
	choice := envelope.Choices[0]
	if choice.Finish != "stop" || choice.Message.Role != "assistant" || len(choice.Message.ToolCalls) > 0 && string(choice.Message.ToolCalls) != "null" && string(choice.Message.ToolCalls) != "[]" || len(choice.Message.FunctionCall) > 0 && string(choice.Message.FunctionCall) != "null" || strings.TrimSpace(choice.Message.Content) == "" {
		return Response{}, ErrResponse
	}
	result := Response{Content: choice.Message.Content, InputTokens: input, OutputTokens: EstimateTokens(choice.Message.Content)}
	if envelope.Usage != nil {
		if envelope.Usage.Input < 0 || envelope.Usage.Output < 0 {
			return Response{}, ErrResponse
		}
		if envelope.Usage.Input > result.InputTokens {
			result.InputTokens = envelope.Usage.Input
		}
		if envelope.Usage.Output > result.OutputTokens {
			result.OutputTokens = envelope.Usage.Output
		}
	}
	if result.InputTokens > h.cfg.MaxInputTokens || result.OutputTokens > out {
		return Response{}, ErrBudget
	}
	return result, nil
}
