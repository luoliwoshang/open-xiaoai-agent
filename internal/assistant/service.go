package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/llm"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/tasks"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/voice"
)

var (
	ErrVoiceChannelBusy      = errors.New("assistant voice channel is busy")
	ErrNoVoiceChannel        = errors.New("no recent voice channel is available")
	ErrNoConversationContext = errors.New("no recent conversation context is available")
)

// IntentDecider 负责主流程里的“意图路由判断”。
//
// 它的职责不是直接生成给用户播报的最终回复，
// 而是根据最近会话 history 和当前这轮用户输入 text，
// 判断这一轮请求应该走哪条处理路径，例如：
// 1. 直接进入普通聊天 reply；
// 2. 调用某个同步工具；
// 3. 受理为异步任务；
// 4. 命中继续任务、查询进度、取消任务等特殊路由。
//
// 返回的 IntentDecision 本质上是一份“主流程决策结果”，
// assistant.Service 会据此决定后续是走 reply、tool 还是 async task 分支。
type IntentDecider interface {
	// Decide 根据上下文和本轮输入做一次意图判定。
	// ctx 用于限制判定时长并支持取消；
	// history 是最近会话窗口；
	// text 是本轮最终 ASR 文本。
	Decide(ctx context.Context, history []llm.Message, text string) (llm.IntentDecision, error)
}

type ReplyStreamer interface {
	Stream(ctx context.Context, history []llm.Message, text string, onDelta func(string) error) error
	StreamToolResult(ctx context.Context, history []llm.Message, userText string, toolName string, toolResult string, onDelta func(string) error) error
	StreamTaskResultReport(ctx context.Context, history []llm.Message, reportContext string, onDelta func(string) error) error
}

type ToolRunner interface {
	Call(ctx context.Context, name string, arguments json.RawMessage) (plugin.Result, error)
}

type TaskManager interface {
	Submit(spec plugin.AsyncTask) (tasks.Task, error)
	ListPendingResultReports(limit int) ([]tasks.ResultReportItem, []string)
	MarkResultReported(ids []string) error
}

type MirrorSender interface {
	MirrorText(text string)
}

type Config struct {
	AbortAfterASR         bool
	PostAbortDelay        time.Duration
	UseParallelIntentChat bool
	StateDSN              string
}

type Service struct {
	config  Config
	intent  IntentDecider
	reply   ReplyStreamer
	tools   ToolRunner
	tasks   TaskManager
	mirror  MirrorSender
	history *historyStore

	runtimeMu         sync.Mutex
	busy              bool
	lastHistoryKey    string
	lastChannel       voice.Channel
	resultReportReady bool
}

// RuntimeStatus 是主语音通道的只读运行时快照。
//
// 这份状态主要给 dashboard 和排障使用，用来回答：
// 1. 当前是否还有一轮会发声的主流程正在执行；
// 2. 当前是否有任务结果已经 ready，但因为语音通道忙而暂时还没做结果汇报；
// 3. 当前是否已经拿到一个可用于主动播报的最近语音通道。
type RuntimeStatus struct {
	Busy              bool `json:"busy"`
	ResultReportReady bool `json:"result_report_ready"`
	HasVoiceChannel   bool `json:"has_voice_channel"`
}

