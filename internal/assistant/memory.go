package assistant

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
)

func (s *Service) recallMemory(memoryKey string) string {
	if s == nil || s.memory == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	text, err := s.memory.Recall(ctx, strings.TrimSpace(memoryKey))
	if err != nil {
		log.Printf("memory recall failed: key=%s err=%v", strings.TrimSpace(memoryKey), err)
		return ""
	}
	return strings.TrimSpace(text)
}

func withMemoryMessage(history []llm.Message, memoryText string) []llm.Message {
	memoryText = strings.TrimSpace(memoryText)
	if memoryText == "" {
		return history
	}

	memoryMessage := llm.Message{
		Role: "system",
		Content: strings.TrimSpace(`
下面是和当前用户长期相关的记忆，仅在确实相关时参考：
1. 不要机械复述这段记忆。
2. 不要把它伪装成用户刚刚这轮说过的话。
3. 如果其中包含 URL、Token、密钥或其他敏感信息，在用户没有明确要求时不要主动泄露。

-----
` + memoryText + `
-----`),
	}

	items := make([]llm.Message, 0, len(history)+1)
	items = append(items, memoryMessage)
	items = append(items, history...)
	return items
}

func (s *Service) appendConversationTurn(memoryKey string, occurredAt time.Time, source string, messages ...llm.Message) {
	if s == nil || s.history == nil {
		return
	}

	var userText string
	var assistantText string
	appendMessages := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		content := strings.TrimSpace(message.Content)
		if role == "" || content == "" {
			continue
		}
		switch role {
		case "user":
			if userText == "" {
				userText = content
			}
		case "assistant":
			if assistantText == "" {
				assistantText = content
			}
		}
		appendMessages = append(appendMessages, llm.Message{
			Role:    role,
			Content: content,
		})
	}
	if len(appendMessages) == 0 {
		return
	}

	// 这里现在只负责把稳定结果写进“会话窗口历史”。
	//
	// 长期记忆更新已经不再按 turn 高频触发：
	// 它会改为在一整段 session 超时结束后，拿那次完整 history
	// 统一调用 memory.UpdateFromSession(...) 做一次低频总结。
	s.history.AppendTurn(historyRef(memoryKey), occurredAt, userText, assistantText)
	_ = source
}
