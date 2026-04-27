package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/config"
)

type IntentDecision struct {
	ShouldHandle  bool   `json:"should_handle"`
	ShouldAbort   bool   `json:"should_abort"`
	ReplyRequired bool   `json:"reply_required"`
	Reason        string `json:"reason"`
	ToolCall      *ToolCall
}

type intentJSONDecision struct {
	ShouldHandle  bool            `json:"should_handle"`
	ShouldAbort   bool            `json:"should_abort"`
	ReplyRequired bool            `json:"reply_required"`
	Reason        string          `json:"reason"`
	ToolName      string          `json:"tool_name"`
	ToolArguments json.RawMessage `json:"tool_arguments"`
}

type IntentRecognizer struct {
	client      *Client
	config      config.ModelConfig
	tools       ToolDefinitionsProvider
	taskContext CompletedTasksProvider
}

type ToolDefinitionsProvider interface {
	Definitions() []ToolDefinition
}

type CompletedTasksProvider interface {
	CompletedTasksForIntent(limit int) string
}

func NewIntentRecognizer(client *Client, cfg config.ModelConfig, tools ToolDefinitionsProvider, taskContext CompletedTasksProvider) *IntentRecognizer {
	return &IntentRecognizer{
		client:      client,
		config:      cfg,
		tools:       tools,
		taskContext: taskContext,
	}
}

func (r *IntentRecognizer) Decide(ctx context.Context, history []Message, text string) (IntentDecision, error) {
	availableTools := []ToolDefinition(nil)
	if r.tools != nil {
		availableTools = r.tools.Definitions()
	}

	messages := []Message{
		{
			Role: "system",
			Content: strings.TrimSpace(`
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
	  "should_handle": true,
	  "should_abort": true,
	  "reply_required": true,
	  "reason": "简短原因",
	  "tool_name": "",
	  "tool_arguments": {}
	}

	规则：
	1. 如果明确命中已注册工具，直接调用工具，不要输出 JSON。
	2. 如果必须退化成 JSON 调工具，should_handle=true，should_abort=true，reply_required=false，并填写 tool_name/tool_arguments。
	3. 如果不调用工具，should_handle=true，should_abort=true，reply_required=true。
	4. reason 用一句短中文说明为什么调用工具，或者为什么不调用工具，改由主回复模型回答。
	5. 如果不调用工具，输出必须是合法 JSON。
	6. 工具只负责取数或执行明确动作，不负责基于已有上下文做建议、解释或延伸聊天。
	7. 对天气工具尤其要严格区分：
	   - 如果用户明确要求查询、确认、刷新某个城市/地区的天气，才调用天气工具。
	   - 如果用户是在已有天气结果基础上继续追问“那穿什么衣服”“要不要带伞”“适不适合出门”“那我该注意什么”这类建议问题，不要调用天气工具，直接走主回复模型。
	8. 如果当前问题没有提供新的城市/地区信息，而且从上下文看已经拿到天气结果，优先认为这是延伸问答，不要重复调用天气工具。
	9. 如果用户明确要求你在当前电脑上实际做事，例如创建文件、修改文件、整理桌面、生成网页、写文档、执行命令、完成一个需要落地产出的多步骤任务，优先调用 complex_task，而不是直接走普通聊天回复。
	10. 对“操作电脑”“帮我在桌面放一个文件”“帮我做个网页并保存下来”“帮我整理一个文档”这类请求，只要需要本机执行和产出物，就优先视为 complex_task。
	11. 如果用户是在补充、修改、继续之前已经完成的任务，例如“刚刚那个网页再加一个按钮”“把上次那个文件改一下”“在刚才那个任务基础上继续做”，优先调用 continue_task。
	12. 调用 continue_task 时，必须提供 plugin_name、task_id、request 三个字段。plugin_name 和 task_id 必须从已完成任务列表里选择，不要编造。
	`),
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("ASR文本：%s", strings.TrimSpace(text)),
		},
	}
	if r.taskContext != nil {
		completedTasks := strings.TrimSpace(r.taskContext.CompletedTasksForIntent(5))
		if completedTasks != "" {
			messages = append(messages[:1], append([]Message{{Role: "system", Content: completedTasks}}, messages[1:]...)...)
		}
	}
	if len(history) > 0 {
		prefix := append([]Message(nil), messages[:len(messages)-1]...)
		currentUser := messages[len(messages)-1]
		messages = append(prefix, append(history, currentUser)...)
	}

	result, err := r.client.CompleteWithTools(ctx, r.config, messages, 0, availableTools)
	if err != nil {
		return IntentDecision{}, err
	}

	if len(result.ToolCalls) > 0 {
		call := result.ToolCalls[0]
		return IntentDecision{
			ShouldHandle:  true,
			ShouldAbort:   true,
			ReplyRequired: false,
			Reason:        fmt.Sprintf("tool call: %s", call.Name),
			ToolCall:      &call,
		}, nil
	}

	jsonText, err := extractJSONObject(result.Content)
	if err != nil {
		return IntentDecision{}, fmt.Errorf("extract intent json: %w", err)
	}

	var decision IntentDecision
	var raw intentJSONDecision
	if err := json.Unmarshal([]byte(jsonText), &raw); err != nil {
		return IntentDecision{}, fmt.Errorf("decode intent json: %w", err)
	}

	decision = IntentDecision{
		ShouldHandle:  raw.ShouldHandle,
		ShouldAbort:   raw.ShouldAbort,
		ReplyRequired: raw.ReplyRequired,
		Reason:        raw.Reason,
	}

	if toolName, ok := resolveToolName(raw.ToolName, raw.Reason, result.Content, availableTools); ok {
		arguments := raw.ToolArguments
		if len(arguments) == 0 {
			arguments = json.RawMessage(`{}`)
		}
		decision.ToolCall = &ToolCall{
			Name:      toolName,
			Arguments: arguments,
		}
		decision.ShouldHandle = true
		decision.ShouldAbort = true
		decision.ReplyRequired = false
	}

	if decision.ShouldHandle && decision.ReplyRequired && !decision.ShouldAbort {
		decision.ShouldAbort = true
	}

	return decision, nil
}

func resolveToolName(explicitName string, reason string, content string, tools []ToolDefinition) (string, bool) {
	explicitName = strings.TrimSpace(explicitName)
	if explicitName != "" {
		for _, tool := range tools {
			if explicitName == tool.Name {
				return explicitName, true
			}
		}
	}

	searchSpace := reason + "\n" + content
	matched := ""
	for _, tool := range tools {
		if tool.Name == "" {
			continue
		}
		if strings.Contains(searchSpace, tool.Name) {
			if matched != "" && matched != tool.Name {
				return "", false
			}
			matched = tool.Name
		}
	}
	if matched != "" {
		return matched, true
	}

	return "", false
}

func extractJSONObject(text string) (string, error) {
	start := strings.IndexByte(text, '{')
	if start == -1 {
		return "", fmt.Errorf("no json object found")
	}

	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(text); i++ {
		ch := text[i]

		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1], nil
			}
		}
	}

	return "", fmt.Errorf("unterminated json object")
}
