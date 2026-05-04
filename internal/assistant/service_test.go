package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/memory"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/voice"
)

const testHistoryKey = "session-test"

type fakeChannel struct {
	mu           sync.Mutex
	order        []string
	scripts      []string
	prepareErr   error
	prepareCalls int
}

func (s *fakeChannel) PreparePlayback(opts voice.PlaybackOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !opts.NativeFlowInterrupted && opts.InterruptNativeFlow {
		s.prepareCalls++
		s.order = append(s.order, "prepare")
		return s.prepareErr
	}
	return nil
}

func (s *fakeChannel) SpeakText(text string, timeout time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.order = append(s.order, "speak")
	s.scripts = append(s.scripts, text)
	return nil
}

func (s *fakeChannel) snapshot() ([]string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order := make([]string, len(s.order))
	copy(order, s.order)
	return order, s.prepareCalls
}

func (s *fakeChannel) snapshotScripts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	scripts := make([]string, len(s.scripts))
	copy(scripts, s.scripts)
	return scripts
}

type fakeIntent struct {
	onDecide func(history []llm.Message, text string) llm.IntentDecision
}

func (f fakeIntent) Decide(ctx context.Context, history []llm.Message, text string) (llm.IntentDecision, error) {
	if f.onDecide != nil {
		return f.onDecide(history, text), nil
	}
	return llm.IntentDecision{}, nil
}

type fakeReply struct{}

func (fakeReply) Stream(ctx context.Context, history []llm.Message, text string, onDelta func(string) error) error {
	return onDelta("你好。")
}

func (fakeReply) StreamToolResult(ctx context.Context, history []llm.Message, userText string, toolName string, toolResult string, onDelta func(string) error) error {
	return onDelta("整理后的回复。")
}

func (fakeReply) StreamTaskResultReport(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error {
	return onDelta("整理后的任务结果汇报。")
}

type scriptedReply struct {
	onStream       func(ctx context.Context, history []llm.Message, text string, onDelta func(string) error) error
	onTool         func(ctx context.Context, history []llm.Message, userText string, toolName string, toolResult string, onDelta func(string) error) error
	onResultReport func(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error
}

func (s scriptedReply) Stream(ctx context.Context, history []llm.Message, text string, onDelta func(string) error) error {
	if s.onStream != nil {
		return s.onStream(ctx, history, text, onDelta)
	}
	return nil
}

func (s scriptedReply) StreamToolResult(ctx context.Context, history []llm.Message, userText string, toolName string, toolResult string, onDelta func(string) error) error {
	if s.onTool != nil {
		return s.onTool(ctx, history, userText, toolName, toolResult, onDelta)
	}
	return nil
}

func (s scriptedReply) StreamTaskResultReport(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error {
	if s.onResultReport != nil {
		return s.onResultReport(ctx, history, reportContext, onDelta)
	}
	return nil
}

type fakeTools struct {
	onCall func(name string) plugin.Result
}

func (f fakeTools) Call(ctx context.Context, name string, arguments json.RawMessage) (plugin.Result, error) {
	if f.onCall != nil {
		return f.onCall(name), nil
	}
	return plugin.Result{}, nil
}

type toolRunnerFunc func(ctx context.Context, name string, arguments json.RawMessage) (plugin.Result, error)

func (f toolRunnerFunc) Call(ctx context.Context, name string, arguments json.RawMessage) (plugin.Result, error) {
	return f(ctx, name, arguments)
}

type fakeTaskManager struct {
	mu                  sync.Mutex
	submittedSpec       plugin.AsyncTask
	resultReportItems   []tasks.ResultReportItem
	resultReportIDs     []string
	artifactDeliveries  []tasks.ArtifactDeliveryItem
	markedResultReports []string
	noChannelMarkedIDs  []string
	deliveredRecords    []string
	failedRecords       []string
}

func (m *fakeTaskManager) Submit(spec plugin.AsyncTask) (tasks.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.submittedSpec = spec
	return tasks.Task{
		ID:    "task_1",
		Title: spec.Title,
		State: tasks.StateAccepted,
	}, nil
}

func (m *fakeTaskManager) ListPendingResultReports(limit int) ([]tasks.ResultReportItem, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = limit
	return append([]tasks.ResultReportItem(nil), m.resultReportItems...), append([]string(nil), m.resultReportIDs...)
}

func (m *fakeTaskManager) ListTaskArtifactDeliveries(taskIDs []string) []tasks.ArtifactDeliveryItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	taskSet := make(map[string]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		taskSet[taskID] = struct{}{}
	}
	var items []tasks.ArtifactDeliveryItem
	for _, item := range m.artifactDeliveries {
		if _, ok := taskSet[item.Delivery.TaskID]; ok {
			items = append(items, item)
		}
	}
	return items
}

func (m *fakeTaskManager) MarkArtifactDeliveriesNoChannel(ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.noChannelMarkedIDs = append([]string(nil), ids...)
	for i := range m.artifactDeliveries {
		for _, id := range ids {
			if m.artifactDeliveries[i].Delivery.ID != id {
				continue
			}
			m.artifactDeliveries[i].Delivery.Status = tasks.ArtifactDeliveryNoChannel
			m.artifactDeliveries[i].Delivery.LastError = "当前没有可用的渠道发送产物"
		}
	}
	return nil
}

func (m *fakeTaskManager) MarkArtifactDeliveryDelivered(deliveryID string, accountID string, targetID string, channelLabel string, providerMessageID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deliveredRecords = append(m.deliveredRecords, deliveryID)
	for i := range m.artifactDeliveries {
		if m.artifactDeliveries[i].Delivery.ID != deliveryID {
			continue
		}
		m.artifactDeliveries[i].Delivery.Status = tasks.ArtifactDeliveryDelivered
		m.artifactDeliveries[i].Delivery.AccountID = accountID
		m.artifactDeliveries[i].Delivery.TargetID = targetID
		m.artifactDeliveries[i].Delivery.ChannelLabel = channelLabel
		m.artifactDeliveries[i].Delivery.ProviderMessageID = providerMessageID
		m.artifactDeliveries[i].Delivery.LastError = ""
	}
	return nil
}

func (m *fakeTaskManager) MarkArtifactDeliveryFailed(deliveryID string, accountID string, targetID string, channelLabel string, lastError string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failedRecords = append(m.failedRecords, deliveryID)
	for i := range m.artifactDeliveries {
		if m.artifactDeliveries[i].Delivery.ID != deliveryID {
			continue
		}
		m.artifactDeliveries[i].Delivery.Status = tasks.ArtifactDeliveryFailed
		m.artifactDeliveries[i].Delivery.AccountID = accountID
		m.artifactDeliveries[i].Delivery.TargetID = targetID
		m.artifactDeliveries[i].Delivery.ChannelLabel = channelLabel
		m.artifactDeliveries[i].Delivery.LastError = lastError
	}
	return nil
}

func (m *fakeTaskManager) MarkResultReported(ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markedResultReports = append([]string(nil), ids...)
	return nil
}

func (m *fakeTaskManager) markedResultReportsSnapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.markedResultReports...)
}

