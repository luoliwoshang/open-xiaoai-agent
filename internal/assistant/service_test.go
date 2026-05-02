package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
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

type fakeTaskManager struct {
	mu                  sync.Mutex
	submittedSpec       plugin.AsyncTask
	resultReportItems   []tasks.ResultReportItem
	resultReportIDs     []string
	markedResultReports []string
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

func newTestService(t *testing.T, config Config, intent IntentDecider, reply ReplyStreamer, tools ToolRunner, taskManager TaskManager) *Service {
	t.Helper()

	service, err := New(config, staticSessionWindow{window: 5 * time.Minute}, intent, reply, tools, taskManager, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return service
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
	)

	service.handleUserText(testHistoryKey, session, "你在干嘛")

	if streamCalls != 1 {
		t.Fatalf("streamCalls = %d, want 1", streamCalls)
	}
}
