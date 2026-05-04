package llm

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/config"
)

type IntentDecision struct {
	ShouldHandle bool   `json:"should_handle"`
	ShouldAbort  bool   `json:"should_abort"`
	Reason       string `json:"reason"`
	ToolCall     *ToolCall
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
// 可用工具定义以及最近可继续的任务链快照一起发给 intent 模型，让模型判断：
// 1. 这轮是否应该继续走普通聊天 reply；
// 2. 是否应该命中某个同步工具；
// 3. 是否应该受理为 complex_task / continue_task / query_task_progress 等特殊工具调用。
//
// 返回值预期为原生 OpenAI tool call。
// 当前策略下，intent 模型总是要从已注册工具里选一个：
// - 普通聊天、解释、建议、延伸问答 => continue_chat
// - 查天气、查股票、复杂任务、续任务、查进度、取消任务 => 对应工具
//
// assistant.Service 会消费这份 IntentDecision，
// 决定主流程后续进入普通 reply、tool 还是 async task 分支。
func (r *IntentRecognizer) Decide(ctx context.Context, history []Message, text string) (IntentDecision, error) {
	availableTools := []ToolDefinition(nil)
	if r.tools != nil {
		// 这里取到的是“当前系统里已经注册好的工具定义”。
		// 它们通常来自 plugin.Registry.Definitions()：
		// registry 先在启动阶段注册 weather / stock / complex_task / continue_task 等工具，
		// 这里再把这些工具的 name / description / input schema 提供给 intent 模型。
		//
		// 也就是说：
		// 1. 注册阶段保存的是完整工具（定义 + handler）；
		// 2. 到 intent 阶段这里只取“定义”，不给模型暴露 Go 里的真实执行函数。
		availableTools = r.tools.Definitions()
	}

	messages := []Message{
		{
			Role: "system",
			Content: strings.TrimSpace(`
	你是一个小爱音箱外部接管器的工具路由器。

	当前系统策略是：拿到 ASR 结果后，外部助手始终接管并负责回复，不再回退给原生小爱。

	规则：
	1. 当用户只是普通聊天、解释、建议、总结、延伸问答、不需要任何外部动作或取数时，调用 continue_chat。
	2. 工具只负责取数或执行明确动作，不负责基于已有上下文做建议、解释或延伸聊天。
	3. 对天气工具尤其要严格区分：
	   - 如果用户明确要求查询、确认、刷新某个城市/地区的天气，才调用天气工具。
	   - 如果用户是在已有天气结果基础上继续追问“那穿什么衣服”“要不要带伞”“适不适合出门”“那我该注意什么”这类建议问题，不要调用天气工具，调用 continue_chat。
	4. 如果当前问题没有提供新的城市/地区信息，而且从上下文看已经拿到天气结果，优先认为这是延伸问答，不要重复调用天气工具，调用 continue_chat。
	5. 如果用户明确要求你在当前电脑上实际做事，例如创建文件、修改文件、整理桌面、生成网页、写文档、执行命令、完成一个需要落地产出的多步骤任务，优先调用 complex_task，而不是 continue_chat。
	6. 对“操作电脑”“帮我在桌面放一个文件”“帮我做个网页并保存下来”“帮我整理一个文档”这类请求，只要需要本机执行和产出物，就优先视为 complex_task。
	7. 如果用户是在补充、修改、继续之前已经完成的任务，例如“刚刚那个网页再加一个按钮”“把上次那个文件改一下”“在刚才那个任务基础上继续做”，优先调用 continue_task。
	8. 当你判断是否命中 continue_task 时，要结合任务链快照里的 root_title、root_input、recent_followups、latest_summary 一起理解，不要只看其中一个字段。
	9. 调用 continue_task 时，必须提供 plugin_name、task_id、request 三个字段。plugin_name 和 task_id 必须从任务链快照里选择，不要编造。
	10. 如果任务链快照里给出了 latest_task_id，那么 task_id 必须填写这个 latest_task_id，不要回退到更早的根任务 ID。
	11. 如果用户这次更像是在追一个仍在执行中的任务状态，而不是继续一个已经完成的任务，不要调用 continue_task，优先考虑 query_task_progress。
	`),
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("ASR文本：%s", strings.TrimSpace(text)),
		},
	}
	if r.taskContext != nil {
		// 这里插入的是“最近已完成任务候选列表”。
		// intent 模型在判断要不要调用 continue_task 之前，
		// 就会先看到这段 system message，再结合当前 ASR 文本做匹配。
		//
		// 当前候选列表来自 tasks.Manager.CompletedTasksForIntent(5)：
		// - 不是平铺 task 行，而是按 parent_task_id 折叠成任务链快照
		// - 每条快照只暴露这条链当前最新的 completed 节点
		// - 每条快照同时带 root_input 和 recent_followups，兼顾原始目标和最近修改语义
		// - 最多带 5 条，避免把过多旧任务塞进 prompt
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

	log.Printf(
		"intent request prepared: model=%s tool_count=%d history_count=%d messages=\n%s",
		strings.TrimSpace(r.config.Model),
		len(availableTools),
		len(history),
		formatIntentMessagesForLog(messages),
	)
	if len(availableTools) > 0 {
		log.Printf("intent request tools: %s", strings.Join(toolNamesForLog(availableTools), ", "))
	}

	// 这里把已注册工具定义一并传给模型：
	// 当前策略要求模型始终返回一个原生 tool call，
	// 普通聊天场景也通过 continue_chat 这个逻辑工具来表达。
	result, err := r.client.CompleteWithTools(ctx, r.config, messages, 0, availableTools)
	if err != nil {
		return IntentDecision{}, err
	}

	if len(result.ToolCalls) > 0 {
		call := result.ToolCalls[0]
		log.Printf("intent tool selected via native tool_call: tool=%s", strings.TrimSpace(call.Name))
		return IntentDecision{
			ShouldHandle: true,
			ShouldAbort:  true,
			Reason:       fmt.Sprintf("tool call: %s", call.Name),
			ToolCall:     &call,
		}, nil
	}

	return IntentDecision{}, fmt.Errorf("intent completion returned no tool call")
}

func formatIntentMessagesForLog(messages []Message) string {
	if len(messages) == 0 {
		return "(empty)"
	}

	var lines []string
	for index, message := range messages {
		lines = append(lines, fmt.Sprintf(
			"[%d] role=%s content=%q",
			index,
			strings.TrimSpace(message.Role),
			strings.TrimSpace(message.Content),
		))
	}
	return strings.Join(lines, "\n")
}

func toolNamesForLog(tools []ToolDefinition) []string {
	if len(tools) == 0 {
		return nil
	}

	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			name = "(unnamed)"
		}
		names = append(names, name)
	}
	return names
}