type fakeArtifactDeliverer struct {
	accountID    string
	targetID     string
	channelLabel string
	ok           bool
	resolveErr   error
	deliverErr   error
	deliveries   []string
}

func (f *fakeArtifactDeliverer) ResolveDefaultTaskArtifactDelivery() (string, string, string, bool, error) {
	return f.accountID, f.targetID, f.channelLabel, f.ok, f.resolveErr
}

func (f *fakeArtifactDeliverer) DeliverTaskArtifact(accountID string, targetID string, artifact tasks.Artifact) (string, error) {
	if f.deliverErr != nil {
		return "", f.deliverErr
	}
	f.deliveries = append(f.deliveries, artifact.FileName)
	return "msg_1", nil
}

type fakeMemoryService struct {
	recallText string

	mu         sync.Mutex
	recallKeys []string
	updated    []expiredSessionUpdate
}

type expiredSessionUpdate struct {
	MemoryKey string
	History   []llm.Message
}

func (f *fakeMemoryService) Recall(ctx context.Context, memoryKey string) (string, error) {
	_ = ctx
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recallKeys = append(f.recallKeys, memoryKey)
	return f.recallText, nil
}

func (f *fakeMemoryService) UpdateFromSession(ctx context.Context, memoryKey string, history []llm.Message) error {
	_ = ctx
	f.mu.Lock()
	defer f.mu.Unlock()
	item := expiredSessionUpdate{
		MemoryKey: memoryKey,
		History:   append([]llm.Message(nil), history...),
	}
	f.updated = append(f.updated, item)
	return nil
}

func (f *fakeMemoryService) snapshotUpdated() []expiredSessionUpdate {
	f.mu.Lock()
	defer f.mu.Unlock()
	items := make([]expiredSessionUpdate, len(f.updated))
	copy(items, f.updated)
	return items
}

func newTestServiceWithMemory(t *testing.T, config Config, intent IntentDecider, reply ReplyStreamer, tools ToolRunner, taskManager TaskManager, artifactDeliverer ArtifactDeliverySender, memoryService memory.Service) *Service {
	t.Helper()

	service, err := New(config, staticSessionWindow{window: 5 * time.Minute}, intent, reply, tools, taskManager, nil, artifactDeliverer, memoryService)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(service.Close)
	return service
}

func newTestService(t *testing.T, config Config, intent IntentDecider, reply ReplyStreamer, tools ToolRunner, taskManager TaskManager, artifactDeliverer ArtifactDeliverySender) *Service {
	t.Helper()

	return newTestServiceWithMemory(t, config, intent, reply, tools, taskManager, artifactDeliverer, nil)
}

func waitUntil(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not satisfied before timeout")
}

