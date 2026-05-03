package tasks

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
)

type Manager struct {
	mu                  sync.Mutex
	store               *Store
	state               fileState
	seq                 uint64
	cancels             map[string]context.CancelFunc
	artifactCache       *artifactCache
	onResultReportReady func()
}

func NewManager(dsn string, artifactCacheDir string) (*Manager, error) {
	store, err := NewStore(dsn)
	if err != nil {
		return nil, err
	}
	state, err := store.Load()
	if err != nil {
		return nil, err
	}
	cache, err := newArtifactCache(artifactCacheDir)
	if err != nil {
		return nil, err
	}

	m := &Manager{
		store:         store,
		state:         state,
		cancels:       make(map[string]context.CancelFunc),
		artifactCache: cache,
	}
	return m, nil
}

func (m *Manager) Submit(spec plugin.AsyncTask) (Task, error) {
	if strings.TrimSpace(spec.Kind) == "" {
		return Task{}, fmt.Errorf("async task kind is required")
	}
	if spec.Run == nil {
		return Task{}, fmt.Errorf("async task runner is required")
	}

	now := time.Now()
	task := Task{
		ID:                  m.nextID("task"),
		Plugin:              strings.TrimSpace(spec.Plugin),
		Kind:                strings.TrimSpace(spec.Kind),
		Title:               strings.TrimSpace(spec.Title),
		Input:               strings.TrimSpace(spec.Input),
		ParentTaskID:        strings.TrimSpace(spec.ParentTaskID),
		State:               StateAccepted,
		Summary:             "任务已受理",
		ResultReportPending: false,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if task.Title == "" {
		task.Title = task.Kind
	}
	if task.Plugin == "" {
		task.Plugin = task.Kind
	}

	m.mu.Lock()
	m.state.Tasks = append(m.state.Tasks, task)
	m.state.Events = append(m.state.Events, Event{
		ID:        m.nextID("event"),
		TaskID:    task.ID,
		Type:      "accepted",
		Message:   "任务已受理",
		CreatedAt: now,
	})
	if err := m.store.Save(m.state); err != nil {
		m.mu.Unlock()
		return Task{}, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancels[task.ID] = cancel
	m.mu.Unlock()

	go m.runTask(ctx, task.ID, spec.Run)
	return task, nil
}

// SetResultReportHook 注册一个轻量回调：当任务刚进入“有待汇报结果”的状态时，通知上层去尝试做任务结果汇报。
// 这不是持续轮询数据库，而是任务状态变化时的一次性事件触发。
func (m *Manager) SetResultReportHook(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onResultReportReady = fn
}

func (m *Manager) runTask(ctx context.Context, taskID string, run func(context.Context, plugin.AsyncReporter) (string, error)) {
	m.updateTask(taskID, func(task *Task, events *[]Event) {
		task.State = StateRunning
		task.Summary = "任务执行中"
		task.UpdatedAt = time.Now()
		*events = append(*events, Event{
			ID:        m.nextID("event"),
			TaskID:    taskID,
			Type:      "running",
			Message:   "任务开始执行",
			CreatedAt: time.Now(),
		})
	})

	result, err := run(ctx, reporter{manager: m, taskID: taskID})
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		m.updateTask(taskID, func(task *Task, events *[]Event) {
			task.State = StateFailed
			task.Summary = strings.TrimSpace(err.Error())
			task.Result = ""
			task.ResultReportPending = true
			task.UpdatedAt = time.Now()
			*events = append(*events, Event{
				ID:        m.nextID("event"),
				TaskID:    taskID,
				Type:      "failed",
				Message:   strings.TrimSpace(err.Error()),
				CreatedAt: time.Now(),
			})
		})
		m.clearCancel(taskID)
		m.notifyResultReportReady()
		return
	}

	_ = m.updateTask(taskID, func(task *Task, events *[]Event) {
		if task.State == StateCanceled {
			return
		}
		task.State = StateCompleted
		task.Result = strings.TrimSpace(result)
		task.Summary = summarizeResult(task.Result)
		task.ResultReportPending = true
		task.UpdatedAt = time.Now()
		*events = append(*events, Event{
			ID:        m.nextID("event"),
			TaskID:    taskID,
			Type:      "completed",
			Message:   task.Summary,
			CreatedAt: time.Now(),
		})
	})
	m.clearCancel(taskID)
	m.notifyResultReportReady()
}

func (m *Manager) CancelLatest() (*Task, error) {
	m.mu.Lock()
	task := m.latestActiveTaskLocked()
	if task == nil {
		m.mu.Unlock()
		return nil, nil
	}
	now := time.Now()
	task.State = StateCanceled
	task.Summary = "任务已取消"
	task.ResultReportPending = true
	task.UpdatedAt = now
	m.state.Events = append(m.state.Events, Event{
		ID:        m.nextID("event"),
		TaskID:    task.ID,
		Type:      "canceled",
		Message:   "任务已取消",
		CreatedAt: now,
	})
	if cancel, ok := m.cancels[task.ID]; ok {
		cancel()
		delete(m.cancels, task.ID)
	}
	if err := m.store.Save(m.state); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	copyTask := *task
	m.mu.Unlock()
	m.notifyResultReportReady()
	return &copyTask, nil
}

func (m *Manager) SummarizeProgress(limit int) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	tasks := append([]Task(nil), m.state.Tasks...)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
	})

	var items []string
	for _, task := range tasks {
		if task.State != StateAccepted && task.State != StateRunning {
			continue
		}
		items = append(items, formatProgressItem(task))
		if len(items) >= limit {
			break
		}
	}
	if len(items) == 0 {
		return "现在没有正在处理的任务。"
	}
	return "我现在手头的任务进度是：" + strings.Join(items, "；") + "。"
}

