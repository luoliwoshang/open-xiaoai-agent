package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/assistant"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/im"
	runtimelogs "github.com/luoliwoshang/open-xiaoai-agent/internal/logs"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/memory/filememory"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugins/complextask"
	agentserver "github.com/luoliwoshang/open-xiaoai-agent/internal/server"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/settings"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/testmysql"
)

type fakeConversations struct {
	resetCalls    int
	runtime       assistant.RuntimeStatus
	submittedText string
	submitErr     error
}

func (f *fakeConversations) SnapshotConversations() []assistant.ConversationSnapshot {
	return nil
}

func (f *fakeConversations) RuntimeStatus() assistant.RuntimeStatus {
	return f.runtime
}

func (f *fakeConversations) ResetConversationData() error {
	f.resetCalls++
	return nil
}

func (f *fakeConversations) SubmitRecognizedText(text string) error {
	if f.submitErr != nil {
		return f.submitErr
	}
	f.submittedText = text
	return nil
}

type fakeSettings struct {
	snapshot settings.Snapshot
}

type fakeXiaoAI struct {
	status agentserver.ConnectionStatus
}

func (f *fakeXiaoAI) ConnectionStatus() agentserver.ConnectionStatus {
	return f.status
}

func (f *fakeSettings) Snapshot() settings.Snapshot {
	return f.snapshot
}

func (f *fakeSettings) UpdateSessionWindowSeconds(seconds int) (settings.Snapshot, error) {
	if err := settings.ValidateSessionWindowSeconds(seconds); err != nil {
		return settings.Snapshot{}, err
	}
	f.snapshot.SessionWindowSeconds = seconds
	return f.snapshot, nil
}

func (f *fakeSettings) UpdateMemoryStorageDir(dir string) (settings.Snapshot, error) {
	if err := settings.ValidateMemoryStorageDir(dir); err != nil {
		return settings.Snapshot{}, err
	}
	f.snapshot.MemoryStorageDir = dir
	return f.snapshot, nil
}

func (f *fakeSettings) UpdateIMDelivery(enabled bool, accountID string, targetID string) (settings.Snapshot, error) {
	if err := settings.ValidateIMDelivery(enabled, accountID, targetID); err != nil {
		return settings.Snapshot{}, err
	}
	f.snapshot.IMDeliveryEnabled = enabled
	f.snapshot.IMSelectedAccountID = accountID
	f.snapshot.IMSelectedTargetID = targetID
	return f.snapshot, nil
}

type fakeIM struct {
	snapshot        im.Snapshot
	lastDeliveryCfg settings.Snapshot
	confirmAccount  im.Account
	confirmSession  string
	debugText       string
	debugImage      im.ImageSendRequest
	debugFile       im.FileSendRequest
	debugReceipt    im.DeliveryReceipt
	resetCalls      int
}

type fakeLogs struct {
	lastQuery runtimelogs.ListQuery
	page      runtimelogs.ListPage
}

type fakeMemory struct {
	file              filememory.ManagedFile
	page              filememory.ListPage
	lastGetMemoryKey  string
	lastSaveMemoryKey string
	lastSaveContent   string
	lastSaveSource    string
	lastListQuery     filememory.ListQuery
}

func (f *fakeMemory) GetFile(memoryKey string) (filememory.ManagedFile, error) {
	f.lastGetMemoryKey = memoryKey
	if f.file.MemoryKey == "" {
		f.file = filememory.ManagedFile{
			MemoryKey: memoryKey,
			Path:      "/tmp/main-voice.md",
			Content:   "# XiaoAiAgent Memory\n",
		}
	}
	return f.file, nil
}

func (f *fakeMemory) SaveFile(memoryKey string, content string, source string) (filememory.ManagedFile, error) {
	f.lastSaveMemoryKey = memoryKey
	f.lastSaveContent = content
	f.lastSaveSource = source
	f.file = filememory.ManagedFile{
		MemoryKey: memoryKey,
		Path:      "/tmp/main-voice.md",
		Content:   content,
	}
	return f.file, nil
}