func TestHandleUserTextPreparesPlaybackBeforeIntent(t *testing.T) {
	t.Parallel()

	session := &fakeChannel{}
	var mu sync.Mutex
	var order []string

	service := newTestService(t,
		Config{AbortAfterASR: true, PostAbortDelay: 0},
		fakeIntent{
			onDecide: func(history []llm.Message, text string) llm.IntentDecision {
				_ = history
				_ = text
				mu.Lock()
				order = append(order, "intent")
				mu.Unlock()
				return llm.IntentDecision{ShouldHandle: false}
			},
		},
		fakeReply{},
		fakeTools{},
		&fakeTaskManager{},
		nil,
	)

	service.handleUserText(testHistoryKey, session, "你是谁")

	sessionOrder, abortCalls := session.snapshot()
	if abortCalls != 1 {
		t.Fatalf("abortCalls = %d, want 1", abortCalls)
	}
	if len(sessionOrder) == 0 || sessionOrder[0] != "prepare" {
		t.Fatalf("sessionOrder = %#v, want first step prepare", sessionOrder)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 1 || order[0] != "intent" {
		t.Fatalf("intent order = %#v, want [intent]", order)
	}
}

func TestHandleUserTextStopsWhenInitialPlaybackPreparationFails(t *testing.T) {
	t.Parallel()

	session := &fakeChannel{prepareErr: errors.New("boom")}
	intentCalled := false

	service := newTestService(t,
		Config{AbortAfterASR: true, PostAbortDelay: 0},
		fakeIntent{
			onDecide: func(history []llm.Message, text string) llm.IntentDecision {
				_ = history
				_ = text
				intentCalled = true
				return llm.IntentDecision{}
			},
		},
		fakeReply{},
		fakeTools{},
		&fakeTaskManager{},
		nil,
	)

	service.handleUserText(testHistoryKey, session, "你是谁")

	if intentCalled {
		t.Fatal("intentCalled = true, want false")
	}
}

func TestHandleUserTextIgnoresNewInputWhileBusy(t *testing.T) {
	t.Parallel()

	intentCalled := false
	service := newTestService(t,
		Config{AbortAfterASR: true, PostAbortDelay: 0},
		fakeIntent{
			onDecide: func(history []llm.Message, text string) llm.IntentDecision {
				intentCalled = true
				return llm.IntentDecision{}
			},
		},
		fakeReply{},
		fakeTools{},
		&fakeTaskManager{},
		nil,
	)

	service.busy = true
	service.HandleUserText("", nil, "这句应该被忽略")
	time.Sleep(50 * time.Millisecond)

	if intentCalled {
		t.Fatal("intentCalled = true, want false")
	}
	if !service.busy {
		t.Fatal("service.busy = false, want true")
	}
}

func TestSubmitRecognizedTextRejectsWhenBusy(t *testing.T) {
	t.Parallel()

	service := newTestService(t,
		Config{AbortAfterASR: true, PostAbortDelay: 0},
		fakeIntent{},
		fakeReply{},
		fakeTools{},
		&fakeTaskManager{},
		nil,
	)

	service.busy = true
	err := service.SubmitRecognizedText("帮我继续刚刚那个任务")
	if !errors.Is(err, ErrVoiceChannelBusy) {
		t.Fatalf("SubmitRecognizedText() error = %v, want %v", err, ErrVoiceChannelBusy)
	}
}

func TestSubmitRecognizedTextUsesMainVoiceHistory(t *testing.T) {
	t.Parallel()

	service := newTestService(t,
		Config{AbortAfterASR: false, PostAbortDelay: 0},
		fakeIntent{
			onDecide: func(history []llm.Message, text string) llm.IntentDecision {
				return llm.IntentDecision{
					ShouldHandle: true,
					ShouldAbort:  false,
				}
			},
		},
		fakeReply{},
		fakeTools{},
		&fakeTaskManager{},
		nil,
	)

	if err := service.SubmitRecognizedText("帮我总结一下今天的任务"); err != nil {
		t.Fatalf("SubmitRecognizedText() error = %v", err)
	}

	waitUntil(t, time.Second, func() bool {
		history := service.history.Snapshot(historyRef(MainVoiceHistoryKey), time.Now())
		return len(history) >= 2
	})

	history := service.history.Snapshot(historyRef(MainVoiceHistoryKey), time.Now())
	if history[len(history)-2].Role != "user" || history[len(history)-2].Content != "帮我总结一下今天的任务" {
		t.Fatalf("user history = %+v", history[len(history)-2])
	}
	if history[len(history)-1].Role != "assistant" || history[len(history)-1].Content != "你好。" {
		t.Fatalf("assistant history = %+v", history[len(history)-1])
	}
	if service.lastHistoryKey != MainVoiceHistoryKey {
		t.Fatalf("lastHistoryKey = %q, want %q", service.lastHistoryKey, MainVoiceHistoryKey)
	}
	if service.lastChannel == nil {
		t.Fatal("lastChannel = nil, want debug channel")
	}
}

func TestHandleUserTextDoesNotPrepareTwiceForToolCall(t *testing.T) {
	t.Parallel()

	session := &fakeChannel{}
	toolReplyCalled := false
	taskManager := &fakeTaskManager{}
	service := newTestService(t,
		Config{AbortAfterASR: true, PostAbortDelay: 0},
		fakeIntent{
			onDecide: func(history []llm.Message, text string) llm.IntentDecision {
				_ = history
				_ = text
				return llm.IntentDecision{
					ShouldHandle: true,
					ShouldAbort:  true,
					ToolCall: &llm.ToolCall{
						Name:      "ask_weather",
						Arguments: json.RawMessage(`{"city":"上海"}`),
					},
				}
			},
		},
		scriptedReply{
			onTool: func(ctx context.Context, history []llm.Message, userText string, toolName string, toolResult string, onDelta func(string) error) error {
				toolReplyCalled = true
				if toolName != "ask_weather" {
					t.Fatalf("toolName = %q", toolName)
				}
				if toolResult != "天气不错！" {
					t.Fatalf("toolResult = %q", toolResult)
				}
				return onDelta("整理后的天气回复。")
			},
		},
		fakeTools{
			onCall: func(name string) plugin.Result {
				return plugin.Result{Text: "天气不错！"}
			},
		},
		taskManager,
		nil,
	)

	service.handleUserText(testHistoryKey, session, "上海天气怎么样")

	_, abortCalls := session.snapshot()
	if abortCalls != 1 {
		t.Fatalf("abortCalls = %d, want 1", abortCalls)
	}
	if !toolReplyCalled {
		t.Fatal("toolReplyCalled = false, want true")
	}
}

func TestHandleUserTextUsesMemoryForReplyButNotIntent(t *testing.T) {
	t.Parallel()

	memoryService := &fakeMemoryService{
		recallText: "用户常用的 Home Assistant 地址是 https://ha.example.com",
	}
	intentHistoryChecked := false
	replyHistoryChecked := false

	service := newTestServiceWithMemory(t,
		Config{AbortAfterASR: false, PostAbortDelay: 0},
		fakeIntent{
			onDecide: func(history []llm.Message, text string) llm.IntentDecision {
				_ = text
				intentHistoryChecked = true
				if len(history) != 0 {
					t.Fatalf("intent history should not contain memory system message, got %#v", history)
				}
				return llm.IntentDecision{ShouldHandle: false}
			},
		},
		scriptedReply{
			onStream: func(ctx context.Context, history []llm.Message, text string, onDelta func(string) error) error {
				_ = ctx
				_ = text
				replyHistoryChecked = true
				if len(history) == 0 {
					t.Fatal("reply history is empty, want prepended memory message")
				}
				if history[0].Role != "system" || !strings.Contains(history[0].Content, "Home Assistant") {
					t.Fatalf("reply memory message = %+v", history[0])
				}
				return onDelta("你好。")
			},
		},
		fakeTools{},
		&fakeTaskManager{},
		nil,
		memoryService,
	)

	service.handleUserText(testHistoryKey, &fakeChannel{}, "帮我记一下")

	if !intentHistoryChecked {
		t.Fatal("intent history was not checked")
	}
	if !replyHistoryChecked {
		t.Fatal("reply history was not checked")
	}
}

func TestHandleUserTextPassesMemoryToToolContextWithoutImmediateSessionSummary(t *testing.T) {
	t.Parallel()

	memoryService := &fakeMemoryService{
		recallText: "用户偏好：尽量用中文，不要暴露内部路径。",
	}
	toolMemoryChecked := false

	service := newTestServiceWithMemory(t,
		Config{AbortAfterASR: false, PostAbortDelay: 0},
		fakeIntent{
			onDecide: func(history []llm.Message, text string) llm.IntentDecision {
				_ = history
				_ = text
				return llm.IntentDecision{
					ShouldHandle: true,
					ShouldAbort:  false,
					ToolCall: &llm.ToolCall{
						Name:      "ask_weather",
						Arguments: json.RawMessage(`{"city":"上海"}`),
					},
				}
			},
		},
		fakeReply{},
		fakeTools{
			onCall: func(name string) plugin.Result {
				return plugin.Result{
					Text:       fmt.Sprintf("%s 可以直接回复。", name),
					OutputMode: plugin.OutputModeDirect,
				}
			},
		},
		&fakeTaskManager{},
		nil,
		memoryService,
	)

	originalTools := service.tools
	service.tools = toolRunnerFunc(func(ctx context.Context, name string, arguments json.RawMessage) (plugin.Result, error) {
		memoryCtx, ok := plugin.MemoryFromContext(ctx)
		if !ok {
			t.Fatal("tool context missing memory")
		}
		if memoryCtx.Key != testHistoryKey {
			t.Fatalf("memoryCtx.Key = %q, want %q", memoryCtx.Key, testHistoryKey)
		}
		if !strings.Contains(memoryCtx.Text, "尽量用中文") {
			t.Fatalf("memoryCtx.Text = %q", memoryCtx.Text)
		}
		toolMemoryChecked = true
		return originalTools.Call(ctx, name, arguments)
	})

	service.handleUserText(testHistoryKey, &fakeChannel{}, "上海天气怎么样")

	if !toolMemoryChecked {
		t.Fatal("tool memory context was not checked")
	}
	updated := memoryService.snapshotUpdated()
	if len(updated) != 0 {
		t.Fatalf("len(updated) = %d, want 0 before session expiry", len(updated))
	}
}

func TestProcessExpiredSessionsUpdatesMemoryFromWholeSessionHistory(t *testing.T) {
	t.Parallel()

	memoryService := &fakeMemoryService{}
	service := newTestServiceWithMemory(t,
		Config{AbortAfterASR: false, PostAbortDelay: 0},
		fakeIntent{},
		fakeReply{},
		fakeTools{},
		&fakeTaskManager{},
		nil,
		memoryService,
	)

	pending := service.processExpiredSessions(context.Background(), []ConversationSnapshot{
		{
			ID:         testHistoryKey,
			StartedAt:  time.Unix(100, 0),
			LastActive: time.Unix(120, 0),
			Messages: []llm.Message{
				{Role: "user", Content: "我家里的 Home Assistant 地址是 https://ha.example.com"},
				{Role: "assistant", Content: "好的，我知道了。"},
				{Role: "user", Content: "另外以后尽量都用中文回答。"},
				{Role: "assistant", Content: "没问题。"},
			},
		},
	})
	if len(pending) != 0 {
		t.Fatalf("len(pending) = %d, want 0", len(pending))
	}

	updated := memoryService.snapshotUpdated()
	if len(updated) != 1 {
		t.Fatalf("len(updated) = %d, want 1", len(updated))
	}
	if updated[0].MemoryKey != testHistoryKey {
		t.Fatalf("MemoryKey = %q, want %q", updated[0].MemoryKey, testHistoryKey)
	}
	if len(updated[0].History) != 4 {
		t.Fatalf("len(updated[0].History) = %d, want 4", len(updated[0].History))
	}
	if updated[0].History[0].Role != "user" || !strings.Contains(updated[0].History[0].Content, "Home Assistant") {
		t.Fatalf("updated history = %#v", updated[0].History)
	}
}

func TestDeliverTaskResultReportsAppendsAssistantHistory(t *testing.T) {
	t.Parallel()

	session := &fakeChannel{}
	taskManager := &fakeTaskManager{
		resultReportItems: []tasks.ResultReportItem{{
			ID:      "task_1",
			Title:   "刚刚那个任务",
			State:   tasks.StateCompleted,
			Summary: "已经创建好了网页文件。",
			Result:  "网页已经做好了，放在桌面上。",
		}},
		resultReportIDs: []string{"task_1"},
	}
	service := newTestService(t,
		Config{AbortAfterASR: false, PostAbortDelay: 0},
		fakeIntent{},
		scriptedReply{
			onResultReport: func(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error {
				if !strings.Contains(reportContext, "标题：刚刚那个任务") {
					t.Fatalf("reportContext = %q", reportContext)
				}
				if !strings.Contains(reportContext, "结果：网页已经做好了，放在桌面上。") {
					t.Fatalf("reportContext = %q", reportContext)
				}
				return onDelta("对了，刚刚那个网页我已经做好了，放在桌面上了。")
			},
		},
		fakeTools{},
		taskManager,
		nil,
	)

	now := time.Now()
	service.history.AppendTurn(historyRef(testHistoryKey), now, "你好", "好的")
	service.deliverTaskResultReports(testHistoryKey, session)

	history := service.history.Snapshot(historyRef(testHistoryKey), time.Now())
	if len(history) != 3 {
		t.Fatalf("len(history) = %d, want 3", len(history))
	}
	last := history[len(history)-1]
	if last.Role != "assistant" {
		t.Fatalf("last.Role = %q, want assistant", last.Role)
	}
	if last.Content != "对了，刚刚那个网页我已经做好了，放在桌面上了。" {
		t.Fatalf("last.Content = %q", last.Content)
	}
	if len(taskManager.markedResultReports) != 1 || taskManager.markedResultReports[0] != "task_1" {
		t.Fatalf("markedResultReports = %#v", taskManager.markedResultReports)
	}
}

func TestDeliverTaskResultReportsMentionsNoChannelWhenTaskHasProducts(t *testing.T) {
	t.Parallel()

	session := &fakeChannel{}
	taskManager := &fakeTaskManager{
		resultReportItems: []tasks.ResultReportItem{{
			ID:      "task_1",
			Title:   "故事网页",
			State:   tasks.StateCompleted,
			Summary: "网页已经完成。",
			Result:  "网页已经做好了。",
		}},
		resultReportIDs: []string{"task_1"},
		artifactDeliveries: []tasks.ArtifactDeliveryItem{{
			Delivery: tasks.ArtifactDelivery{
				ID:         "delivery_1",
				TaskID:     "task_1",
				ArtifactID: "artifact_1",
				Status:     tasks.ArtifactDeliveryPending,
			},
			Artifact: tasks.Artifact{
				ID:          "artifact_1",
				TaskID:      "task_1",
				Kind:        "file",
				FileName:    "story.txt",
				MIMEType:    "text/plain",
				StoragePath: "/tmp/story.txt",
				SizeBytes:   12,
			},
		}},
	}

	service := newTestService(t,
		Config{AbortAfterASR: false, PostAbortDelay: 0},
		fakeIntent{},
		scriptedReply{
			onResultReport: func(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error {
				if !strings.Contains(reportContext, "产物交付：本次任务有1个产物，但当前没有可用的渠道发送。") {
					t.Fatalf("reportContext = %q", reportContext)
				}
				return onDelta("那个网页已经做好了，不过现在还没有可用的渠道发送产物。")
			},
		},
		fakeTools{},
		taskManager,
		&fakeArtifactDeliverer{ok: false},
	)

	service.deliverTaskResultReports(testHistoryKey, session)

	if got := taskManager.noChannelMarkedIDs; len(got) != 1 || got[0] != "delivery_1" {
		t.Fatalf("noChannelMarkedIDs = %#v", got)
	}
}

func TestDeliverTaskResultReportsMentionsDeliveredProductsWhenSendSucceeded(t *testing.T) {
	t.Parallel()

	session := &fakeChannel{}
	taskManager := &fakeTaskManager{
		resultReportItems: []tasks.ResultReportItem{{
			ID:      "task_1",
			Title:   "故事网页",
			State:   tasks.StateCompleted,
			Summary: "网页已经完成。",
			Result:  "网页已经做好了。",
		}},
		resultReportIDs: []string{"task_1"},
		artifactDeliveries: []tasks.ArtifactDeliveryItem{{
			Delivery: tasks.ArtifactDelivery{
				ID:         "delivery_1",
				TaskID:     "task_1",
				ArtifactID: "artifact_1",
				Status:     tasks.ArtifactDeliveryPending,
			},
			Artifact: tasks.Artifact{
				ID:          "artifact_1",
				TaskID:      "task_1",
				Kind:        "file",
				FileName:    "story.txt",
				MIMEType:    "text/plain",
				StoragePath: "/tmp/story.txt",
				SizeBytes:   12,
			},
		}},
	}
	deliverer := &fakeArtifactDeliverer{
		accountID:    "imacct_1",
		targetID:     "imtarget_1",
		channelLabel: "微信",
		ok:           true,
	}

	service := newTestService(t,
		Config{AbortAfterASR: false, PostAbortDelay: 0},
		fakeIntent{},
		scriptedReply{
			onResultReport: func(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error {
				if !strings.Contains(reportContext, "产物交付：本次任务有1个产物，已发送到微信。") {
					t.Fatalf("reportContext = %q", reportContext)
				}
				return onDelta("那个网页已经做好了，相关产物我也已经发到微信了。")
			},
		},
		fakeTools{},
		taskManager,
		deliverer,
	)

	service.deliverTaskResultReports(testHistoryKey, session)

	if len(deliverer.deliveries) != 1 || deliverer.deliveries[0] != "story.txt" {
		t.Fatalf("deliveries = %#v", deliverer.deliveries)
	}
	if len(taskManager.deliveredRecords) != 1 || taskManager.deliveredRecords[0] != "delivery_1" {
		t.Fatalf("deliveredRecords = %#v", taskManager.deliveredRecords)
	}
}

func TestDeliverTaskResultReportsUsesChunkedPlayback(t *testing.T) {
	t.Parallel()

	session := &fakeChannel{}
	taskManager := &fakeTaskManager{
		resultReportItems: []tasks.ResultReportItem{{
			ID:      "task_1",
			Title:   "第一个任务",
			State:   tasks.StateCompleted,
			Summary: "第一个结果",
			Result:  "第一个任务已经准备好了。第二句也要一起播。",
		}},
		resultReportIDs: []string{"task_1"},
	}
	service := newTestService(t,
		Config{AbortAfterASR: false, PostAbortDelay: 0},
		fakeIntent{},
		scriptedReply{
			onResultReport: func(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error {
				return onDelta("对了，第一个任务已经准备好了。第二句也要一起播。")
			},
		},
		fakeTools{},
		taskManager,
		nil,
	)

	service.deliverTaskResultReports(testHistoryKey, session)

	scripts := session.snapshotScripts()
	if len(scripts) < 2 {
		t.Fatalf("len(scripts) = %d, want at least 2 chunked tts calls", len(scripts))
	}
}

func TestTryDeliverTaskResultReportsStartsWhenIdle(t *testing.T) {
	t.Parallel()

	session := &fakeChannel{}
	taskManager := &fakeTaskManager{
		resultReportItems: []tasks.ResultReportItem{{
			ID:      "task_1",
			Title:   "刚刚那个任务",
			State:   tasks.StateCompleted,
			Summary: "已经处理完成。",
			Result:  "结果已经准备好了。",
		}},
		resultReportIDs: []string{"task_1"},
	}
	service := newTestService(t,
		Config{AbortAfterASR: false, PostAbortDelay: 0},
		fakeIntent{},
		scriptedReply{
			onResultReport: func(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error {
				if !strings.Contains(reportContext, "标题：刚刚那个任务") {
					t.Fatalf("reportContext = %q", reportContext)
				}
				return onDelta("对了，刚刚那个任务已经处理好了。")
			},
		},
		fakeTools{},
		taskManager,
		nil,
	)

	service.lastChannel = session
	service.lastHistoryKey = testHistoryKey
	service.TryDeliverTaskResultReports()

	waitUntil(t, time.Second, func() bool {
		return len(taskManager.markedResultReportsSnapshot()) == 1
	})

	if got := taskManager.markedResultReportsSnapshot(); len(got) != 1 || got[0] != "task_1" {
		t.Fatalf("markedResultReports = %#v", got)
	}
	history := service.history.Snapshot(historyRef(testHistoryKey), time.Now())
	if len(history) == 0 {
		t.Fatal("history is empty")
	}
	last := history[len(history)-1]
	if last.Role != "assistant" || last.Content != "对了，刚刚那个任务已经处理好了。" {
		t.Fatalf("last = %+v", last)
	}
}

func TestTryDeliverTaskResultReportsWaitsUntilCurrentTurnFinishes(t *testing.T) {
	t.Parallel()

	session := &fakeChannel{}
	taskManager := &fakeTaskManager{
		resultReportItems: []tasks.ResultReportItem{{
			ID:      "task_1",
			Title:   "后台任务",
			State:   tasks.StateCompleted,
			Summary: "已经完成。",
			Result:  "最终结果已经准备好了。",
		}},
		resultReportIDs: []string{"task_1"},
	}
	service := newTestService(t,
		Config{AbortAfterASR: false, PostAbortDelay: 0},
		fakeIntent{},
		scriptedReply{
			onResultReport: func(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error {
				return onDelta("对了，后台任务已经完成了。")
			},
		},
		fakeTools{},
		taskManager,
		nil,
	)

	service.lastChannel = session
	service.lastHistoryKey = testHistoryKey
	service.busy = true

	service.TryDeliverTaskResultReports()

	if got := taskManager.markedResultReportsSnapshot(); len(got) != 0 {
		t.Fatalf("markedResultReports before finish = %#v, want empty", got)
	}
	if !service.resultReportReady {
		t.Fatal("resultReportReady = false, want true")
	}

	service.finishVoiceTurn()

	waitUntil(t, time.Second, func() bool {
		return len(taskManager.markedResultReportsSnapshot()) == 1
	})

	if got := taskManager.markedResultReportsSnapshot(); len(got) != 1 || got[0] != "task_1" {
		t.Fatalf("markedResultReports after finish = %#v", got)
	}
}

func TestHandleUserTextCanDirectReplyForToolCall(t *testing.T) {
	t.Parallel()

	session := &fakeChannel{}
	toolReplyCalled := false
	service := newTestService(t,
		Config{AbortAfterASR: true, PostAbortDelay: 0},
		fakeIntent{
			onDecide: func(history []llm.Message, text string) llm.IntentDecision {
				return llm.IntentDecision{
					ShouldHandle: true,
					ShouldAbort:  true,
					ToolCall: &llm.ToolCall{
						Name:      "ask_weather",
						Arguments: json.RawMessage(`{"city":"上海"}`),
					},
				}
			},
		},
		scriptedReply{
			onTool: func(ctx context.Context, history []llm.Message, userText string, toolName string, toolResult string, onDelta func(string) error) error {
				toolReplyCalled = true
				return onDelta("不该走到这里")
			},
		},
		fakeTools{
			onCall: func(name string) plugin.Result {
				return plugin.Result{Text: "直接回复", OutputMode: plugin.OutputModeDirect}
			},
		},
		&fakeTaskManager{},
		nil,
	)

	service.handleUserText(testHistoryKey, session, "上海天气怎么样")

	if toolReplyCalled {
		t.Fatal("toolReplyCalled = true, want false")
	}
}

func TestHandleUserTextRoutesContinueChatToolToReply(t *testing.T) {
	t.Parallel()

	session := &fakeChannel{}
	toolCalled := false
	replyCalled := false
	service := newTestService(t,
		Config{AbortAfterASR: true, PostAbortDelay: 0},
		fakeIntent{
			onDecide: func(history []llm.Message, text string) llm.IntentDecision {
				return llm.IntentDecision{
					ShouldHandle: true,
					ShouldAbort:  true,
					ToolCall: &llm.ToolCall{
						Name:      "continue_chat",
						Arguments: json.RawMessage(`{}`),
					},
				}
			},
		},
		scriptedReply{
			onStream: func(ctx context.Context, history []llm.Message, text string, onDelta func(string) error) error {
				replyCalled = true
				return onDelta("继续聊。")
			},
		},
		fakeTools{
			onCall: func(name string) plugin.Result {
				toolCalled = true
				return plugin.Result{}
			},
		},
		&fakeTaskManager{},
		nil,
	)

	service.handleUserText(testHistoryKey, session, "你是谁")

	if toolCalled {
		t.Fatal("toolCalled = true, want false")
	}
	if !replyCalled {
		t.Fatal("replyCalled = false, want true")
	}
}

func TestHandleUserTextAcceptsAsyncTask(t *testing.T) {
	t.Parallel()

	session := &fakeChannel{}
	taskManager := &fakeTaskManager{}
	service := newTestService(t,
		Config{AbortAfterASR: true, PostAbortDelay: 0},
		fakeIntent{
			onDecide: func(history []llm.Message, text string) llm.IntentDecision {
				return llm.IntentDecision{
					ShouldHandle: true,
					ShouldAbort:  true,
					ToolCall: &llm.ToolCall{
						Name:      "complex_task",
						Arguments: json.RawMessage(`{"request":"做一个小游戏网页"}`),
					},
				}
			},
		},
		fakeReply{},
		fakeTools{
			onCall: func(name string) plugin.Result {
				return plugin.Result{
					Text:       "收到，我先去处理。",
					OutputMode: plugin.OutputModeAsyncAccept,
					AsyncTask: &plugin.AsyncTask{
						Kind:  "complex_task",
						Title: "小游戏网页",
						Input: "做一个小游戏网页",
						Run: func(ctx context.Context, reporter plugin.AsyncReporter) (string, error) {
							return "done", nil
						},
					},
				}
			},
		},
		taskManager,
		nil,
	)

	service.handleUserText(testHistoryKey, session, "做一个小游戏网页")

	if taskManager.submittedSpec.Title != "小游戏网页" {
		t.Fatalf("submitted title = %q", taskManager.submittedSpec.Title)
	}
}

func TestHandleUserTextForcesExternalReplyWhenIntentSaysNo(t *testing.T) {
	t.Parallel()

	session := &fakeChannel{}
	service := newTestService(t,
		Config{AbortAfterASR: true, PostAbortDelay: 0},
		fakeIntent{
			onDecide: func(history []llm.Message, text string) llm.IntentDecision {
				_ = history
				_ = text
				return llm.IntentDecision{
					ShouldHandle: false,
					ShouldAbort:  false,
					Reason:       "原生处理",
				}
			},
		},
		fakeReply{},
		fakeTools{},
		&fakeTaskManager{},
		nil,
	)

	service.handleUserText(testHistoryKey, session, "你在干啥呀")

	order, abortCalls := session.snapshot()
	if abortCalls != 1 {
		t.Fatalf("abortCalls = %d, want 1", abortCalls)
	}
	if len(order) < 2 {
		t.Fatalf("order = %#v, want prepare then speak", order)
	}
	if order[0] != "prepare" || order[1] != "speak" {
		t.Fatalf("order = %#v, want [prepare speak ...]", order)
	}
}

func TestHandleUserTextIncludesSessionHistory(t *testing.T) {
	t.Parallel()

	session := &fakeChannel{}
	var seenHistory []llm.Message

	service := newTestService(t,
		Config{AbortAfterASR: true, PostAbortDelay: 0},
		fakeIntent{
			onDecide: func(history []llm.Message, text string) llm.IntentDecision {
				seenHistory = append([]llm.Message(nil), history...)
				if text == "第二句呢" {
					if len(history) != 2 {
						t.Fatalf("len(history) = %d, want 2", len(history))
					}
					if history[0].Role != "user" || history[0].Content != "你是谁" {
						t.Fatalf("history[0] = %+v", history[0])
					}
					if history[1].Role != "assistant" || history[1].Content != "你好。" {
						t.Fatalf("history[1] = %+v", history[1])
					}
				}
				return llm.IntentDecision{
					ShouldHandle: true,
					ShouldAbort:  true,
				}
			},
		},
		fakeReply{},
		fakeTools{},
		&fakeTaskManager{},
		nil,
	)

	service.handleUserText(testHistoryKey, session, "你是谁")
	if len(seenHistory) != 0 {
		t.Fatalf("first history = %#v, want empty", seenHistory)
	}

	service.handleUserText(testHistoryKey, session, "第二句呢")
}

func TestHandleUserTextUsesSpeculativeReplyWhenEnabled(t *testing.T) {
	t.Parallel()

	session := &fakeChannel{}
	replyStarted := make(chan struct{}, 1)
	intentMayReturn := make(chan struct{})
	streamCalls := 0

	service := newTestService(t,
		Config{
			AbortAfterASR:         true,
			PostAbortDelay:        0,
			UseParallelIntentChat: true,
		},
		fakeIntent{
			onDecide: func(history []llm.Message, text string) llm.IntentDecision {
				select {
				case <-replyStarted:
				case <-time.After(time.Second):
					t.Fatal("reply did not start before intent returned")
				}
				close(intentMayReturn)
				return llm.IntentDecision{
					ShouldHandle: true,
					ShouldAbort:  true,
					ToolCall: &llm.ToolCall{
						Name:      "continue_chat",
						Arguments: json.RawMessage(`{}`),
					},
				}
			},
		},
		scriptedReply{
			onStream: func(ctx context.Context, history []llm.Message, text string, onDelta func(string) error) error {
				streamCalls++
				select {
				case replyStarted <- struct{}{}:
				default:
				}
				<-intentMayReturn
				return onDelta("并行回复。")
			},
		},
		fakeTools{},
		&fakeTaskManager{},
		nil,
	)

	service.handleUserText(testHistoryKey, session, "你在干嘛")

	if streamCalls != 1 {
		t.Fatalf("streamCalls = %d, want 1", streamCalls)
	}
}
