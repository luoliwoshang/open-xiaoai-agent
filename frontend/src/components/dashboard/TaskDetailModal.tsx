import { useEffect } from 'react'
import { X } from 'lucide-react'
import type { ClaudeRecord, Task, TaskArtifact, TaskEvent } from '../../types'
import { TaskDetailPane } from './TaskDetailPane'
import { TaskSideCards } from './TaskSideCards'

interface TaskDetailModalProps {
  task: Task
  events: TaskEvent[]
  artifacts: TaskArtifact[]
  claudeRecords: ClaudeRecord[]
  onClose: () => void
}

export function TaskDetailModal({
  task,
  events,
  artifacts,
  claudeRecords,
  onClose,
}: TaskDetailModalProps) {
  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        onClose()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [onClose])

  return (
    <div className="modal-overlay task-modal-overlay" onClick={onClose}>
      <div className="modal task-modal" onClick={(event) => event.stopPropagation()}>
        <div className="task-modal-header">
          <div>
            <div className="task-modal-eyebrow">TASK INSPECTOR</div>
            <div className="task-modal-title">{task.title || task.kind}</div>
          </div>
          <button className="task-modal-close" onClick={onClose} type="button" aria-label="关闭任务详情">
            <X />
          </button>
        </div>

        <div className="task-modal-layout">
          <TaskDetailPane task={task} />
          <TaskSideCards
            task={task}
            events={events}
            artifacts={artifacts}
            claudeRecords={claudeRecords}
          />
        </div>
      </div>
    </div>
  )
}
