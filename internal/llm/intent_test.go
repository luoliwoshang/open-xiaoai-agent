package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/config"
)

type staticToolProvider struct {
	tools []ToolDefinition
}

func (p staticToolProvider) Definitions() []ToolDefinition {
	return p.tools
}

type staticCompletedTaskProvider struct {
	text string
}

func (p staticCompletedTaskProvider) CompletedTasksForIntent(limit int) string {
	return p.text
}

func TestIntentRecognizerDecide(t *testing.T) {
	t.Parallel()

	t.Run("returns continue_chat tool when model chooses normal chat", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"continue_chat","arguments":"{}"}}]}}]}`)
		}))
		defer server.Close()

		recognizer := NewIntentRecognizer(NewClient(), config.ModelConfig{
			Model:   "intent-model",
			BaseURL: server.URL,
			APIKey:  "test-key",
		}, staticToolProvider{
			tools: []ToolDefinition{
				{
					Name:        "continue_chat",
					Description: "继续普通聊天",
					InputSchema: map[string]any{"type": "object"},
				},
			},
		}, nil)

		decision, err := recognizer.Decide(context.Background(), nil, "解释一下量子纠缠")
		if err != nil {
			t.Fatalf("Decide() error = %v", err)
		}
		if decision.ToolCall == nil {
			t.Fatalf("decision.ToolCall = nil, want non-nil")
		}
		if decision.ToolCall.Name != "continue_chat" {
			t.Fatalf("decision.ToolCall.Name = %q", decision.ToolCall.Name)
		}
		if !decision.ShouldHandle || !decision.ShouldAbort {
			t.Fatalf("decision = %+v, want handle+abort", decision)
		}
	})

	t.Run("returns error when model emits no tool call", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"content":"普通文本，没有工具调用"}}]}`)
		}))
		defer server.Close()

		recognizer := NewIntentRecognizer(NewClient(), config.ModelConfig{
			Model:   "intent-model",
			BaseURL: server.URL,
			APIKey:  "test-key",
		}, nil, nil)

		if _, err := recognizer.Decide(context.Background(), nil, "打开客厅空调"); err == nil {
			t.Fatalf("Decide() error = nil, want non-nil")
		}
	})

	t.Run("returns tool call when model chooses tool", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
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
		if !decision.ShouldHandle || !decision.ShouldAbort {
			t.Fatalf("decision = %+v, want handle+abort", decision)
		}
	})

	t.Run("injects task chain snapshot prompt into intent request", func(t *testing.T) {
		t.Parallel()

		var requestBody string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			requestBody = string(raw)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"continue_task","arguments":"{\"plugin_name\":\"complex_task\",\"task_id\":\"task_130\",\"request\":\"刚刚那个天气小游戏再加个音效\"}"}}]}}]}`)
		}))
		defer server.Close()

		recognizer := NewIntentRecognizer(NewClient(), config.ModelConfig{
			Model:   "intent-model",
			BaseURL: server.URL,
			APIKey:  "test-key",
		}, staticToolProvider{
			tools: []ToolDefinition{
				{
					Name:        "continue_task",
					Description: "续任务",
					InputSchema: map[string]any{"type": "object"},
				},
			},
		}, staticCompletedTaskProvider{
			text: strings.TrimSpace(`
下面是最近可继续的任务链快照。每条只代表一条任务链当前最新的已完成节点。
如果用户现在是在补充、修改、继续之前已经做完的任务，请优先从下面选择最匹配的一条，并调用 continue_task。
注意：调用 continue_task 时，task_id 必须填写 latest_task_id，plugin_name 必须填写 plugin，不要自己编造。

- latest_task_id=task_130
  plugin=complex_task
  root_title=天气小游戏
  root_input=帮我做一个关于天气的小游戏
  recent_followups=加一点动画；再炫酷一点
  latest_summary=当前版本已经加入更强的动画效果和视觉强化
`),
		})

		decision, err := recognizer.Decide(context.Background(), nil, "刚刚那个天气小游戏再加个音效")
		if err != nil {
			t.Fatalf("Decide() error = %v", err)
		}
		if decision.ToolCall == nil || decision.ToolCall.Name != "continue_task" {
			t.Fatalf("decision.ToolCall = %#v, want continue_task", decision.ToolCall)
		}

		for _, expected := range []string{
			"最近可继续的任务链快照",
			"latest_task_id",
			"root_input",
			"recent_followups",
			"task_id 必须填写这个 latest_task_id",
			"刚刚那个天气小游戏再加个音效",
		} {
			if !strings.Contains(requestBody, expected) {
				t.Fatalf("request body missing %q in %q", expected, requestBody)
			}
		}
	})
}
