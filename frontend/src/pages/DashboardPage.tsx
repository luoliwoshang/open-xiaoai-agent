import { useState } from 'react'
import { Activity, Zap, CheckCircle2, XCircle, Ban } from 'lucide-react'
import type { DashboardState } from '../../types'
import { countByState, formatTime } from '../lib/dashboard'
import { TaskListPane } from '../components/dashboard/TaskListPane'
import { TaskDetailPane } from '../components/dashboard/TaskDetailPane'
import { TaskSideCards } from '../components/dashboard/TaskSideCards'

interface DashboardPageProps {
  state: DashboardState
}

export function DashboardPage({ state }: DashboardPageProps) {
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null)
  const counts = countByState(state.tasks)
  const total = state.tasks.length
  const selectedTask = state.tasks.find((t) => t.id === selectedTaskId) ?? null

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
