package llm

import (
	"context"
	"strings"
)

// DeepSeek defaults, kept for the one-time backfill of the legacy
// llm_deepseek_api_key setting into an LLM profile.
const (
	DeepSeekBaseURL      = "https://api.deepseek.com/v1"
	DeepSeekDefaultModel = "deepseek-chat"
)

// DeepSeekModels are the models exposed by the DeepSeek endpoint.
var DeepSeekModels = []string{"deepseek-chat", "deepseek-reasoner"}

// Result holds an LLM completion result.
type Result struct {
	Content string
	Usage   Usage
}

// Usage holds token/telemetry details for a completion.
type Usage struct {
	Model                string
	RequestID            string
	FinishReason         string
	PromptTokens         int
	CompletionTokens     int
	TotalTokens          int
	PromptCacheHitTokens int
	ReasoningTokens      int
}

// Client is a universal OpenAI-compatible LLM client.
type Client interface {
	// Complete runs a normal generation (for summary: markdown, no imposed JSON).
	Complete(ctx context.Context, systemPrompt, userText string) (Result, error)
	// CompleteDeterministic runs with the given temperature and imposes
	// response_format: json_object.
	CompleteDeterministic(ctx context.Context, systemPrompt, userText string, temp float64) (Result, error)
}

// cleanContent strips markdown code fences from LLM responses.
// Handles: ```json ... ```, ``` ... ```, ```text``` (inline), or no fences.
func cleanContent(raw string) string {
	s := strings.TrimSpace(raw)

	// Quick check: both opening and closing fences present?
	if !strings.HasPrefix(s, "```") || !strings.HasSuffix(s, "```") {
		return s
	}
	// Must have at least 7 chars (``` + 1 content char + ```)
	if len(s) <= 6 {
		return s
	}

	// Strip opening ``` and optional language tag line
	rest := s[3:]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[nl+1:] // skip "json", "python", etc.
	}

	// Strip closing ```
	rest, _ = strings.CutSuffix(rest, "```")

	return strings.TrimSpace(rest)
}
