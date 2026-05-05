import { useEffect, useState } from 'react'
import { fetchTaskChain } from '../../lib/api'
import { StatusBadge } from '../ui/StatusBadge'
import type { Task } from '../../types'

interface TaskChainBarProps {
  taskId: string
  onSelect?: (taskId: string) => void
}

export function TaskChainBar({ taskId, onSelect }: TaskChainBarProps) {
  const [chain, setChain] = useState<Task[]>([])

  useEffect(() => {
    let cancelled = false
    fetchTaskChain(taskId)
      .then((tasks) => {
        if (!cancelled) setChain(tasks)
      })
      .catch(() => {
        if (!cancelled) setChain([])
      })
    return () => {
      cancelled = true
    }
  }, [taskId])

  if (chain.length <= 1) return null

  return (
    <div className="task-chain-bar">
      <span className="task-chain-label">任务链</span>
      <div className="task-chain-nodes">
        {chain.map((t, i) => {
          const isCurrent = t.id === taskId
          const isLast = i === chain.length - 1
          const label = t.title || t.kind
          const tip = `${t.id.slice(0, 8)} · ${label}`
          return (
            <span key={t.id} className="task-chain-node">
              <button
                className={`task-chain-item ${isCurrent ? 'current' : ''}`}
                onClick={() => !isCurrent && onSelect?.(t.id)}
                disabled={isCurrent}
                title={tip}
              >
                <span className="task-chain-title">{label}</span>
                <StatusBadge state={t.state} />
              </button>
              {!isLast && <span className="task-chain-sep">&rsaquo;</span>}
            </span>
          )
        })}
      </div>
    </div>
  )
}
