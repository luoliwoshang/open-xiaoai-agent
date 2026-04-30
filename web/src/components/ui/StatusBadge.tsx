import type { TaskState } from '../../types'
import { stateLabels } from '../../lib/dashboard'

type Props = {
  state: TaskState
}

export function StatusBadge({ state }: Props) {
  return <span className={`status-badge status-${state}`}>{stateLabels[state]}</span>
}
