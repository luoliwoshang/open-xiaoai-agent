import { Clock3, FolderHeart, Sparkles } from 'lucide-react'
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
  return (
    <SectionCard
      className="dashboard-rail-card"
      description="把复杂任务交给小爱之后，这里会像任务收纳盒一样把每一条进展都排好。"
      eyebrow="TASK POCKET"
      title="任务队列"
    >
      {tasks.length === 0 ? (
        <EmptyState
          title="暂时还没有任务"
          description="先让小爱接一个网页、整理、写作或资料任务，这里就会热闹起来。"
        />
      ) : (
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
                  <StatusBadge state={task.state} />
                  {task.report_pending ? (
                    <span className="soft-chip">
                      <Sparkles size={14} />
                      待补报
                    </span>
                  ) : null}
                </div>

                <div className="task-teaser-copy">
                  <strong>{task.title}</strong>
                  <p>{task.summary || task.input || '这条任务还没有阶段摘要。'}</p>
                </div>

                <div className="task-teaser-meta">
                  <span>
                    <Clock3 size={14} />
                    {formatTime(task.updated_at)}
                  </span>
                  <span>
                    <FolderHeart size={14} />
                    {task.kind}
                  </span>
                </div>
              </button>
            )
          })}
        </div>
      )}
    </SectionCard>
  )
}
