package continuetask

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
)

type fakeTaskLookup struct {
	task       *tasks.Task
	callOrder  *[]string
	superseded bool
	interrupts int
}

func (f fakeTaskLookup) GetTask(taskID string) (*tasks.Task, bool) {
	if f.task == nil || f.task.ID != taskID {
		return nil, false
	}
	copyTask := *f.task
	return &copyTask, true
}

func (f fakeTaskLookup) InterruptTask(taskID string) error {
	if f.callOrder != nil {
		*f.callOrder = append(*f.callOrder, "manager_interrupt")
	}
	if f.task != nil && f.task.ID == taskID {
		f.interrupts++
	}
	return nil
}

func (f fakeTaskLookup) MarkTaskSuperseded(taskID string, summary string) (bool, error) {
	if f.callOrder != nil {
		*f.callOrder = append(*f.callOrder, "manager_supersede")
	}
	if f.task == nil || f.task.ID != taskID {
		return false, nil
	}
	if f.task.State != tasks.StateAccepted && f.task.State != tasks.StateRunning {
		return false, nil
	}
	f.task.State = tasks.StateSuperseded
	f.task.Summary = summary
	f.superseded = true
	return true, nil
}

type fakeResumer struct {
	callOrder *[]string
}

func (f fakeResumer) InterruptTask(ctx context.Context, taskID string) error {
	if f.callOrder != nil {
		*f.callOrder = append(*f.callOrder, "plugin_interrupt")
	}
	return nil
}

func (f fakeResumer) ResumeTask(ctx context.Context, taskID string, request string, reporter plugin.AsyncReporter) (string, error) {
	if f.callOrder != nil {
		*f.callOrder = append(*f.callOrder, "plugin_resume")
	}
	return "ok", nil
}

type fakeAsyncReporter struct {
	taskID string
}

func (f fakeAsyncReporter) TaskID() string                               { return f.taskID }
func (f fakeAsyncReporter) Update(summary string) error                  { return nil }
func (f fakeAsyncReporter) Event(eventType string, message string) error { return nil }
func (f fakeAsyncReporter) PutArtifact(req plugin.PutArtifactRequest) (plugin.ArtifactRef, error) {
	return plugin.ArtifactRef{}, nil
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

	result, err := registry.Call(context.Background(), "continue_task", json.RawMessage(`{"task_id":"task_1","request":"再加一个开始按钮"}`))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if result.AsyncTask == nil {
		t.Fatal("result.AsyncTask = nil")
	}
	if result.Text != "我这就去做！" {
		t.Fatalf("result.Text = %q", result.Text)
	}
	if result.AsyncTask.Plugin != "complex_task" {
		t.Fatalf("result.AsyncTask.Plugin = %q", result.AsyncTask.Plugin)
	}
	if result.AsyncTask.ParentTaskID != "task_1" {
		t.Fatalf("result.AsyncTask.ParentTaskID = %q", result.AsyncTask.ParentTaskID)
	}
	if result.AsyncTask.Title != "再加一个开始按钮" {
		t.Fatalf("result.AsyncTask.Title = %q", result.AsyncTask.Title)
	}
	if result.AsyncTask.Input != "再加一个开始按钮" {
		t.Fatalf("result.AsyncTask.Input = %q", result.AsyncTask.Input)
	}
}

func TestContinueTaskRunningSourceInterruptsThenResumes(t *testing.T) {
	t.Parallel()

	registry := plugin.NewRegistry()
	order := make([]string, 0, 4)
	task := &tasks.Task{
		ID:     "task_1",
		Plugin: "complex_task",
		Kind:   "complex_task",
		Title:  "小游戏网页",
		State:  tasks.StateRunning,
	}
	manager := fakeTaskLookup{task: task, callOrder: &order}
	resumes := NewResumeRegistry()
	resumes.Register("complex_task", fakeResumer{callOrder: &order})

	if err := Register(registry, manager, resumes); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	result, err := registry.Call(context.Background(), "continue_task", json.RawMessage(`{"task_id":"task_1","request":"再加一个开始按钮"}`))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if result.AsyncTask == nil {
		t.Fatal("result.AsyncTask = nil")
	}
	if result.Text != "我这就去做！" {
		t.Fatalf("result.Text = %q", result.Text)
	}
	if _, err := result.AsyncTask.Run(context.Background(), fakeAsyncReporter{taskID: "task_2"}); err != nil {
		t.Fatalf("AsyncTask.Run() error = %v", err)
	}

	want := []string{"manager_interrupt", "plugin_interrupt", "manager_supersede", "plugin_resume"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
	if task.State != tasks.StateSuperseded {
		t.Fatalf("task.State = %q, want %q", task.State, tasks.StateSuperseded)
	}
}