func (f *fakeMemory) ListLogs(query filememory.ListQuery) (filememory.ListPage, error) {
	f.lastListQuery = query
	if f.page.Page == 0 {
		f.page = filememory.ListPage{
			Page:     query.Page,
			PageSize: query.PageSize,
			Total:    1,
			HasMore:  false,
			Items: []filememory.UpdateLog{
				{
					ID:        "memlog_1",
					MemoryKey: "main-voice",
					Source:    "assistant_reply",
					Before:    "old",
					After:     "new",
				},
			},
		}
	}
	return f.page, nil
}

func (f *fakeLogs) List(query runtimelogs.ListQuery) (runtimelogs.ListPage, error) {
	f.lastQuery = query
	if f.page.Page == 0 {
		f.page = runtimelogs.ListPage{
			Page:     query.Page,
			PageSize: query.PageSize,
			Total:    1,
			HasMore:  false,
			Items: []runtimelogs.Entry{
				{
					ID:      "log_1",
					Level:   "error",
					Source:  "assistant/service.go:141",
					Message: "intent classify failed",
					Raw:     "2026/04/28 12:00:00.000000 assistant/service.go:141: intent classify failed",
				},
			},
		}
	}
	return f.page, nil
}

func (f *fakeIM) Snapshot() im.Snapshot {
	return f.snapshot
}

func (f *fakeIM) StartWeChatLogin() (im.WeChatLoginStart, error) {
	return im.WeChatLoginStart{SessionKey: "sess"}, nil
}

func (f *fakeIM) PollWeChatLogin(sessionKey string) (im.WeChatLoginStatus, error) {
	return im.WeChatLoginStatus{Status: "pending", Message: sessionKey}, nil
}

func (f *fakeIM) ConfirmWeChatLogin(sessionKey string) (im.Account, error) {
	f.confirmSession = sessionKey
	if f.confirmAccount.ID == "" {
		f.confirmAccount = im.Account{
			ID:              "account_1",
			Platform:        im.PlatformWeChat,
			RemoteAccountID: "bot@im.bot",
			OwnerUserID:     "user@im.wechat",
			DisplayName:     "bot@im.bot",
			BaseURL:         "https://example.com",
		}
	}
	return f.confirmAccount, nil
}

func (f *fakeIM) SendTextToDefaultChannel(text string) (im.DeliveryReceipt, error) {
	f.debugText = text
	if f.debugReceipt.MessageID == "" {
		f.debugReceipt = im.DeliveryReceipt{
			Account: im.Account{
				ID:              "account_1",
				Platform:        im.PlatformWeChat,
				RemoteAccountID: "bot@im.bot",
				DisplayName:     "bot@im.bot",
			},
			Target: im.Target{
				ID:           "target_1",
				AccountID:    "account_1",
				Name:         "我的微信",
				TargetUserID: "user@im.wechat",
			},
			MessageID: "msg_1",
			Text:      text,
		}
	}
	return f.debugReceipt, nil
}

func (f *fakeIM) SendImageToDefaultChannel(req im.ImageSendRequest) (im.DeliveryReceipt, error) {
	f.debugImage = req
	return im.DeliveryReceipt{
		Account: im.Account{
			ID:              "account_1",
			Platform:        im.PlatformWeChat,
			RemoteAccountID: "bot@im.bot",
			DisplayName:     "bot@im.bot",
		},
		Target: im.Target{
			ID:           "target_1",
			AccountID:    "account_1",
			Name:         "我的微信",
			TargetUserID: "user@im.wechat",
		},
		MessageID:     "img_1",
		Kind:          im.DeliveryKindImage,
		Caption:       req.Caption,
		MediaFileName: req.FileName,
		MediaMimeType: req.MimeType,
	}, nil
}