func New(config Config, sessionSettings sessionWindowProvider, intent IntentDecider, reply ReplyStreamer, tools ToolRunner, taskManager TaskManager, mirror MirrorSender) (*Service, error) {
	if config.PostAbortDelay < 0 {
		config.PostAbortDelay = 0
	}
	history, err := newHistoryStore(sessionSettings, config.StateDSN)
	if err != nil {
		return nil, err
	}
	return &Service{
		config:  config,
		intent:  intent,
		reply:   reply,
		tools:   tools,
		tasks:   taskManager,
		mirror:  mirror,
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

// RuntimeStatus 返回当前 assistant 主语音通道的运行时快照。
func (s *Service) RuntimeStatus() RuntimeStatus {
	if s == nil {
		return RuntimeStatus{}
	}

	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()

	return RuntimeStatus{
		Busy:              s.busy,
		ResultReportReady: s.resultReportReady,
		HasVoiceChannel:   s.lastChannel != nil,
	}
}

type historyRef string

func (r historyRef) HistoryKey() string {
	return string(r)
}

// HandleUserText 是“用户文本进入主流程”的统一入口。
// 当前实现把“会发声的主流程”串成单通道：如果上一轮还没结束，新的输入会被直接忽略。
func (s *Service) HandleUserText(historyKey string, channel voice.Channel, text string) {
	if !s.tryBeginVoiceTurn(historyKey, channel) {
		log.Printf("ignore user text while assistant busy: %s", previewText(text, 80))
		return
	}

	go func() {
		defer s.finishVoiceTurn()
		s.handleUserText(historyKey, channel, text)
	}()
}

// SubmitRecognizedText 用于把一段“已经识别完成的用户文本”注入当前主流程。
//
// 这个入口主要给 dashboard 等调试入口使用：
// - 它不会自己创建新的语音通道；
// - 而是复用最近一次成功进入主流程的 historyKey 和 voice channel；
// - 如果当前语音通道正忙，或者最近还没有可复用的通道/上下文，就返回明确错误。
func (s *Service) SubmitRecognizedText(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("text is required")
	}

	historyKey, channel, err := s.tryBeginInjectedVoiceTurn()
	if err != nil {
		return err
	}

	go func() {
		defer s.finishVoiceTurn()
		s.handleUserText(historyKey, channel, text)
	}()
	return nil
}

// TryDeliverTaskResultReports 在任务系统通知“有待汇报结果”时被调用。
// 如果当前语音通道空闲，就主动做任务结果汇报；如果当前仍在处理别的语音主流程，就先挂起，等本轮结束后再汇报。
func (s *Service) TryDeliverTaskResultReports() {
	historyKey, channel, ok := s.tryBeginResultReportTurn()
	if !ok {
		return
	}

	go func() {
		defer s.finishVoiceTurn()
		s.deliverTaskResultReports(historyKey, channel)
	}()
}

func (s *Service) handleUserText(historyKey string, channel voice.Channel, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	// 这不是“本轮是否处理成功”的标记，而是：
	// 当前这轮返回前，是否适合顺手做任务结果汇报。
	//
	// 例如：
	// - 普通 reply / tool reply / async accept 成功播出后 => 适合顺手汇报任务结果；
	// - 工具执行失败、当前回复根本没播出来 => 不适合汇报，
	//   否则用户可能听到一条和当前问题完全无关的任务结果汇报。
	shouldDeliverResultReports := false
	defer func() {
		if !shouldDeliverResultReports {
			return
		}
		s.deliverTaskResultReports(historyKey, channel)
	}()

	now := time.Now()
	history := s.history.Snapshot(historyRef(historyKey), now)
	metrics := newTurnMetrics(now, text, len(history))

	log.Printf("user text: %s", text)

	if handled := s.handleDemo(historyKey, channel, text); handled {
		return
	}

	interrupted := false
	// 如果配置为“ASR 后立即接管”，就先打断原生小爱后续链路，
	// 避免它继续自己播报或执行，确保后面的回复由当前 Agent 服务统一接管。
	if s.config.AbortAfterASR {
		if !s.preparePlayback(channel, false, true) {
			return
		}
		interrupted = true
		log.Printf("voice channel prepared before intent")
	}

	var speculative *speculativeReply
	var speculativeCancel context.CancelFunc
	// 这里提前并行启动一条普通聊天回复：
	// 如果后面的 intent 结果表明“不需要调工具，只是继续聊天”，
	// 就可以直接复用这条猜测性回复，减少整轮响应时延。
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
			ShouldHandle: true,
			ShouldAbort:  true,
			Reason:       "intent failed, fallback to reply",
		}
	}

	log.Printf(
		"intent decision: handle=%t abort=%t reason=%s",
		decision.ShouldHandle,
		decision.ShouldAbort,
		strings.TrimSpace(decision.Reason),
	)

	if decision.ToolCall != nil {
		// intent 现在统一返回 tool call。
		// 其中 continue_chat 不是一个真正要执行的外部工具，
		// 而是“这轮只是继续聊天”的逻辑标记：
		// 1. 不进入 tool runner；
		// 2. 直接回到普通 reply 主线；
		// 3. 如果前面已经抢跑了 speculative reply，就继续复用它。
		if isContinueChatTool(*decision.ToolCall) {
			log.Printf("intent selected continue_chat tool; route to normal reply and keep speculative result")
			decision.ShouldHandle = true
			decision.ShouldAbort = true
		} else {
			// 其它 tool call 则表示要真正进入工具/任务分支。
			// 这时之前为了降延迟而并行启动的普通聊天回复已经不再适用，
			// 需要先取消 speculative reply，再按工具调用继续往下处理。
			if speculative != nil {
				speculative.Cancel()
			}
			shouldDeliverResultReports = s.handleToolCall(historyKey, channel, history, now, text, *decision.ToolCall, interrupted, decision.ShouldAbort, metrics)
			return
		}
	}

	if !decision.ShouldHandle || !decision.ShouldAbort {
		log.Printf("intent fallback normalized: force external reply")
	}

	decision.ShouldHandle = true
	decision.ShouldAbort = true

	if !s.preparePlayback(channel, interrupted, decision.ShouldAbort) {
		return
	}

	player := voice.NewStreamSpeaker(channel, 30*time.Second, 100*time.Millisecond)

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

	s.history.AppendTurn(historyRef(historyKey), now, text, strings.TrimSpace(replyText.String()))
	s.mirrorReply(replyText.String())
	shouldDeliverResultReports = true
	metrics.LogCompleted("reply playback")
}

