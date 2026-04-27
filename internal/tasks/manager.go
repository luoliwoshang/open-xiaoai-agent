package tasks

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/plugin"
)

type Manager struct {
	mu      sync.Mutex
	store   *Store
	state   fileState
	seq     uint64
	cancels map[string]context.CancelFunc
}

func NewManager(path string) (*Manager, error) {
	store := NewStore(path)
	state, err := store.Load()
	if err != nil {
		return nil, err
	}

	m := &Manager{
		store:   store,
		state:   state,
		cancels: make(map[string]context.CancelFunc),
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
		ID:            m.nextID("task"),
		Plugin:        strings.TrimSpace(spec.Plugin),
		Kind:          strings.TrimSpace(spec.Kind),
		Title:         strings.TrimSpace(spec.Title),
		Input:         strings.TrimSpace(spec.Input),
		ParentTaskID:  strings.TrimSpace(spec.ParentTaskID),
		State:         StateAccepted,
		Summary:       "任务已受理",
		ReportPending: false,
		CreatedAt:     now,
		UpdatedAt:     now,
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
			task.ReportPending = true
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
		return
	}

	m.updateTask(taskID, func(task *Task, events *[]Event) {
		if task.State == StateCanceled {
			return
		}
		task.State = StateCompleted
		task.Result = strings.TrimSpace(result)
		task.Summary = summarizeResult(task.Result)
		task.ReportPending = true
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
}

func (m *Manager) CancelLatest() (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task := m.latestActiveTaskLocked()
	if task == nil {
		return nil, nil
	}
	now := time.Now()
	task.State = StateCanceled
	task.Summary = "任务已取消"
	task.ReportPending = true
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
		return nil, err
	}
	copyTask := *task
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

func (m *Manager) BuildPendingReport(limit int) (string, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tasks := append([]Task(nil), m.state.Tasks...)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt)
	})

	var ids []string
	var items []string
	for _, task := range tasks {
		if !task.ReportPending {
			continue
		}
		ids = append(ids, task.ID)
		switch task.State {
		case StateCompleted:
			items = append(items, formatPendingItem(task, "已经完成了"))
		case StateFailed:
			items = append(items, formatPendingItem(task, "失败了"))
		case StateCanceled:
			items = append(items, formatPendingItem(task, "已经取消了"))
		}
		if len(items) >= limit {
			break
		}
	}
	if len(items) == 0 {
		return "", nil
	}
	return "对了，刚刚有任务有新进展：" + strings.Join(items, "；") + "。", ids
}

func (m *Manager) PendingReports(limit int) ([]PendingReportItem, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tasks := append([]Task(nil), m.state.Tasks...)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt)
	})

	items := make([]PendingReportItem, 0, limit)
	ids := make([]string, 0, limit)
	for _, task := range tasks {
		if !task.ReportPending {
			continue
		}
		switch task.State {
		case StateCompleted, StateFailed, StateCanceled:
		default:
			continue
		}
		items = append(items, PendingReportItem{
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

func formatPendingItem(task Task, stateText string) string {
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

func (m *Manager) MarkReported(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return m.updateTask("", func(task *Task, events *[]Event) {
		if _, ok := set[task.ID]; ok {
			task.ReportPending = false
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

func (m *Manager) updateTask(taskID string, mutator func(task *Task, events *[]Event)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.state.Tasks {
		if taskID != "" && m.state.Tasks[i].ID != taskID {
			continue
		}
		mutator(&m.state.Tasks[i], &m.state.Events)
	}
	return m.store.Save(m.state)
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
