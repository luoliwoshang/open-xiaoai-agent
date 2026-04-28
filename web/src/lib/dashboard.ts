import type {
  DashboardState,
  IMTarget,
  SettingsSnapshot,
  Task,
  TaskState,
} from '../types'

export type Page = 'dashboard' | 'settings' | 'logs'

export const emptyState: DashboardState = {
  tasks: [],
  events: [],
  artifacts: [],
  claude_records: [],
  conversations: [],
  settings: {
    session_window_seconds: 300,
    im_delivery_enabled: false,
    im_selected_account_id: '',
    im_selected_target_id: '',
  },
  im: {
    accounts: [],
    targets: [],
    events: [],
  },
}

export const stateLabels: Record<TaskState, string> = {
  accepted: '已受理',
  running: '执行中',
  completed: '已完成',
  failed: '失败',
  canceled: '已取消',
}

export function formatTime(value: string) {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(date)
}

export function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let size = value
  let unitIndex = 0
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex += 1
  }
  const digits = size >= 10 || unitIndex === 0 ? 0 : 1
  return `${size.toFixed(digits)} ${units[unitIndex]}`
}

export function countByState(tasks: Task[], state: TaskState) {
  return tasks.filter((task) => task.state === state).length
}

export function latest<T extends { created_at?: string; updated_at?: string }>(items: T[]) {
  return items[0] ?? null
}

export function currentPageFromHash(): Page {
  switch (window.location.hash) {
    case '#/settings':
      return 'settings'
    case '#/logs':
      return 'logs'
    default:
      return 'dashboard'
  }
}

export function selectBestTarget(targets: IMTarget[], accountID: string) {
  const accountTargets = targets.filter((target) => target.account_id === accountID)
  return accountTargets.find((target) => target.is_default)?.id ?? accountTargets[0]?.id ?? ''
}

export function normalizeState(raw: Partial<DashboardState> | undefined): DashboardState {
  return {
    tasks: raw?.tasks ?? [],
    events: raw?.events ?? [],
    artifacts: raw?.artifacts ?? [],
    claude_records: raw?.claude_records ?? [],
    conversations: (raw?.conversations ?? []).map((conversation) => ({
      ...conversation,
      messages: conversation?.messages ?? [],
    })),
    settings: normalizeSettings(raw?.settings),
    im: {
      accounts: raw?.im?.accounts ?? [],
      targets: raw?.im?.targets ?? [],
      events: raw?.im?.events ?? [],
    },
  }
}

export function normalizeSettings(raw: Partial<SettingsSnapshot> | undefined): SettingsSnapshot {
  return {
    session_window_seconds: raw?.session_window_seconds ?? 300,
    im_delivery_enabled: raw?.im_delivery_enabled ?? false,
    im_selected_account_id: raw?.im_selected_account_id ?? '',
    im_selected_target_id: raw?.im_selected_target_id ?? '',
  }
}
