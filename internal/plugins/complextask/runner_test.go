package complextask

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/testmysql"
)

type fakeReporter struct {
	taskID      string
	updates     []string
	events      []string
	artifacts   []plugin.ArtifactRef
	artifactSeq int
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

func (f *fakeReporter) PutArtifact(req plugin.PutArtifactRequest) (plugin.ArtifactRef, error) {
	f.artifactSeq++
	if closer, ok := req.Reader.(io.Closer); ok {
		defer closer.Close()
	}
	if req.Reader != nil {
		if _, err := io.ReadAll(req.Reader); err != nil {
			return plugin.ArtifactRef{}, err
		}
	}
	ref := plugin.ArtifactRef{
		ID:       fmt.Sprintf("artifact_%d", f.artifactSeq),
		TaskID:   f.taskID,
		Kind:     req.Kind,
		FileName: req.Name,
		MIMEType: req.MIMEType,
		Size:     req.Size,
	}
	f.artifacts = append(f.artifacts, ref)
	return ref, nil
}

func TestStreamParserHandlesClaudeOutput(t *testing.T) {
	t.Parallel()

	store, err := NewStore(testmysql.NewDSN(t))
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

	prompt := buildClaudePrompt("task_1", "帮我做一个网页")
	for _, expected := range []string{
		"执行以下任务：帮我做一个网页",
		"进度汇报要相对简短",
		"不要使用特殊符号、emoji、Markdown 列表、代码块",
		"如果任务还没有真正结束，不要提前说已经完成",
		".open-xiaoai-agent/deliverables/task_1",
		".open-xiaoai-agent/artifacts/task_1.json",
		"这个 JSON 文件只负责声明交付产物位置和元数据",
		"name 必须优先带上和真实文件一致的后缀",
		"不要直接说“保存为 xxx.png / xxx.html / xxx.txt”",
		"不要提工作目录、相对路径、绝对路径、manifest、终端命令",
		"普通人能直接听懂",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("buildClaudePrompt() missing %q in %q", expected, prompt)
		}
	}
}

