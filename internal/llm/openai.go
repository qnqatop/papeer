package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// Compile-time check that OpenAIClient satisfies the Client interface.
var _ Client = (*OpenAIClient)(nil)

// OpenAIClient is a universal client over the official openai-go/v3 SDK.
// It works with any OpenAI-compatible endpoint via baseURL.
type OpenAIClient struct {
	client      openai.Client
	model       string
	temperature float32
}

// NewOpenAIClient builds a client for any OpenAI-compatible endpoint.
// An empty baseURL falls back to the SDK default (api.openai.com).
func NewOpenAIClient(token, baseURL, model string, temperature float32) *OpenAIClient {
	opts := []option.RequestOption{option.WithAPIKey(strings.TrimSpace(token))}
	if bu := strings.TrimSpace(baseURL); bu != "" {
		opts = append(opts, option.WithBaseURL(bu))
	}
	return &OpenAIClient{
		client:      openai.NewClient(opts...),
		model:       strings.TrimSpace(model),
		temperature: temperature,
	}
}

// Model returns the model this client was configured with.
func (o *OpenAIClient) Model() string { return o.model }

// Complete runs a normal (markdown) generation applying the constructor
// temperature. No response_format is imposed.
func (o *OpenAIClient) Complete(ctx context.Context, systemPrompt, userText string) (Result, error) {
	params := openai.ChatCompletionNewParams{
		Model: shared.ChatModel(o.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userText),
		},
		Temperature: openai.Float(float64(o.temperature)),
	}
	return o.run(ctx, params)
}

// CompleteDeterministic runs a generation with the given temperature and
// imposes response_format: json_object.
func (o *OpenAIClient) CompleteDeterministic(ctx context.Context, systemPrompt, userText string, temp float64) (Result, error) {
	params := openai.ChatCompletionNewParams{
		Model: shared.ChatModel(o.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userText),
		},
		Temperature: openai.Float(temp),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
		},
	}
	return o.run(ctx, params)
}

func (o *OpenAIClient) run(ctx context.Context, params openai.ChatCompletionNewParams) (Result, error) {
	resp, err := o.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return Result{}, fmt.Errorf("chat completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return Result{}, fmt.Errorf("llm returned no choices")
	}

	choice := resp.Choices[0]
	usage := Usage{
		Model:            resp.Model,
		RequestID:        resp.ID,
		FinishReason:     choice.FinishReason,
		PromptTokens:     int(resp.Usage.PromptTokens),
		CompletionTokens: int(resp.Usage.CompletionTokens),
		TotalTokens:      int(resp.Usage.TotalTokens),
		ReasoningTokens:  int(resp.Usage.CompletionTokensDetails.ReasoningTokens),
		// Standard OpenAI reports cached prompt tokens under
		// prompt_tokens_details.cached_tokens.
		PromptCacheHitTokens: int(resp.Usage.PromptTokensDetails.CachedTokens),
	}
	// DeepSeek uses a non-standard top-level `prompt_cache_hit_tokens` field
	// that the SDK does not model. Pull it from the raw JSON if present.
	if usage.PromptCacheHitTokens == 0 {
		if hit := extractPromptCacheHit(resp.Usage.RawJSON()); hit > 0 {
			usage.PromptCacheHitTokens = hit
		}
	}

	return Result{
		Content: cleanContent(choice.Message.Content),
		Usage:   usage,
	}, nil
}

// extractPromptCacheHit reads DeepSeek's non-standard prompt_cache_hit_tokens
// from the raw usage JSON. Returns 0 if absent.
func extractPromptCacheHit(raw string) int {
	if raw == "" {
		return 0
	}
	var v struct {
		PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return 0
	}
	return v.PromptCacheHitTokens
}