func (m *Manager) GetTask(taskID string) (*Task, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, task := range m.state.Tasks {
		if task.ID == taskID {
			copyTask := task
			return &copyTask, true
		}
	}
	return nil, false
}

func (m *Manager) CompletedTasksForIntent(limit int) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	tasks := append([]Task(nil), m.state.Tasks...)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt)
	})

	var items []string
	for _, task := range tasks {
		if task.State != StateCompleted {
			continue
		}
		items = append(items, formatCompletedTaskForIntent(task))
		if len(items) >= limit {
			break
		}
	}
	if len(items) == 0 {
		return ""
	}
	return "最近已完成任务列表如下。如果用户现在是在补充、修改、继续之前做完的任务，请优先从下面选择最匹配的任务并调用 continue_task：\n" + strings.Join(items, "\n")
}

func formatProgressItem(task Task) string {
	title := strings.TrimSpace(task.Title)
	if title == "" {
		title = "未命名任务"
	}
	summary := strings.TrimSpace(task.Summary)
	if summary == "" {
		summary = "暂无阶段摘要"
	}
	return fmt.Sprintf(
		"任务：%s，任务状态：%s，任务目前阶段summary：%s",
		title,
		taskStateLabel(task.State),
		summary,
	)
}

func formatCompletedTaskForIntent(task Task) string {
	title := strings.TrimSpace(task.Title)
	if title == "" {
		title = "未命名任务"
	}
	pluginName := strings.TrimSpace(task.Plugin)
	if pluginName == "" {
		pluginName = strings.TrimSpace(task.Kind)
	}
	summary := compactTaskText(task.Summary)
	if summary == "" {
		summary = compactTaskText(task.Result)
	}
	if summary == "" {
		summary = "暂无摘要"
	}

	return fmt.Sprintf(
		"- task_id=%s plugin=%s title=%s summary=%s",
		task.ID,
		pluginName,
		title,
		summary,
	)
}

func (m *Manager) BuildResultReport(limit int) (string, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tasks := append([]Task(nil), m.state.Tasks...)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt)
	})

	var ids []string
	var items []string
	for _, task := range tasks {
		if !task.ResultReportPending {
			continue
		}
		ids = append(ids, task.ID)
		switch task.State {
		case StateCompleted:
			items = append(items, formatResultReportItem(task, "已经完成了"))
		case StateFailed:
			items = append(items, formatResultReportItem(task, "失败了"))
		case StateCanceled:
			items = append(items, formatResultReportItem(task, "已经取消了"))
		}
		if len(items) >= limit {
			break
		}
	}
	if len(items) == 0 {
		return "", nil
	}
	return "对了，刚刚有任务结果可以汇报：" + strings.Join(items, "；") + "。", ids
}