func TestBuildClaudeResumePrompt(t *testing.T) {
	t.Parallel()

	prompt := buildClaudeResumePrompt("task_2", "把刚刚那个网页再加一个按钮")
	for _, expected := range []string{
		"继续基于刚才已经完成的同一个任务接着处理",
		"补充要求如下：把刚刚那个网页再加一个按钮",
		"把这次输入视为对上一个任务的补充、修改或追加要求",
		".open-xiaoai-agent/deliverables/task_2",
		".open-xiaoai-agent/artifacts/task_2.json",
		"name 必须优先带上和真实文件一致的后缀",
		"不要直接说“保存为 xxx.png / xxx.html / xxx.txt”",
		"不要提工作目录、相对路径、绝对路径、manifest、终端命令",
		"普通人能直接听懂",
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

func TestImportArtifactsFromManifest(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	outputDir := filepath.Join(cwd, ".open-xiaoai-agent", "deliverables", "task_1", "rabbit-game")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	indexPath := filepath.Join(outputDir, "index.html")
	if err := os.WriteFile(indexPath, []byte("<html>rabbit</html>"), 0o644); err != nil {
		t.Fatalf("WriteFile(index) error = %v", err)
	}
	readmePath := filepath.Join(outputDir, "README.txt")
	if err := os.WriteFile(readmePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile(readme) error = %v", err)
	}

	manifestDir := filepath.Join(cwd, ".open-xiaoai-agent", "artifacts")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(manifest) error = %v", err)
	}
	manifestPath := filepath.Join(manifestDir, "task_1.json")
	manifest := artifactManifest{
		Deliver: []artifactManifestEntry{
			{
				Path:     ".open-xiaoai-agent/deliverables/task_1/rabbit-game/index.html",
				Name:     "rabbit-game.html",
				Kind:     "file",
				MIMEType: "text/html",
			},
			{
				Path:     ".open-xiaoai-agent/deliverables/task_1/rabbit-game/README.txt",
				Name:     "README.txt",
				Kind:     "file",
				MIMEType: "text/plain",
			},
		},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	reporter := &fakeReporter{taskID: "task_1"}
	runner := NewClaudeRunner(nil, cwd)
	if err := runner.importArtifacts("task_1", reporter); err != nil {
		t.Fatalf("importArtifacts() error = %v", err)
	}

	if len(reporter.artifacts) != 2 {
		t.Fatalf("len(artifacts) = %d, want 2", len(reporter.artifacts))
	}
	if reporter.artifacts[0].FileName != "rabbit-game.html" {
		t.Fatalf("artifacts[0].FileName = %q", reporter.artifacts[0].FileName)
	}
	if reporter.artifacts[1].FileName != "README.txt" {
		t.Fatalf("artifacts[1].FileName = %q", reporter.artifacts[1].FileName)
	}
	if len(reporter.events) == 0 || reporter.events[len(reporter.events)-1] != "claude_artifacts:Claude 已登记 2 个交付产物" {
		t.Fatalf("events = %#v", reporter.events)
	}
}

func TestImportArtifactsAppendsFileExtensionWhenManifestNameOmitsIt(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	outputDir := filepath.Join(cwd, ".open-xiaoai-agent", "deliverables", "task_1")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	filePath := filepath.Join(outputDir, "breathing-exercise.html")
	if err := os.WriteFile(filePath, []byte("<html>breathing</html>"), 0o644); err != nil {
		t.Fatalf("WriteFile(file) error = %v", err)
	}

	manifestDir := filepath.Join(cwd, ".open-xiaoai-agent", "artifacts")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(manifest) error = %v", err)
	}
	manifestPath := filepath.Join(manifestDir, "task_1.json")
	raw, err := json.Marshal(artifactManifest{
		Deliver: []artifactManifestEntry{
			{
				Path:     ".open-xiaoai-agent/deliverables/task_1/breathing-exercise.html",
				Name:     "呼吸练习",
				Kind:     "file",
				MIMEType: "text/html",
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	reporter := &fakeReporter{taskID: "task_1"}
	runner := NewClaudeRunner(nil, cwd)
	if err := runner.importArtifacts("task_1", reporter); err != nil {
		t.Fatalf("importArtifacts() error = %v", err)
	}

	if len(reporter.artifacts) != 1 {
		t.Fatalf("len(artifacts) = %d, want 1", len(reporter.artifacts))
	}
	if reporter.artifacts[0].FileName != "呼吸练习.html" {
		t.Fatalf("artifacts[0].FileName = %q, want %q", reporter.artifacts[0].FileName, "呼吸练习.html")
	}
}

func TestImportArtifactsRejectsEscapingPath(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	manifestDir := filepath.Join(cwd, ".open-xiaoai-agent", "artifacts")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(manifest) error = %v", err)
	}
	manifestPath := filepath.Join(manifestDir, "task_1.json")
	raw, err := json.Marshal(artifactManifest{
		Deliver: []artifactManifestEntry{
			{Path: "../escape.txt", Name: "escape.txt", Kind: "file"},
		},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	reporter := &fakeReporter{taskID: "task_1"}
	runner := NewClaudeRunner(nil, cwd)
	if err := runner.importArtifacts("task_1", reporter); err == nil {
		t.Fatal("importArtifacts() error = nil, want non-nil")
	}
}

func TestImportArtifactsRejectsPathOutsideTaskDeliverableDir(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	outputDir := filepath.Join(cwd, "outputs")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(outputs) error = %v", err)
	}
	filePath := filepath.Join(outputDir, "night_sky.png")
	if err := os.WriteFile(filePath, []byte("fake-png"), 0o644); err != nil {
		t.Fatalf("WriteFile(file) error = %v", err)
	}

	manifestDir := filepath.Join(cwd, ".open-xiaoai-agent", "artifacts")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(manifest) error = %v", err)
	}
	manifestPath := filepath.Join(manifestDir, "task_1.json")
	raw, err := json.Marshal(artifactManifest{
		Deliver: []artifactManifestEntry{
			{Path: "outputs/night_sky.png", Name: "night_sky.png", Kind: "file", MIMEType: "image/png"},
		},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	reporter := &fakeReporter{taskID: "task_1"}
	runner := NewClaudeRunner(nil, cwd)
	err = runner.importArtifacts("task_1", reporter)
	if err == nil {
		t.Fatal("importArtifacts() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), ".open-xiaoai-agent/deliverables/task_1") {
		t.Fatalf("importArtifacts() error = %q, want mention deliverable dir", err)
	}
}

var _ plugin.AsyncReporter = (*fakeReporter)(nil)
