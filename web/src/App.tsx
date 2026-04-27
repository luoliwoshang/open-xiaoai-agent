import { useEffect, useMemo, useState } from 'react'
import type { ClaudeRecord, ConversationSnapshot, DashboardState, Task, TaskEvent, TaskState } from './types'

const emptyState: DashboardState = {
  tasks: [],
  events: [],
  claude_records: [],
  conversations: [],
}

const stateLabels: Record<TaskState, string> = {
  accepted: '已受理',
  running: '执行中',
  completed: '已完成',
  failed: '失败',
  canceled: '已取消',
}

function formatTime(value: string) {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(date)
}

function countByState(tasks: Task[], state: TaskState) {
  return tasks.filter((task) => task.state === state).length
}

function latest<T extends { created_at?: string; updated_at?: string }>(items: T[]) {
  return items[0] ?? null
}

export default function App() {
  const [data, setData] = useState<DashboardState>(emptyState)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null)

  useEffect(() => {
    let active = true

    async function refresh() {
      try {
        const response = await fetch('/api/state', { cache: 'no-store' })
        if (!response.ok) {
          throw new Error(`API ${response.status}`)
        }
        const raw = (await response.json()) as Partial<DashboardState>
        const next: DashboardState = {
          tasks: raw.tasks ?? [],
          events: raw.events ?? [],
          claude_records: raw.claude_records ?? [],
          conversations: (raw.conversations ?? []).map((conversation) => ({
            ...conversation,
            messages: conversation?.messages ?? [],
          })),
        }
        if (!active) return
        setData(next)
        setError(null)
      } catch (err) {
        if (!active) return
        setError(err instanceof Error ? err.message : '未知错误')
      } finally {
        if (active) {
          setLoading(false)
        }
      }
    }

    refresh()
    const timer = window.setInterval(refresh, 2000)
    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [])

  const metrics = useMemo(() => {
    const completed = countByState(data.tasks, 'completed')
    const running = countByState(data.tasks, 'running')
    const failed = countByState(data.tasks, 'failed')
    const pendingReports = data.tasks.filter((task) => task.report_pending).length

    return [
      { label: '总任务', value: data.tasks.length, accent: 'cyan' },
      { label: '执行中', value: running, accent: 'amber' },
      { label: '已完成', value: completed, accent: 'mint' },
      { label: '待补报', value: pendingReports, accent: 'rose' },
      { label: '失败数', value: failed, accent: 'violet' },
    ]
  }, [data.tasks])

  const heroTask = latest(data.tasks)
  const eventsByTask = useMemo(() => {
    const grouped = new Map<string, TaskEvent[]>()
    for (const event of data.events) {
      const items = grouped.get(event.task_id)
      if (items) {
        items.push(event)
      } else {
        grouped.set(event.task_id, [event])
      }
    }
    return grouped
  }, [data.events])

  const selectedTask =
    data.tasks.find((task) => task.id === selectedTaskId) ?? heroTask ?? null
  const selectedEvents = selectedTask ? eventsByTask.get(selectedTask.id) ?? [] : []
  const claudeRecord = useMemo(() => {
    if (!selectedTask) return null
    return data.claude_records.find((record) => record.task_id === selectedTask.id) ?? null
  }, [data.claude_records, selectedTask])
  const activeConversation = useMemo<ConversationSnapshot | null>(() => {
    return data.conversations[0] ?? null
  }, [data.conversations])
  const activeConversationMessages = activeConversation?.messages ?? []

  useEffect(() => {
    if (data.tasks.length === 0) {
      setSelectedTaskId(null)
      return
    }
    if (selectedTaskId && data.tasks.some((task) => task.id === selectedTaskId)) {
      return
    }
    setSelectedTaskId(data.tasks[0].id)
  }, [data.tasks, selectedTaskId])

  return (
    <div className="app-shell">
      <div className="aurora aurora-left" />
      <div className="aurora aurora-right" />

      <header className="hero">
        <div className="hero-copy">
          <p className="eyebrow">LINGXI / TASK CONTROL</p>
          <h1>异步任务看板</h1>
          <p className="hero-text">
            React 前端只负责看板和状态感知。Go 侧退回纯 API 服务，任务状态、事件流和补报都通过
            <code>/api/state</code> 提供。
          </p>
        </div>

        <section className="hero-panel hero-panel-conversation">
          <div className="hero-panel-head">
            <div>
              <div className="hero-panel-label">CURRENT CONTEXT</div>
              <h2>会话历史</h2>
            </div>
            {activeConversation ? (
              <span className="panel-meta">
                {activeConversationMessages.length} 条消息 · 最近活跃 {formatTime(activeConversation.last_active)}
              </span>
            ) : null}
          </div>
          {activeConversation ? (
            <div className="conversation-list conversation-list-hero">
              {activeConversationMessages.map((message, index) => (
                <article
                  className={`conversation-bubble conversation-${message.role}`}
                  key={`${activeConversation.id}-${index}`}
                >
                  <div className="conversation-bubble-head">
                    <span>{message.role === 'user' ? 'USER' : 'ASSISTANT'}</span>
                  </div>
                  <p>{message.content}</p>
                </article>
              ))}
            </div>
          ) : (
            <div className="empty-card">当前还没有活跃会话历史。</div>
          )}
        </section>

        <section className="hero-panel hero-panel-task">
          <div className="hero-panel-label">最近任务</div>
          {heroTask ? (
            <>
              <h2>{heroTask.title}</h2>
              <p>{heroTask.summary || '还没有摘要。'}</p>
              <div className={`badge badge-${heroTask.state}`}>{stateLabels[heroTask.state]}</div>
            </>
          ) : (
            <div className="empty-card">还没有任务进入系统。</div>
          )}
        </section>
      </header>

      <section className="metrics-grid">
        {metrics.map((metric) => (
          <article className={`metric-card metric-${metric.accent}`} key={metric.label}>
            <span>{metric.label}</span>
            <strong>{metric.value}</strong>
          </article>
        ))}
      </section>

      <main className="content-grid">
        <section className="panel panel-tasks">
          <div className="panel-head">
            <div>
              <p className="eyebrow">TASKS</p>
              <h2>任务总览</h2>
            </div>
            <span className="panel-meta">{loading ? '正在加载' : `每 2 秒刷新 · ${data.tasks.length} 条`}</span>
          </div>

          {error ? <div className="error-banner">接口异常：{error}</div> : null}

          <div className="task-list">
            {data.tasks.length === 0 ? (
              <div className="empty-card">当前还没有任务。先让灵矽接一个复杂任务试试看。</div>
            ) : (
              data.tasks.map((task) => (
                <article
                  className={`task-card ${selectedTask?.id === task.id ? 'task-card-selected' : ''}`}
                  key={task.id}
                  onClick={() => setSelectedTaskId(task.id)}
                  role="button"
                  tabIndex={0}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault()
                      setSelectedTaskId(task.id)
                    }
                  }}
                >
                  <div className="task-card-head">
                    <div>
                      <p className="task-title">{task.title}</p>
                      <p className="task-input">{task.input || '没有原始输入。'}</p>
                    </div>
                    <span className={`badge badge-${task.state}`}>{stateLabels[task.state]}</span>
                  </div>

                  <div className="task-card-body">
                    <div className="task-meta">
                      <span>摘要</span>
                      <p>{task.summary || '暂无摘要。'}</p>
                    </div>
                    {task.result ? (
                      <div className="task-meta">
                        <span>结果</span>
                        <p>{task.result}</p>
                      </div>
                    ) : null}
                  </div>

                  <div className="task-card-foot">
                    <span>{task.id}</span>
                    <span>{formatTime(task.updated_at)}</span>
                  </div>
                </article>
              ))
            )}
          </div>
        </section>

        <aside className="panel panel-side">
          <section className="timeline-card">
            <div className="panel-head compact">
              <div>
                <p className="eyebrow">FOCUS</p>
                <h2>任务详情</h2>
              </div>
              {selectedTask ? (
                <span className="panel-meta">
                  {selectedEvents.length} 条事件 · {stateLabels[selectedTask.state]}
                </span>
              ) : null}
            </div>

            {selectedTask ? (
              <>
                <article className="focus-card">
                  <div className="focus-card-head">
                    <div>
                      <h3>{selectedTask.title}</h3>
                      <p>{selectedTask.input || '没有原始输入。'}</p>
                    </div>
                    <span className={`badge badge-${selectedTask.state}`}>{stateLabels[selectedTask.state]}</span>
                  </div>

                  <div className="focus-grid">
                    <div className="task-meta">
                      <span>任务摘要</span>
                      <p>{selectedTask.summary || '暂无摘要。'}</p>
                    </div>
                    <div className="task-meta">
                      <span>最近更新时间</span>
                      <p>{formatTime(selectedTask.updated_at)}</p>
                    </div>
                    {selectedTask.result ? (
                      <div className="task-meta task-meta-wide">
                        <span>最终结果</span>
                        <p>{selectedTask.result}</p>
                      </div>
                    ) : null}
                  </div>
                </article>

                <article className="focus-card focus-card-plugin">
                  <div className="focus-card-head">
                    <div>
                      <p className="eyebrow">CLAUDE PLUGIN</p>
                      <h3>插件调用记录</h3>
                    </div>
                    {claudeRecord ? (
                      <span className={`badge badge-${claudeRecord.status}`}>{stateLabels[claudeRecord.status]}</span>
                    ) : null}
                  </div>

                  {claudeRecord ? (
                    <div className="focus-grid">
                      <div className="task-meta">
                        <span>Claude Session</span>
                        <p><code>{claudeRecord.session_id || '尚未建立'}</code></p>
                      </div>
                      <div className="task-meta">
                        <span>工作目录</span>
                        <p>{claudeRecord.working_directory || '—'}</p>
                      </div>
                      <div className="task-meta task-meta-wide">
                        <span>任务提示词</span>
                        <p>{claudeRecord.prompt || '—'}</p>
                      </div>
                      <div className="task-meta">
                        <span>最新摘要</span>
                        <p>{claudeRecord.last_summary || '暂无摘要。'}</p>
                      </div>
                      <div className="task-meta">
                        <span>最近记录时间</span>
                        <p>{formatTime(claudeRecord.updated_at)}</p>
                      </div>
                      {claudeRecord.last_assistant_text ? (
                        <div className="task-meta task-meta-wide">
                          <span>最近输出</span>
                          <p>{claudeRecord.last_assistant_text}</p>
                        </div>
                      ) : null}
                      {claudeRecord.error ? (
                        <div className="task-meta task-meta-wide">
                          <span>错误</span>
                          <p>{claudeRecord.error}</p>
                        </div>
                      ) : null}
                    </div>
                  ) : (
                    <div className="empty-card">这个任务还没有 Claude 插件记录。</div>
                  )}
                </article>

                <div className="timeline">
                  {selectedEvents.length === 0 ? (
                    <div className="empty-card">这个任务还没有事件流。</div>
                  ) : (
                    selectedEvents.map((event: TaskEvent) => (
                      <article className="timeline-item" key={event.id}>
                        <span className="timeline-dot" />
                        <div>
                          <div className="timeline-head">
                            <strong>{event.type}</strong>
                            <span>{formatTime(event.created_at)}</span>
                          </div>
                          <p>{event.message}</p>
                        </div>
                      </article>
                    ))
                  )}
                </div>
              </>
            ) : (
              <div className="empty-card">还没有任务，右侧暂时没有焦点内容。</div>
            )}
          </section>
        </aside>
      </main>
    </div>
  )
}
