package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/server"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/speaker"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
)

type xiaoAISession interface {
	speaker.ShellRunner
	AbortXiaoAI(timeout time.Duration) error
}

type IntentDecider interface {
	Decide(ctx context.Context, history []llm.Message, text string) (llm.IntentDecision, error)
}

type ReplyStreamer interface {
	Stream(ctx context.Context, history []llm.Message, text string, onDelta func(string) error) error
	StreamToolResult(ctx context.Context, history []llm.Message, userText string, toolName string, toolResult string, onDelta func(string) error) error
	StreamPendingTaskNotice(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error
}

type ToolRunner interface {
	Call(ctx context.Context, name string, arguments json.RawMessage) (plugin.Result, error)
}

type TaskManager interface {
	Submit(spec plugin.AsyncTask) (tasks.Task, error)
	PendingReports(limit int) ([]tasks.PendingReportItem, []string)
	MarkReported(ids []string) error
}

type Config struct {
	AbortAfterASR         bool
	PostAbortDelay        time.Duration
	SessionWindow         time.Duration
	UseParallelIntentChat bool
	StateDSN              string
}

type Service struct {
	config  Config
	intent  IntentDecider
	reply   ReplyStreamer
	tools   ToolRunner
	tasks   TaskManager
	spk     *speaker.Speaker
	history *historyStore
}

func New(config Config, intent IntentDecider, reply ReplyStreamer, tools ToolRunner, taskManager TaskManager, spk *speaker.Speaker) (*Service, error) {
	if config.SessionWindow <= 0 {
		config.SessionWindow = 5 * time.Minute
	}
	if config.PostAbortDelay < 0 {
		config.PostAbortDelay = 0
	}
	history, err := newHistoryStore(config.SessionWindow, config.StateDSN)
	if err != nil {
		return nil, err
	}
	return &Service{
		config:  config,
		intent:  intent,
		reply:   reply,
		tools:   tools,
		tasks:   taskManager,
		spk:     spk,
		history: history,
	}, nil
}

func (s *Service) SnapshotConversations() []ConversationSnapshot {
	if s == nil || s.history == nil {
		return nil
	}
	return s.history.SnapshotAll(time.Now())
}

func (s *Service) ResetConversationData() error {
	if s == nil || s.history == nil {
		return nil
	}
	return s.history.Reset()
}

func (s *Service) OnASR(session *server.Session, text string) {
	go s.handleASR(session, text)
}

func (s *Service) handleASR(session xiaoAISession, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	now := time.Now()
	history := s.history.Snapshot(session, now)
	metrics := newTurnMetrics(now, text, len(history))

	log.Printf("xiaoai command: %s", text)

	if handled := s.handleDemo(session, text); handled {
		return
	}

	interrupted := false
	if s.config.AbortAfterASR {
		if !s.abort(session) {
			return
		}
		interrupted = true
		log.Printf("xiaoai aborted before intent")
	}

	var speculative *speculativeReply
	var speculativeCancel context.CancelFunc
	if s.config.UseParallelIntentChat {
		speculativeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		speculativeCancel = cancel
		defer speculativeCancel()
		speculative = startSpeculativeReply(speculativeCtx, s.reply, history, text)
		log.Printf("speculative reply started")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	intentStartedAt := time.Now()
	decision, err := s.intent.Decide(ctx, history, text)
	cancel()
	log.Printf("intent completed: duration=%s", time.Since(intentStartedAt).Round(time.Millisecond))
	if err != nil {
		log.Printf("intent classify failed: %v", err)
		decision = llm.IntentDecision{
			ShouldHandle:  true,
			ShouldAbort:   true,
			ReplyRequired: true,
			Reason:        "intent failed, fallback to reply",
		}
	}

	log.Printf(
		"intent decision: handle=%t abort=%t reply=%t reason=%s",
		decision.ShouldHandle,
		decision.ShouldAbort,
		decision.ReplyRequired,
		strings.TrimSpace(decision.Reason),
	)

	if decision.ToolCall != nil {
		if speculative != nil {
			speculative.Cancel()
		}
		s.handleToolCall(session, history, now, text, *decision.ToolCall, interrupted, decision.ShouldAbort, metrics)
		return
	}

	if !decision.ShouldHandle || !decision.ShouldAbort || !decision.ReplyRequired {
		log.Printf("intent fallback normalized: force external reply")
	}

	decision.ShouldHandle = true
	decision.ShouldAbort = true
	decision.ReplyRequired = true

	if !s.preparePlayback(session, interrupted, decision.ShouldAbort) {
		return
	}

	player := speaker.NewStreamPlayer(s.spk, session, 30*time.Second, 100*time.Millisecond)

	var replyText strings.Builder
	if speculative != nil {
		log.Printf("intent continue_chat: use speculative reply")
		text, err := speculative.Play(func(delta string) error {
			metrics.MarkOutputStart("reply")
			replyText.WriteString(delta)
			return player.Push(delta)
		})
		if err != nil {
			log.Printf("speculative reply failed: %v", err)
			return
		}
		if strings.TrimSpace(replyText.String()) == "" && strings.TrimSpace(text) != "" {
			replyText.WriteString(text)
		}
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := s.reply.Stream(ctx, history, text, func(delta string) error {
			metrics.MarkOutputStart("reply")
			replyText.WriteString(delta)
			return player.Push(delta)
		}); err != nil {
			log.Printf("reply stream failed: %v", err)
			return
		}
	}
	if err := player.Close(); err != nil {
		log.Printf("reply flush failed: %v", err)
		return
	}

	s.history.AppendTurn(session, now, text, strings.TrimSpace(replyText.String()))
	s.deliverPendingReports(session)
	metrics.LogCompleted("reply playback")
}

func (s *Service) handleToolCall(session xiaoAISession, history []llm.Message, turnStartedAt time.Time, userText string, call llm.ToolCall, interrupted bool, shouldAbort bool, metrics *turnMetrics) {
	if s.tools == nil {
		log.Printf("tool runner not configured: %s", call.Name)
		return
	}

	if !s.preparePlayback(session, interrupted, shouldAbort) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	argsText := strings.TrimSpace(string(call.Arguments))
	if argsText == "" {
		argsText = "{}"
	}
	log.Printf("tool invoke: tool=%s arguments=%s", call.Name, argsText)

	result, err := s.tools.Call(ctx, call.Name, call.Arguments)
	if err != nil {
		log.Printf("tool call failed: tool=%s err=%v", call.Name, err)
		return
	}
	if strings.TrimSpace(result.Text) == "" {
		log.Printf("tool returned empty text: tool=%s", call.Name)
		return
	}
	log.Printf("tool result text: tool=%s text=%q", call.Name, strings.TrimSpace(result.Text))

	log.Printf("tool result mode: tool=%s mode=%s", call.Name, result.NormalizedOutputMode())

	if result.NormalizedOutputMode() == plugin.OutputModeAsyncAccept {
		s.handleAsyncTask(session, turnStartedAt, userText, call, result, metrics)
		return
	}

	if result.NormalizedOutputMode() == plugin.OutputModeDirect {
		metrics.MarkOutputStart("tool")
		if err := s.spk.PlayText(session, result.Text, 30*time.Second); err != nil {
			log.Printf("tool reply playback failed: tool=%s err=%v", call.Name, err)
			return
		}

		s.history.AppendTurn(session, turnStartedAt, userText, strings.TrimSpace(result.Text))
		s.deliverPendingReports(session)
		metrics.LogCompleted("tool reply playback")
		log.Printf("tool reply playback completed: tool=%s", call.Name)
		return
	}

	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	player := speaker.NewStreamPlayer(s.spk, session, 30*time.Second, 100*time.Millisecond)
	var replyText strings.Builder
	if err := s.reply.StreamToolResult(ctx, history, userText, call.Name, result.Text, func(delta string) error {
		metrics.MarkOutputStart("tool_reply")
		replyText.WriteString(delta)
		return player.Push(delta)
	}); err != nil {
		log.Printf("tool reply model failed: tool=%s err=%v", call.Name, err)
		return
	}
	if err := player.Close(); err != nil {
		log.Printf("tool reply flush failed: tool=%s err=%v", call.Name, err)
		return
	}

	finalReply := strings.TrimSpace(replyText.String())
	if finalReply == "" {
		log.Printf("tool reply model returned empty text: tool=%s", call.Name)
		return
	}

	s.history.AppendTurn(session, turnStartedAt, userText, finalReply)
	s.deliverPendingReports(session)
	metrics.LogCompleted("tool reply playback")
	log.Printf("tool reply playback completed: tool=%s", call.Name)
}

func (s *Service) handleAsyncTask(session xiaoAISession, turnStartedAt time.Time, userText string, call llm.ToolCall, result plugin.Result, metrics *turnMetrics) {
	if s.tasks == nil {
		log.Printf("task manager not configured: tool=%s", call.Name)
		return
	}
	if result.AsyncTask == nil {
		log.Printf("async task spec missing: tool=%s", call.Name)
		return
	}

	task, err := s.tasks.Submit(*result.AsyncTask)
	if err != nil {
		log.Printf("submit async task failed: tool=%s err=%v", call.Name, err)
		return
	}
	log.Printf("async task accepted: id=%s title=%s", task.ID, task.Title)

	replyText := strings.TrimSpace(result.Text)
	if replyText == "" {
		replyText = "收到，这个任务我先去处理。"
	}

	metrics.MarkOutputStart("async_accept")
	if err := s.spk.PlayText(session, replyText, 30*time.Second); err != nil {
		log.Printf("async accept playback failed: tool=%s err=%v", call.Name, err)
		return
	}
	s.history.AppendTurn(session, turnStartedAt, userText, replyText)
	s.deliverPendingReports(session)
	metrics.LogCompleted("async task accept")
}

func (s *Service) handleDemo(session xiaoAISession, text string) bool {
	switch text {
	case "测试播放文字":
		if !s.abort(session) {
			return true
		}
		time.Sleep(s.config.PostAbortDelay)
		if err := s.spk.PlayText(session, "你好，很高兴认识你！", 30*time.Second); err != nil {
			log.Printf("play text failed: %v", err)
			return true
		}
		log.Printf("played demo reply text")
		return true
	case "测试长段播放文字":
		if !s.abort(session) {
			return true
		}
		time.Sleep(s.config.PostAbortDelay)
		if err := s.spk.PlayTextStream(session, []string{
			"你好，我现在开始演示流式文字播放。",
			"这段回复不会一次性整段播完，",
			"而是像 migpt 一样，",
			"按多段文字顺序调用音箱本地 TTS。",
			"每一段播完之后，",
			"再继续播放下一段。",
		}, 30*time.Second, 100*time.Millisecond); err != nil {
			log.Printf("play text stream failed: %v", err)
			return true
		}
		log.Printf("played demo reply text stream")
		return true
	default:
		return false
	}
}

func (s *Service) abort(session xiaoAISession) bool {
	if err := session.AbortXiaoAI(5 * time.Second); err != nil {
		log.Printf("abort xiaoai failed: %v", err)
		return false
	}
	return true
}

func (s *Service) preparePlayback(session xiaoAISession, interrupted bool, shouldAbort bool) bool {
	if !interrupted && shouldAbort {
		if !s.abort(session) {
			return false
		}
		interrupted = true
	}
	if interrupted {
		time.Sleep(s.config.PostAbortDelay)
	}
	return true
}

func (s *Service) deliverPendingReports(session xiaoAISession) {
	if s.tasks == nil {
		return
	}
	items, ids := s.tasks.PendingReports(3)
	if len(items) == 0 || len(ids) == 0 {
		return
	}

	reportContext := buildPendingTaskNoticeContext(items)
	history := s.history.Snapshot(session, time.Now())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	player := speaker.NewStreamPlayer(s.spk, session, 30*time.Second, 100*time.Millisecond)
	var replyText strings.Builder
	if err := s.reply.StreamPendingTaskNotice(ctx, history, reportContext, func(delta string) error {
		replyText.WriteString(delta)
		return player.Push(delta)
	}); err != nil {
		log.Printf("pending task notice model failed: %v", err)
		return
	}
	if err := player.Close(); err != nil {
		log.Printf("pending task report playback failed: %v", err)
		return
	}

	finalReply := strings.TrimSpace(replyText.String())
	if finalReply == "" {
		log.Printf("pending task notice returned empty text")
		return
	}

	s.history.AppendTurn(session, time.Now(), "", finalReply)
	if err := s.tasks.MarkReported(ids); err != nil {
		log.Printf("mark pending task report failed: %v", err)
	}
}

func buildPendingTaskNoticeContext(items []tasks.PendingReportItem) string {
	var b strings.Builder
	b.WriteString("最近有异步任务需要主动补报，任务信息如下：\n")
	for index, item := range items {
		fmt.Fprintf(&b, "%d. 标题：%s\n", index+1, fallbackTaskNoticeValue(item.Title, "未命名任务"))
		fmt.Fprintf(&b, "   状态：%s\n", taskNoticeStateLabel(item.State))
		if summary := strings.TrimSpace(item.Summary); summary != "" {
			fmt.Fprintf(&b, "   摘要：%s\n", summary)
		}
		if result := strings.TrimSpace(item.Result); result != "" {
			fmt.Fprintf(&b, "   结果：%s\n", result)
		}
	}
	return strings.TrimSpace(b.String())
}

func fallbackTaskNoticeValue(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func taskNoticeStateLabel(state tasks.State) string {
	switch state {
	case tasks.StateCompleted:
		return "已完成"
	case tasks.StateFailed:
		return "失败"
	case tasks.StateCanceled:
		return "已取消"
	default:
		return string(state)
	}
}
