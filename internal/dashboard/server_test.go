package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/assistant"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/complextask"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
)

type fakeConversations struct {
	resetCalls int
}

func (f *fakeConversations) SnapshotConversations() []assistant.ConversationSnapshot {
	return nil
}

func (f *fakeConversations) ResetConversationData() error {
	f.resetCalls++
	return nil
}

func TestHandleResetClearsRuntimeData(t *testing.T) {
	t.Parallel()

	dsn := "sqlite://" + t.TempDir() + "/agent.db"
	manager, err := tasks.NewManager(dsn)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	store, err := complextask.NewStore(dsn)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	claude := complextask.NewService(store, nil)
	conversations := &fakeConversations{}

	_, err = manager.Submit(plugin.AsyncTask{
		Plugin: "complex_task",
		Kind:   "complex_task",
		Title:  "重置测试",
		Input:  "重置测试",
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
	if err := store.Start("task_1", "做一个网页", "/tmp/work"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	server := New(":0", manager, claude, conversations)
	req := httptest.NewRequest(http.MethodPost, "/api/reset", nil)
	recorder := httptest.NewRecorder()

	server.handleReset(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if conversations.resetCalls != 1 {
		t.Fatalf("resetCalls = %d, want 1", conversations.resetCalls)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		tasksList, events := manager.Snapshot()
		if len(tasksList) == 0 && len(events) == 0 && len(store.Snapshot()) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	tasksList, events := manager.Snapshot()
	if len(tasksList) != 0 || len(events) != 0 || len(store.Snapshot()) != 0 {
		t.Fatalf("tasks=%d events=%d claude=%d", len(tasksList), len(events), len(store.Snapshot()))
	}
}

func TestHandleResetRejectsNonPost(t *testing.T) {
	t.Parallel()

	server := New(":0", nil, nil, &fakeConversations{})
	req := httptest.NewRequest(http.MethodGet, "/api/reset", nil)
	recorder := httptest.NewRecorder()

	server.handleReset(recorder, req)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}