// handleToolCall 负责处理“intent 已经明确命中某个真实工具”的分支。
//
// 这条路径和 continue_chat 不同：
// - continue_chat 会回到普通 reply 主线；
// - handleToolCall 只处理那些真的要执行工具/任务的调用。
//
// 返回值表示：handleASR 收尾阶段是否应该顺手做任务结果汇报。
// 当前实现里，通常只有“当前这轮已经成功播出一段主回复”时才返回 true。
//
// 当前工具执行完后会按 OutputMode 分成 3 类：
//  1. async_accept
//     工具本身只是受理一个异步任务，马上播报“我先去处理”，后续结果延迟汇报；
//  2. direct
//     工具返回的文本可以直接播，不需要再过 reply 模型包装；
//  3. 默认模式
//     先拿到工具原始结果，再交给 reply 模型整理成更适合语音播报的话术后播放。
func (s *Service) handleToolCall(historyKey string, channel voice.Channel, history []llm.Message, turnStartedAt time.Time, userText string, call llm.ToolCall, interrupted bool, shouldAbort bool, metrics *turnMetrics) bool {
	if s.tools == nil {
		log.Printf("tool runner not configured: %s", call.Name)
		return false
	}

	// 进入工具分支前，先确保设备侧已经完成接管并准备好播放。
	// shouldAbort 来自 intent 判定，用于决定这里是否还需要再补一次 abort/等待。
	if !s.preparePlayback(channel, interrupted, shouldAbort) {
		return false
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
		return false
	}
	if strings.TrimSpace(result.Text) == "" {
		log.Printf("tool returned empty text: tool=%s", call.Name)
		return false
	}
	log.Printf("tool result text: tool=%s text=%q", call.Name, strings.TrimSpace(result.Text))

	log.Printf("tool result mode: tool=%s mode=%s", call.Name, result.NormalizedOutputMode())

	// async_accept 表示工具并不在当前这轮里直接给最终结果，
	// 而是先受理成一个后台任务；当前只播报“收到，我先处理”，
	// 真正完成后的结果会走“任务结果汇报”机制在后续时机继续汇报。
	if result.NormalizedOutputMode() == plugin.OutputModeAsyncAccept {
		return s.handleAsyncTask(historyKey, channel, turnStartedAt, userText, call, result, metrics)
	}

	// direct 表示工具自己已经产出了最终可播报文本，
	// 不再经过 reply 模型二次整理，直接播放原文即可。
	if result.NormalizedOutputMode() == plugin.OutputModeDirect {
		metrics.MarkOutputStart("tool")
		if err := channel.SpeakText(result.Text, 30*time.Second); err != nil {
			log.Printf("tool reply playback failed: tool=%s err=%v", call.Name, err)
			return false
		}

		s.history.AppendTurn(historyRef(historyKey), turnStartedAt, userText, strings.TrimSpace(result.Text))
		s.mirrorReply(result.Text)
		metrics.LogCompleted("tool reply playback")
		log.Printf("tool reply playback completed: tool=%s", call.Name)
		return true
	}

	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 默认模式下，工具返回的是“原始结果”而不是最终话术。
	// 这里再调用 reply 模型把工具结果整理成更自然、适合 TTS 的口语化回复，
	// 然后走流式播放器一边生成一边播报。
	player := voice.NewStreamSpeaker(channel, 30*time.Second, 100*time.Millisecond)
	var replyText strings.Builder
	if err := s.reply.StreamToolResult(ctx, history, userText, call.Name, result.Text, func(delta string) error {
		metrics.MarkOutputStart("tool_reply")
		replyText.WriteString(delta)
		return player.Push(delta)
	}); err != nil {
		log.Printf("tool reply model failed: tool=%s err=%v", call.Name, err)
		return false
	}
	if err := player.Close(); err != nil {
		log.Printf("tool reply flush failed: tool=%s err=%v", call.Name, err)
		return false
	}

	finalReply := strings.TrimSpace(replyText.String())
	if finalReply == "" {
		log.Printf("tool reply model returned empty text: tool=%s", call.Name)
		return false
	}

	s.history.AppendTurn(historyRef(historyKey), turnStartedAt, userText, finalReply)
	s.mirrorReply(finalReply)
	metrics.LogCompleted("tool reply playback")
	log.Printf("tool reply playback completed: tool=%s", call.Name)
	return true
}

