package llm

import (
	"context"
	"fmt"
	"log"
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

type IntentRecognizer struct {
	client      *Client
	config      config.ModelConfig
	tools       ToolDefinitionsProvider
	taskContext CompletedTasksProvider
}

// ToolDefinitionsProvider 提供“当前已经注册的工具定义列表”。
//
// 注意这里拿到的是给 LLM 做路由判断用的工具元数据，
// 例如 name / description / input schema，
// 不是工具真正执行时的 handler 函数本身。
//
// 当前主流程里通常会把 plugin.Registry 注入进来；
// registry 会把所有已注册工具转换成 []llm.ToolDefinition，
// 再交给 intent 模型作为可调用工具集合。
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

// Decide 对当前这轮用户输入做一次“主流程路由判定”。
//
// 它不会直接生成最终播报给用户的回复，而是把当前 text、最近会话 history、
// 可用工具定义以及最近已完成任务摘要一起发给 intent 模型，让模型判断：
// 1. 这轮是否应该继续走普通聊天 reply；
// 2. 是否应该命中某个同步工具；
// 3. 是否应该受理为 complex_task / continue_task / query_task_progress 等特殊工具调用。
//
// 当前实现只接受模型返回的原生 tool call。
//
// 其中 continue_chat 也是一个正常注册进来的“路由工具”：
// - intent 命中 continue_chat，表示后续继续走普通 reply 主线；
// - intent 命中其它工具，则进入对应的 tool / async task 分支。
//
// assistant.Service 会消费这份 IntentDecision，
// 决定主流程后续进入 reply、tool 还是 async task 分支。
func (r *IntentRecognizer) Decide(ctx context.Context, history []Message, text string) (IntentDecision, error) {
	availableTools := []ToolDefinition(nil)
	if r.tools != nil {
		// 这里取到的是“当前系统里已经注册好的工具定义”。
		// 它们通常来自 plugin.Registry.Definitions()：
		// registry 先在启动阶段注册 weather / complex_task / continue_task 等工具，
		// 这里再把这些工具的 name / description / input schema 提供给 intent 模型。
		//
		// 也就是说：
		// 1. 注册阶段保存的是完整工具（定义 + handler）；
		// 2. 到 intent 阶段这里只取“定义”，不给模型暴露 Go 里的真实执行函数。
		availableTools = r.tools.Definitions()
	}

	completedTasks := ""
	if r.taskContext != nil {
		completedTasks = strings.TrimSpace(r.taskContext.CompletedTasksForIntent(5))
	}
	messages := buildIntentMessages(history, text, completedTasks)
	logPreparedIntentRequest(r.config.Model, history, messages, availableTools)

	// 这里把已注册工具定义一并传给模型，要求模型直接返回原生 tool call。
	result, err := r.client.CompleteWithTools(ctx, r.config, messages, 0, availableTools)
	if err != nil {
		return IntentDecision{}, err
	}

	if len(result.ToolCalls) == 0 {
		return IntentDecision{}, fmt.Errorf("intent response missing native tool call: content=%q", strings.TrimSpace(result.Content))
	}
	if len(result.ToolCalls) > 1 {
		log.Printf("intent returned multiple tool calls; only the first one will be used: count=%d", len(result.ToolCalls))
	}
	call := result.ToolCalls[0]
	log.Printf("intent tool selected via native tool_call: tool=%s", strings.TrimSpace(call.Name))
	return IntentDecision{
		ShouldHandle:  true,
		ShouldAbort:   true,
		ReplyRequired: false,
		Reason:        fmt.Sprintf("tool call: %s", call.Name),
		ToolCall:      &call,
	}, nil
}

// buildIntentMessages 负责把这轮意图识别真正要发给模型的消息列表固定下来。
//
// 这里把 continue_task 的任务链摘要单独作为一条 system message 插进去，
// 而 history 这一段则允许上层提前拼入长期记忆 system message。
// 这样后续测试可以直接对整个 []Message 做全文断言，而不用再从 HTTP 请求体里反推 prompt。
func buildIntentMessages(history []Message, text string, completedTasks string) []Message {
	messages := []Message{
		{
			Role:    "system",
			Content: buildIntentSystemPrompt(),
		},
	}
	completedTasks = strings.TrimSpace(completedTasks)
	if completedTasks != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: completedTasks,
		})
	}
	if len(history) > 0 {
		messages = append(messages, history...)
	}
	messages = append(messages, Message{
		Role:    "user",
		Content: fmt.Sprintf("ASR文本：%s", strings.TrimSpace(text)),
	})
	return messages
}

