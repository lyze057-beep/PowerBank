package data

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-kratos/kratos-layout/internal/biz"
	"github.com/go-kratos/kratos-layout/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
)

type openAIChatClient struct {
	cfg        *conf.AI_OpenAICompatible
	httpClient *http.Client
	log        *log.Helper
}

// NewOpenAIChatClient creates OpenAI-compatible client.
func NewOpenAIChatClient(c *conf.AI, logger log.Logger) biz.ModelChatGateway {
	var cfg *conf.AI_OpenAICompatible
	if c != nil {
		cfg = c.OpenaiCompatible
	}
	if cfg == nil {
		cfg = &conf.AI_OpenAICompatible{Enabled: false}
	}

	timeout := 5 * time.Second
	if cfg.Timeout != nil && cfg.Timeout.AsDuration() > 0 {
		timeout = cfg.Timeout.AsDuration()
	}
	return &openAIChatClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: timeout},
		log:        log.NewHelper(log.With(logger, "module", "data/openai")),
	}
}

func (c *openAIChatClient) Chat(ctx context.Context, in biz.ModelChatInput) (*biz.ModelChatOutput, error) {
	if !c.cfg.Enabled {
		return nil, fmt.Errorf("openai compatible client disabled")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(c.cfg.BaseUrl), "/")
	if baseURL == "" || strings.TrimSpace(c.cfg.ApiKey) == "" || strings.TrimSpace(c.cfg.Model) == "" {
		return nil, fmt.Errorf("openai compatible config incomplete")
	}

	reqBody := map[string]any{
		"model":    c.cfg.Model,
		"messages": toOpenAIMessages(in.Messages),
	}
	if len(in.Tools) > 0 {
		reqBody["tools"] = toOpenAITools(in.Tools)
	}
	if c.cfg.MaxTokens > 0 {
		reqBody["max_tokens"] = c.cfg.MaxTokens
	}
	if c.cfg.Temperature > 0 {
		reqBody["temperature"] = c.cfg.Temperature
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.ApiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai compatible status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	type chatResp struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int32 `json:"prompt_tokens"`
			CompletionTokens int32 `json:"completion_tokens"`
		} `json:"usage"`
	}
	var parsed chatResp
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("openai compatible no choices")
	}

	choice := parsed.Choices[0].Message
	reply := strings.TrimSpace(choice.Content)

	var toolCalls []biz.ToolCall
	for _, tc := range choice.ToolCalls {
		if tc.Type == "function" {
			toolCalls = append(toolCalls, biz.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
	}

	return &biz.ModelChatOutput{
		Reply:            reply,
		ToolCalls:        toolCalls,
		Model:            parsed.Model,
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
	}, nil
}

func toOpenAIMessages(messages []biz.ModelMessage) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		m := map[string]any{
			"role":    msg.Role,
			"content": msg.Content,
		}
		out = append(out, m)
	}
	return out
}

func toOpenAITools(tools []biz.ToolDefinition) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.Parameters,
			},
		})
	}
	return out
}
