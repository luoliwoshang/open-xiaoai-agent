import { useEffect, useMemo, useState } from 'react'
import type {
  ClaudeRecord,
  ConversationSnapshot,
  DashboardState,
  IMAccount,
  IMEvent,
  IMTarget,
  SessionSettings,
  SettingsSnapshot,
  Task,
  TaskEvent,
  TaskState,
  WeChatLoginStart,
  WeChatLoginStatus,
} from './types'

type Page = 'dashboard' | 'settings'

type LoginPanelState = {
  loading: boolean
  polling: boolean
  sessionKey: string | null
  qrDataUrl: string | null
  qrRawText: string | null
  expiresAt: string | null
  status: WeChatLoginStatus['status'] | null
  message: string | null
  error: string | null
}

const emptyState: DashboardState = {
  tasks: [],
  events: [],
  claude_records: [],
  conversations: [],
  settings: {
    session_window_seconds: 300,
    im_delivery_enabled: false,
    im_selected_account_id: '',
    im_selected_target_id: '',
  },
  im: {
    accounts: [],
    targets: [],
    events: [],
  },
}

const emptyLoginState: LoginPanelState = {
  loading: false,
  polling: false,
  sessionKey: null,
  qrDataUrl: null,
  qrRawText: null,
  expiresAt: null,
  status: null,
  message: null,
  error: null,
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

function currentPageFromHash(): Page {
  return window.location.hash === '#/settings' ? 'settings' : 'dashboard'
}

function selectBestTarget(targets: IMTarget[], accountID: string) {
  const accountTargets = targets.filter((target) => target.account_id === accountID)
  return accountTargets.find((target) => target.is_default)?.id ?? accountTargets[0]?.id ?? ''
}

async function fetchState() {
  const response = await fetch('/api/state', { cache: 'no-store' })
  if (!response.ok) {
    throw new Error(`API ${response.status}`)
  }
  const raw = (await response.json()) as Partial<DashboardState>
  return normalizeState(raw)
}

async function postJSON<T>(url: string, payload: unknown) {
  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(payload),
  })
  if (!response.ok) {
    throw new Error(await response.text())
  }
  return (await response.json()) as T
}

