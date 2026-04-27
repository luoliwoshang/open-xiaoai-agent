package continuetask

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
)

type fakeTaskLookup struct {
	task *tasks.Task
}

func (f fakeTaskLookup) GetTask(taskID string) (*tasks.Task, bool) {
	if f.task == nil || f.task.ID != taskID {
		return nil, false
	}
	copyTask := *f.task
	return &copyTask, true
}

type fakeResumer struct{}

func (fakeResumer) ResumeTask(ctx context.Context, taskID string, request string, reporter plugin.AsyncReporter) (string, error) {
	return "ok", nil
}

func TestContinueTaskRegisterAndCall(t *testing.T) {
	t.Parallel()

	registry := plugin.NewRegistry()
	resumes := NewResumeRegistry()
	resumes.Register("complex_task", fakeResumer{})

	task := &tasks.Task{
		ID:     "task_1",
		Plugin: "complex_task",
		Kind:   "complex_task",
		Title:  "小游戏网页",
		State:  tasks.StateCompleted,
	}
	if err := Register(registry, fakeTaskLookup{task: task}, resumes); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	result, err := registry.Call(context.Background(), "continue_task", json.RawMessage(`{"plugin_name":"complex_task","task_id":"task_1","request":"再加一个开始按钮"}`))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if result.AsyncTask == nil {
		t.Fatal("result.AsyncTask = nil")
	}
	if result.AsyncTask.Plugin != "complex_task" {
		t.Fatalf("result.AsyncTask.Plugin = %q", result.AsyncTask.Plugin)
	}
	if result.AsyncTask.ParentTaskID != "task_1" {
		t.Fatalf("result.AsyncTask.ParentTaskID = %q", result.AsyncTask.ParentTaskID)
	}
	if result.AsyncTask.Title != "接续：小游戏网页" {
		t.Fatalf("result.AsyncTask.Title = %q", result.AsyncTask.Title)
	}
}