func (m *Manager) ListPendingResultReports(limit int) ([]ResultReportItem, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tasks := append([]Task(nil), m.state.Tasks...)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt)
	})

	items := make([]ResultReportItem, 0, limit)
	ids := make([]string, 0, limit)
	for _, task := range tasks {
		if !task.ResultReportPending {
			continue
		}
		switch task.State {
		case StateCompleted, StateFailed, StateCanceled:
		default:
			continue
		}
		items = append(items, ResultReportItem{
			ID:      task.ID,
			Title:   strings.TrimSpace(task.Title),
			State:   task.State,
			Summary: strings.TrimSpace(task.Summary),
			Result:  strings.TrimSpace(task.Result),
		})
		ids = append(ids, task.ID)
		if len(items) >= limit {
			break
		}
	}
	return items, ids
}

func formatResultReportItem(task Task, stateText string) string {
	title := strings.TrimSpace(task.Title)
	summary := strings.TrimSpace(task.Summary)
	if title == "" {
		title = "这个任务"
	}
	if summary == "" {
		return fmt.Sprintf("%s%s", title, stateText)
	}
	return fmt.Sprintf("%s%s：%s", title, stateText, summary)
}

func taskStateLabel(state State) string {
	switch state {
	case StateAccepted:
		return "已受理"
	case StateRunning:
		return "进行中"
	case StateCompleted:
		return "已完成"
	case StateFailed:
		return "失败"
	case StateCanceled:
		return "已取消"
	default:
		return string(state)
	}
}

func (m *Manager) MarkResultReported(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return m.updateTask("", func(task *Task, events *[]Event) {
		if _, ok := set[task.ID]; ok {
			task.ResultReportPending = false
			task.UpdatedAt = time.Now()
		}
		_ = events
	})
}

func (m *Manager) Snapshot() ([]Task, []Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tasks := append([]Task(nil), m.state.Tasks...)
	events := make([]Event, 0, len(m.state.Events))
	for _, event := range m.state.Events {
		if strings.TrimSpace(event.Type) == "claude_output" {
			continue
		}
		events = append(events, event)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt)
	})
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt.After(events[j].CreatedAt)
	})
	return tasks, events
}

func (m *Manager) ArtifactsSnapshot() []Artifact {
	m.mu.Lock()
	defer m.mu.Unlock()

	artifacts := append([]Artifact(nil), m.state.Artifacts...)
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].CreatedAt.After(artifacts[j].CreatedAt)
	})
	return artifacts
}

func (m *Manager) GetArtifact(taskID string, artifactID string) (*Artifact, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	taskID = strings.TrimSpace(taskID)
	artifactID = strings.TrimSpace(artifactID)
	for _, artifact := range m.state.Artifacts {
		if artifact.ID != artifactID || artifact.TaskID != taskID {
			continue
		}
		copyArtifact := artifact
		return &copyArtifact, true
	}
	return nil, false
}

// ListTaskArtifactDeliveries 返回给定任务下的“产物 + 交付状态”联合视图。
//
// 主流程在做任务结果汇报前，会先读取这份视图：
// 1. 看看这次任务到底有没有产物；
// 2. 看看这些产物当前是待发送、已发送、失败还是没有可用渠道；
// 3. 再决定要不要先尝试发 IM，并把交付结果带进结果汇报 prompt。
func (m *Manager) ListTaskArtifactDeliveries(taskIDs []string) []ArtifactDeliveryItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	taskSet := make(map[string]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			continue
		}
		taskSet[taskID] = struct{}{}
	}

	if len(taskSet) == 0 {
		return nil
	}

	artifactsByID := make(map[string]Artifact, len(m.state.Artifacts))
	for _, artifact := range m.state.Artifacts {
		if _, ok := taskSet[artifact.TaskID]; !ok {
			continue
		}
		artifactsByID[artifact.ID] = artifact
	}

	items := make([]ArtifactDeliveryItem, 0, len(m.state.Deliveries))
	for _, delivery := range m.state.Deliveries {
		if _, ok := taskSet[delivery.TaskID]; !ok {
			continue
		}
		artifact, ok := artifactsByID[delivery.ArtifactID]
		if !ok {
			continue
		}
		items = append(items, ArtifactDeliveryItem{
			Delivery: delivery,
			Artifact: artifact,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Artifact.CreatedAt.Before(items[j].Artifact.CreatedAt)
	})
	return items
}

