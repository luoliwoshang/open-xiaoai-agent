package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/assistant"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/complextask"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/settings"
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

type fakeSettings struct {
	snapshot settings.Snapshot
}

func (f *fakeSettings) Snapshot() settings.Snapshot {
	return f.snapshot
}

func (f *fakeSettings) UpdateSessionWindowSeconds(seconds int) (settings.Snapshot, error) {
	if err := settings.ValidateSessionWindowSeconds(seconds); err != nil {
		return settings.Snapshot{}, err
	}
	f.snapshot = settings.Snapshot{SessionWindowSeconds: seconds}
	return f.snapshot, nil
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
	runtimeSettings := &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}

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

	server := New(":0", manager, claude, conversations, runtimeSettings)
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

	server := New(":0", nil, nil, &fakeConversations{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}})
	req := httptest.NewRequest(http.MethodGet, "/api/reset", nil)
	recorder := httptest.NewRecorder()

	server.handleReset(recorder, req)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleSettingsReturnsSnapshot(t *testing.T) {
	t.Parallel()

	server := New(":0", nil, nil, nil, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}})
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	recorder := httptest.NewRecorder()

	server.handleSettings(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var payload struct {
		Session settings.Snapshot `json:"session"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload.Session.SessionWindowSeconds != 300 {
		t.Fatalf("SessionWindowSeconds = %d, want 300", payload.Session.SessionWindowSeconds)
	}
}

func TestHandleSessionSettingsUpdatesWindowSeconds(t *testing.T) {
	t.Parallel()

	runtimeSettings := &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}
	server := New(":0", nil, nil, nil, runtimeSettings)
	req := httptest.NewRequest(http.MethodPost, "/api/settings/session", strings.NewReader(`{"window_seconds":420}`))
	recorder := httptest.NewRecorder()

	server.handleSessionSettings(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if runtimeSettings.snapshot.SessionWindowSeconds != 420 {
		t.Fatalf("SessionWindowSeconds = %d, want 420", runtimeSettings.snapshot.SessionWindowSeconds)
	}
}

func TestHandleSessionSettingsRejectsInvalidValue(t *testing.T) {
	t.Parallel()

	server := New(":0", nil, nil, nil, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}})
	req := httptest.NewRequest(http.MethodPost, "/api/settings/session", strings.NewReader(`{"window_seconds":1}`))
	recorder := httptest.NewRecorder()

	server.handleSessionSettings(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
