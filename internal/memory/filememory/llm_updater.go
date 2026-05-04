package filememory

import (
	"context"
	"fmt"
	"strings"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/config"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
)

type LLMUpdater struct {
	client *llm.Client
	config config.ModelConfig
}

const memoryUpdateSystemPrompt = `
你负责整理 XiaoAiAgent 的长期记忆。

你不会生成聊天回复，你只会根据“已有记忆内容”和“最近的一次对话”，输出新的完整记忆文件内容。

要求：
1. 只保留以后仍然值得记住的信息，例如用户偏好、固定环境说明、常用服务地址、长期项目背景、明确要求记住的事实。
2. 琐碎的助手回答、不重要的寒暄、临时过程、一次性噪音，不要记。
3. 不要把最近对话逐轮抄写进记忆，也不要写成长篇总结。
4. 绝对禁止凭空补全信息。名字、身份、关系、地点、偏好、经历等内容，只有在“已有记忆内容”或“最近的对话”里被明确说出时才能写入；不能猜、不能脑补、不能为了让语句更完整而擅自补充。
5. 尽量保留已有记忆里仍然有效的内容；如果最近对话修正了旧信息，就直接更新。
6. 如果最近对话里的新信息与已有记忆冲突，应以最近对话中明确的新事实为准，对旧记忆做更新、删减或替换，而不是把相互冲突的两份说法同时保留下来。
7. 如果最近对话明确否定了旧记忆中的某一条内容，就应该删除或改写那条错误记忆。
8. 输出必须是“更新后的完整记忆文件内容”，不要输出解释、前言、代码块或 JSON。
9. 如果最近对话没有带来新的长期价值信息，就返回整理后的原记忆内容；如果原来就是空的，也可以返回空内容。`

func NewLLMUpdater(client *llm.Client, cfg config.ModelConfig) *LLMUpdater {
	return &LLMUpdater{
		client: client,
		config: cfg,
	}
}

func (u *LLMUpdater) UpdateFromSession(ctx context.Context, memoryKey string, currentMemory string, history []llm.Message) (string, error) {
	if u == nil || u.client == nil {
		return "", fmt.Errorf("llm memory updater is not configured")
	}
	if strings.TrimSpace(u.config.Model) == "" {
		return "", fmt.Errorf("llm memory updater model is not configured")
	}

	history = normalizeMessages(history)
	if len(history) == 0 {
		return strings.TrimSpace(currentMemory), nil
	}

	messages := buildUpdateMessages(memoryKey, currentMemory, history)

	text, err := u.client.Complete(ctx, u.config, messages, 0.2)
	if err != nil {
		return "", err
	}
	text = normalizeSavedContent(text)
	return text, nil
}

func buildUpdateMessages(memoryKey string, currentMemory string, history []llm.Message) []llm.Message {
	return []llm.Message{
		{
			Role:    "system",
			Content: strings.TrimSpace(memoryUpdateSystemPrompt),
		},
		{
			Role: "user",
			Content: strings.TrimSpace(fmt.Sprintf(`
记忆键：%s

这是记忆内容【
%s
】

这是最近的对话【
%s
】

请你记住你觉得应该长期记住的内容，并直接输出更新后的完整记忆文件内容。`,
				strings.TrimSpace(memoryKey),
				strings.TrimSpace(currentMemory),
				renderSessionHistory(history),
			)),
		},
	}
}

func renderSessionHistory(history []llm.Message) string {
	if len(history) == 0 {
		return "（空）"
	}

	var b strings.Builder
	for _, message := range history {
		role := strings.TrimSpace(message.Role)
		content := strings.TrimSpace(message.Content)
		if role == "" || content == "" {
			continue
		}
		fmt.Fprintf(&b, "- %s：%s\n", role, content)
	}
	if b.Len() == 0 {
		return "（空）"
	}
	return strings.TrimSpace(b.String())
}
