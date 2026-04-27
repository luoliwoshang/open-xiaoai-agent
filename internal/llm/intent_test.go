package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/config"
)

type staticToolProvider struct {
	tools []ToolDefinition
}

func (p staticToolProvider) Definitions() []ToolDefinition {
	return p.tools
}

func TestIntentRecognizerDecide(t *testing.T) {
	t.Parallel()

	t.Run("decodes json decision", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"should_handle\":true,\"should_abort\":true,\"reply_required\":true,\"reason\":\"开放式问答\"}"}}]}`)
		}))
		defer server.Close()

		recognizer := NewIntentRecognizer(NewClient(), config.ModelConfig{
			Model:   "intent-model",
			BaseURL: server.URL,
			APIKey:  "test-key",
		}, nil, nil)

		decision, err := recognizer.Decide(context.Background(), nil, "解释一下量子纠缠")
		if err != nil {
			t.Fatalf("Decide() error = %v", err)
		}
		if !decision.ShouldHandle || !decision.ShouldAbort || !decision.ReplyRequired {
			t.Fatalf("decision = %+v, want handle+abort+reply", decision)
		}
	})

	t.Run("extracts json from wrapped content", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"结果如下：\n{\"should_handle\":false,\"should_abort\":false,\"reply_required\":false,\"reason\":\"原生设备控制\"}"}}]}`)
		}))
		defer server.Close()

		recognizer := NewIntentRecognizer(NewClient(), config.ModelConfig{
			Model:   "intent-model",
			BaseURL: server.URL,
			APIKey:  "test-key",
		}, nil, nil)

		decision, err := recognizer.Decide(context.Background(), nil, "打开客厅空调")
		if err != nil {
			t.Fatalf("Decide() error = %v", err)
		}
		if decision.ShouldHandle {
			t.Fatalf("decision = %+v, want should_handle=false", decision)
		}
	})

	t.Run("returns tool call when model chooses tool", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"ask_weather","arguments":"{\"city\":\"上海\"}"}}]}}]}`)
		}))
		defer server.Close()

		recognizer := NewIntentRecognizer(NewClient(), config.ModelConfig{
			Model:   "intent-model",
			BaseURL: server.URL,
			APIKey:  "test-key",
		}, staticToolProvider{
			tools: []ToolDefinition{
				{
					Name:        "ask_weather",
					Description: "查询天气",
					InputSchema: map[string]any{"type": "object"},
				},
			},
		}, nil)

		decision, err := recognizer.Decide(context.Background(), nil, "上海天气怎么样")
		if err != nil {
			t.Fatalf("Decide() error = %v", err)
		}
		if decision.ToolCall == nil {
			t.Fatalf("decision.ToolCall = nil, want non-nil")
		}
		if decision.ToolCall.Name != "ask_weather" {
			t.Fatalf("decision.ToolCall.Name = %q", decision.ToolCall.Name)
		}
		if !decision.ShouldHandle || !decision.ShouldAbort || decision.ReplyRequired {
			t.Fatalf("decision = %+v, want handle+abort and no reply", decision)
		}
	})

	t.Run("returns tool call when model encodes tool_name in json", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"should_handle\":true,\"should_abort\":true,\"reply_required\":false,\"reason\":\"用户询问功能列表，需要调用工具\",\"tool_name\":\"list_tools\",\"tool_arguments\":{}}"}}]}`)
		}))
		defer server.Close()

		recognizer := NewIntentRecognizer(NewClient(), config.ModelConfig{
			Model:   "intent-model",
			BaseURL: server.URL,
			APIKey:  "test-key",
		}, staticToolProvider{
			tools: []ToolDefinition{
				{
					Name:        "list_tools",
					Description: "查看能力列表",
					InputSchema: map[string]any{"type": "object"},
				},
			},
		}, nil)

		decision, err := recognizer.Decide(context.Background(), nil, "你能做什么")
		if err != nil {
			t.Fatalf("Decide() error = %v", err)
		}
		if decision.ToolCall == nil {
			t.Fatalf("decision.ToolCall = nil, want non-nil")
		}
		if decision.ToolCall.Name != "list_tools" {
			t.Fatalf("decision.ToolCall.Name = %q", decision.ToolCall.Name)
		}
		if decision.ReplyRequired {
			t.Fatalf("decision.ReplyRequired = true, want false")
		}
	})

	t.Run("matches tool from reason fallback", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"should_handle\":true,\"should_abort\":true,\"reply_required\":true,\"reason\":\"用户询问功能列表，需要调用 list_tools 工具\"}"}}]}`)
		}))
		defer server.Close()

		recognizer := NewIntentRecognizer(NewClient(), config.ModelConfig{
			Model:   "intent-model",
			BaseURL: server.URL,
			APIKey:  "test-key",
		}, staticToolProvider{
			tools: []ToolDefinition{
				{
					Name:        "list_tools",
					Description: "查看能力列表",
					InputSchema: map[string]any{"type": "object"},
				},
			},
		}, nil)

		decision, err := recognizer.Decide(context.Background(), nil, "你能做什么")
		if err != nil {
			t.Fatalf("Decide() error = %v", err)
		}
		if decision.ToolCall == nil {
			t.Fatalf("decision.ToolCall = nil, want non-nil")
		}
		if decision.ToolCall.Name != "list_tools" {
			t.Fatalf("decision.ToolCall.Name = %q", decision.ToolCall.Name)
		}
		if decision.ReplyRequired {
			t.Fatalf("decision.ReplyRequired = true, want false")
		}
	})
}
