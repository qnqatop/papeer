package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockServer returns an httptest server that emulates an OpenAI-compatible
// /chat/completions endpoint. It records the last request body.
func mockServer(t *testing.T, content string, capture *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if capture != nil {
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			*capture = m
		}
		resp := map[string]any{
			"id":     "chatcmpl-test-123",
			"object": "chat.completion",
			"model":  "test-model",
			"choices": []map[string]any{
				{
					"index":         0,
					"finish_reason": "stop",
					"message":       map[string]any{"role": "assistant", "content": content},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     11,
				"completion_tokens": 22,
				"total_tokens":      33,
				"completion_tokens_details": map[string]any{
					"reasoning_tokens": 7,
				},
				"prompt_tokens_details": map[string]any{
					"cached_tokens": 5,
				},
				"prompt_cache_hit_tokens": 9,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestComplete(t *testing.T) {
	var captured map[string]any
	srv := mockServer(t, "# Summary\n\nHello.", &captured)
	defer srv.Close()

	cli := NewOpenAIClient("sk-test", srv.URL, "test-model", 0.5)
	res, err := cli.Complete(context.Background(), "system prompt", "user text")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.Content != "# Summary\n\nHello." {
		t.Errorf("content = %q", res.Content)
	}
	if res.Usage.PromptTokens != 11 || res.Usage.CompletionTokens != 22 || res.Usage.TotalTokens != 33 {
		t.Errorf("token usage = %+v", res.Usage)
	}
	if res.Usage.ReasoningTokens != 7 {
		t.Errorf("reasoning tokens = %d, want 7", res.Usage.ReasoningTokens)
	}
	if res.Usage.FinishReason != "stop" {
		t.Errorf("finish reason = %q", res.Usage.FinishReason)
	}
	if res.Usage.RequestID != "chatcmpl-test-123" {
		t.Errorf("request id = %q", res.Usage.RequestID)
	}
	// cached_tokens=5 present, so PromptCacheHitTokens should be 5 (standard path).
	if res.Usage.PromptCacheHitTokens != 5 {
		t.Errorf("prompt cache hit = %d, want 5", res.Usage.PromptCacheHitTokens)
	}

	// Temperature from constructor must be applied, no response_format imposed.
	if captured["temperature"] != 0.5 {
		t.Errorf("temperature = %v, want 0.5", captured["temperature"])
	}
	if _, ok := captured["response_format"]; ok {
		t.Errorf("Complete must not impose response_format")
	}
}

func TestCompleteDeterministic(t *testing.T) {
	var captured map[string]any
	srv := mockServer(t, `{"ok":true}`, &captured)
	defer srv.Close()

	cli := NewOpenAIClient("sk-test", srv.URL, "test-model", 0.2)
	res, err := cli.CompleteDeterministic(context.Background(), "sys", "usr", 0.0)
	if err != nil {
		t.Fatalf("CompleteDeterministic: %v", err)
	}
	if res.Content != `{"ok":true}` {
		t.Errorf("content = %q", res.Content)
	}
	if captured["temperature"] != 0.0 {
		t.Errorf("temperature = %v, want 0", captured["temperature"])
	}
	rf, ok := captured["response_format"].(map[string]any)
	if !ok || rf["type"] != "json_object" {
		t.Errorf("response_format = %v, want json_object", captured["response_format"])
	}
}

func TestComplete_PromptCacheHitFallback(t *testing.T) {
	// When cached_tokens is absent, fall back to DeepSeek's prompt_cache_hit_tokens.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id":    "id1",
			"model": "m",
			"choices": []map[string]any{
				{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": "hi"}},
			},
			"usage": map[string]any{
				"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2,
				"prompt_cache_hit_tokens": 42,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cli := NewOpenAIClient("k", srv.URL, "m", 0.2)
	res, err := cli.Complete(context.Background(), "s", "u")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.Usage.PromptCacheHitTokens != 42 {
		t.Errorf("prompt cache hit = %d, want 42", res.Usage.PromptCacheHitTokens)
	}
}

func TestComplete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"bad key"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	cli := NewOpenAIClient("k", srv.URL, "m", 0.2)
	if _, err := cli.Complete(context.Background(), "s", "u"); err == nil {
		t.Error("expected error on 401")
	}
}

func TestCleanContent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no fences", "hello world", "hello world"},
		{"json fence", "```json\n{\"a\":1}\n```", "{\"a\":1}"},
		{"no lang fence", "```\nplain text\n```", "plain text"},
		{"fence with language", "```python\nprint(1)\n```", "print(1)"},
		{"trailing newline in fence", "```json\n{\"a\":1}\n```\n", "{\"a\":1}"},
		{"only backticks no newline", "```just this```", "just this"},
		{"empty", "", ""},
		{"markdown content", "# Title\n\nParagraph.", "# Title\n\nParagraph."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanContent(tt.in)
			if got != tt.want {
				t.Errorf("cleanContent(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