func (f *fakeIM) SendFileToDefaultChannel(req im.FileSendRequest) (im.DeliveryReceipt, error) {
	f.debugFile = req
	return im.DeliveryReceipt{
		Account: im.Account{
			ID:              "account_1",
			Platform:        im.PlatformWeChat,
			RemoteAccountID: "bot@im.bot",
			DisplayName:     "bot@im.bot",
		},
		Target: im.Target{
			ID:           "target_1",
			AccountID:    "account_1",
			Name:         "我的微信",
			TargetUserID: "user@im.wechat",
		},
		MessageID:     "file_1",
		Kind:          im.DeliveryKindFile,
		Caption:       req.Caption,
		MediaFileName: req.FileName,
		MediaMimeType: req.MimeType,
	}, nil
}

func (f *fakeIM) UpsertTarget(accountID string, name string, targetUserID string, setDefault bool) (im.Target, error) {
	return im.Target{ID: "target_1", AccountID: accountID, Name: name, TargetUserID: targetUserID, IsDefault: setDefault}, nil
}

func (f *fakeIM) SetDefaultTarget(accountID string, targetID string) error {
	return nil
}

func (f *fakeIM) DeleteTarget(targetID string) error {
	return nil
}

func (f *fakeIM) DeleteAccount(accountID string) error {
	return nil
}

func (f *fakeIM) UpdateDeliveryConfig(enabled bool, accountID string, targetID string) (settings.Snapshot, error) {
	if err := settings.ValidateIMDelivery(enabled, accountID, targetID); err != nil {
		return settings.Snapshot{}, err
	}
	f.lastDeliveryCfg = settings.Snapshot{
		SessionWindowSeconds: 300,
		IMDeliveryEnabled:    enabled,
		IMSelectedAccountID:  accountID,
		IMSelectedTargetID:   targetID,
	}
	return f.lastDeliveryCfg, nil
}

func (f *fakeIM) Reset() error {
	f.resetCalls++
	return nil
}

