import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchState } from '../lib/api'
import { normalizeState } from '../lib/dashboard'
import type { DashboardState } from '../types'

export function useDashboardState(pollMs = 2000) {
  const [state, setState] = useState<DashboardState>(normalizeState(null))
  const [error, setError] = useState<string | null>(null)
  const timer = useRef<ReturnType<typeof setInterval> | null>(null)

  const load = useCallback(async () => {
    try {
      const raw = await fetchState()
      setState(normalizeState(raw))
      setError(null)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Unknown error')
    }
  }, [])

  useEffect(() => {
    load()
    timer.current = setInterval(load, pollMs)
    return () => {
      if (timer.current) clearInterval(timer.current)
    }
  }, [load, pollMs])

  return { state, error, reload: load }
}