// handleAsyncTask 处理“工具调用的结果不是立即给最终答案，而是受理为后台任务”的分支。
//
// 这里不会真正执行任务主体逻辑；真正的执行由 tasks.Manager / plugin.AsyncTask.Run
// 在后台继续推进。当前函数只做 3 件事：
// 1. 把工具返回的 AsyncTask 规格提交给任务系统，生成任务记录；
// 2. 立刻向用户播报一段“已受理，我先去处理”的受理文案；
// 3. 把这段受理文案写入会话历史，方便后续上下文继续引用。
//
// 也就是说：
// - 这里处理的是“任务受理”；
// - 真正完成后的结果不会在这里播报；
// - 完成结果会在后台任务结束后，走“任务结果汇报”机制延迟汇报。
//
// 返回值表示：handleASR 收尾阶段是否应该顺手做任务结果汇报。
// 当前实现里，受理文案成功播出后才返回 true。
func (s *Service) handleAsyncTask(historyKey string, channel voice.Channel, turnStartedAt time.Time, userText string, call llm.ToolCall, result plugin.Result, metrics *turnMetrics) bool {
	if s.tasks == nil {
		log.Printf("task manager not configured: tool=%s", call.Name)
		return false
	}
	if result.AsyncTask == nil {
		log.Printf("async task spec missing: tool=%s", call.Name)
		return false
	}

	task, err := s.tasks.Submit(*result.AsyncTask)
	if err != nil {
		log.Printf("submit async task failed: tool=%s err=%v", call.Name, err)
		return false
	}
	log.Printf("async task accepted: id=%s title=%s", task.ID, task.Title)

	// result.Text 是工具给出的“任务已受理”文案；
	// 如果工具没显式提供，就使用一条默认受理话术。
	replyText := strings.TrimSpace(result.Text)
	if replyText == "" {
		replyText = "收到，这个任务我先去处理。"
	}

	// 这里只播放受理反馈，不等待后台任务完成。
	metrics.MarkOutputStart("async_accept")
	if err := channel.SpeakText(replyText, 30*time.Second); err != nil {
		log.Printf("async accept playback failed: tool=%s err=%v", call.Name, err)
		return false
	}
	s.history.AppendTurn(historyRef(historyKey), turnStartedAt, userText, replyText)
	s.mirrorReply(replyText)
	metrics.LogCompleted("async task accept")
	return true
}

func (s *Service) handleDemo(historyKey string, channel voice.Channel, text string) bool {
	switch text {
	case "测试播放文字":
		if !s.preparePlayback(channel, false, true) {
			return true
		}
		if err := channel.SpeakText("你好，很高兴认识你！", 30*time.Second); err != nil {
			log.Printf("play text failed: %v", err)
			return true
		}
		s.history.AppendTurn(historyRef(historyKey), time.Now(), text, "你好，很高兴认识你！")
		log.Printf("played demo reply text")
		return true
	case "测试长段播放文字":
		if !s.preparePlayback(channel, false, true) {
			return true
		}
		player := voice.NewStreamSpeaker(channel, 30*time.Second, 100*time.Millisecond)
		chunks := []string{
			"你好，我现在开始演示流式文字播放。",
			"这段回复不会一次性整段播完，",
			"而是像 migpt 一样，",
			"按多段文字顺序调用音箱本地 TTS。",
			"每一段播完之后，",
			"再继续播放下一段。",
		}
		for _, chunk := range chunks {
			if err := player.Push(chunk); err != nil {
				log.Printf("play text stream failed: %v", err)
				return true
			}
		}
		if err := player.Close(); err != nil {
			log.Printf("play text stream failed: %v", err)
			return true
		}
		s.history.AppendTurn(historyRef(historyKey), time.Now(), text, strings.Join(chunks, ""))
		log.Printf("played demo reply text stream")
		return true
	default:
		return false
	}
}

