import type { TaskState } from '../../types'
import { stateLabels } from '../../lib/dashboard'

interface StatusBadgeProps {
  state: TaskState
}

export function StatusBadge({ state }: StatusBadgeProps) {
  return (
    <span className={`status-badge ${state}`}>
      <span className="status-badge-dot" />
      {stateLabels[state]}
    </span>
  )
}
