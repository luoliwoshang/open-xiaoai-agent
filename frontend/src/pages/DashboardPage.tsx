import { useState, type FormEvent } from 'react'
import { Zap, CheckCircle2, XCircle, Ban } from 'lucide-react'
import type { DashboardState } from '../types'
import { countByState, formatTime } from '../lib/dashboard'
import { submitAssistantASR } from '../lib/api'
import { TaskListPane } from '../components/dashboard/TaskListPane'
import { RecentConversationPane } from '../components/dashboard/RecentConversationPane'
import { TaskDetailModal } from '../components/dashboard/TaskDetailModal'

interface DashboardPageProps {
  state: DashboardState
  onReload: () => Promise<void>
}

export function DashboardPage({ state, onReload }: DashboardPageProps) {
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null)
  const [asrText, setAsrText] = useState('')
  const [asrSending, setAsrSending] = useState(false)
  const [asrFeedback, setAsrFeedback] = useState<string | null>(null)
  const [asrError, setAsrError] = useState<string | null>(null)
  const counts = countByState(state.tasks)
  const total = state.tasks.length
  const selectedTask = state.tasks.find((t) => t.id === selectedTaskId) ?? null
  const asrBlocked = state.assistant.busy

  async function handleASRSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const nextText = asrText.trim()
    if (!nextText) {
      setAsrError('请输入一段要送入主流程的识别文本。')
      setAsrFeedback(null)
      return
    }

    try {
      setAsrSending(true)
      setAsrError(null)
      setAsrFeedback(null)
      const payload = await submitAssistantASR(nextText)
      setAsrText('')
      setAsrFeedback(`已送入主流程：${payload.text}`)
      await onReload()
    } catch (error) {
      setAsrFeedback(null)
      setAsrError(error instanceof Error ? error.message : '发送失败')
    } finally {
      setAsrSending(false)
    }
  }

  return (
    <div>
      <div className="page-header">
        <h2>调试台</h2>
        <div className="page-header-sub">
          调试主流程、观察任务与排查运行状态 · 上次更新 {formatTime(state.tasks[0]?.updated_at || new Date().toISOString())}
          {state.assistant.busy && ' · 执行中'}
        </div>
      </div>

      <div className="stat-cards">
        <div className="stat-card total">
          <div className="stat-label">总任务</div>
          <div className="stat-value">{total}</div>
        </div>
        <div className="stat-card running">
          <div className="stat-label">
            <Zap style={{ width: 10, height: 10, marginRight: 4, verticalAlign: 'middle' }} />
            运行中
          </div>
          <div className="stat-value">{counts.running}</div>
        </div>
        <div className="stat-card completed">
          <div className="stat-label">
            <CheckCircle2 style={{ width: 10, height: 10, marginRight: 4, verticalAlign: 'middle' }} />
            已完成
          </div>
          <div className="stat-value">{counts.completed}</div>
        </div>
        <div className="stat-card failed">
          <div className="stat-label">
            <XCircle style={{ width: 10, height: 10, marginRight: 4, verticalAlign: 'middle' }} />
            失败
          </div>
          <div className="stat-value">{counts.failed}</div>
        </div>
        <div className="stat-card canceled">
          <div className="stat-label">
            <Ban style={{ width: 10, height: 10, marginRight: 4, verticalAlign: 'middle' }} />
            已取消
          </div>
          <div className="stat-value">{counts.canceled}</div>
        </div>
      </div>

      <div className="dashboard-inject-card">
        <div className="dashboard-inject-copy">
          <div className="dashboard-inject-eyebrow">VOICE DEBUG</div>
          <h3>手动送入一条识别文本</h3>
          <p>这是调试入口，不是普通用户对话入口。它会在服务端创建一条 debug ASR 输入，直接送进主流程；它会共享主语音上下文，但不依赖真实小爱设备在线。</p>
        </div>
        <form className="dashboard-inject-form" onSubmit={(event) => void handleASRSubmit(event)}>
          <textarea
            className="dashboard-inject-textarea"
            rows={3}
            value={asrText}
            disabled={asrSending || asrBlocked}
            placeholder="例如：帮我继续刚刚那个网页任务，再炫酷一点。"
            onChange={(event) => setAsrText(event.target.value)}
          />
          <div className="dashboard-inject-actions">
            <div className="dashboard-inject-hint">
              {asrError ? (
                <span className="dashboard-inject-error">{asrError}</span>
              ) : asrFeedback ? (
                <span className="dashboard-inject-success">{asrFeedback}</span>
              ) : asrBlocked ? (
                <span>当前主流程执行中，等这一轮结束后再送入新的 debug ASR。</span>
              ) : !state.xiaoai.connected ? (
                <span>当前小爱未连接，但这不影响这里的主流程调试；历史仍会写进主语音上下文。</span>
              ) : (
                <span>这里会复用主语音上下文；真实小爱连接状态只影响设备侧播报链路。</span>
              )}
            </div>
            <button className="dashboard-inject-button" disabled={asrSending || asrBlocked} type="submit">
              {asrSending ? '发送中...' : '送入主流程'}
            </button>
          </div>
        </form>
      </div>

      <div className="dashboard-grid">
        <TaskListPane
          tasks={state.tasks}
          selectedId={selectedTaskId}
          onSelect={setSelectedTaskId}
        />
        <RecentConversationPane conversations={state.conversations} />
      </div>

      {selectedTask && (
        <TaskDetailModal
          task={selectedTask}
          events={state.events}
          artifacts={state.artifacts}
          claudeRecords={state.claude_records}
          onClose={() => setSelectedTaskId(null)}
          onSelectTask={setSelectedTaskId}
        />
      )}
    </div>
  )
}