// MarkArtifactDeliveriesNoChannel 把一批产物交付记录标记为“当前没有可用渠道”。
//
// 这一步不会把 result_report_pending 清掉；它只负责把“产物暂时发不出去”的事实
// 写回到任务产物交付记录里，方便后面的语音结果汇报自然带一句：
// “现在还没有可用的渠道发送这些产物。”
func (m *Manager) MarkArtifactDeliveriesNoChannel(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		set[id] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}

	return m.updateState(func(state *fileState) error {
		now := time.Now()
		for i := range state.Deliveries {
			delivery := &state.Deliveries[i]
			if _, ok := set[delivery.ID]; !ok || delivery.Status == ArtifactDeliveryDelivered {
				continue
			}
			if delivery.Status == ArtifactDeliveryNoChannel && strings.TrimSpace(delivery.LastError) == "当前没有可用的渠道发送产物" {
				continue
			}
			artifact := findArtifactByID(state.Artifacts, delivery.ArtifactID)
			task := findTaskByID(state.Tasks, delivery.TaskID)
			if artifact == nil || task == nil {
				continue
			}
			delivery.AccountID = ""
			delivery.TargetID = ""
			delivery.ChannelLabel = ""
			delivery.ProviderMessageID = ""
			delivery.LastError = "当前没有可用的渠道发送产物"
			delivery.Status = ArtifactDeliveryNoChannel
			delivery.UpdatedAt = now
			task.UpdatedAt = now
			state.Events = append(state.Events, Event{
				ID:        m.nextID("event"),
				TaskID:    delivery.TaskID,
				Type:      "artifact_delivery_no_channel",
				Message:   fmt.Sprintf("产物暂未发送：%s（当前没有可用的渠道）", artifact.FileName),
				CreatedAt: now,
			})
		}
		return nil
	})
}

// MarkArtifactDeliveryDelivered 在单个产物发送成功后，写回成功状态和目标信息。
func (m *Manager) MarkArtifactDeliveryDelivered(deliveryID string, accountID string, targetID string, channelLabel string, providerMessageID string) error {
	deliveryID = strings.TrimSpace(deliveryID)
	if deliveryID == "" {
		return fmt.Errorf("artifact delivery id is required")
	}

	return m.updateState(func(state *fileState) error {
		now := time.Now()
		for i := range state.Deliveries {
			delivery := &state.Deliveries[i]
			if delivery.ID != deliveryID {
				continue
			}
			artifact := findArtifactByID(state.Artifacts, delivery.ArtifactID)
			task := findTaskByID(state.Tasks, delivery.TaskID)
			if artifact == nil || task == nil {
				return fmt.Errorf("artifact delivery %q is missing related task or artifact", deliveryID)
			}
			delivery.AccountID = strings.TrimSpace(accountID)
			delivery.TargetID = strings.TrimSpace(targetID)
			delivery.ChannelLabel = strings.TrimSpace(channelLabel)
			delivery.ProviderMessageID = strings.TrimSpace(providerMessageID)
			delivery.LastError = ""
			delivery.Status = ArtifactDeliveryDelivered
			delivery.UpdatedAt = now
			delivery.DeliveredAt = now
			task.UpdatedAt = now
			state.Events = append(state.Events, Event{
				ID:        m.nextID("event"),
				TaskID:    delivery.TaskID,
				Type:      "artifact_delivery_delivered",
				Message:   fmt.Sprintf("产物已发送到%s：%s", fallbackDeliveryChannelLabel(delivery.ChannelLabel), artifact.FileName),
				CreatedAt: now,
			})
			return nil
		}
		return fmt.Errorf("artifact delivery %q not found", deliveryID)
	})
}

