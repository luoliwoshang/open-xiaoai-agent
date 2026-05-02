import { useState, type FormEvent } from 'react'
import { Zap, CheckCircle2, XCircle, Ban } from 'lucide-react'
import type { DashboardState } from '../types'
import { countByState, formatTime } from '../lib/dashboard'
import { submitAssistantASR } from '../lib/api'
import { TaskListPane } from '../components/dashboard/TaskListPane'
import { TaskDetailPane } from '../components/dashboard/TaskDetailPane'
import { TaskSideCards } from '../components/dashboard/TaskSideCards'

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
        <h2>仪表盘</h2>
        <div className="page-header-sub">
          上次更新 {formatTime(state.tasks[0]?.updated_at || new Date().toISOString())}
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
          <p>把这段文字当成已经识别完成的用户输入，直接送进当前最近可用的语音主流程。</p>
        </div>
        <form className="dashboard-inject-form" onSubmit={(event) => void handleASRSubmit(event)}>
          <textarea
            className="dashboard-inject-textarea"
            rows={3}
            value={asrText}
            disabled={asrSending}
            placeholder="例如：帮我继续刚刚那个网页任务，再炫酷一点。"
            onChange={(event) => setAsrText(event.target.value)}
          />
          <div className="dashboard-inject-actions">
            <div className="dashboard-inject-hint">
              {asrError ? (
                <span className="dashboard-inject-error">{asrError}</span>
              ) : asrFeedback ? (
                <span className="dashboard-inject-success">{asrFeedback}</span>
              ) : (
                <span>需要最近有可用语音通道；如果当前正在播报，会直接提示忙。</span>
              )}
            </div>
            <button className="dashboard-inject-button" disabled={asrSending} type="submit">
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
        <TaskDetailPane task={selectedTask} />
        <TaskSideCards
          task={selectedTask}
          events={state.events}
          artifacts={state.artifacts}
          claudeRecords={state.claude_records}
          conversations={state.conversations}
        />
      </div>
    </div>
  )
}
