import { ListTodo } from 'lucide-react'
import type { Task, TaskState } from '../../types'
import { countByState, formatTime, stateLabels } from '../../lib/dashboard'
import { StatusBadge } from '../ui/StatusBadge'
import { EmptyState } from '../ui/EmptyState'

interface TaskListPaneProps {
  tasks: Task[]
  selectedId: string | null
  onSelect: (id: string) => void
}

export function TaskListPane({ tasks, selectedId, onSelect }: TaskListPaneProps) {
  const counts = countByState(tasks)
  const ordered: TaskState[] = ['running', 'accepted', 'completed', 'superseded', 'failed', 'canceled']

  return (
    <div className="section-card" style={{ maxHeight: 'calc(100vh - 200px)' }}>
      <div className="section-card-header">
        <div className="section-card-title">
          <ListTodo />
          任务列表
        </div>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-ghost)' }}>
          {tasks.length} 总计
        </span>
      </div>
      <div className="section-card-body">
        <div className="task-summary-row">
          {ordered.map((s) => (
            <div key={s} className="task-summary-pill">
              <span className="pill-count">{counts[s]}</span>
              <span className="pill-label">{stateLabels[s]}</span>
            </div>
          ))}
        </div>
        {tasks.length === 0 ? (
          <EmptyState title="暂无任务" description="当有任务派发时会出现在这里" />
        ) : (
          <div className="task-list">
            {[...tasks]
              .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
              .map((task) => (
                <div
                  key={task.id}
                  className={`task-item ${selectedId === task.id ? 'selected' : ''}`}
                  onClick={() => onSelect(task.id)}
                >
                  <div className="task-item-title">{task.title || task.kind}</div>
                  <div className="task-item-meta">
                    <StatusBadge state={task.state} />
                    <span>{formatTime(task.created_at)}</span>
                    {task.result_report_pending && (
                      <span style={{ color: 'var(--yellow)', fontSize: 10 }}>待推送</span>
                    )}
                  </div>
                </div>
              ))}
          </div>
        )}
      </div>
    </div>
  )
}
