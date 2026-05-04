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

	messages := []llm.Message{
		{
			Role: "system",
			Content: strings.TrimSpace(`
你是 XiaoAiAgent 的长期记忆整理器。

你的职责不是生成聊天回复，而是维护一份“长期记忆 Markdown 文件”。

要求：
1. 你会看到“当前已有长期记忆”和“刚结束的一次完整会话 history”。
2. 只提炼以后仍然有价值的稳定信息，例如：
   - 用户偏好
   - 常用服务地址
   - 固定环境说明
   - 长期项目背景
   - 明确要求记住的事实
3. 不要把整段对话逐字抄进记忆。
4. 不要把一次性的寒暄、临时任务过程、无长期价值的瞬时状态写进长期记忆。
5. 要尽量保留已有记忆中用户手动维护的有效内容；如果本次会话明确修正了旧信息，可以更新。
6. 输出必须是“更新后的完整 Markdown 文件正文”，不要输出解释、前言、代码块或 JSON。
7. 保持文件结构清晰，至少保留这两个部分：
   - ## 长期记忆
   - ## 最近一次会话整理
8. “最近一次会话整理”只需要简洁概括这次刚结束会话的重点，不要写成逐轮 transcript。
9. 如果本次会话没有带来新的长期价值信息，也仍然输出完整文件，但只做最小必要更新。`),
		},
		{
			Role: "user",
			Content: strings.TrimSpace(fmt.Sprintf(`
记忆键：%s

当前已有长期记忆文件：
-----
%s
-----

刚结束的一次完整会话 history：
-----
%s
-----

请直接输出更新后的完整 Markdown 文件正文。`,
				strings.TrimSpace(memoryKey),
				strings.TrimSpace(currentMemory),
				renderSessionHistory(history),
			)),
		},
	}

	text, err := u.client.Complete(ctx, u.config, messages, 0.2)
	if err != nil {
		return "", err
	}
	text = normalizeSavedContent(text)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("memory updater returned empty content")
	}
	return text, nil
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
