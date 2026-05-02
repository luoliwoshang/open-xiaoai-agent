import type {
  DashboardState,
  LogPage,
  SessionSettings,
  WeChatLoginStart,
  WeChatLoginStatus,
} from '../types'

const BASE = '/api'

async function json<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  const raw = await res.text()
  if (!res.ok) {
    throw new Error(raw.trim() || `${res.status} ${res.statusText}`)
  }
  return raw ? (JSON.parse(raw) as T) : (undefined as T)
}

export function fetchState(): Promise<DashboardState> {
  return json('/state')
}

export function submitAssistantASR(text: string): Promise<{ ok: boolean; text: string }> {
  return json('/assistant/asr', {
    method: 'POST',
    body: JSON.stringify({ text }),
  })
}

export function fetchLogs(page: number, pageSize: number): Promise<LogPage> {
  return json(`/logs?page=${page}&page_size=${pageSize}`)
}

export function saveSessionSettings(s: SessionSettings): Promise<void> {
  return json('/settings/session', { method: 'POST', body: JSON.stringify(s) })
}

export function saveIMDeliverySettings(settings: {
  im_delivery_enabled: boolean
  im_selected_account_id: string
  im_selected_target_id: string
}): Promise<void> {
  return json('/settings/im-delivery', { method: 'POST', body: JSON.stringify(settings) })
}

export function startWeChatLogin(): Promise<WeChatLoginStart> {
  return json('/im/wechat/login/start', { method: 'POST' })
}

export function getWeChatLoginStatus(sessionKey: string): Promise<WeChatLoginStatus> {
  return json(`/im/wechat/login/status?session_key=${encodeURIComponent(sessionKey)}`)
}

export function confirmWeChatLogin(sessionKey: string): Promise<void> {
  return json('/im/wechat/login/confirm', {
    method: 'POST',
    body: JSON.stringify({ session_key: sessionKey }),
  })
}

export function createTarget(data: {
  account_id: string
  name: string
  target_user_id: string
  is_default: boolean
}): Promise<void> {
  return json('/im/targets', { method: 'POST', body: JSON.stringify(data) })
}

export function setDefaultTarget(accountId: string, targetId: string): Promise<void> {
  return json('/im/targets/default', {
    method: 'POST',
    body: JSON.stringify({ account_id: accountId, target_id: targetId }),
  })
}

export function deleteTarget(targetId: string): Promise<void> {
  return json('/im/targets/delete', {
    method: 'POST',
    body: JSON.stringify({ target_id: targetId }),
  })
}

export function deleteAccount(accountId: string): Promise<void> {
  return json('/im/accounts/delete', {
    method: 'POST',
    body: JSON.stringify({ account_id: accountId }),
  })
}

export function sendDebugText(text: string): Promise<void> {
  return json('/im/debug/send-default', {
    method: 'POST',
    body: JSON.stringify({ text }),
  })
}

export function sendDebugImage(file: File, caption?: string): Promise<void> {
  const form = new FormData()
  form.append('image', file)
  if (caption) form.append('caption', caption)
  return fetch(`${BASE}/im/debug/send-image-default`, { method: 'POST', body: form }).then((r) => {
    if (!r.ok) throw new Error(`${r.status} ${r.statusText}`)
  })
}

export function sendDebugFile(file: File, caption?: string): Promise<void> {
  const form = new FormData()
  form.append('file', file)
  if (caption) form.append('caption', caption)
  return fetch(`${BASE}/im/debug/send-file-default`, { method: 'POST', body: form }).then((r) => {
    if (!r.ok) throw new Error(`${r.status} ${r.statusText}`)
  })
}

export function resetBackend(): Promise<void> {
  return json('/reset', { method: 'POST' })
}
