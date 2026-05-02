export type TaskState = 'accepted' | 'running' | 'completed' | 'failed' | 'canceled'

export interface Task {
  id: string
  plugin?: string
  kind: string
  title: string
  input: string
  parent_task_id?: string
  state: TaskState
  summary: string
  result: string
  result_report_pending: boolean
  created_at: string
  updated_at: string
}

export interface TaskEvent {
  id: string
  task_id: string
  type: string
  message: string
  created_at: string
}

export interface TaskArtifact {
  id: string
  task_id: string
  kind: string
  file_name: string
  mime_type: string
  size_bytes: number
  deliver: boolean
  created_at: string
}

export interface ClaudeRecord {
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

export interface ConversationMessage {
  role: string
  content: string
}

export interface ConversationSnapshot {
  id: string
  started_at: string
  last_active: string
  messages: ConversationMessage[]
}

export interface AssistantRuntimeStatus {
  busy: boolean
  result_report_ready: boolean
  has_voice_channel: boolean
}

export interface SettingsSnapshot {
  session_window_seconds: number
  im_delivery_enabled: boolean
  im_selected_account_id: string
  im_selected_target_id: string
}

export interface SessionSettings {
  session_window_seconds: number
}

export interface IMAccount {
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

export interface IMTarget {
  id: string
  account_id: string
  name: string
  target_user_id: string
  is_default: boolean
  created_at: string
  updated_at: string
}

export interface IMEvent {
  id: string
  account_id: string
  type: string
  message: string
  created_at: string
}

export interface IMSnapshot {
  accounts: IMAccount[]
  targets: IMTarget[]
  events: IMEvent[]
}

export interface WeChatLoginStart {
  session_key: string
  qr_raw_text: string
  qr_code_data_url: string
  expires_at: string
}

export interface WeChatLoginCandidate {
  remote_account_id: string
  owner_user_id: string
  display_name: string
  base_url: string
}

export interface WeChatLoginStatus {
  status: 'pending' | 'scanned' | 'confirmed' | 'expired' | 'failed'
  message: string
  candidate?: WeChatLoginCandidate
}

export interface LogEntry {
  id: string
  level: 'debug' | 'info' | 'warn' | 'error' | 'fatal'
  source: string
  message: string
  raw: string
  created_at: string
}

export interface LogPage {
  items: LogEntry[]
  page: number
  page_size: number
  total: number
  has_more: boolean
}

export interface DashboardState {
  tasks: Task[]
  events: TaskEvent[]
  artifacts: TaskArtifact[]
  claude_records: ClaudeRecord[]
  conversations: ConversationSnapshot[]
  assistant: AssistantRuntimeStatus
  settings: SettingsSnapshot
  im: IMSnapshot
}

export type Page = 'dashboard' | 'settings' | 'logs'
