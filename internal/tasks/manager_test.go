package tasks

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/testmysql"
)

func TestManagerSubmitCompletesAndPreparesResultReport(t *testing.T) {
	t.Helper()

	manager, err := NewManager(testmysql.NewDSN(t), t.TempDir())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	task, err := manager.Submit(plugin.AsyncTask{
		Plugin: "complex_task",
		Kind:   "complex_task",
		Title:  "小游戏网页",
		Input:  "做一个小游戏网页",
		Run: func(ctx context.Context, reporter plugin.AsyncReporter) (string, error) {
			if err := reporter.Update("正在生成第一版"); err != nil {
				return "", err
			}
			return "小游戏网页已经完成，可以开始体验了。", nil
		},
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if task.State != StateAccepted {
		t.Fatalf("submitted task.State = %q, want %q", task.State, StateAccepted)
	}
	if task.Plugin != "complex_task" {
		t.Fatalf("submitted task.Plugin = %q, want %q", task.Plugin, "complex_task")
	}

	completed := waitForTaskState(t, manager, task.ID, StateCompleted)
	if !completed.ResultReportPending {
		t.Fatal("completed.ResultReportPending = false, want true")
	}
	if completed.Result == "" {
		t.Fatal("completed.Result = empty, want non-empty")
	}

	report, ids := manager.BuildResultReport(3)
	if !strings.Contains(report, "小游戏网页已经完成了：小游戏网页已经完成，可以开始体验了。") {
		t.Fatalf("report = %q", report)
	}
	if len(ids) != 1 || ids[0] != task.ID {
		t.Fatalf("ids = %#v, want [%q]", ids, task.ID)
	}

	if err := manager.MarkResultReported(ids); err != nil {
		t.Fatalf("MarkResultReported() error = %v", err)
	}

	report, ids = manager.BuildResultReport(3)
	if report != "" || len(ids) != 0 {
		t.Fatalf("after MarkResultReported report=%q ids=%#v, want empty", report, ids)
	}
}

func TestManagerCancelLatest(t *testing.T) {
	t.Helper()

	manager, err := NewManager(testmysql.NewDSN(t), t.TempDir())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	task, err := manager.Submit(plugin.AsyncTask{
		Plugin: "complex_task",
		Kind:   "complex_task",
		Title:  "旅行攻略",
		Input:  "做一份旅行攻略",
		Run: func(ctx context.Context, reporter plugin.AsyncReporter) (string, error) {
			if err := reporter.Update("正在搜集目的地信息"); err != nil {
				return "", err
			}
			<-ctx.Done()
			return "", ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	waitForTaskState(t, manager, task.ID, StateRunning)

	canceled, err := manager.CancelLatest()
	if err != nil {
		t.Fatalf("CancelLatest() error = %v", err)
	}
	if canceled == nil {
		t.Fatal("CancelLatest() = nil, want task")
	}
	if canceled.State != StateCanceled {
		t.Fatalf("canceled.State = %q, want %q", canceled.State, StateCanceled)
	}

	finalTask := waitForTaskState(t, manager, task.ID, StateCanceled)
	if !finalTask.ResultReportPending {
		t.Fatal("finalTask.ResultReportPending = false, want true")
	}

	report, ids := manager.BuildResultReport(3)
	if !strings.Contains(report, "旅行攻略已经取消了：任务已取消") {
		t.Fatalf("report = %q", report)
	}
	if len(ids) != 1 || ids[0] != task.ID {
		t.Fatalf("ids = %#v, want [%q]", ids, task.ID)
	}
}

func TestCompletedTasksForIntentIncludesPluginSummary(t *testing.T) {
	t.Helper()

	manager, err := NewManager(testmysql.NewDSN(t), t.TempDir())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	now := time.Now()
	manager.state.Tasks = []Task{
		{
			ID:        "task_1",
			Plugin:    "complex_task",
			Kind:      "complex_task",
			Title:     "做网页",
			State:     StateCompleted,
			Summary:   "已经做好一个可交付网页，文件放在桌面。",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	text := manager.CompletedTasksForIntent(3)
	if !strings.Contains(text, "task_id=task_1 plugin=complex_task title=做网页 summary=已经做好一个可交付网页，文件放在桌面。") {
		t.Fatalf("text = %q", text)
	}
}

func TestSnapshotFiltersClaudeOutputEvents(t *testing.T) {
	t.Helper()

	manager, err := NewManager(testmysql.NewDSN(t), t.TempDir())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	now := time.Now()
	manager.state.Events = []Event{
		{
			ID:        "event_1",
			TaskID:    "task_1",
			Type:      "progress",
			Message:   "正在执行",
			CreatedAt: now,
		},
		{
			ID:        "event_2",
			TaskID:    "task_1",
			Type:      "claude_output",
			Message:   "重复的 Claude 输出",
			CreatedAt: now.Add(1 * time.Second),
		},
	}

	_, events := manager.Snapshot()
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].Type != "progress" {
		t.Fatalf("events[0].Type = %q, want %q", events[0].Type, "progress")
	}
}

func TestSummarizeProgressIncludesStateAndSummary(t *testing.T) {
	t.Helper()

	manager, err := NewManager(testmysql.NewDSN(t), t.TempDir())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	now := time.Now()
	manager.state.Tasks = []Task{
		{
			ID:        "task_1",
			Title:     "做网页",
			State:     StateRunning,
			Summary:   "Claude 正在生成页面结构",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	text := manager.SummarizeProgress(3)
	if !strings.Contains(text, "任务：做网页，任务状态：进行中，任务目前阶段summary：Claude 正在生成页面结构") {
		t.Fatalf("text = %q", text)
	}
}

func TestManagerResetClearsTasksAndEvents(t *testing.T) {
	t.Helper()

	manager, err := NewManager(testmysql.NewDSN(t), t.TempDir())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	task, err := manager.Submit(plugin.AsyncTask{
		Plugin: "complex_task",
		Kind:   "complex_task",
		Title:  "清理测试",
		Input:  "清理测试",
		Run: func(ctx context.Context, reporter plugin.AsyncReporter) (string, error) {
			if err := reporter.Update("正在执行"); err != nil {
				return "", err
			}
			<-ctx.Done()
			return "", ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	waitForTaskState(t, manager, task.ID, StateRunning)

	if err := manager.Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	tasksList, events := manager.Snapshot()
	if len(tasksList) != 0 {
		t.Fatalf("len(tasksList) = %d, want 0", len(tasksList))
	}
	if len(events) != 0 {
		t.Fatalf("len(events) = %d, want 0", len(events))
	}
}

func TestManagerPutArtifactAndDownloadMetadata(t *testing.T) {
	t.Helper()

	manager, err := NewManager(testmysql.NewDSN(t), t.TempDir())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	task, err := manager.Submit(plugin.AsyncTask{
		Plugin: "artifact_test",
		Kind:   "artifact_test",
		Title:  "产物测试",
		Input:  "生成一个测试文件",
		Run: func(ctx context.Context, reporter plugin.AsyncReporter) (string, error) {
			_, err := reporter.PutArtifact(plugin.PutArtifactRequest{
				Name:     "story.txt",
				Kind:     "file",
				MIMEType: "text/plain",
				Reader:   strings.NewReader("hello artifact"),
				Size:     int64(len("hello artifact")),
			})
			if err != nil {
				return "", err
			}
			return "测试文件已经准备好了。", nil
		},
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	completed := waitForTaskState(t, manager, task.ID, StateCompleted)
	if completed.Result != "测试文件已经准备好了。" {
		t.Fatalf("completed.Result = %q", completed.Result)
	}

	artifacts := manager.ArtifactsSnapshot()
	if len(artifacts) != 1 {
		t.Fatalf("len(artifacts) = %d, want 1", len(artifacts))
	}
	if artifacts[0].TaskID != task.ID {
		t.Fatalf("artifacts[0].TaskID = %q, want %q", artifacts[0].TaskID, task.ID)
	}
	if got := strings.TrimSpace(artifacts[0].StoragePath); got == "" {
		t.Fatal("artifacts[0].StoragePath = empty")
	}
	if _, ok := manager.GetArtifact(task.ID, artifacts[0].ID); !ok {
		t.Fatal("GetArtifact() = not found, want artifact")
	}

	deliveries := manager.ListTaskArtifactDeliveries([]string{task.ID})
	if len(deliveries) != 1 {
		t.Fatalf("len(deliveries) = %d, want 1", len(deliveries))
	}
	if deliveries[0].Delivery.Status != ArtifactDeliveryPending {
		t.Fatalf("deliveries[0].Delivery.Status = %q, want %q", deliveries[0].Delivery.Status, ArtifactDeliveryPending)
	}
}

func TestManagerRejectsArtifactUpdatesAfterCompletion(t *testing.T) {
	t.Helper()

	manager, err := NewManager(testmysql.NewDSN(t), t.TempDir())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	task, err := manager.Submit(plugin.AsyncTask{
		Plugin: "artifact_test",
		Kind:   "artifact_test",
		Title:  "完成后禁止追加产物",
		Input:  "先完成再试着上报",
		Run: func(ctx context.Context, reporter plugin.AsyncReporter) (string, error) {
			return "任务完成。", nil
		},
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	waitForTaskState(t, manager, task.ID, StateCompleted)
	_, err = reporter{manager: manager, taskID: task.ID}.PutArtifact(plugin.PutArtifactRequest{
		Name:     "late.txt",
		Kind:     "file",
		MIMEType: "text/plain",
		Reader:   strings.NewReader("late"),
		Size:     int64(len("late")),
	})
	if err == nil {
		t.Fatal("PutArtifact() after completion error = nil, want non-nil")
	}
}

func waitForTaskState(t *testing.T, manager *Manager, taskID string, want State) Task {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		tasks, _ := manager.Snapshot()
		for _, task := range tasks {
			if task.ID == taskID && task.State == want {
				return task
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("task %q did not reach state %q before timeout", taskID, want)
	return Task{}
}