// MarkArtifactDeliveryFailed 在单个产物发送失败后，写回失败原因和当前目标信息。
func (m *Manager) MarkArtifactDeliveryFailed(deliveryID string, accountID string, targetID string, channelLabel string, lastError string) error {
	deliveryID = strings.TrimSpace(deliveryID)
	if deliveryID == "" {
		return fmt.Errorf("artifact delivery id is required")
	}
	lastError = strings.TrimSpace(lastError)
	if lastError == "" {
		lastError = "产物发送失败"
	}

	return m.updateState(func(state *fileState) error {
		now := time.Now()
		for i := range state.Deliveries {
			delivery := &state.Deliveries[i]
			if delivery.ID != deliveryID {
				continue
			}
			artifact := findArtifactByID(state.Artifacts, delivery.ArtifactID)
			task := findTaskByID(state.Tasks, delivery.TaskID)
			if artifact == nil || task == nil {
				return fmt.Errorf("artifact delivery %q is missing related task or artifact", deliveryID)
			}
			delivery.AccountID = strings.TrimSpace(accountID)
			delivery.TargetID = strings.TrimSpace(targetID)
			delivery.ChannelLabel = strings.TrimSpace(channelLabel)
			delivery.ProviderMessageID = ""
			delivery.LastError = lastError
			delivery.Status = ArtifactDeliveryFailed
			delivery.UpdatedAt = now
			task.UpdatedAt = now
			state.Events = append(state.Events, Event{
				ID:        m.nextID("event"),
				TaskID:    delivery.TaskID,
				Type:      "artifact_delivery_failed",
				Message:   fmt.Sprintf("产物发送失败：%s（%s）", artifact.FileName, lastError),
				CreatedAt: now,
			})
			return nil
		}
		return fmt.Errorf("artifact delivery %q not found", deliveryID)
	})
}

func (m *Manager) updateTask(taskID string, mutator func(task *Task, events *[]Event)) error {
	return m.updateState(func(state *fileState) error {
		matched := false
		for i := range state.Tasks {
			if taskID != "" && state.Tasks[i].ID != taskID {
				continue
			}
			matched = true
			mutator(&state.Tasks[i], &state.Events)
		}
		if taskID != "" && !matched {
			return fmt.Errorf("task %q not found", taskID)
		}
		return nil
	})
}

// updateState 是任务状态持久化的统一入口。
//
// 它的用途不是只给 task 主记录服务，而是：
// - 更新任务主表快照；
// - 更新任务事件流；
// - 更新产物；
// - 更新产物交付记录；
//
// 所有这些内存态修改完成后，都会在同一把锁里一次性落库，
// 避免主流程看到“任务已经完成了，但交付记录还是旧的”这种半更新状态。
func (m *Manager) updateState(mutator func(state *fileState) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := mutator(&m.state); err != nil {
		return err
	}
	return m.store.Save(m.state)
}

func findTaskByID(tasks []Task, taskID string) *Task {
	for i := range tasks {
		if tasks[i].ID == taskID {
			return &tasks[i]
		}
	}
	return nil
}

func findArtifactByID(artifacts []Artifact, artifactID string) *Artifact {
	for i := range artifacts {
		if artifacts[i].ID == artifactID {
			return &artifacts[i]
		}
	}
	return nil
}

func fallbackDeliveryChannelLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return "通知渠道"
	}
	return label
}

func (m *Manager) latestActiveTaskLocked() *Task {
	var best *Task
	for i := range m.state.Tasks {
		task := &m.state.Tasks[i]
		if task.State != StateAccepted && task.State != StateRunning {
			continue
		}
		if best == nil || task.UpdatedAt.After(best.UpdatedAt) {
			best = task
		}
	}
	return best
}

func (m *Manager) clearCancel(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cancels, taskID)
}

func (m *Manager) notifyResultReportReady() {
	m.mu.Lock()
	fn := m.onResultReportReady
	m.mu.Unlock()
	if fn == nil {
		return
	}
	go fn()
}

