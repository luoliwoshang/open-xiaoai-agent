package filememory

import (
	"context"
	"strings"
	"testing"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/settings"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/testmysql"
)

type fakeSettings struct {
	snapshot settings.Snapshot
}

func (f fakeSettings) Snapshot() settings.Snapshot {
	return f.snapshot
}

func (f fakeSettings) MemoryStorageDir() string {
	return f.snapshot.MemoryStorageDir
}

type fakeUpdater struct {
	result string
}

func (f fakeUpdater) UpdateFromSession(ctx context.Context, memoryKey string, currentMemory string, history []llm.Message) (string, error) {
	_ = ctx
	_ = memoryKey
	_ = currentMemory
	_ = history
	return f.result, nil
}

func TestRecallCreatesDefaultFile(t *testing.T) {
	t.Parallel()

	service, err := New(testmysql.NewDSN(t), fakeSettings{
		snapshot: settings.Snapshot{MemoryStorageDir: t.TempDir()},
	}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := service.Recall(context.Background(), "main-voice")
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if !strings.Contains(got, "# XiaoAiAgent Memory") {
		t.Fatalf("Recall() = %q", got)
	}
}

func TestUpdateFromSessionRewritesFileAndWritesLog(t *testing.T) {
	t.Parallel()

	service, err := New(testmysql.NewDSN(t), fakeSettings{
		snapshot: settings.Snapshot{MemoryStorageDir: t.TempDir()},
	}, fakeUpdater{
		result: "# XiaoAiAgent Memory\n\n## 长期记忆\n\n- 用户家里 Home Assistant 地址是 http://ha.local:8123\n\n## 最近一次会话整理\n\n- 这次提到了 Home Assistant 地址。\n",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = service.UpdateFromSession(context.Background(), "main-voice", []llm.Message{
		{Role: "user", Content: "我家里 Home Assistant 的地址是 http://ha.local:8123"},
		{Role: "assistant", Content: "好的，我记一下这个地址。"},
	})
	if err != nil {
		t.Fatalf("UpdateFromSession() error = %v", err)
	}

	file, err := service.GetFile("main-voice")
	if err != nil {
		t.Fatalf("GetFile() error = %v", err)
	}
	if !strings.Contains(file.Content, "Home Assistant") {
		t.Fatalf("file.Content = %q", file.Content)
	}

	page, err := service.ListLogs(ListQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListLogs() error = %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("len(page.Items) = %d, want 1", len(page.Items))
	}
	if page.Items[0].Source != SessionSummarySource {
		t.Fatalf("page.Items[0].Source = %q", page.Items[0].Source)
	}
}

func TestSaveFileWritesManualLog(t *testing.T) {
	t.Parallel()

	service, err := New(testmysql.NewDSN(t), fakeSettings{
		snapshot: settings.Snapshot{MemoryStorageDir: t.TempDir()},
	}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	file, err := service.SaveFile("main-voice", "# 手动维护\n\n- 常用地址：https://ha.example.com\n", DashboardManualSource)
	if err != nil {
		t.Fatalf("SaveFile() error = %v", err)
	}
	if !strings.Contains(file.Content, "ha.example.com") {
		t.Fatalf("file.Content = %q", file.Content)
	}

	page, err := service.ListLogs(ListQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListLogs() error = %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("len(page.Items) = %d, want 1", len(page.Items))
	}
	if page.Items[0].Source != DashboardManualSource {
		t.Fatalf("page.Items[0].Source = %q", page.Items[0].Source)
	}
}
