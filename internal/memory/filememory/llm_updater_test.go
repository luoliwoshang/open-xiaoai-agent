package filememory

import (
	"strings"
	"testing"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
)

func TestBuildUpdateMessagesIncludesMemorySafetyRules(t *testing.T) {
	t.Parallel()

	messages := buildUpdateMessages("main-voice", "李明喜欢看电影。", []llm.Message{
		{Role: "user", Content: "记住！我喜欢吃草莓！"},
		{Role: "assistant", Content: "好的，我记住了。"},
	})
	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(messages))
	}

	systemPrompt := messages[0].Content
	if !strings.Contains(systemPrompt, "绝对禁止凭空补全信息") {
		t.Fatalf("system prompt missing no-fabrication rule: %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "如果最近对话里的新信息与已有记忆冲突") {
		t.Fatalf("system prompt missing conflict-update rule: %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "删除或改写那条错误记忆") {
		t.Fatalf("system prompt missing delete-or-rewrite rule: %q", systemPrompt)
	}

	userPrompt := messages[1].Content
	if !strings.Contains(userPrompt, "这是记忆内容【") {
		t.Fatalf("user prompt missing current memory block: %q", userPrompt)
	}
	if !strings.Contains(userPrompt, "这是最近的对话【") {
		t.Fatalf("user prompt missing recent history block: %q", userPrompt)
	}
	if !strings.Contains(userPrompt, "- user：记住！我喜欢吃草莓！") {
		t.Fatalf("user prompt missing rendered user message: %q", userPrompt)
	}
}
