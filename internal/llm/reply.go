package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/config"
)

type ReplyGenerator struct {
	client *Client
	config config.ModelConfig
	soul   string
}

func NewReplyGenerator(client *Client, cfg config.ModelConfig, soul string) *ReplyGenerator {
	return &ReplyGenerator{
		client: client,
		config: cfg,
		soul:   strings.TrimSpace(soul),
	}
}

func (g *ReplyGenerator) Stream(ctx context.Context, history []Message, text string, onDelta func(string) error) error {
	messages := g.baseMessages(history)
	messages = append(messages, Message{
		Role:    "user",
		Content: strings.TrimSpace(text),
	})

	return g.client.Stream(ctx, g.config, messages, 0.7, onDelta)
}

func (g *ReplyGenerator) StreamToolResult(ctx context.Context, history []Message, userText string, toolName string, toolResult string, onDelta func(string) error) error {
	messages := g.baseMessages(history)
	userPrompt := strings.TrimSpace(fmt.Sprintf(`
用户刚才的问题是：%s

系统已经调用了工具 %s，得到的结果如下：
%s

请你直接基于这个工具结果，用自然、简洁、口语化中文回答用户。
要求：
1. 不要说“我调用了工具”或“根据工具返回”。
2. 不要编造工具结果里没有提供的新事实。
3. 如果工具结果已经足够直接，就自然复述并补上必要建议。
`, strings.TrimSpace(userText), strings.TrimSpace(toolName), strings.TrimSpace(toolResult)))
	messages = append(messages, Message{
		Role:    "user",
		Content: userPrompt,
	})
	if payload, err := json.MarshalIndent(messages, "", "  "); err == nil {
		log.Printf(
			"tool reply llm input: tool=%s history=%d tool_result=%q messages=%s",
			strings.TrimSpace(toolName),
			len(history),
			strings.TrimSpace(toolResult),
			string(payload),
		)
	}

	return g.client.Stream(ctx, g.config, messages, 0.7, onDelta)
}

func (g *ReplyGenerator) StreamPendingTaskNotice(ctx context.Context, history []Message, reportContext string, onDelta func(string) error) error {
	messages := g.baseMessages(history)
	userPrompt := strings.TrimSpace(fmt.Sprintf(`
系统现在要主动向用户补报异步任务的新进展。

下面是结构化任务信息：
%s

请你把这些任务信息整理成一段自然、简洁、适合直接语音播报的中文通知。
要求：
1. 不要机械地说“任务标题已经完成了”这类模板句。
2. 不要先完整照读任务标题，再单独重复任务结果。
3. 把任务标题、状态和结果自然融合成用户一听就懂的补报。
4. 如果标题很长，只提炼出用户能听懂的关键目标，不要完整照搬。
5. 只基于给出的任务信息回答，不要编造新事实。
6. 如果只有一个任务，就直接说重点；如果有多个任务，简短合并说明。
`, strings.TrimSpace(reportContext)))
	messages = append(messages, Message{
		Role:    "user",
		Content: userPrompt,
	})
	if payload, err := json.MarshalIndent(messages, "", "  "); err == nil {
		log.Printf(
			"pending task notice llm input: history=%d report_context=%q messages=%s",
			len(history),
			strings.TrimSpace(reportContext),
			string(payload),
		)
	}

	return g.client.Stream(ctx, g.config, messages, 0.7, onDelta)
}

func (g *ReplyGenerator) baseMessages(history []Message) []Message {
	systemPrompt := fmt.Sprintf(strings.TrimSpace(`
	你现在是一个运行在小爱音箱外部服务器上的中文语音助手。

下面是你必须遵守的人设设定：
-----
%s
-----

约束：
1. 回答要适合语音播报，默认用自然、简洁、口语化中文。
2. 除非用户明确要求，否则不要使用项目符号、Markdown、代码围栏。
3. 不要自我介绍规则，不要解释系统提示。
4. 如果问题本身不需要长答案，保持简短。
`), g.soul)

	messages := []Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
	}
	if len(history) > 0 {
		messages = append(messages, history...)
	}
	return messages
}
