package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/config"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ToolDefinition struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type CompletionResult struct {
	Content   string
	ToolCalls []ToolCall
}

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{},
	}
}

type chatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content   *string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type streamCompletionChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) Complete(ctx context.Context, cfg config.ModelConfig, messages []Message, temperature float64) (string, error) {
	result, err := c.CompleteWithTools(ctx, cfg, messages, temperature, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Content), nil
}

func (c *Client) CompleteWithTools(ctx context.Context, cfg config.ModelConfig, messages []Message, temperature float64, tools []ToolDefinition) (CompletionResult, error) {
	requestURL := chatCompletionsURL(cfg.BaseURL)
	log.Printf(
		"llm request: mode=completion model=%s url=%s messages=%d tools=%d user=%q sdk=openai-go",
		cfg.Model,
		requestURL,
		len(messages),
		len(tools),
		lastUserMessagePreview(messages),
	)

	clientOptions := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(strings.TrimRight(cfg.BaseURL, "/")),
	}
	if c != nil && c.httpClient != nil {
		clientOptions = append(clientOptions, option.WithHTTPClient(c.httpClient))
	}
	sdkClient := openai.NewClient(clientOptions...)

	params := openai.ChatCompletionNewParams{
		Model:       openai.ChatModel(cfg.Model),
		Messages:    makeSDKMessages(messages),
		Temperature: openai.Float(temperature),
	}
	if len(tools) > 0 {
		params.Tools = makeSDKTools(tools)
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.String("required"),
		}
	}

	resp, err := sdkClient.Chat.Completions.New(ctx, params)
	if err != nil {
		return CompletionResult{}, fmt.Errorf("completion request failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return CompletionResult{}, fmt.Errorf("completion response has no choices")
	}
	log.Printf(
		"llm choice: mode=completion model=%s choice=%s",
		cfg.Model,
		resp.Choices[0].RawJSON(),
	)

	reply := CompletionResult{
		Content: strings.TrimSpace(resp.Choices[0].Message.Content),
	}
	for _, call := range resp.Choices[0].Message.ToolCalls {
		switch variant := call.AsAny().(type) {
		case openai.ChatCompletionMessageFunctionToolCall:
			reply.ToolCalls = append(reply.ToolCalls, ToolCall{
				ID:        strings.TrimSpace(variant.ID),
				Name:      strings.TrimSpace(variant.Function.Name),
				Arguments: json.RawMessage(variant.Function.Arguments),
			})
		}
	}

	return reply, nil
}

func (c *Client) Stream(ctx context.Context, cfg config.ModelConfig, messages []Message, temperature float64, onDelta func(string) error) error {
	reqBody := chatCompletionRequest{
		Model:       cfg.Model,
		Messages:    messages,
		Temperature: temperature,
		Stream:      true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("encode stream request: %w", err)
	}

	requestURL := chatCompletionsURL(cfg.BaseURL)
	log.Printf(
		"llm request: mode=stream model=%s url=%s messages=%d tools=%d user=%q",
		cfg.Model,
		requestURL,
		len(messages),
		0,
		lastUserMessagePreview(messages),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create stream request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do stream request: %w", err)
	}
	defer resp.Body.Close()
	log.Printf(
		"llm response: mode=stream model=%s status=%d url=%s",
		cfg.Model,
		resp.StatusCode,
		requestURL,
	)

	if resp.StatusCode >= 400 {
		var result chatCompletionResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.Error != nil && strings.TrimSpace(result.Error.Message) != "" {
			return fmt.Errorf("stream request failed: %s", strings.TrimSpace(result.Error.Message))
		}
		return fmt.Errorf("stream request failed: status=%d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return nil
		}

		var chunk streamCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("decode stream chunk: %w", err)
		}
		if chunk.Error != nil && strings.TrimSpace(chunk.Error.Message) != "" {
			return fmt.Errorf("stream request failed: %s", strings.TrimSpace(chunk.Error.Message))
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta.Content
		if delta == "" {
			continue
		}
		if err := onDelta(delta); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stream response: %w", err)
	}

	return nil
}

func makeSDKMessages(messages []Message) []openai.ChatCompletionMessageParamUnion {
	items := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		switch strings.TrimSpace(message.Role) {
		case "system":
			items = append(items, openai.SystemMessage(content))
		case "assistant":
			items = append(items, openai.AssistantMessage(content))
		default:
			items = append(items, openai.UserMessage(content))
		}
	}
	return items
}

func makeSDKTools(tools []ToolDefinition) []openai.ChatCompletionToolUnionParam {
	items := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		items = append(items, openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        strings.TrimSpace(tool.Name),
			Description: openai.String(strings.TrimSpace(tool.Description)),
			Parameters:  openai.FunctionParameters(normalizeToolParameters(tool.InputSchema)),
		}))
	}
	return items
}

func normalizeToolParameters(raw map[string]any) map[string]any {
	if raw == nil {
		return map[string]any{"type": "object"}
	}

	normalized := make(map[string]any, len(raw)+1)
	for k, v := range raw {
		normalized[k] = v
	}
	if schemaType, ok := normalized["type"].(string); !ok || strings.TrimSpace(schemaType) == "" {
		normalized["type"] = "object"
	}
	return normalized
}

func chatCompletionsURL(baseURL string) string {
	u, err := url.JoinPath(strings.TrimRight(baseURL, "/"), "chat/completions")
	if err != nil {
		return strings.TrimRight(baseURL, "/") + "/chat/completions"
	}
	return u
}

func lastUserMessagePreview(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		return preview(messages[i].Content, 120)
	}
	return ""
}

func preview(text string, max int) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	if max <= 0 || len(text) <= max {
		return text
	}
	return text[:max] + "..."
}