func TestHandleResetClearsRuntimeData(t *testing.T) {
	t.Parallel()

	dsn := testmysql.NewDSN(t)
	manager, err := tasks.NewManager(dsn, t.TempDir())
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

	imGateway := &fakeIM{}
	server := New(":0", manager, claude, conversations, &fakeXiaoAI{}, runtimeSettings, &fakeMemory{}, imGateway, &fakeLogs{})
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

	server := New(":0", nil, nil, &fakeConversations{}, &fakeXiaoAI{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, &fakeMemory{}, &fakeIM{}, &fakeLogs{})
	req := httptest.NewRequest(http.MethodGet, "/api/reset", nil)
	recorder := httptest.NewRecorder()

	server.handleReset(recorder, req)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleStateIncludesAssistantRuntime(t *testing.T) {
	t.Parallel()

	conversations := &fakeConversations{
		runtime: assistant.RuntimeStatus{
			Busy:              true,
			ResultReportReady: true,
			HasVoiceChannel:   true,
		},
	}
	xiaoai := &fakeXiaoAI{
		status: agentserver.ConnectionStatus{
			Connected:      true,
			ActiveSessions: 1,
			LastRemoteAddr: "192.168.1.10:34567",
		},
	}
	server := New(":0", nil, nil, conversations, xiaoai, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, &fakeMemory{}, &fakeIM{}, &fakeLogs{})
	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	recorder := httptest.NewRecorder()

	server.handleState(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var payload struct {
		Assistant assistant.RuntimeStatus      `json:"assistant"`
		XiaoAI    agentserver.ConnectionStatus `json:"xiaoai"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !payload.Assistant.Busy {
		t.Fatal("Assistant.Busy = false, want true")
	}
	if !payload.Assistant.ResultReportReady {
		t.Fatal("Assistant.ResultReportReady = false, want true")
	}
	if !payload.Assistant.HasVoiceChannel {
		t.Fatal("Assistant.HasVoiceChannel = false, want true")
	}
	if !payload.XiaoAI.Connected {
		t.Fatal("XiaoAI.Connected = false, want true")
	}
	if payload.XiaoAI.ActiveSessions != 1 {
		t.Fatalf("XiaoAI.ActiveSessions = %d, want 1", payload.XiaoAI.ActiveSessions)
	}
}

func TestHandleXiaoAIStatusReturnsRuntimeStatus(t *testing.T) {
	t.Parallel()

	xiaoai := &fakeXiaoAI{
		status: agentserver.ConnectionStatus{
			Connected:          true,
			ActiveSessions:     1,
			LastRemoteAddr:     "192.168.1.10:34567",
			LastConnectedAt:    time.Unix(100, 0),
			LastDisconnectedAt: time.Unix(90, 0),
		},
	}
	server := New(":0", nil, nil, &fakeConversations{}, xiaoai, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, &fakeMemory{}, &fakeIM{}, &fakeLogs{})
	req := httptest.NewRequest(http.MethodGet, "/api/xiaoai/status", nil)
	recorder := httptest.NewRecorder()

	server.handleXiaoAIStatus(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var payload struct {
		XiaoAI agentserver.ConnectionStatus `json:"xiaoai"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !payload.XiaoAI.Connected {
		t.Fatal("XiaoAI.Connected = false, want true")
	}
	if payload.XiaoAI.LastRemoteAddr != "192.168.1.10:34567" {
		t.Fatalf("XiaoAI.LastRemoteAddr = %q", payload.XiaoAI.LastRemoteAddr)
	}
}

func TestHandleAssistantASRAcceptsPostedText(t *testing.T) {
	t.Parallel()

	conversations := &fakeConversations{}
	server := New(":0", nil, nil, conversations, &fakeXiaoAI{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, &fakeMemory{}, &fakeIM{}, &fakeLogs{})
	req := httptest.NewRequest(http.MethodPost, "/api/assistant/asr", strings.NewReader(`{"text":"帮我总结一下今天的任务"}`))
	recorder := httptest.NewRecorder()

	server.handleAssistantASR(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if conversations.submittedText != "帮我总结一下今天的任务" {
		t.Fatalf("submittedText = %q, want payload text", conversations.submittedText)
	}

	var payload struct {
		OK   bool   `json:"ok"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !payload.OK {
		t.Fatal("OK = false, want true")
	}
	if payload.Text != "帮我总结一下今天的任务" {
		t.Fatalf("Text = %q, want payload text", payload.Text)
	}
}

func TestHandleAssistantASRMapsBusyToConflict(t *testing.T) {
	t.Parallel()

	conversations := &fakeConversations{submitErr: assistant.ErrVoiceChannelBusy}
	server := New(":0", nil, nil, conversations, &fakeXiaoAI{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, &fakeMemory{}, &fakeIM{}, &fakeLogs{})
	req := httptest.NewRequest(http.MethodPost, "/api/assistant/asr", strings.NewReader(`{"text":"帮我继续刚刚那个任务"}`))
	recorder := httptest.NewRecorder()

	server.handleAssistantASR(recorder, req)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
}

func TestHandleSettingsReturnsSnapshot(t *testing.T) {
	t.Parallel()

	server := New(":0", nil, nil, nil, &fakeXiaoAI{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, &fakeMemory{}, &fakeIM{}, &fakeLogs{})
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	recorder := httptest.NewRecorder()

	server.handleSettings(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var payload struct {
		Settings settings.Snapshot `json:"settings"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload.Settings.SessionWindowSeconds != 300 {
		t.Fatalf("SessionWindowSeconds = %d, want 300", payload.Settings.SessionWindowSeconds)
	}
}

func TestHandleSessionSettingsUpdatesWindowSeconds(t *testing.T) {
	t.Parallel()

	runtimeSettings := &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}
	server := New(":0", nil, nil, nil, &fakeXiaoAI{}, runtimeSettings, &fakeMemory{}, &fakeIM{}, &fakeLogs{})
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

	server := New(":0", nil, nil, nil, &fakeXiaoAI{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, &fakeMemory{}, &fakeIM{}, &fakeLogs{})
	req := httptest.NewRequest(http.MethodPost, "/api/settings/session", strings.NewReader(`{"window_seconds":1}`))
	recorder := httptest.NewRecorder()

	server.handleSessionSettings(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestHandleMemorySettingsUpdatesStorageDir(t *testing.T) {
	t.Parallel()

	runtimeSettings := &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300, MemoryStorageDir: ".open-xiaoai-agent/memory"}}
	server := New(":0", nil, nil, nil, &fakeXiaoAI{}, runtimeSettings, &fakeMemory{}, &fakeIM{}, &fakeLogs{})
	req := httptest.NewRequest(http.MethodPost, "/api/settings/memory", strings.NewReader(`{"memory_storage_dir":"./memory-dev"}`))
	recorder := httptest.NewRecorder()

	server.handleMemorySettings(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if runtimeSettings.snapshot.MemoryStorageDir != "./memory-dev" {
		t.Fatalf("MemoryStorageDir = %q, want ./memory-dev", runtimeSettings.snapshot.MemoryStorageDir)
	}
}

func TestHandleMemoryFileDefaultsToMainVoice(t *testing.T) {
	t.Parallel()

	memoryStore := &fakeMemory{}
	server := New(":0", nil, nil, nil, &fakeXiaoAI{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, memoryStore, &fakeIM{}, &fakeLogs{})
	req := httptest.NewRequest(http.MethodGet, "/api/memory/file", nil)
	recorder := httptest.NewRecorder()

	server.handleMemoryFile(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if memoryStore.lastGetMemoryKey != assistant.MainVoiceHistoryKey {
		t.Fatalf("lastGetMemoryKey = %q, want %q", memoryStore.lastGetMemoryKey, assistant.MainVoiceHistoryKey)
	}
}

func TestHandleMemoryFileSavesContent(t *testing.T) {
	t.Parallel()

	memoryStore := &fakeMemory{}
	server := New(":0", nil, nil, nil, &fakeXiaoAI{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, memoryStore, &fakeIM{}, &fakeLogs{})
	req := httptest.NewRequest(http.MethodPost, "/api/memory/file", strings.NewReader(`{"content":"# 手动维护\n\n- 常用地址：https://ha.example.com\n"}`))
	recorder := httptest.NewRecorder()

	server.handleMemoryFile(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if memoryStore.lastSaveMemoryKey != assistant.MainVoiceHistoryKey {
		t.Fatalf("lastSaveMemoryKey = %q, want %q", memoryStore.lastSaveMemoryKey, assistant.MainVoiceHistoryKey)
	}
	if memoryStore.lastSaveSource != filememory.DashboardManualSource {
		t.Fatalf("lastSaveSource = %q, want %q", memoryStore.lastSaveSource, filememory.DashboardManualSource)
	}
	if !strings.Contains(memoryStore.lastSaveContent, "ha.example.com") {
		t.Fatalf("lastSaveContent = %q", memoryStore.lastSaveContent)
	}
}

func TestHandleMemoryLogsReturnsPage(t *testing.T) {
	t.Parallel()

	memoryStore := &fakeMemory{}
	server := New(":0", nil, nil, nil, &fakeXiaoAI{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, memoryStore, &fakeIM{}, &fakeLogs{})
	req := httptest.NewRequest(http.MethodGet, "/api/memory/logs?page=2&page_size=10&memory_key=main-voice", nil)
	recorder := httptest.NewRecorder()

	server.handleMemoryLogs(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if memoryStore.lastListQuery.Page != 2 || memoryStore.lastListQuery.PageSize != 10 || memoryStore.lastListQuery.MemoryKey != "main-voice" {
		t.Fatalf("lastListQuery = %+v", memoryStore.lastListQuery)
	}

	var payload filememory.ListPage
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload.Page != 2 || payload.PageSize != 10 {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestHandleWeChatLoginConfirmPersistsAfterExplicitConfirmation(t *testing.T) {
	t.Parallel()

	imGateway := &fakeIM{}
	server := New(":0", nil, nil, nil, &fakeXiaoAI{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, &fakeMemory{}, imGateway, &fakeLogs{})
	req := httptest.NewRequest(http.MethodPost, "/api/im/wechat/login/confirm", strings.NewReader(`{"session_key":"sess-1"}`))
	recorder := httptest.NewRecorder()

	server.handleWeChatLoginConfirm(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if imGateway.confirmSession != "sess-1" {
		t.Fatalf("confirmSession = %q, want sess-1", imGateway.confirmSession)
	}

	var payload struct {
		OK      bool       `json:"ok"`
		Account im.Account `json:"account"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !payload.OK {
		t.Fatal("OK = false, want true")
	}
	if payload.Account.RemoteAccountID != "bot@im.bot" {
		t.Fatalf("RemoteAccountID = %q, want bot@im.bot", payload.Account.RemoteAccountID)
	}
}

func TestHandleIMDebugSendDefaultUsesPostedText(t *testing.T) {
	t.Parallel()

	imGateway := &fakeIM{}
	server := New(":0", nil, nil, nil, &fakeXiaoAI{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, &fakeMemory{}, imGateway, &fakeLogs{})
	req := httptest.NewRequest(http.MethodPost, "/api/im/debug/send-default", strings.NewReader(`{"text":"调试消息"}`))
	recorder := httptest.NewRecorder()

	server.handleIMDebugSendDefault(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if imGateway.debugText != "调试消息" {
		t.Fatalf("debugText = %q, want 调试消息", imGateway.debugText)
	}

	var payload struct {
		OK      bool               `json:"ok"`
		Receipt im.DeliveryReceipt `json:"receipt"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !payload.OK {
		t.Fatal("OK = false, want true")
	}
	if payload.Receipt.MessageID != "msg_1" {
		t.Fatalf("MessageID = %q, want msg_1", payload.Receipt.MessageID)
	}
	if payload.Receipt.Text != "调试消息" {
		t.Fatalf("Text = %q, want 调试消息", payload.Receipt.Text)
	}
}

func TestHandleIMDebugSendImageDefaultUsesUploadedFile(t *testing.T) {
	t.Parallel()

	imGateway := &fakeIM{}
	server := New(":0", nil, nil, nil, &fakeXiaoAI{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, &fakeMemory{}, imGateway, &fakeLogs{})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("file", "rabbit.png")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := fileWriter.Write([]byte("png-data")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.WriteField("caption", "测试图片"); err != nil {
		t.Fatalf("WriteField() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/im/debug/send-image-default", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	server.handleIMDebugSendImageDefault(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if imGateway.debugImage.FileName != "rabbit.png" {
		t.Fatalf("FileName = %q, want rabbit.png", imGateway.debugImage.FileName)
	}
	if string(imGateway.debugImage.Content) != "png-data" {
		t.Fatalf("Content = %q, want png-data", string(imGateway.debugImage.Content))
	}
	if imGateway.debugImage.Caption != "测试图片" {
		t.Fatalf("Caption = %q, want 测试图片", imGateway.debugImage.Caption)
	}

	var payload struct {
		OK      bool               `json:"ok"`
		Receipt im.DeliveryReceipt `json:"receipt"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !payload.OK {
		t.Fatal("OK = false, want true")
	}
	if payload.Receipt.Kind != im.DeliveryKindImage {
		t.Fatalf("Kind = %q, want %q", payload.Receipt.Kind, im.DeliveryKindImage)
	}
	if payload.Receipt.MediaFileName != "rabbit.png" {
		t.Fatalf("MediaFileName = %q, want rabbit.png", payload.Receipt.MediaFileName)
	}
}

func TestHandleIMDebugSendFileDefaultUsesUploadedFile(t *testing.T) {
	t.Parallel()

	imGateway := &fakeIM{}
	server := New(":0", nil, nil, nil, &fakeXiaoAI{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, &fakeMemory{}, imGateway, &fakeLogs{})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("file", "story.txt")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := fileWriter.Write([]byte("file-data")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.WriteField("caption", "测试文件"); err != nil {
		t.Fatalf("WriteField() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/im/debug/send-file-default", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	server.handleIMDebugSendFileDefault(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if imGateway.debugFile.FileName != "story.txt" {
		t.Fatalf("FileName = %q, want story.txt", imGateway.debugFile.FileName)
	}
	if string(imGateway.debugFile.Content) != "file-data" {
		t.Fatalf("Content = %q, want file-data", string(imGateway.debugFile.Content))
	}
	if imGateway.debugFile.Caption != "测试文件" {
		t.Fatalf("Caption = %q, want 测试文件", imGateway.debugFile.Caption)
	}

	var payload struct {
		OK      bool               `json:"ok"`
		Receipt im.DeliveryReceipt `json:"receipt"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !payload.OK {
		t.Fatal("OK = false, want true")
	}
	if payload.Receipt.Kind != im.DeliveryKindFile {
		t.Fatalf("Kind = %q, want %q", payload.Receipt.Kind, im.DeliveryKindFile)
	}
	if payload.Receipt.MediaFileName != "story.txt" {
		t.Fatalf("MediaFileName = %q, want story.txt", payload.Receipt.MediaFileName)
	}
}

func TestHandleTaskArtifactDownloadServesSavedArtifact(t *testing.T) {
	t.Parallel()

	dsn := testmysql.NewDSN(t)
	manager, err := tasks.NewManager(dsn, t.TempDir())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	task, err := manager.Submit(plugin.AsyncTask{
		Plugin: "artifact_test",
		Kind:   "artifact_test",
		Title:  "下载测试",
		Input:  "生成一个可下载文件",
		Run: func(ctx context.Context, reporter plugin.AsyncReporter) (string, error) {
			_, err := reporter.PutArtifact(plugin.PutArtifactRequest{
				Name:     "story.txt",
				Kind:     "file",
				MIMEType: "text/plain",
				Reader:   strings.NewReader("download-me"),
				Size:     int64(len("download-me")),
			})
			if err != nil {
				return "", err
			}
			return "文件已经就绪。", nil
		},
	})
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	var artifact tasks.Artifact
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		artifacts := manager.ArtifactsSnapshot()
		if len(artifacts) == 1 {
			artifact = artifacts[0]
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if artifact.ID == "" {
		t.Fatal("artifact not found before timeout")
	}

	server := New(":0", manager, nil, nil, &fakeXiaoAI{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, &fakeMemory{}, &fakeIM{}, &fakeLogs{})
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/tasks/%s/artifacts/%s/download", task.ID, artifact.ID), nil)
	recorder := httptest.NewRecorder()

	server.handleTaskArtifactDownload(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if body := recorder.Body.String(); body != "download-me" {
		t.Fatalf("body = %q, want %q", body, "download-me")
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/plain" {
		t.Fatalf("Content-Type = %q, want %q", got, "text/plain")
	}
	if got := recorder.Header().Get("Content-Disposition"); !strings.Contains(got, "story.txt") {
		t.Fatalf("Content-Disposition = %q, want contains story.txt", got)
	}
}

func TestHandleLogsReturnsPaginatedEntries(t *testing.T) {
	t.Parallel()

	logStore := &fakeLogs{}
	server := New(":0", nil, nil, nil, &fakeXiaoAI{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, &fakeMemory{}, &fakeIM{}, logStore)
	req := httptest.NewRequest(http.MethodGet, "/api/logs?page=2&page_size=25", nil)
	recorder := httptest.NewRecorder()

	server.handleLogs(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if logStore.lastQuery.Page != 2 {
		t.Fatalf("Page = %d, want 2", logStore.lastQuery.Page)
	}
	if logStore.lastQuery.PageSize != 25 {
		t.Fatalf("PageSize = %d, want 25", logStore.lastQuery.PageSize)
	}

	var payload runtimelogs.ListPage
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(payload.Items))
	}
	if payload.Items[0].Source != "assistant/service.go:141" {
		t.Fatalf("Source = %q, want assistant/service.go:141", payload.Items[0].Source)
	}
}

func TestHandleLogsRejectsInvalidPage(t *testing.T) {
	t.Parallel()

	server := New(":0", nil, nil, nil, &fakeXiaoAI{}, &fakeSettings{snapshot: settings.Snapshot{SessionWindowSeconds: 300}}, &fakeMemory{}, &fakeIM{}, &fakeLogs{})
	req := httptest.NewRequest(http.MethodGet, "/api/logs?page=oops", nil)
	recorder := httptest.NewRecorder()

	server.handleLogs(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
