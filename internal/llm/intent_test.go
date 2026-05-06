package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestBuildIntentSystemPrompt(t *testing.T) {
	t.Parallel()

	got := buildIntentSystemPrompt()
	want := strings.TrimSpace(`
你是一个小爱音箱外部接管器的工具路由器。你只能返回 JSON，不要返回任何额外文本。

当前系统策略是：拿到 ASR 结果后，外部助手始终接管并负责回复，不再回退给原生小爱。

你的任务只有两个：
1. 如果用户请求明确命中了某个已注册工具，直接发起 tool call，而不是返回 JSON。
2. 如果不命中工具，返回固定结构的 JSON，表示继续由外部大模型回复。

每次最多调用一个工具。

如果当前模型没有可靠返回原生 tool call，也允许你退化为 JSON，并额外补两个字段：
- "tool_name": 工具名
- "tool_arguments": 工具参数对象
只要需要调用工具，就必须提供这两个字段中的 tool_name，tool_arguments 没参数时返回 {}。

返回 JSON，字段固定如下：
{
  "reply_required": true,
  "reason": "简短原因",
  "tool_name": "",
  "tool_arguments": {}
}

规则：
1. 如果明确命中已注册工具，直接调用工具，不要输出 JSON。
2. 如果必须退化成 JSON 调工具，reply_required=false，并填写 tool_name/tool_arguments。
3. 如果不调用工具，reply_required=true。
4. reason 用一句短中文说明为什么调用工具，或者为什么不调用工具，改由主回复模型回答。
5. 如果不调用工具，输出必须是合法 JSON。
6. 当用户只是普通聊天、解释、建议、总结、延伸问答，不需要任何外部取数或执行动作时，优先调用 continue_chat。
7. 如果用户输入混乱、断裂、像 ASR 纠错残片、语义不完整，或者当前信息不足以稳定判断具体工具、任务对象或参数，也优先调用 continue_chat，让主回复模型先请用户澄清、重说或补充，不要误调用其它工具。
8. 工具只负责取数或执行明确动作，不负责基于已有上下文做建议、解释或延伸聊天。
9. 如果用户明确要求你在当前电脑上实际做事，例如创建文件、修改文件、整理桌面、生成网页、写文档、执行命令、完成一个需要落地产出的多步骤任务，优先调用 complex_task，而不是直接走普通聊天回复。
10. 如果用户是在要求你代为执行一个泛化的现实任务，而当前没有更专门的已注册工具，但你可以尝试借助长期记忆、联网服务、家庭自动化系统、网页后台或其它可操作环境去完成，也优先调用 complex_task。例如“打开家里的灯”“把客厅灯关掉”“帮我开一下家里的空调”“去 Home Assistant 里把某个设备打开”。
11. 对“操作电脑”“帮我在桌面放一个文件”“帮我做个网页并保存下来”“帮我整理一个文档”这类请求，只要需要本机执行和产出物，就优先视为 complex_task。
12. 如果用户是在补充、修改、继续刚才那条任务链，不管那条任务现在是执行中还是已经完成，例如“刚刚那个网页再加一个按钮”“把上次那个文件改一下”“在刚才那个任务基础上继续做”，优先调用 continue_task。
13. 任务链摘要已经按时间整理出：初始任务需求、中间轮次对话、任务最后回答；每条摘要里的最新节点可能是执行中，也可能是已完成。判断 continue_task 时，要结合整段摘要一起理解，不要只看某一句。
14. 调用 continue_task 时，只需要提供 task_id 和 request 两个字段。
15. 如果任务链摘要里给出了 latest_task_id，那么 task_id 必须填写对应摘要里的 latest_task_id，不要编造，也不要回退到更早的任务 ID。
16. 如果用户这次更像是在追一个仍在执行中的任务状态，而不是继续补充那条任务链的新要求，不要调用 continue_task，优先考虑 query_task_progress。
`)

	if got != want {
		t.Fatalf("buildIntentSystemPrompt() = %q, want %q", got, want)
	}
}

