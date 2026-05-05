import { Clock3, Funnel, Sparkles } from 'lucide-react'
import { countByState } from '../../lib/dashboard'
import { formatTime } from '../../lib/dashboard'
import type { Task } from '../../types'
import { EmptyState } from '../ui/EmptyState'
import { SectionCard } from '../ui/SectionCard'
import { StatusBadge } from '../ui/StatusBadge'

type Props = {
  tasks: Task[]
  selectedTaskID: string | null
  onSelect: (taskID: string) => void
}

export function TaskListPane({ tasks, selectedTaskID, onSelect }: Props) {
  const summary = [
    { label: '全部', value: tasks.length, tone: 'neutral' },
    { label: '运行中', value: countByState(tasks, 'running'), tone: 'running' },
    { label: '已完成', value: countByState(tasks, 'completed'), tone: 'completed' },
    { label: '已接续', value: countByState(tasks, 'superseded'), tone: 'neutral' },
    { label: '失败', value: countByState(tasks, 'failed'), tone: 'failed' },
    { label: '已取消', value: countByState(tasks, 'canceled'), tone: 'canceled' },
  ]

  return (
    <SectionCard
      actions={(
        <span className="card-inline-icon">
          <Funnel size={16} />
        </span>
      )}
      className="dashboard-rail-card"
      description="按最近更新时间排序，方便快速切到当前最需要盯住的那条任务。"
      eyebrow="TASK CENTER"
      title="任务列表"
    >
      {tasks.length === 0 ? (
        <EmptyState
          title="暂时还没有任务"
          description="先让小爱接一个网页、整理、写作或资料任务，这里就会热闹起来。"
        />
      ) : (
        <div className="task-list-shell">
          <div className="task-list-summary-grid">
            {summary.map((item) => (
              <article className={`task-list-summary-card task-list-summary-${item.tone}`} key={item.label}>
                <strong>{item.value}</strong>
                <span>{item.label}</span>
              </article>
            ))}
          </div>

          <div className="task-list-stack">
          {tasks.map((task) => {
            const selected = task.id === selectedTaskID
            return (
              <button
                key={task.id}
                className={`task-teaser ${selected ? 'task-teaser-active' : ''}`}
                onClick={() => onSelect(task.id)}
                type="button"
              >
                <div className="task-teaser-head">
                  <div className="task-teaser-copy">
                    <strong>{task.title}</strong>
                    <p>{formatTime(task.updated_at)}</p>
                  </div>
                  <div className="task-teaser-side">
                    <StatusBadge state={task.state} />
                    {task.result_report_pending ? (
                      <span className="soft-chip">
                        <Sparkles size={14} />
                        待汇报
                      </span>
                    ) : null}
                  </div>
                </div>

                <p className="task-teaser-summary">{task.summary || task.input || '这条任务还没有阶段摘要。'}</p>

                <div className="task-teaser-meta">
                  <span>
                    <Clock3 size={14} />
                    {formatTime(task.created_at)}
                  </span>
                  <span>
                    {task.kind}
                  </span>
                </div>
              </button>
            )
          })}
          </div>
        </div>
      )}
    </SectionCard>
  )
}