func (m *Manager) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, cancel := range m.cancels {
		cancel()
	}
	m.cancels = make(map[string]context.CancelFunc)
	m.state = fileState{Version: 1}
	if m.artifactCache != nil {
		if err := m.artifactCache.reset(); err != nil {
			return err
		}
	}

	if m.store == nil {
		return nil
	}
	return m.store.Reset()
}

func (m *Manager) nextID(prefix string) string {
	n := atomic.AddUint64(&m.seq, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixMilli(), n)
}

type reporter struct {
	manager *Manager
	taskID  string
}

func (r reporter) TaskID() string {
	return r.taskID
}

func (r reporter) Update(summary string) error {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	return r.manager.updateTask(r.taskID, func(task *Task, events *[]Event) {
		if task.State == StateCanceled {
			return
		}
		task.Summary = summary
		task.UpdatedAt = time.Now()
		*events = append(*events, Event{
			ID:        r.manager.nextID("event"),
			TaskID:    r.taskID,
			Type:      "progress",
			Message:   summary,
			CreatedAt: time.Now(),
		})
	})
}

func (r reporter) Event(eventType string, message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil
	}
	return r.manager.updateTask(r.taskID, func(task *Task, events *[]Event) {
		*events = append(*events, Event{
			ID:        r.manager.nextID("event"),
			TaskID:    r.taskID,
			Type:      strings.TrimSpace(eventType),
			Message:   message,
			CreatedAt: time.Now(),
		})
		task.UpdatedAt = time.Now()
	})
}

func (r reporter) PutArtifact(req plugin.PutArtifactRequest) (plugin.ArtifactRef, error) {
	return r.manager.putArtifact(r.taskID, req)
}

func (m *Manager) putArtifact(taskID string, req plugin.PutArtifactRequest) (plugin.ArtifactRef, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return plugin.ArtifactRef{}, fmt.Errorf("task id is required")
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Kind = strings.TrimSpace(req.Kind)
	if req.Name == "" {
		return plugin.ArtifactRef{}, fmt.Errorf("artifact name is required")
	}
	if req.Kind == "" {
		return plugin.ArtifactRef{}, fmt.Errorf("artifact kind is required")
	}

	artifactID := m.nextID("artifact")
	artifact, err := m.artifactCache.put(taskID, req, artifactID, m.nextID)
	if err != nil {
		return plugin.ArtifactRef{}, err
	}
	artifact.CreatedAt = time.Now()
	delivery := ArtifactDelivery{
		ID:         m.nextID("artifact_delivery"),
		TaskID:     taskID,
		ArtifactID: artifact.ID,
		Status:     ArtifactDeliveryPending,
		CreatedAt:  artifact.CreatedAt,
		UpdatedAt:  artifact.CreatedAt,
	}

	var opErr error
	if err := m.updateTask(taskID, func(task *Task, events *[]Event) {
		if task.State != StateAccepted && task.State != StateRunning {
			opErr = fmt.Errorf("task %q is not accepting new artifacts", taskID)
			return
		}
		task.UpdatedAt = time.Now()
		m.state.Artifacts = append(m.state.Artifacts, artifact)
		m.state.Deliveries = append(m.state.Deliveries, delivery)
		*events = append(*events, Event{
			ID:        m.nextID("event"),
			TaskID:    taskID,
			Type:      "artifact",
			Message:   fmt.Sprintf("新增产物：%s", artifact.FileName),
			CreatedAt: artifact.CreatedAt,
		})
	}); err != nil {
		_ = os.Remove(artifact.StoragePath)
		return plugin.ArtifactRef{}, err
	}
	if opErr != nil {
		_ = os.Remove(artifact.StoragePath)
		return plugin.ArtifactRef{}, opErr
	}

	return plugin.ArtifactRef{
		ID:       artifact.ID,
		TaskID:   artifact.TaskID,
		Kind:     artifact.Kind,
		FileName: artifact.FileName,
		MIMEType: artifact.MIMEType,
		Size:     artifact.SizeBytes,
	}, nil
}

func summarizeResult(result string) string {
	result = strings.TrimSpace(result)
	if result == "" {
		return "任务已完成"
	}
	return result
}

func compactTaskText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > 180 {
		return string(runes[:180]) + "..."
	}
	return text
}
