package inference

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalResponseFormatAndFinalAnswerBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		reasoning  bool
	}{
		{name: "default constrained schema"},
		{name: "explicit prompt schema", mode: "prompt_json"},
		{name: "reasoning is not a final answer", mode: "prompt_json", reasoning: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema := json.RawMessage(`{"type":"object"}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Messages []Message       `json:"messages"`
					Format   json.RawMessage `json:"response_format"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if len(body.Messages) != 2 || body.Messages[0].Role != "system" || body.Messages[1].Content != "synthetic input" {
					t.Error("message ownership changed")
				} else if tc.mode == "prompt_json" {
					if len(body.Format) != 0 || !strings.Contains(body.Messages[0].Content, string(schema)) {
						t.Error("explicit compatibility mode did not send the schema in the system prompt")
					}
				} else if body.Messages[0].Content != "Return JSON." || !strings.Contains(string(body.Format), `"strict":true`) {
					t.Error("default schema-constrained request changed")
				}
				content := `{}`
				if tc.reasoning {
					content = ""
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
					"finish_reason": "stop", "message": map[string]any{
						"role": "assistant", "content": content, "reasoning_content": `{"untrusted":"reasoning"}`,
					},
				}}})
			}))
			defer server.Close()
			client, err := NewHTTP(Config{Enabled: true, Endpoint: server.URL, Model: "synthetic", ResponseFormat: tc.mode})
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.Complete(context.Background(), Request{
				System: "Return JSON.", Messages: []Message{{Role: "user", Content: "synthetic input"}}, Schema: schema, SchemaName: "synthetic",
			})
			if tc.reasoning {
				if !errors.Is(err, ErrResponse) {
					t.Fatal("reasoning-only response accepted:", err)
				}
			} else if err != nil || result.Content != `{}` {
				t.Fatal("final answer was not preserved:", err)
			}
		})
	}
	if _, err := NewHTTP(Config{Enabled: true, Endpoint: "http://127.0.0.1:1", Model: "synthetic", ResponseFormat: "unknown"}); !errors.Is(err, ErrResponse) {
		t.Fatal("unknown response format accepted")
	}
}
