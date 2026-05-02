import type { DashboardState, TaskState } from '../types'

export const stateLabels: Record<TaskState, string> = {
  accepted: '已接收',
  running: '执行中',
  completed: '已完成',
  failed: '失败',
  canceled: '已取消',
}

export const emptyState: DashboardState = {
  tasks: [],
  events: [],
  artifacts: [],
  claude_records: [],
  conversations: [],
  assistant: { busy: false, result_report_ready: false, has_voice_channel: false },
  xiaoai: {
    connected: false,
    active_sessions: 0,
    last_connected_at: '',
    last_disconnected_at: '',
    last_remote_addr: '',
  },
  settings: {
    session_window_seconds: 300,
    im_delivery_enabled: false,
    im_selected_account_id: '',
    im_selected_target_id: '',
  },
  im: { accounts: [], targets: [], events: [] },
}

export function formatTime(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  const now = new Date()
  const diff = now.getTime() - d.getTime()
  if (diff < 60_000) return '刚刚'
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`
  return d.toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
}

export function countByState(tasks: DashboardState['tasks']): Record<TaskState, number> {
  const counts: Record<TaskState, number> = { accepted: 0, running: 0, completed: 0, failed: 0, canceled: 0 }
  for (const t of tasks) counts[t.state]++
  return counts
}

export function latest<T extends { created_at: string }>(items: T[]): T | undefined {
  if (items.length === 0) return undefined
  return [...items].sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())[0]
}

export function currentPageFromHash(): string {
  const hash = window.location.hash.replace('#/', '') || ''
  return hash.split('/')[0] || 'dashboard'
}

export function normalizeState(raw: Partial<DashboardState> | null | undefined): DashboardState {
  if (!raw) return { ...emptyState }
  return {
    tasks: raw.tasks ?? [],
    events: raw.events ?? [],
    artifacts: raw.artifacts ?? [],
    claude_records: raw.claude_records ?? [],
    conversations: raw.conversations ?? [],
    assistant: raw.assistant ?? { busy: false, result_report_ready: false, has_voice_channel: false },
    xiaoai: raw.xiaoai ?? emptyState.xiaoai,
    settings: raw.settings ?? emptyState.settings,
    im: raw.im ?? emptyState.im,
  }
}

export function selectBestTarget(targets: DashboardState['im']['targets']): string {
  const def = targets.find((t) => t.is_default)
  return def?.id ?? targets[0]?.id ?? ''
}