export default function App() {
  const [page, setPage] = useState<Page>(() => currentPageFromHash())
  const [data, setData] = useState<DashboardState>(emptyState)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null)

  const [windowInput, setWindowInput] = useState('300')
  const [windowDirty, setWindowDirty] = useState(false)
  const [settingsSaving, setSettingsSaving] = useState(false)
  const [settingsFeedback, setSettingsFeedback] = useState<string | null>(null)
  const [settingsError, setSettingsError] = useState<string | null>(null)

  const [deliveryEnabled, setDeliveryEnabled] = useState(false)
  const [deliveryAccountID, setDeliveryAccountID] = useState('')
  const [deliveryTargetID, setDeliveryTargetID] = useState('')
  const [deliveryDirty, setDeliveryDirty] = useState(false)
  const [deliverySaving, setDeliverySaving] = useState(false)
  const [deliveryFeedback, setDeliveryFeedback] = useState<string | null>(null)
  const [deliveryError, setDeliveryError] = useState<string | null>(null)

  const [loginPanel, setLoginPanel] = useState<LoginPanelState>(emptyLoginState)

  const [targetAccountID, setTargetAccountID] = useState('')
  const [targetName, setTargetName] = useState('')
  const [targetUserID, setTargetUserID] = useState('')
  const [targetDefault, setTargetDefault] = useState(true)
  const [targetSaving, setTargetSaving] = useState(false)
  const [targetFeedback, setTargetFeedback] = useState<string | null>(null)
  const [targetError, setTargetError] = useState<string | null>(null)

  useEffect(() => {
    const handleHashChange = () => {
      setPage(currentPageFromHash())
    }

    if (!window.location.hash) {
      window.location.hash = '#/'
    }
    handleHashChange()
    window.addEventListener('hashchange', handleHashChange)
    return () => {
      window.removeEventListener('hashchange', handleHashChange)
    }
  }, [])

  useEffect(() => {
    let active = true

    async function refresh() {
      try {
        const next = await fetchState()
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

    void refresh()
    const timer = window.setInterval(() => {
      void refresh()
    }, 2000)

    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [])

  useEffect(() => {
    if (windowDirty || settingsSaving) return
    setWindowInput(String(data.settings.session_window_seconds))
  }, [data.settings.session_window_seconds, settingsSaving, windowDirty])

  useEffect(() => {
    if (deliveryDirty || deliverySaving) return
    setDeliveryEnabled(data.settings.im_delivery_enabled)
    setDeliveryAccountID(data.settings.im_selected_account_id)
    setDeliveryTargetID(data.settings.im_selected_target_id)
  }, [
    data.settings.im_delivery_enabled,
    data.settings.im_selected_account_id,
    data.settings.im_selected_target_id,
    deliveryDirty,
    deliverySaving,
  ])

  useEffect(() => {
    if (targetAccountID) return
    const firstAccount = data.im.accounts[0]?.id ?? ''
    if (firstAccount) {
      setTargetAccountID(firstAccount)
    }
  }, [data.im.accounts, targetAccountID])

  useEffect(() => {
    if (page !== 'settings' || !loginPanel.polling || !loginPanel.sessionKey) return

    let active = true
    const timer = window.setInterval(async () => {
      try {
        const response = await fetch(`/api/im/wechat/login/status?session_key=${encodeURIComponent(loginPanel.sessionKey ?? '')}`, {
          cache: 'no-store',
        })
        if (!response.ok) {
          throw new Error(await response.text())
        }
        const payload = (await response.json()) as { status?: WeChatLoginStatus }
        if (!active || !payload.status) return

        const nextStatus = payload.status.status
        setLoginPanel((current) => ({
          ...current,
          status: payload.status?.status ?? current.status,
          message: payload.status?.message ?? current.message,
          error: null,
          polling: nextStatus === 'pending' || nextStatus === 'scanned',
          sessionKey:
            nextStatus === 'pending' || nextStatus === 'scanned'
              ? current.sessionKey
              : null,
        }))

        if (nextStatus === 'confirmed') {
          const next = await fetchState()
          if (!active) return
          setData(next)
          setDeliveryFeedback('微信账号已登录，现在可以选择它作为 IM 文本触达渠道。')
          setDeliveryError(null)
        }
      } catch (err) {
        if (!active) return
        setLoginPanel((current) => ({
          ...current,
          polling: false,
          error: err instanceof Error ? err.message : '登录状态查询失败',
        }))
      }
    }, 2000)

    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [loginPanel.polling, loginPanel.sessionKey, page])

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

  const deliveryTargets = useMemo(() => {
    return data.im.targets.filter((target) => target.account_id === deliveryAccountID)
  }, [data.im.targets, deliveryAccountID])
  const targetFormTargets = useMemo(() => {
    return data.im.targets.filter((target) => target.account_id === targetAccountID)
  }, [data.im.targets, targetAccountID])

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

  async function refreshNow() {
    const next = await fetchState()
    setData(next)
  }

  async function saveSessionWindowSettings() {
    const nextValue = Number(windowInput)
    if (!Number.isInteger(nextValue)) {
      setSettingsError('请输入整数秒数。')
      setSettingsFeedback(null)
      return
    }

    setSettingsSaving(true)
    setSettingsError(null)
    setSettingsFeedback(null)

    try {
      const payload = await postJSON<{ session?: SessionSettings }>('/api/settings/session', {
        window_seconds: nextValue,
      })
      const nextSettings = normalizeSettings(payload.session)
      setData((current) => ({
        ...current,
        settings: {
          ...current.settings,
          ...nextSettings,
        },
      }))
      setWindowInput(String(nextSettings.session_window_seconds))
      setWindowDirty(false)
      setSettingsFeedback('已保存，后续请求会立即按新的滑动窗口秒数生效。')
    } catch (err) {
      setSettingsError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSettingsSaving(false)
    }
  }

  async function saveDeliverySettings() {
    setDeliverySaving(true)
    setDeliveryError(null)
    setDeliveryFeedback(null)

    try {
      const payload = await postJSON<{ settings?: SettingsSnapshot }>('/api/settings/im-delivery', {
        enabled: deliveryEnabled,
        selected_account_id: deliveryAccountID,
        selected_target_id: deliveryTargetID,
      })
      const nextSettings = normalizeSettings(payload.settings)
      setData((current) => ({
        ...current,
        settings: nextSettings,
      }))
      setDeliveryDirty(false)
      setDeliveryFeedback('IM 文本触达设置已保存。')
    } catch (err) {
      setDeliveryError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setDeliverySaving(false)
    }
  }

  async function startWeChatLogin() {
    setLoginPanel({
      ...emptyLoginState,
      loading: true,
    })
    try {
      const payload = await postJSON<{ login?: WeChatLoginStart }>('/api/im/wechat/login/start', {})
      if (!payload.login) {
        throw new Error('登录二维码返回为空')
      }
      setLoginPanel({
        loading: false,
        polling: true,
        sessionKey: payload.login.session_key,
        qrDataUrl: payload.login.qr_code_data_url,
        qrRawText: payload.login.qr_raw_text,
        expiresAt: payload.login.expires_at,
        status: 'pending',
        message: '请使用微信扫描下方二维码。',
        error: null,
      })
    } catch (err) {
      setLoginPanel({
        ...emptyLoginState,
        error: err instanceof Error ? err.message : '启动微信登录失败',
      })
    }
  }

  async function createTarget() {
    setTargetSaving(true)
    setTargetError(null)
    setTargetFeedback(null)
    try {
      await postJSON('/api/im/targets', {
        account_id: targetAccountID,
        name: targetName,
        target_user_id: targetUserID,
        set_default: targetDefault,
      })
      await refreshNow()
      setTargetName('')
      setTargetUserID('')
      setTargetDefault(true)
      setTargetFeedback('触达目标已保存。')
      if (!deliveryAccountID) {
        setDeliveryAccountID(targetAccountID)
      }
    } catch (err) {
      setTargetError(err instanceof Error ? err.message : '保存目标失败')
    } finally {
      setTargetSaving(false)
    }
  }

  async function setDefaultTarget(accountID: string, targetID: string) {
    try {
      await postJSON('/api/im/targets/default', {
        account_id: accountID,
        target_id: targetID,
      })
      await refreshNow()
      setTargetFeedback('默认触达目标已更新。')
      setTargetError(null)
    } catch (err) {
      setTargetError(err instanceof Error ? err.message : '更新默认目标失败')
    }
  }

  async function deleteTarget(targetID: string) {
    if (!window.confirm('确定删除这个触达目标吗？')) return
    try {
      await postJSON('/api/im/targets/delete', {
        target_id: targetID,
      })
      await refreshNow()
      setTargetFeedback('触达目标已删除。')
      setTargetError(null)
    } catch (err) {
      setTargetError(err instanceof Error ? err.message : '删除触达目标失败')
    }
  }

  async function deleteAccount(accountID: string) {
    if (!window.confirm('确定删除这个微信账号吗？这会同时删除它下面的触达目标。')) return
    try {
      await postJSON('/api/im/accounts/delete', {
        account_id: accountID,
      })
      await refreshNow()
      setDeliveryFeedback('微信账号已删除。')
      setDeliveryError(null)
    } catch (err) {
      setDeliveryError(err instanceof Error ? err.message : '删除微信账号失败')
    }
  }

  return (
    <div className="app-shell">
      <div className="aurora aurora-left" />
      <div className="aurora aurora-right" />

      <header className="topbar">
        <div>
          <p className="eyebrow">OPEN XIAOAI AGENT</p>
          <h1 className="topbar-title">灵矽控制台</h1>
        </div>

        <nav className="topbar-nav">
          <a className={`topbar-link ${page === 'dashboard' ? 'topbar-link-active' : ''}`} href="#/">
            任务看板
          </a>
          <a className={`topbar-link ${page === 'settings' ? 'topbar-link-active' : ''}`} href="#/settings">
            系统设置
          </a>
        </nav>
      </header>

      {page === 'dashboard' ? (
        <>
          <header className="hero">
            <div className="hero-copy">
              <p className="eyebrow">LINGXI / TASK CONTROL</p>
              <h2>异步任务看板</h2>
              <p className="hero-text">
                React 前端只负责看板和状态感知。Go 侧退回纯 API 服务，任务状态、事件流和补报都通过
                <code>/api/state</code> 提供。
              </p>
            </div>

            <section className="hero-panel hero-panel-conversation">
              <div className="hero-panel-head">
                <div>
                  <div className="hero-panel-label">CURRENT CONTEXT</div>
                  <h3>会话历史</h3>
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
                  <h3>{heroTask.title}</h3>
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
              <section className="focus-card settings-card">
                <div className="focus-card-head">
                  <div>
                    <p className="eyebrow">SESSION SETTINGS</p>
                    <h3>会话窗口</h3>
                    <p>当前只保留滑动窗口策略。这里控制最近活跃多久后，系统才把上下文切到新会话。</p>
                  </div>
                  <a className="settings-jump" href="#/settings">
                    去系统设置
                  </a>
                </div>

                <div className="settings-form">
                  <label className="settings-field">
                    <span>会话窗口秒数</span>
                    <input
                      className="settings-input"
                      inputMode="numeric"
                      min={30}
                      max={3600}
                      step={1}
                      type="number"
                      value={windowInput}
                      onChange={(event) => {
                        setWindowInput(event.target.value)
                        setWindowDirty(true)
                        setSettingsFeedback(null)
                        setSettingsError(null)
                      }}
                    />
                  </label>

                  <div className="settings-actions">
                    <button
                      className="settings-button"
                      disabled={settingsSaving || !windowDirty}
                      onClick={() => void saveSessionWindowSettings()}
                      type="button"
                    >
                      {settingsSaving ? '保存中...' : '保存设置'}
                    </button>
                    <span className="settings-note">建议范围 30 - 3600 秒，默认值 300 秒。</span>
                  </div>

                  {settingsFeedback ? <div className="settings-feedback">{settingsFeedback}</div> : null}
                  {settingsError ? <div className="error-banner settings-error">{settingsError}</div> : null}
                </div>
              </section>

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
        </>
      ) : (
        <main className="settings-page">
          <section className="settings-hero-card">
            <div>
              <p className="eyebrow">SYSTEM SETTINGS</p>
              <h2>IM Gateway 与系统设置</h2>
              <p className="hero-text">
                这里单独负责运行期设置和微信账号管理。第一期只做微信文本触达：
                小爱的回复在设备播报成功后，会异步再镜像到你选中的微信目标。
              </p>
            </div>
            <div className="settings-hero-stats">
              <div className="metric-card metric-mint">
                <span>微信账号</span>
                <strong>{data.im.accounts.length}</strong>
              </div>
              <div className="metric-card metric-cyan">
                <span>触达目标</span>
                <strong>{data.im.targets.length}</strong>
              </div>
              <div className="metric-card metric-amber">
                <span>镜像状态</span>
                <strong>{data.settings.im_delivery_enabled ? '已开启' : '未开启'}</strong>
              </div>
            </div>
          </section>

          {error ? <div className="error-banner">接口异常：{error}</div> : null}

          <section className="settings-grid-page">
            <article className="panel settings-panel">
              <div className="panel-head compact">
                <div>
                  <p className="eyebrow">SESSION</p>
                  <h3>会话窗口</h3>
                </div>
              </div>

              <div className="settings-form">
                <label className="settings-field">
                  <span>会话窗口秒数</span>
                  <input
                    className="settings-input"
                    inputMode="numeric"
                    min={30}
                    max={3600}
                    step={1}
                    type="number"
                    value={windowInput}
                    onChange={(event) => {
                      setWindowInput(event.target.value)
                      setWindowDirty(true)
                      setSettingsFeedback(null)
                      setSettingsError(null)
                    }}
                  />
                </label>

                <div className="settings-actions">
                  <button
                    className="settings-button"
                    disabled={settingsSaving || !windowDirty}
                    onClick={() => void saveSessionWindowSettings()}
                    type="button"
                  >
                    {settingsSaving ? '保存中...' : '保存设置'}
                  </button>
                  <span className="settings-note">默认值 300 秒，只保留滑动窗口策略。</span>
                </div>

                {settingsFeedback ? <div className="settings-feedback">{settingsFeedback}</div> : null}
                {settingsError ? <div className="error-banner settings-error">{settingsError}</div> : null}
              </div>
            </article>

            <article className="panel settings-panel">
              <div className="panel-head compact">
                <div>
                  <p className="eyebrow">IM DELIVERY</p>
                  <h3>回复镜像到微信</h3>
                </div>
              </div>

              <div className="settings-form">
                <label className="checkbox-row">
                  <input
                    checked={deliveryEnabled}
                    onChange={(event) => {
                      setDeliveryEnabled(event.target.checked)
                      setDeliveryDirty(true)
                      setDeliveryFeedback(null)
                      setDeliveryError(null)
                    }}
                    type="checkbox"
                  />
                  <span>开启后，小爱的正常回复会异步再发一份到微信。</span>
                </label>

                <label className="settings-field">
                  <span>激活微信账号</span>
                  <select
                    className="settings-select"
                    value={deliveryAccountID}
                    onChange={(event) => {
                      const nextAccountID = event.target.value
                      setDeliveryAccountID(nextAccountID)
                      setDeliveryTargetID(selectBestTarget(data.im.targets, nextAccountID))
                      setDeliveryDirty(true)
                      setDeliveryFeedback(null)
                      setDeliveryError(null)
                    }}
                  >
                    <option value="">请选择账号</option>
                    {data.im.accounts.map((account) => (
                      <option key={account.id} value={account.id}>
                        {account.display_name || account.remote_account_id}
                      </option>
                    ))}
                  </select>
                </label>

                <label className="settings-field">
                  <span>默认触达对象</span>
                  <select
                    className="settings-select"
                    value={deliveryTargetID}
                    onChange={(event) => {
                      setDeliveryTargetID(event.target.value)
                      setDeliveryDirty(true)
                      setDeliveryFeedback(null)
                      setDeliveryError(null)
                    }}
                  >
                    <option value="">请选择目标</option>
                    {deliveryTargets.map((target) => (
                      <option key={target.id} value={target.id}>
                        {target.name} · {target.target_user_id}
                      </option>
                    ))}
                  </select>
                </label>

                <div className="settings-actions">
                  <button
                    className="settings-button"
                    disabled={deliverySaving || !deliveryDirty}
                    onClick={() => void saveDeliverySettings()}
                    type="button"
                  >
                    {deliverySaving ? '保存中...' : '保存镜像设置'}
                  </button>
                  <span className="settings-note">发送为异步副作用，不阻塞小爱当前播报。</span>
                </div>

                {deliveryFeedback ? <div className="settings-feedback">{deliveryFeedback}</div> : null}
                {deliveryError ? <div className="error-banner settings-error">{deliveryError}</div> : null}
              </div>
            </article>

            <article className="panel settings-panel panel-wide">
              <div className="panel-head compact">
                <div>
                  <p className="eyebrow">WECHAT LOGIN</p>
                  <h3>微信扫码登录</h3>
                </div>
                <button className="settings-button" disabled={loginPanel.loading || loginPanel.polling} onClick={() => void startWeChatLogin()} type="button">
                  {loginPanel.loading ? '启动中...' : loginPanel.polling ? '等待扫码中' : '新增微信账号'}
                </button>
              </div>

              <div className="settings-login-grid">
                <div className="qr-card">
                  {loginPanel.qrDataUrl ? (
                    <img alt="微信登录二维码" className="qr-image" src={loginPanel.qrDataUrl} />
                  ) : (
                    <div className="empty-card qr-empty">点击右上角按钮后，这里会出现微信登录二维码。</div>
                  )}
                </div>

                <div className="login-copy">
                  <p className="settings-note">
                    当前阶段只做微信文本触达，不做 IM 入站会话。扫码成功后，会自动保存微信账号登录态，并尝试把扫码用户登记为默认触达对象。
                  </p>
                  {loginPanel.message ? <div className="settings-feedback">{loginPanel.message}</div> : null}
                  {loginPanel.error ? <div className="error-banner settings-error">{loginPanel.error}</div> : null}
                  {loginPanel.expiresAt ? (
                    <div className="task-meta">
                      <span>二维码过期时间</span>
                      <p>{formatTime(loginPanel.expiresAt)}</p>
                    </div>
                  ) : null}
                  {loginPanel.qrRawText ? (
                    <div className="task-meta task-meta-wide">
                      <span>二维码原始内容</span>
                      <p><code>{loginPanel.qrRawText}</code></p>
                    </div>
                  ) : null}
                </div>
              </div>
            </article>

            <article className="panel settings-panel panel-wide">
              <div className="panel-head compact">
                <div>
                  <p className="eyebrow">WECHAT ACCOUNTS</p>
                  <h3>账号管理</h3>
                </div>
              </div>

              <div className="account-grid">
                {data.im.accounts.length === 0 ? (
                  <div className="empty-card">还没有微信账号。先扫码登录一个账号。</div>
                ) : (
                  data.im.accounts.map((account: IMAccount) => (
                    <article className="account-card" key={account.id}>
                      <div className="account-card-head">
                        <div>
                          <h4>{account.display_name || account.remote_account_id}</h4>
                          <p>{account.remote_account_id}</p>
                        </div>
                        <button className="ghost-button" onClick={() => void deleteAccount(account.id)} type="button">
                          删除
                        </button>
                      </div>

                      <div className="focus-grid">
                        <div className="task-meta">
                          <span>平台</span>
                          <p>{account.platform}</p>
                        </div>
                        <div className="task-meta">
                          <span>扫码用户</span>
                          <p>{account.owner_user_id || '—'}</p>
                        </div>
                        <div className="task-meta task-meta-wide">
                          <span>Base URL</span>
                          <p>{account.base_url}</p>
                        </div>
                        <div className="task-meta">
                          <span>最近发送</span>
                          <p>{formatTime(account.last_sent_at)}</p>
                        </div>
                        <div className="task-meta">
                          <span>最近错误</span>
                          <p>{account.last_error || '—'}</p>
                        </div>
                      </div>
                    </article>
                  ))
                )}
              </div>
            </article>

            <article className="panel settings-panel panel-wide">
              <div className="panel-head compact">
                <div>
                  <p className="eyebrow">TARGETS</p>
                  <h3>触达目标管理</h3>
                </div>
              </div>

              <div className="target-editor">
                <div className="settings-form">
                  <label className="settings-field">
                    <span>所属微信账号</span>
                    <select
                      className="settings-select"
                      value={targetAccountID}
                      onChange={(event) => {
                        setTargetAccountID(event.target.value)
                        setTargetFeedback(null)
                        setTargetError(null)
                      }}
                    >
                      <option value="">请选择账号</option>
                      {data.im.accounts.map((account) => (
                        <option key={account.id} value={account.id}>
                          {account.display_name || account.remote_account_id}
                        </option>
                      ))}
                    </select>
                  </label>

                  <label className="settings-field">
                    <span>目标备注名</span>
                    <input
                      className="settings-input"
                      placeholder="例如：我的微信"
                      type="text"
                      value={targetName}
                      onChange={(event) => setTargetName(event.target.value)}
                    />
                  </label>

                  <label className="settings-field">
                    <span>微信用户 ID</span>
                    <input
                      className="settings-input"
                      placeholder="例如：xxx@im.wechat"
                      type="text"
                      value={targetUserID}
                      onChange={(event) => setTargetUserID(event.target.value)}
                    />
                  </label>

                  <label className="checkbox-row">
                    <input checked={targetDefault} onChange={(event) => setTargetDefault(event.target.checked)} type="checkbox" />
                    <span>保存后设为该账号的默认触达目标</span>
                  </label>

                  <div className="settings-actions">
                    <button
                      className="settings-button"
                      disabled={targetSaving || !targetAccountID || !targetUserID}
                      onClick={() => void createTarget()}
                      type="button"
                    >
                      {targetSaving ? '保存中...' : '保存目标'}
                    </button>
                    <span className="settings-note">如果扫码返回了用户 ID，系统通常会自动补一个“扫码用户”目标。</span>
                  </div>

                  {targetFeedback ? <div className="settings-feedback">{targetFeedback}</div> : null}
                  {targetError ? <div className="error-banner settings-error">{targetError}</div> : null}
                </div>

                <div className="target-list">
                  {targetFormTargets.length === 0 ? (
                    <div className="empty-card">当前账号还没有触达目标。</div>
                  ) : (
                    targetFormTargets.map((target) => (
                      <article className="target-card" key={target.id}>
                        <div>
                          <p className="task-title">{target.name}</p>
                          <p className="task-input">{target.target_user_id}</p>
                        </div>
                        <div className="target-actions">
                          {target.is_default ? <span className="badge badge-running">默认</span> : null}
                          <button className="ghost-button" onClick={() => void setDefaultTarget(target.account_id, target.id)} type="button">
                            设为默认
                          </button>
                          <button className="ghost-button ghost-danger" onClick={() => void deleteTarget(target.id)} type="button">
                            删除
                          </button>
                        </div>
                      </article>
                    ))
                  )}
                </div>
              </div>
            </article>

            <article className="panel settings-panel panel-wide">
              <div className="panel-head compact">
                <div>
                  <p className="eyebrow">RECENT EVENTS</p>
                  <h3>网关事件</h3>
                </div>
              </div>

              <div className="timeline">
                {data.im.events.length === 0 ? (
                  <div className="empty-card">暂时还没有网关事件。</div>
                ) : (
                  data.im.events.map((event: IMEvent) => (
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
            </article>
          </section>
        </main>
      )}
    </div>
  )
}

function normalizeState(raw: Partial<DashboardState> | undefined): DashboardState {
  return {
    tasks: raw?.tasks ?? [],
    events: raw?.events ?? [],
    claude_records: raw?.claude_records ?? [],
    conversations: (raw?.conversations ?? []).map((conversation) => ({
      ...conversation,
      messages: conversation?.messages ?? [],
    })),
    settings: normalizeSettings(raw?.settings),
    im: {
      accounts: raw?.im?.accounts ?? [],
      targets: raw?.im?.targets ?? [],
      events: raw?.im?.events ?? [],
    },
  }
}

function normalizeSettings(raw: Partial<SettingsSnapshot> | undefined): SettingsSnapshot {
  return {
    session_window_seconds: raw?.session_window_seconds ?? 300,
    im_delivery_enabled: raw?.im_delivery_enabled ?? false,
    im_selected_account_id: raw?.im_selected_account_id ?? '',
    im_selected_target_id: raw?.im_selected_target_id ?? '',
  }
}

