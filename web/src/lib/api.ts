import type { DashboardState } from '../types'
import { normalizeState } from './dashboard'

export async function fetchState() {
  const response = await fetch('/api/state', { cache: 'no-store' })
  if (!response.ok) {
    throw new Error(`API ${response.status}`)
  }
  const raw = (await response.json()) as Partial<DashboardState>
  return normalizeState(raw)
}

export async function postJSON<T>(url: string, payload: unknown) {
  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(payload),
  })
  if (!response.ok) {
    throw new Error(await response.text())
  }
  return (await response.json()) as T
}