// buildIntentSystemPrompt 只负责“意图识别路由规则”本身。
//
// 这里会明确告诉模型：
// 1. continue_task 现在只需要 task_id 和 request；
// 2. 判断 continue_task 时，要结合任务链摘要里的初始任务需求、中间轮次对话和任务最后回答；
// 3. 候选摘要里的最新节点现在可能是执行中，也可能是已完成；
// 4. 如果摘要里给出了 latest_task_id，就必须用它，不要回退到更早任务。
func buildIntentSystemPrompt() string {
	return strings.TrimSpace(`
你是一个工具路由器。

你的任务是根据当前信息，决策应该使用哪个已注册工具，并直接返回原生 tool call。

不要输出普通文本，不要输出 JSON，不要解释原因。
每次只能调用一个工具。

规则：
1. 当用户只是普通聊天、解释、建议、总结、延伸问答，不需要任何外部取数或执行动作时，优先调用 continue_chat。
2. 如果用户输入混乱、断裂、像 ASR 纠错残片、语义不完整，或者当前信息不足以稳定判断具体工具、任务对象或参数，也优先调用 continue_chat，让主回复模型先请用户澄清、重说或补充，不要误调用其它工具。
3. 工具只负责取数或执行明确动作，不负责基于已有上下文做建议、解释或延伸聊天。
4. 如果用户明确要求你在当前电脑上实际做事，例如创建文件、修改文件、整理桌面、生成网页、写文档、执行命令、完成一个需要落地产出的多步骤任务，优先调用 complex_task，而不是 continue_chat。
5. 如果用户是在要求你代为执行一个泛化的现实任务，而当前没有更专门的已注册工具，但你可以尝试借助长期记忆、联网服务、家庭自动化系统、网页后台或其它可操作环境去完成，也优先调用 complex_task。例如“打开家里的灯”“把客厅灯关掉”“帮我开一下家里的空调”“去 Home Assistant 里把某个设备打开”。
6. 对“操作电脑”“帮我在桌面放一个文件”“帮我做个网页并保存下来”“帮我整理一个文档”这类请求，只要需要本机执行和产出物，就优先视为 complex_task。
7. 如果用户是在补充、修改、继续刚才那条任务链，不管那条任务现在是执行中还是已经完成，例如“刚刚那个网页再加一个按钮”“把上次那个文件改一下”“在刚才那个任务基础上继续做”，优先调用 continue_task。
8. 任务链摘要已经按时间整理出：初始任务需求、中间轮次对话、任务最后回答；每条摘要里的最新节点可能是执行中，也可能是已完成。判断 continue_task 时，要结合整段摘要一起理解，不要只看某一句。
9. 调用 continue_task 时，只需要提供 task_id 和 request 两个字段。
10. 如果任务链摘要里给出了 latest_task_id，那么 task_id 必须填写对应摘要里的 latest_task_id，不要编造，也不要回退到更早的任务 ID。
11. 如果用户这次更像是在追一个仍在执行中的任务状态，而不是继续补充那条任务链的新要求，不要调用 continue_task，优先考虑 query_task_progress。
`)
}

// logPreparedIntentRequest 把“这一轮 intent 到底给模型发了什么”完整记下来。
//
// 这类日志主要给调试 continue_task 命中、历史拼接以及 system prompt 变更使用，
// 因此这里按 message 粒度逐条打印，而不是只记一个 messages=N 的摘要。
// content 用 %q 输出，避免多行 prompt 在日志页里被拆成难读的多条记录。
func logPreparedIntentRequest(model string, history []Message, messages []Message, tools []ToolDefinition) {
	log.Printf(
		"intent request prepared: model=%s tool_count=%d history_count=%d messages=%d",
		strings.TrimSpace(model),
		len(tools),
		len(history),
		len(messages),
	)
	for i, message := range messages {
		log.Printf(
			"intent message[%d]: role=%s content=%q",
			i,
			strings.TrimSpace(message.Role),
			strings.TrimSpace(message.Content),
		)
	}
}
