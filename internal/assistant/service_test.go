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
	"github.com/luoliwoshang/open-xiaoai-agent/internal/server"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/speaker"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
)

type fakeSession struct {
	mu         sync.Mutex
	order      []string
	scripts    []string
	abortErr   error
	abortCalls int
}

func (s *fakeSession) AbortXiaoAI(timeout time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.abortCalls++
	s.order = append(s.order, "abort")
	return s.abortErr
}

func (s *fakeSession) RunShell(script string, timeout time.Duration) (server.CommandResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.order = append(s.order, "run_shell")
	s.scripts = append(s.scripts, script)
	return server.CommandResult{ExitCode: 0}, nil
}

func (s *fakeSession) snapshot() ([]string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order := make([]string, len(s.order))
	copy(order, s.order)
	return order, s.abortCalls
}

func (s *fakeSession) snapshotScripts() []string {
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

func (fakeReply) StreamPendingTaskNotice(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error {
	return onDelta("整理后的任务补报。")
}

type scriptedReply struct {
	onStream  func(ctx context.Context, history []llm.Message, text string, onDelta func(string) error) error
	onTool    func(ctx context.Context, history []llm.Message, userText string, toolName string, toolResult string, onDelta func(string) error) error
	onPending func(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error
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

func (s scriptedReply) StreamPendingTaskNotice(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error {
	if s.onPending != nil {
		return s.onPending(ctx, history, reportContext, onDelta)
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
	submittedSpec plugin.AsyncTask
	pendingItems  []tasks.PendingReportItem
	pendingIDs    []string
	markReported  []string
}

func (m *fakeTaskManager) Submit(spec plugin.AsyncTask) (tasks.Task, error) {
	m.submittedSpec = spec
	return tasks.Task{
		ID:    "task_1",
		Title: spec.Title,
		State: tasks.StateAccepted,
	}, nil
}

func (m *fakeTaskManager) PendingReports(limit int) ([]tasks.PendingReportItem, []string) {
	_ = limit
	return append([]tasks.PendingReportItem(nil), m.pendingItems...), append([]string(nil), m.pendingIDs...)
}

func (m *fakeTaskManager) MarkReported(ids []string) error {
	m.markReported = append([]string(nil), ids...)
	return nil
}

func newTestService(t *testing.T, config Config, intent IntentDecider, reply ReplyStreamer, tools ToolRunner, taskManager TaskManager, spk *speaker.Speaker) *Service {
	t.Helper()

	service, err := New(config, staticSessionWindow{window: 5 * time.Minute}, intent, reply, tools, taskManager, nil, spk)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return service
}

func TestHandleASRAbortsBeforeIntent(t *testing.T) {
	t.Parallel()

	session := &fakeSession{}
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
		speaker.New(),
	)

	service.handleASR(session, "你是谁")

	sessionOrder, abortCalls := session.snapshot()
	if abortCalls != 1 {
		t.Fatalf("abortCalls = %d, want 1", abortCalls)
	}
	if len(sessionOrder) == 0 || sessionOrder[0] != "abort" {
		t.Fatalf("sessionOrder = %#v, want first step abort", sessionOrder)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 1 || order[0] != "intent" {
		t.Fatalf("intent order = %#v, want [intent]", order)
	}
}

func TestHandleASRStopsWhenImmediateAbortFails(t *testing.T) {
	t.Parallel()

	session := &fakeSession{abortErr: errors.New("boom")}
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
		speaker.New(),
	)

	service.handleASR(session, "你是谁")

	if intentCalled {
		t.Fatal("intentCalled = true, want false")
	}
}

func TestHandleASRDoesNotAbortTwiceForToolCall(t *testing.T) {
	t.Parallel()

	session := &fakeSession{}
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
		speaker.New(),
	)

	service.handleASR(session, "上海天气怎么样")

	_, abortCalls := session.snapshot()
	if abortCalls != 1 {
		t.Fatalf("abortCalls = %d, want 1", abortCalls)
	}
	if !toolReplyCalled {
		t.Fatal("toolReplyCalled = false, want true")
	}
}

func TestDeliverPendingReportsAppendsAssistantHistory(t *testing.T) {
	t.Parallel()

	session := &fakeSession{}
	taskManager := &fakeTaskManager{
		pendingItems: []tasks.PendingReportItem{{
			ID:      "task_1",
			Title:   "刚刚那个任务",
			State:   tasks.StateCompleted,
			Summary: "已经创建好了网页文件。",
			Result:  "网页已经做好了，放在桌面上。",
		}},
		pendingIDs: []string{"task_1"},
	}
	service := newTestService(t,
		Config{AbortAfterASR: false, PostAbortDelay: 0},
		fakeIntent{},
		scriptedReply{
			onPending: func(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error {
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
		speaker.New(),
	)

	now := time.Now()
	service.history.AppendTurn(session, now, "你好", "好的")
	service.deliverPendingReports(session)

	history := service.history.Snapshot(session, time.Now())
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
	if len(taskManager.markReported) != 1 || taskManager.markReported[0] != "task_1" {
		t.Fatalf("markReported = %#v", taskManager.markReported)
	}
}

func TestDeliverPendingReportsUsesChunkedPlayback(t *testing.T) {
	t.Parallel()

	session := &fakeSession{}
	taskManager := &fakeTaskManager{
		pendingItems: []tasks.PendingReportItem{{
			ID:      "task_1",
			Title:   "第一个任务",
			State:   tasks.StateCompleted,
			Summary: "第一个结果",
			Result:  "第一个任务已经准备好了。第二句也要一起播。",
		}},
		pendingIDs: []string{"task_1"},
	}
	service := newTestService(t,
		Config{AbortAfterASR: false, PostAbortDelay: 0},
		fakeIntent{},
		scriptedReply{
			onPending: func(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error {
				return onDelta("对了，第一个任务已经准备好了。第二句也要一起播。")
			},
		},
		fakeTools{},
		taskManager,
		speaker.New(),
	)

	service.deliverPendingReports(session)

	scripts := session.snapshotScripts()
	if len(scripts) < 2 {
		t.Fatalf("len(scripts) = %d, want at least 2 chunked tts calls", len(scripts))
	}
}

func TestHandleASRCanDirectReplyForToolCall(t *testing.T) {
	t.Parallel()

	session := &fakeSession{}
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
		speaker.New(),
	)

	service.handleASR(session, "上海天气怎么样")

	if toolReplyCalled {
		t.Fatal("toolReplyCalled = true, want false")
	}
}

func TestHandleASRAcceptsAsyncTask(t *testing.T) {
	t.Parallel()

	session := &fakeSession{}
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
		speaker.New(),
	)

	service.handleASR(session, "做一个小游戏网页")

	if taskManager.submittedSpec.Title != "小游戏网页" {
		t.Fatalf("submitted title = %q", taskManager.submittedSpec.Title)
	}
}

func TestHandleASRForcesExternalReplyWhenIntentSaysNo(t *testing.T) {
	t.Parallel()

	session := &fakeSession{}
	service := newTestService(t,
		Config{AbortAfterASR: true, PostAbortDelay: 0},
		fakeIntent{
			onDecide: func(history []llm.Message, text string) llm.IntentDecision {
				_ = history
				_ = text
				return llm.IntentDecision{
					ShouldHandle:  false,
					ShouldAbort:   false,
					ReplyRequired: false,
					Reason:        "原生处理",
				}
			},
		},
		fakeReply{},
		fakeTools{},
		&fakeTaskManager{},
		speaker.New(),
	)

	service.handleASR(session, "你在干啥呀")

	order, abortCalls := session.snapshot()
	if abortCalls != 1 {
		t.Fatalf("abortCalls = %d, want 1", abortCalls)
	}
	if len(order) < 2 {
		t.Fatalf("order = %#v, want abort then run_shell", order)
	}
	if order[0] != "abort" || order[1] != "run_shell" {
		t.Fatalf("order = %#v, want [abort run_shell ...]", order)
	}
}

func TestHandleASRIncludesSessionHistory(t *testing.T) {
	t.Parallel()

	session := &fakeSession{}
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
					ShouldHandle:  true,
					ShouldAbort:   true,
					ReplyRequired: true,
				}
			},
		},
		fakeReply{},
		fakeTools{},
		&fakeTaskManager{},
		speaker.New(),
	)

	service.handleASR(session, "你是谁")
	if len(seenHistory) != 0 {
		t.Fatalf("first history = %#v, want empty", seenHistory)
	}

	service.handleASR(session, "第二句呢")
}

func TestHandleASRUsesSpeculativeReplyWhenEnabled(t *testing.T) {
	t.Parallel()

	session := &fakeSession{}
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
					ShouldHandle:  true,
					ShouldAbort:   true,
					ReplyRequired: true,
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
		speaker.New(),
	)

	service.handleASR(session, "你在干嘛")

	if streamCalls != 1 {
		t.Fatalf("streamCalls = %d, want 1", streamCalls)
	}
}
