export type TaskState = 'accepted' | 'running' | 'completed' | 'failed' | 'canceled'

export type Task = {
  id: string
  plugin?: string
  kind: string
  title: string
  input: string
  parent_task_id?: string
  state: TaskState
  summary: string
  result: string
  report_pending: boolean
  created_at: string
  updated_at: string
}

export type TaskEvent = {
  id: string
  task_id: string
  type: string
  message: string
  created_at: string
}

export type DashboardState = {
  tasks: Task[]
  events: TaskEvent[]
  artifacts: TaskArtifact[]
  claude_records: ClaudeRecord[]
  conversations: ConversationSnapshot[]
  assistant: AssistantRuntimeStatus
  settings: SettingsSnapshot
  im: IMSnapshot
}

export type AssistantRuntimeStatus = {
  busy: boolean
  pending_report_ready: boolean
  has_session: boolean
}

export type TaskArtifact = {
  id: string
  task_id: string
  kind: string
  file_name: string
  mime_type: string
  size_bytes: number
  deliver: boolean
  created_at: string
}

export type SettingsSnapshot = {
  session_window_seconds: number
  im_delivery_enabled: boolean
  im_selected_account_id: string
  im_selected_target_id: string
}

export type SessionSettings = {
  session_window_seconds: number
}

export type ConversationMessage = {
  role: string
  content: string
}

export type ConversationSnapshot = {
  id: string
  started_at: string
  last_active: string
  messages: ConversationMessage[]
}

export type ClaudeRecord = {
  task_id: string
  session_id: string
  prompt: string
  working_directory: string
  status: 'accepted' | 'running' | 'completed' | 'failed'
  last_summary: string
  last_assistant_text: string
  result: string
  error: string
  created_at: string
  updated_at: string
}

export type IMAccount = {
  id: string
  platform: string
  remote_account_id: string
  owner_user_id: string
  display_name: string
  base_url: string
  last_error: string
  last_sent_at: string
  created_at: string
  updated_at: string
}

export type IMTarget = {
  id: string
  account_id: string
  name: string
  target_user_id: string
  is_default: boolean
  created_at: string
  updated_at: string
}

export type IMEvent = {
  id: string
  account_id: string
  type: string
  message: string
  created_at: string
}

export type IMSnapshot = {
  accounts: IMAccount[]
  targets: IMTarget[]
  events: IMEvent[]
}

export type IMDeliveryReceipt = {
  account: IMAccount
  target: IMTarget
  message_id: string
  kind: 'text' | 'image' | 'file'
  text?: string
  caption?: string
  media_file_name?: string
  media_mime_type?: string
}

export type WeChatLoginStart = {
  session_key: string
  qr_raw_text: string
  qr_code_data_url: string
  expires_at: string
}

export type WeChatLoginCandidate = {
  remote_account_id: string
  owner_user_id: string
  display_name: string
  base_url: string
}

export type WeChatLoginStatus = {
  status: 'pending' | 'scanned' | 'confirmed' | 'expired' | 'failed'
  message: string
  candidate?: WeChatLoginCandidate
}

export type LogEntry = {
  id: string
  level: 'debug' | 'info' | 'warn' | 'error' | 'fatal'
  source: string
  message: string
  raw: string
  created_at: string
}

export type LogPage = {
  items: LogEntry[]
  page: number
  page_size: number
  total: number
  has_more: boolean
}
