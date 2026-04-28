import { useEffect, useState } from 'react'
import type { DashboardState } from '../types'
import { fetchState } from '../lib/api'
import { emptyState } from '../lib/dashboard'

export function useDashboardState(enabled = true) {
  const [data, setData] = useState<DashboardState>(emptyState)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!enabled) {
      return
    }

    let active = true

    async function refreshWithGuard() {
      try {
        const next = await fetchState()
        if (!active) return
        setData(next)
        setError(null)
      } catch (err) {
        if (!active) return
        setError(err instanceof Error ? err.message : '未知错误')
      } finally {
        if (active) {
          setLoading(false)
        }
      }
    }

    void refreshWithGuard()
    const timer = window.setInterval(() => {
      void refreshWithGuard()
    }, 2000)

    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [enabled])

  async function refresh() {
    const next = await fetchState()
    setData(next)
    setError(null)
    setLoading(false)
  }

  return {
    data,
    setData,
    loading,
    error,
    refresh,
  }
}