func isContinueChatTool(call llm.ToolCall) bool {
	return strings.TrimSpace(call.Name) == "continue_chat"
}

func (s *Service) tryBeginVoiceTurn(historyKey string, channel voice.Channel) bool {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()

	if s.busy {
		return false
	}
	s.busy = true
	if channel != nil {
		s.lastChannel = channel
	}
	if strings.TrimSpace(historyKey) != "" {
		s.lastHistoryKey = strings.TrimSpace(historyKey)
	}
	return true
}

func (s *Service) tryBeginInjectedVoiceTurn() (string, voice.Channel, error) {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()

	if s.busy {
		return "", nil, ErrVoiceChannelBusy
	}
	if s.lastChannel == nil {
		return "", nil, ErrNoVoiceChannel
	}
	if strings.TrimSpace(s.lastHistoryKey) == "" {
		return "", nil, ErrNoConversationContext
	}

	s.busy = true
	return s.lastHistoryKey, s.lastChannel, nil
}

func (s *Service) tryBeginResultReportTurn() (string, voice.Channel, bool) {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()

	s.resultReportReady = true
	if s.busy || s.lastChannel == nil || strings.TrimSpace(s.lastHistoryKey) == "" {
		return "", nil, false
	}

	historyKey := s.lastHistoryKey
	channel := s.lastChannel
	s.busy = true
	s.resultReportReady = false
	return historyKey, channel, true
}

func (s *Service) finishVoiceTurn() {
	var historyKey string
	var channel voice.Channel
	startPending := false

	s.runtimeMu.Lock()
	s.busy = false
	if s.resultReportReady && s.lastChannel != nil && strings.TrimSpace(s.lastHistoryKey) != "" {
		historyKey = s.lastHistoryKey
		channel = s.lastChannel
		s.busy = true
		s.resultReportReady = false
		startPending = true
	}
	s.runtimeMu.Unlock()

	if !startPending {
		return
	}

	go func() {
		defer s.finishVoiceTurn()
		s.deliverTaskResultReports(historyKey, channel)
	}()
}

func (s *Service) preparePlayback(channel voice.Channel, interrupted bool, shouldAbort bool) bool {
	if channel == nil {
		log.Printf("voice channel is not configured")
		return false
	}
	if err := channel.PreparePlayback(voice.PlaybackOptions{
		InterruptNativeFlow:   shouldAbort,
		NativeFlowInterrupted: interrupted,
		PostInterruptDelay:    s.config.PostAbortDelay,
	}); err != nil {
		log.Printf("prepare playback failed: %v", err)
		return false
	}
	return true
}

func (s *Service) deliverTaskResultReports(historyKey string, channel voice.Channel) {
	if s.tasks == nil {
		return
	}
	items, ids := s.tasks.ListPendingResultReports(3)
	if len(items) == 0 || len(ids) == 0 {
		return
	}

	reportContext := buildTaskResultReportContext(items)
	history := s.history.Snapshot(historyRef(historyKey), time.Now())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	player := voice.NewStreamSpeaker(channel, 30*time.Second, 100*time.Millisecond)
	var replyText strings.Builder
	if err := s.reply.StreamTaskResultReport(ctx, history, reportContext, func(delta string) error {
		replyText.WriteString(delta)
		return player.Push(delta)
	}); err != nil {
		log.Printf("task result report model failed: %v", err)
		return
	}
	if err := player.Close(); err != nil {
		log.Printf("task result report playback failed: %v", err)
		return
	}

	finalReply := strings.TrimSpace(replyText.String())
	if finalReply == "" {
		log.Printf("task result report returned empty text")
		return
	}

	s.history.AppendTurn(historyRef(historyKey), time.Now(), "", finalReply)
	s.mirrorReply(finalReply)
	if err := s.tasks.MarkResultReported(ids); err != nil {
		log.Printf("mark task result report failed: %v", err)
	}
}

func (s *Service) mirrorReply(text string) {
	if s == nil || s.mirror == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	s.mirror.MirrorText(text)
}

func buildTaskResultReportContext(items []tasks.ResultReportItem) string {
	var b strings.Builder
	b.WriteString("最近有异步任务结果需要主动汇报，任务信息如下：\n")
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

func previewText(text string, max int) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	if max <= 0 || len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "..."
}
