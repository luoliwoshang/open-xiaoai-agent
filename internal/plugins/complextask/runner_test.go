package complextask

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
)

type fakeReporter struct {
	taskID  string
	updates []string
	events  []string
}

func (f *fakeReporter) TaskID() string {
	return f.taskID
}

func (f *fakeReporter) Update(summary string) error {
	f.updates = append(f.updates, summary)
	return nil
}

func (f *fakeReporter) Event(eventType string, message string) error {
	f.events = append(f.events, eventType+":"+message)
	return nil
}

func TestStreamParserHandlesClaudeOutput(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir() + "/claude.json")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	reporter := &fakeReporter{taskID: "task_1"}
	if err := store.Start("task_1", "做个网页", "/tmp/work"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	parser := newStreamParser("task_1", store, reporter)

	lines := []any{
		map[string]any{
			"type":       "system",
			"subtype":    "init",
			"session_id": "session_xyz",
		},
		map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"content": []map[string]any{
					{"type": "thinking", "thinking": "ignore"},
					{"type": "text", "text": "### 第一版页面已经搭出来了。🙂\n- 现在正在补交互细节"},
				},
			},
		},
		map[string]any{
			"type":   "result",
			"result": "\n网页已经完成。\n",
		},
	}

	for _, item := range lines {
		raw, err := json.Marshal(item)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		if err := parser.HandleLine(raw); err != nil {
			t.Fatalf("HandleLine() error = %v", err)
		}
	}

	if parser.result != "网页已经完成。" {
		t.Fatalf("parser.result = %q", parser.result)
	}
	if len(reporter.updates) != 1 || reporter.updates[0] != "第一版页面已经搭出来了。" {
		t.Fatalf("updates = %#v", reporter.updates)
	}

	records := store.Snapshot()
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].SessionID != "session_xyz" {
		t.Fatalf("SessionID = %q", records[0].SessionID)
	}
	if !strings.Contains(records[0].LastAssistantText, "现在正在补交互细节") {
		t.Fatalf("LastAssistantText = %q", records[0].LastAssistantText)
	}
	if records[0].LastSummary != "第一版页面已经搭出来了。" {
		t.Fatalf("LastSummary = %q", records[0].LastSummary)
	}
}

func TestBuildClaudePrompt(t *testing.T) {
	t.Parallel()

	prompt := buildClaudePrompt("帮我做一个网页")
	for _, expected := range []string{
		"执行以下任务：帮我做一个网页",
		"进度汇报要相对简短",
		"不要使用特殊符号、emoji、Markdown 列表、代码块",
		"如果任务还没有真正结束，不要提前说已经完成",
		"最终总结也要简短精炼",
		"尽量控制在 2 到 4 句",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("buildClaudePrompt() missing %q in %q", expected, prompt)
		}
	}
}

func TestBuildClaudeResumePrompt(t *testing.T) {
	t.Parallel()

	prompt := buildClaudeResumePrompt("把刚刚那个网页再加一个按钮")
	for _, expected := range []string{
		"继续基于刚才已经完成的同一个任务接着处理",
		"补充要求如下：把刚刚那个网页再加一个按钮",
		"把这次输入视为对上一个任务的补充、修改或追加要求",
		"最终总结也要简短精炼",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("buildClaudeResumePrompt() missing %q in %q", expected, prompt)
		}
	}
}

func TestSummarizeProgressText(t *testing.T) {
	t.Parallel()

	text := "### 我来帮你在桌面创建一个包含小故事的 txt 文件。🙂\n- 现在正在写入内容"
	got := summarizeProgressText(text)
	if got != "我来帮你在桌面创建一个包含小故事的 txt 文件。" {
		t.Fatalf("summarizeProgressText() = %q", got)
	}
}

var _ plugin.AsyncReporter = (*fakeReporter)(nil)