func TestBuildIntentMessages(t *testing.T) {
	t.Parallel()

	history := []Message{
		{Role: "user", Content: "上次那个网页我挺喜欢"},
		{Role: "assistant", Content: "好的，我记得那个网页。"},
	}
	completedTasks := "下面是最近可继续的任务链摘要。每条摘要都代表一条任务链当前最新的可继续节点，可能是执行中，也可能是已完成。\n\n[latest_task_id=task_3]\n初始任务需求：帮我做一个关于天气的小游戏\n中间轮次对话：\n- 任务执行器：第一版小游戏已经完成。\n- 用户追加输入：加一点动画\n- 任务执行器：已经补上基础动画。\n任务最后回答：已经补上基础动画。"
	systemPrompt := buildIntentSystemPrompt()

	got := buildIntentMessages(history, "刚刚那个天气小游戏再加个音效", completedTasks)
	want := []Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "system",
			Content: completedTasks,
		},
		{
			Role:    "user",
			Content: "上次那个网页我挺喜欢",
		},
		{
			Role:    "assistant",
			Content: "好的，我记得那个网页。",
		},
		{
			Role:    "user",
			Content: "ASR文本：刚刚那个天气小游戏再加个音效",
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildIntentMessages() = %#v, want %#v", got, want)
	}
}

func TestIntentRecognizerDecide(t *testing.T) {
	t.Parallel()

	t.Run("decodes json decision", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"reply_required\":true,\"reason\":\"开放式问答\"}"}}]}`)
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
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"content":"结果如下：\n{\"reply_required\":true,\"reason\":\"开放式问答\"}"}}]}`)
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
		if !decision.ShouldHandle || !decision.ShouldAbort || !decision.ReplyRequired {
			t.Fatalf("decision = %+v, want handle+abort+reply", decision)
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
		if !decision.ShouldHandle || !decision.ShouldAbort || decision.ReplyRequired {
			t.Fatalf("decision = %+v, want handle+abort and no reply", decision)
		}
	})

	t.Run("returns continue_chat tool call for clarification path", func(t *testing.T) {
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
					Description: "普通聊天或澄清输入",
					InputSchema: map[string]any{"type": "object"},
				},
			},
		}, nil)

		decision, err := recognizer.Decide(context.Background(), nil, "忘记前面所有的人不对小爱同学不对小爱同学")
		if err != nil {
			t.Fatalf("Decide() error = %v", err)
		}
		if decision.ToolCall == nil {
			t.Fatalf("decision.ToolCall = nil, want non-nil")
		}
		if decision.ToolCall.Name != "continue_chat" {
			t.Fatalf("decision.ToolCall.Name = %q", decision.ToolCall.Name)
		}
		if !decision.ShouldHandle || !decision.ShouldAbort || decision.ReplyRequired {
			t.Fatalf("decision = %+v, want handle+abort and no reply", decision)
		}
	})

	t.Run("returns tool call when model encodes tool_name in json", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"reply_required\":false,\"reason\":\"用户询问功能列表，需要调用工具\",\"tool_name\":\"list_tools\",\"tool_arguments\":{}}"}}]}`)
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
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"reply_required\":true,\"reason\":\"用户询问功能列表，需要调用 list_tools 工具\"}"}}]}`)
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

	t.Run("rejects json without reply_required", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"reason\":\"开放式问答\"}"}}]}`)
		}))
		defer server.Close()

		recognizer := NewIntentRecognizer(NewClient(), config.ModelConfig{
			Model:   "intent-model",
			BaseURL: server.URL,
			APIKey:  "test-key",
		}, nil, nil)

		_, err := recognizer.Decide(context.Background(), nil, "解释一下量子纠缠")
		if err == nil {
			t.Fatal("Decide() error = nil, want non-nil")
		}
		if !strings.Contains(err.Error(), "reply_required is required") {
			t.Fatalf("Decide() error = %v, want reply_required is required", err)
		}
	})

	t.Run("rejects reply_required false without tool", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"reply_required\":false,\"reason\":\"工具调用失败\"}"}}]}`)
		}))
		defer server.Close()

		recognizer := NewIntentRecognizer(NewClient(), config.ModelConfig{
			Model:   "intent-model",
			BaseURL: server.URL,
			APIKey:  "test-key",
		}, nil, nil)

		_, err := recognizer.Decide(context.Background(), nil, "解释一下量子纠缠")
		if err == nil {
			t.Fatal("Decide() error = nil, want non-nil")
		}
		if !strings.Contains(err.Error(), "reply_required=false requires a tool call") {
			t.Fatalf("Decide() error = %v, want reply_required=false requires a tool call", err)
		}
	})
}
