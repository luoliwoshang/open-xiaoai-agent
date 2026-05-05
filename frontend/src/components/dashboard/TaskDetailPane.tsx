import { Cpu } from 'lucide-react'
import type { Task } from '../../types'
import { formatTime, stateLabels } from '../../lib/dashboard'
import { StatusBadge } from '../ui/StatusBadge'
import { TaskChainBar } from './TaskChainBar'

interface TaskDetailPaneProps {
  task: Task | null
  onSelectTask?: (taskId: string) => void
}

function getProgressPercent(task: Task): number {
  switch (task.state) {
    case 'accepted': return 10
    case 'running': return 55
    case 'completed': return 100
    case 'failed': return 100
    case 'canceled': return 100
  }
}

const PROGRESS_STEPS = ['接收', '调度', '执行', '完成', '交付']

export function TaskDetailPane({ task, onSelectTask }: TaskDetailPaneProps) {
  if (!task) {
    return (
      <div className="section-card task-detail-card">
        <div className="section-card-body">
          <div className="task-detail-empty">
            <Cpu />
            <span>选择一个任务查看详情</span>
          </div>
        </div>
      </div>
    )
  }

  const percent = getProgressPercent(task)
  const stepIndex = Math.min(Math.floor(percent / 25), 4)

  return (
    <div className="section-card task-detail-card">
      <div className="task-detail-header">
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 10 }}>
          <StatusBadge state={task.state} />
          <span style={{ fontSize: 11, color: 'var(--text-ghost)', fontFamily: 'var(--font-mono)' }}>
            {stateLabels[task.state]}
          </span>
        </div>
        <div className="task-detail-title">{task.title || task.kind}</div>
        <TaskChainBar taskId={task.id} onSelect={onSelectTask} />
        <div className="task-detail-meta">
          <div className="meta-item">
            <span className="meta-label">ID</span>
            <span className="meta-value">{task.id}</span>
          </div>
          <div className="meta-item">
            <span className="meta-label">类型</span>
            <span className="meta-value">{task.kind}</span>
          </div>
          <div className="meta-item">
            <span className="meta-label">创建时间</span>
            <span className="meta-value">{formatTime(task.created_at)}</span>
          </div>
          <div className="meta-item">
            <span className="meta-label">更新时间</span>
            <span className="meta-value">{formatTime(task.updated_at)}</span>
          </div>
        </div>
      </div>

      <div className="task-detail-scroll">
        <div className="progress-section">
          <div className="progress-header">
            <span className="progress-label">执行进度</span>
            <span className="progress-percent">{percent}%</span>
          </div>
          <div className="progress-track">
            <div className="progress-fill" style={{ width: `${percent}%` }} />
          </div>
          <div className="progress-steps">
            {PROGRESS_STEPS.map((step, i) => (
              <span
                key={step}
                className={`progress-step ${i < stepIndex ? 'done' : i === stepIndex ? 'active' : ''}`}
              >
                {step}
              </span>
            ))}
          </div>
        </div>

        {task.input && (
          <div className="task-detail-section">
            <div className="task-detail-section-title">输入</div>
            <div className="task-detail-section-content">{task.input}</div>
          </div>
        )}

        {task.summary && (
          <div className="task-detail-section">
            <div className="task-detail-section-title">摘要</div>
            <div className="task-detail-section-content">{task.summary}</div>
          </div>
        )}

        {task.result && (
          <div className="task-detail-section">
            <div className="task-detail-section-title">结果</div>
            <div className="task-detail-section-content">{task.result}</div>
          </div>
        )}
      </div>
    </div>
  )
}
